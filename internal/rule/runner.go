package rule

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/notify"
)

const (
	// DefaultDebounce は、EPG やルールが変わってから照合するまで待つ時間。立て続けの変更を 1 回の照合にまとめる (設計書 4.3)。
	DefaultDebounce = 2 * time.Second
	// DefaultInterval は、変更がなくても照合する間隔。通知を取りこぼしても、これより長く遅れない。
	DefaultInterval = 10 * time.Minute
)

// RunnerOptions は Runner の設定。0 の項目は既定値にする。
type RunnerOptions struct {
	Debounce time.Duration
	Interval time.Duration
	// PollOnly は、変更の通知を LISTEN で受けずに、ポーリングだけで知るかどうか。
	// トランザクションプーリングを経由するときは true にする。
	PollOnly bool
	// PollInterval は、変更の通知の版数をポーリングする間隔。0 なら notify の既定値。
	PollInterval time.Duration
}

// Runner は、EPG やルールが変わるたびに、ルールの照合を走らせる (設計書 4.3)。
type Runner struct {
	engine *Engine
	pool   *pgxpool.Pool
	logger *slog.Logger
	opts   RunnerOptions
}

// NewRunner は Runner を作る。opts は nil でもよい。
func NewRunner(engine *Engine, pool *pgxpool.Pool, logger *slog.Logger, opts *RunnerOptions) *Runner {
	o := RunnerOptions{Debounce: DefaultDebounce, Interval: DefaultInterval, PollOnly: false, PollInterval: notify.DefaultPollInterval}
	if opts != nil {
		o.PollOnly = opts.PollOnly
		if opts.Debounce > 0 {
			o.Debounce = opts.Debounce
		}
		if opts.Interval > 0 {
			o.Interval = opts.Interval
		}
		if opts.PollInterval > 0 {
			o.PollInterval = opts.PollInterval
		}
	}

	return &Runner{engine: engine, pool: pool, logger: logger, opts: o}
}

// Run は、ctx が終わるまで、起動したとき、EPG かルールが変わったとき、Interval ごとに照合する。
// 照合に失敗したら、ログに出して次のきっかけを待つ。
func (r *Runner) Run(ctx context.Context) error {
	return notify.Loop(ctx, r.pool, r.logger, &notify.LoopOptions{
		Channels: []string{epg.NotifyChannel, NotifyChannel},
		Debounce: r.opts.Debounce,
		Interval: r.opts.Interval,
		Watcher:  notify.WatcherOptions{UseListen: !r.opts.PollOnly, PollInterval: r.opts.PollInterval},
	}, r.runOnce)
}

func (r *Runner) runOnce(ctx context.Context) {
	result, err := r.engine.Run(ctx)
	if err != nil {
		if ctx.Err() == nil {
			r.logger.WarnContext(ctx, "failed to match rules", slog.Any("error", err))
		}

		return
	}
	if len(result.Skipped) > 0 {
		r.logger.WarnContext(ctx, "skipped rules whose conditions could not be read; their reservations are kept",
			slog.Any("rule_ids", result.Skipped))
	}
	if result.Created > 0 || result.Deleted > 0 || result.Linked > 0 || result.Unlinked > 0 {
		r.logger.InfoContext(ctx, "matched rules",
			slog.Int("created", result.Created), slog.Int("deleted", result.Deleted),
			slog.Int("linked", result.Linked), slog.Int("unlinked", result.Unlinked))
	}
}
