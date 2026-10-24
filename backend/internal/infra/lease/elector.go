package lease

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// Lead は、リーダーになっている間に動く処理。ctx はリースを失ったときと Run が止まるときに終わる。
// generation は、書き込みを Manager.Guard で守るときに渡す世代番号。
type Lead func(ctx context.Context, generation int64) error

// Elector は、リースを取り合ってリーダーを 1 つに決める。
type Elector struct {
	manager       *Manager
	clock         clock.Clock
	logger        *slog.Logger
	ttl           time.Duration
	renewInterval time.Duration
}

// NewElector は Elector を作る。renewInterval ごとにリースの取得と延長を試みる。
//
// リーダーは、最後に延長した時刻から ttl - renewInterval が過ぎても延長できなければ、リースを失ったものとして
// Lead を止める。リースの期限 (ttl) より renewInterval だけ早く止めるので、新しいリーダーが動き始める前に、
// 古いリーダーは止まっている。そのため renewInterval は ttl の半分より短くする。
func NewElector(manager *Manager, logger *slog.Logger, renewInterval time.Duration) (*Elector, error) {
	if renewInterval <= 0 || renewInterval*2 > manager.ttl {
		return nil, fmt.Errorf("renew interval %s must be positive and at most half of the lease TTL %s", renewInterval, manager.ttl)
	}

	return &Elector{
		manager:       manager,
		clock:         manager.clock,
		logger:        logger.With(slog.String("lease", manager.name)),
		ttl:           manager.ttl,
		renewInterval: renewInterval,
	}, nil
}

// Run は、ctx が終わるまでリースを取り合い、リーダーになっている間は lead を動かす。
// lead がエラーを返したときは、リースを手放し、次の間隔で取り直す。ctx が終わるまで返らない (#528)。
func (e *Elector) Run(ctx context.Context, lead Lead) error {
	ticker := time.NewTicker(e.renewInterval)
	defer ticker.Stop()

	for {
		generation, acquired, err := e.manager.TryAcquire(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			e.logger.WarnContext(ctx, "failed to acquire the lease", slog.Any("error", err))
		case acquired:
			// リーダーの処理が失敗しても、Run は終えずにリーダーを降り、次の間隔で取り直す (#528)。
			// 終えると、全ロールを 1 つのプロセスで動かす構成では、一時的な失敗で、録画中の mikan ごと止まる。
			if err := e.lead(ctx, generation, lead, ticker); err != nil {
				e.logger.ErrorContext(ctx, "the leader failed; stepping down and retrying", slog.Any("error", err))
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// lead は、generation の世代のリースを延長しながら lead を動かす。リースを失うか ctx が終わると戻る。
func (e *Elector) lead(ctx context.Context, generation int64, lead Lead, ticker *time.Ticker) error {
	logger := e.logger.With(slog.Int64("generation", generation))
	logger.InfoContext(ctx, "became the leader")

	leaderCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- lead(leaderCtx, generation)
	}()

	// 延長できないまま、この時間が過ぎたらリースを失ったものとみなす。
	gracePeriod := e.ttl - e.renewInterval
	deadline := time.NewTimer(gracePeriod)
	defer deadline.Stop()

	stop := func(reason string) error {
		cancel()
		err := <-done
		e.release(ctx, logger, generation)
		logger.InfoContext(ctx, "stopped leading", slog.String("reason", reason))
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("leader of the lease %s failed: %w", e.manager.name, err)
		}

		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return stop("shutting down")
		case err := <-done:
			done <- err

			return stop("lead returned")
		case <-deadline.C:
			return stop("could not renew the lease in time")
		case <-ticker.C:
			renewedAt, err := e.manager.Renew(ctx, generation)
			if errors.Is(err, ErrLost) {
				return stop("lease lost")
			}
			if err != nil {
				// 一時的な DB の障害かもしれないので、deadline までは延長を試み続ける。
				logger.WarnContext(ctx, "failed to renew the lease", slog.Any("error", err))

				continue
			}
			// 延長の要求を出した時刻から数え直す。DB の期限も同じ時刻から数えている。
			deadline.Reset(gracePeriod - e.clock.Now().Sub(renewedAt))
		}
	}
}

// release はリースを手放す。終了のシグナルで ctx が終わっていても手放せるよう、別の context を使う。
func (e *Elector) release(ctx context.Context, logger *slog.Logger, generation int64) {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), e.renewInterval)
	defer cancel()
	if err := e.manager.Release(releaseCtx, generation); err != nil {
		logger.WarnContext(releaseCtx, "failed to release the lease", slog.Any("error", err))
	}
}
