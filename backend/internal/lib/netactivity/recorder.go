package netactivity

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// flushInterval は、集計を DB へ書く間隔。
const flushInterval = time.Minute

// Key は、集計の単位。
type Key struct {
	// Hour は、通信した時間帯の始まり (UTC)。
	Hour      time.Time
	Purpose   Purpose
	Host      string
	Initiator Initiator
	Success   bool
}

// Totals は、集計の値。
type Totals struct {
	Count         int64
	SentBytes     int64
	ReceivedBytes int64
}

// Bucket は、DB へ書く集計の 1 行。
type Bucket struct {
	Key
	Totals
}

// Store は、集計の書き込み先。
type Store interface {
	// Enabled は、記録が有効かを返す。
	Enabled(ctx context.Context) (bool, error)
	// Save は、集計を足し込む。書けなければ、次の機会にもう一度渡す。
	Save(ctx context.Context, buckets []Bucket) error
}

// Recorder は、通信の集計をメモリに持ち、定期的に Store へ書く。
//
// 記録の処理が通信を妨げないよう、DB に触るのは Run だけにし、書けなくても通信は止めない。
// nil の Recorder は、何も記録しない。
type Recorder struct {
	clock   clock.Clock
	enabled atomic.Bool

	mu      sync.Mutex
	buckets map[Key]*Totals
}

// NewRecorder は Recorder を返す。Run が記録の有効を確かめるまでは、何も数えない。
func NewRecorder(clk clock.Clock) *Recorder {
	return &Recorder{clock: clk, enabled: atomic.Bool{}, mu: sync.Mutex{}, buckets: map[Key]*Totals{}}
}

// Add は、目的 purpose の host との通信を足し込む。
func (r *Recorder) Add(purpose Purpose, host string, success bool, delta Totals) {
	if r == nil || !r.enabled.Load() {
		return
	}
	key := Key{Hour: r.clock.Now().UTC().Truncate(time.Hour), Purpose: purpose, Host: visibleHost(purpose, host), Initiator: initiatorOf(purpose), Success: success}

	r.mu.Lock()
	defer r.mu.Unlock()
	totals, ok := r.buckets[key]
	if !ok {
		totals = new(Totals)
		r.buckets[key] = totals
	}
	totals.Count += delta.Count
	totals.SentBytes += delta.SentBytes
	totals.ReceivedBytes += delta.ReceivedBytes
}

// Run は、ctx が終わるまで、記録の有効を確かめ、集計を定期的に store へ書く。終わるときに残りを書く。
func (r *Recorder) Run(ctx context.Context, store Store, logger *slog.Logger) {
	r.flush(ctx, store, logger)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// 止まる前に、残りを書く。呼び出し元の ctx は終わっているので、切り離す。
			final, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			r.flush(final, store, logger)

			return
		case <-ticker.C:
			r.flush(ctx, store, logger)
		}
	}
}

// flush は、記録の有効を読み直し、集計を書く。書けなかった分は、次に足し合わせて書く。
func (r *Recorder) flush(ctx context.Context, store Store, logger *slog.Logger) {
	enabled, err := store.Enabled(ctx)
	if err != nil {
		logger.WarnContext(ctx, "failed to read whether network activity is recorded", slog.Any("error", err))

		return
	}
	r.enabled.Store(enabled)

	r.mu.Lock()
	pending := r.buckets
	r.buckets = map[Key]*Totals{}
	r.mu.Unlock()
	// 無効に戻したら、それまでの集計は捨てる。
	if !enabled || len(pending) == 0 {
		return
	}

	buckets := make([]Bucket, 0, len(pending))
	for key, totals := range pending {
		buckets = append(buckets, Bucket{Key: key, Totals: *totals})
	}
	if err := store.Save(ctx, buckets); err != nil {
		logger.WarnContext(ctx, "failed to save network activity", slog.Any("error", err))
		r.restore(pending)
	}
}

// restore は、書けなかった集計を戻す。
func (r *Recorder) restore(pending map[Key]*Totals) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, totals := range pending {
		current, ok := r.buckets[key]
		if !ok {
			r.buckets[key] = totals

			continue
		}
		current.Count += totals.Count
		current.SentBytes += totals.SentBytes
		current.ReceivedBytes += totals.ReceivedBytes
	}
}
