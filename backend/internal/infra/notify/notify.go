// Package notify は、ロール間の変更の通知を扱う (設計書 1.4)。
//
// 変更を書く側は、そのトランザクションの中で Publish を呼ぶ。Publish はチャンネルの版数を上げ、
// PostgreSQL の NOTIFY を送る。NOTIFY はコミットしたときにだけ送られる。
//
// 受け取る側の Watcher は、LISTEN で通知を待ちつつ、版数を定期的にポーリングする。
// PgBouncer などのトランザクションプーリングを経由すると LISTEN を使えないので、そのときはポーリングだけで動く。
// LISTEN の接続が切れている間も、ポーリングで変更に気づける。
// どちらの経路でも、届くのは「版数が上がった」ことだけである。何が変わったかは、受け取った側が DB を読み直して知る。
package notify

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// RecordingsChannel は、録画が変わったことを知らせる変更通知のチャンネル (#868)。録画を始めた、終えた、消した、
// 保護やタグを変えた、サムネイルや解析の結果を書いたときに版数を上げる。録画中にファイルが伸びるだけでは上げない。
// 録画は recording、library、thumbnail、analysis、epgstation が書くので、互いに import しなくて済むよう、ここに置く。
const RecordingsChannel = "recordings"

// AuditChannel は、監査ログに残る操作があったことを知らせる変更通知のチャンネル (#869)。
// 管理の画面 (チャンネル、ユーザー、Webhook、設定、監査ログ) のデータは、どれも監査ログに残る操作で変わるので、
// 監査ログを書くたびに版数を上げ、管理の画面の読み直しの合図にする。
const AuditChannel = "audit"

// DefaultPollInterval は、版数をポーリングする既定の間隔。
const DefaultPollInterval = 5 * time.Second

// Publish は、db で channel の版数を上げて NOTIFY を送り、上げた後の版数を返す。
// db がトランザクションなら、版数はコミットしたときに上がり、NOTIFY もコミットしたときに送られる。
// トランザクションの外で書いた変更は、書いた後に、トランザクションでない db を渡して知らせる。
func Publish(ctx context.Context, db sqlc.DBTX, clk clock.Clock, channel string) (int64, error) {
	q := sqlc.New(db)
	version, err := q.BumpChangeVersion(ctx, sqlc.BumpChangeVersionParams{Channel: channel, Now: clk.Now()})
	if err != nil {
		return 0, fmt.Errorf("failed to bump the version of %s: %w", channel, err)
	}
	if err := q.NotifyChange(ctx, sqlc.NotifyChangeParams{
		Channel: channel,
		Payload: strconv.FormatInt(version, 10),
	}); err != nil {
		return 0, fmt.Errorf("failed to notify %s: %w", channel, err)
	}

	return version, nil
}

// WatcherOptions は Watcher の設定。
type WatcherOptions struct {
	// UseListen は LISTEN を使うかどうか。トランザクションプーリングを経由するときは false にする。
	UseListen bool
	// PollInterval は版数をポーリングする間隔。LISTEN を使うときも、取りこぼしに備えてポーリングする。
	PollInterval time.Duration
}

// Watcher は、チャンネルの版数が上がったことを知らせる。
type Watcher struct {
	pool     *pgxpool.Pool
	logger   *slog.Logger
	channels []string
	opts     WatcherOptions

	mu       sync.Mutex
	versions map[string]int64

	changes chan struct{}
}

// NewWatcher は channels を見張る Watcher を作る。
// 今の版数をこの時点で読むので、NewWatcher が返った後の変更を Run が知らせる。
func NewWatcher(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, channels []string, opts WatcherOptions) (*Watcher, error) {
	w := &Watcher{
		pool:     pool,
		logger:   logger,
		channels: slices.Clone(channels),
		opts:     opts,
		mu:       sync.Mutex{},
		versions: map[string]int64{},
		changes:  make(chan struct{}, 1),
	}
	if _, err := w.poll(ctx); err != nil {
		return nil, err
	}

	return w, nil
}

// Changes は、いずれかのチャンネルの版数が上がると値を受け取れるチャンネルを返す。
// 受け取る前に何度変更があっても、受け取れるのは 1 回にまとまる。受け取ったら Version で版数を確かめる。
func (w *Watcher) Changes() <-chan struct{} {
	return w.changes
}

// Version は、channel について最後に確かめた版数を返す。一度も変更のないチャンネルは 0 を返す。
func (w *Watcher) Version(channel string) int64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.versions[channel]
}

// Run は、ctx が終わるまでチャンネルを見張る。
func (w *Watcher) Run(ctx context.Context) error {
	kick := make(chan struct{}, 1)
	if w.opts.UseListen {
		go w.listen(ctx, kick)
	}

	ticker := time.NewTicker(w.opts.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		case <-kick:
		}

		changed, err := w.poll(ctx)
		if err != nil {
			if ctx.Err() == nil {
				w.logger.WarnContext(ctx, "failed to poll change versions", slog.Any("error", err))
			}

			continue
		}
		if changed {
			select {
			case w.changes <- struct{}{}:
			default:
				// まだ受け取られていない通知があるので、それにまとめる。
			}
		}
	}
}

// poll は版数を読み、上がったチャンネルがあれば true を返す。
func (w *Watcher) poll(ctx context.Context) (bool, error) {
	rows, err := sqlc.New(w.pool).ListChangeVersions(ctx, w.channels)
	if err != nil {
		return false, fmt.Errorf("failed to list change versions: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	changed := false
	for _, row := range rows {
		if row.Version > w.versions[row.Channel] {
			w.versions[row.Channel] = row.Version
			changed = true
		}
	}

	return changed, nil
}

// listen は、LISTEN の通知を受けるたびに kick に送る。接続が切れたら、PollInterval をおいてつなぎ直す。
func (w *Watcher) listen(ctx context.Context, kick chan<- struct{}) {
	for {
		if err := w.listenOnce(ctx, kick); err != nil && ctx.Err() == nil {
			w.logger.WarnContext(ctx, "LISTEN connection failed; relying on polling until reconnected", slog.Any("error", err))
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(w.opts.PollInterval):
		}
	}
}

func (w *Watcher) listenOnce(ctx context.Context, kick chan<- struct{}) error {
	// LISTEN はその接続にだけ効くので、プールとは別に専用の接続を持つ。
	conn, err := pgx.ConnectConfig(ctx, w.pool.Config().ConnConfig.Copy())
	if err != nil {
		return fmt.Errorf("failed to connect for LISTEN: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()

	for _, channel := range w.channels {
		if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
			return fmt.Errorf("failed to LISTEN %s: %w", channel, err)
		}
	}
	// つなぎ直している間の変更を取りこぼさないよう、一度ポーリングさせる。
	trigger(kick)

	for {
		if _, err := conn.WaitForNotification(ctx); err != nil {
			return fmt.Errorf("failed to wait for notifications: %w", err)
		}
		trigger(kick)
	}
}

func trigger(kick chan<- struct{}) {
	select {
	case kick <- struct{}{}:
	default:
	}
}
