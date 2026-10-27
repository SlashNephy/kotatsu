package database

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

const (
	// FallbackTTL は、DB に届かない間に、視聴の認可で覚えた結果を使う時間の上限 (設計書 9.5)。
	// DB に届かない間は、セッションの取り消しやユーザーの無効化を知れないので、取り消しが効くまでの遅れをこの時間で抑える。
	// CloudNativePG で PostgreSQL を更新すると DB は数十秒止まるので、それより十分に長くする。
	FallbackTTL = 5 * time.Minute
	// FallbackQueryTimeout は、Fallback が DB の応答を待つ上限。DB への接続が応答のないまま詰まっても、覚えた結果に切り替える。
	// mikan が録画の途中で DB に書くときの上限 (設計書 1.5) とそろえる。
	FallbackQueryTimeout = 2 * time.Second
	// FallbackRetryAfter は、DB に届かないと分かってから、覚えた結果があるキーについて DB を引き直すまでの間隔。
	// 詰まった DB を、リクエストのたびに FallbackQueryTimeout だけ待たないようにする。
	FallbackRetryAfter = 5 * time.Second
)

// Fallback は、DB から読めた結果をキーごとに覚え、DB に届かない間だけ、覚えた結果を返す (#1084)。
// DB に届くときは、毎回 DB から読む。読めなかった理由が DB に届かないこと (IsUnavailable) でなければ、覚えた結果を忘れる。
// 取り消されたセッションのように、DB から「使えない」と分かったものを、DB が止まった後に覚えた結果で通さないためである。
type Fallback[K comparable, V any] struct {
	name   string
	ttl    time.Duration
	clock  clock.Clock
	logger *slog.Logger

	mu        sync.Mutex
	entries   map[K]*fallbackEntry[V]
	lastSweep time.Time
	// retryAt は、DB に届かないと分かったときに、次に DB を引き直す時刻。届いている間はゼロ値。
	retryAt time.Time
	// degraded は、前回の読み込みで覚えた結果を返したかどうか。ログを、切り替わったときだけ出すのに使う。
	degraded bool

	used atomic.Int64
}

type fallbackEntry[V any] struct {
	value    V
	storedAt time.Time
}

// NewFallback は、name の結果を覚える Fallback を返す。name はログとメトリクスで見分けるのに使う。
// 覚えた結果は、DB から読めた時刻から ttl の間だけ使う。ttl が 0 なら、次に読めるまで使い続ける。
func NewFallback[K comparable, V any](name string, ttl time.Duration, clk clock.Clock, logger *slog.Logger) *Fallback[K, V] {
	return &Fallback[K, V]{
		name: name, ttl: ttl, clock: clk, logger: logger,
		mu: sync.Mutex{}, entries: map[K]*fallbackEntry[V]{}, lastSweep: clk.Now(), retryAt: time.Time{}, degraded: false,
		used: atomic.Int64{},
	}
}

// Load は、load で key の値を DB から読み、覚えて返す。DB に届かず、key の値を覚えていれば、覚えた値を返す。
// load には、FallbackQueryTimeout で打ち切る ctx を渡す。
// 返す 2 つ目の値は、覚えた結果を返したかどうか。
func (f *Fallback[K, V]) Load(ctx context.Context, key K, load func(ctx context.Context) (V, error)) (V, bool, error) {
	if cached, ok := f.cachedWhileRetrying(key); ok {
		f.serveStale(ctx, nil)

		return cached, true, nil
	}

	loadCtx, cancel := context.WithTimeout(ctx, FallbackQueryTimeout)
	value, err := load(loadCtx)
	cancel()
	if err == nil {
		f.store(ctx, key, value)

		return value, false, nil
	}
	// 呼び出し元が待つのをやめたなら、DB の状態は分からないので、覚えた結果をそのままにしておく。
	if ctx.Err() != nil {
		return value, false, err
	}
	if !IsUnavailable(err) {
		f.forget(key)

		return value, false, err
	}
	cached, ok := f.cached(key, true)
	if !ok {
		return value, false, err
	}
	f.serveStale(ctx, err)

	return cached, true, nil
}

// Forget は、key の覚えた結果を忘れる。DB を書き換えて、覚えた結果が使えなくなったときに呼ぶ。
func (f *Fallback[K, V]) Forget(key K) {
	f.forget(key)
}

// ForgetIf は、覚えた結果のうち、forget が true を返すものを忘れる。
func (f *Fallback[K, V]) ForgetIf(forget func(V) bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, entry := range f.entries {
		if forget(entry.value) {
			delete(f.entries, key)
		}
	}
}

// Used は、これまでに覚えた結果を返した回数を返す。
func (f *Fallback[K, V]) Used() int64 {
	return f.used.Load()
}

// cachedWhileRetrying は、DB に届かないと分かってから FallbackRetryAfter が過ぎていなければ、key の覚えた結果を返す。
func (f *Fallback[K, V]) cachedWhileRetrying(key K) (V, bool) {
	f.mu.Lock()
	retrying := !f.retryAt.IsZero() && f.clock.Now().Before(f.retryAt)
	f.mu.Unlock()
	if !retrying {
		var zero V

		return zero, false
	}

	return f.cached(key, false)
}

// cached は、key の覚えた結果が ttl の中にあれば返す。unreachable なら、DB に届かないと分かったので、次に引き直す時刻を決める。
func (f *Fallback[K, V]) cached(key K, unreachable bool) (V, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := f.clock.Now()
	if unreachable {
		f.retryAt = now.Add(FallbackRetryAfter)
	}
	entry, ok := f.entries[key]
	if !ok || f.expired(entry, now) {
		var zero V

		return zero, false
	}

	return entry.value, true
}

// serveStale は、覚えた結果を返したことを数え、DB に届かなくなって初めて返したときに、ログに出す。
func (f *Fallback[K, V]) serveStale(ctx context.Context, cause error) {
	f.used.Add(1)
	f.mu.Lock()
	first := !f.degraded
	f.degraded = true
	f.mu.Unlock()
	if first {
		f.logger.WarnContext(ctx, "the database is unreachable; serving cached results", slog.String("cache", f.name), slog.Any("error", cause))
	}
}

// store は、key の値を覚える。DB に届くようになったら、覚えた結果に頼るのをやめたことをログに出す。
// ttl が過ぎた結果は、ttl ごとにまとめて消す。
func (f *Fallback[K, V]) store(ctx context.Context, key K, value V) {
	f.mu.Lock()
	now := f.clock.Now()
	f.entries[key] = &fallbackEntry[V]{value: value, storedAt: now}
	f.retryAt = time.Time{}
	recovered := f.degraded
	f.degraded = false
	if f.ttl > 0 && now.Sub(f.lastSweep) >= f.ttl {
		for k, entry := range f.entries {
			if f.expired(entry, now) {
				delete(f.entries, k)
			}
		}
		f.lastSweep = now
	}
	f.mu.Unlock()
	if recovered {
		f.logger.InfoContext(ctx, "the database is reachable again; stopped serving cached results", slog.String("cache", f.name))
	}
}

func (f *Fallback[K, V]) forget(key K) {
	f.mu.Lock()
	delete(f.entries, key)
	f.mu.Unlock()
}

func (f *Fallback[K, V]) expired(entry *fallbackEntry[V], now time.Time) bool {
	return f.ttl > 0 && !now.Before(entry.storedAt.Add(f.ttl))
}
