package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// errInvalidLoop は、Loop の設定が正しくないことを表す。
var errInvalidLoop = errors.New("invalid loop options")

// LoopOptions は Loop の設定。
type LoopOptions struct {
	// Channels は見張る変更通知のチャンネル。
	Channels []string
	// Debounce は、変更を受けてから run を呼ぶまで待つ時間。待つ間に来た変更は 1 回にまとめる。
	Debounce time.Duration
	// Interval は、変更がなくても run を呼ぶ間隔。通知を取りこぼしても、これより長く遅れない。
	Interval time.Duration
	Watcher  WatcherOptions
}

// Loop は、ctx が終わるまで、始めたとき、Channels のどれかが変わったとき、Interval ごとに run を呼ぶ。
// 立て続けの変更は、Debounce の間待ってから 1 回にまとめる。
func Loop(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, opts *LoopOptions, run func(ctx context.Context)) error {
	if opts.Interval <= 0 {
		return fmt.Errorf("%w: interval must be positive: %s", errInvalidLoop, opts.Interval)
	}
	watcher, err := NewWatcher(ctx, pool, logger, opts.Channels, opts.Watcher)
	if err != nil {
		return fmt.Errorf("failed to watch %v: %w", opts.Channels, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- watcher.Run(ctx)
	}()

	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for {
		run(ctx)

		select {
		case <-ctx.Done():
			return <-watchErr
		case err := <-watchErr:
			return err
		case <-ticker.C:
		case <-watcher.Changes():
			select {
			case <-ctx.Done():
				return <-watchErr
			case <-time.After(opts.Debounce):
			}
			// 待つ間に来た変更は、これからの run で反映するので捨てる。
			select {
			case <-watcher.Changes():
			default:
			}
		}
	}
}
