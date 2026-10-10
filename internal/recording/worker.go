package recording

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/notify"
)

const (
	// DefaultLeaseTTL と DefaultRenewInterval は、mikan の割り振りのリースの有効期間と延長の間隔 (付録 A)。
	DefaultLeaseTTL      = 10 * time.Second
	DefaultRenewInterval = 3 * time.Second
	// DefaultAcquireInterval は、変更の通知がなくても、取れる割り振りを探す間隔。リースの切れた割り振りに気づくのに使う。
	DefaultAcquireInterval = 5 * time.Second
)

// ErrLeaseLost は、割り振りのリースを失ったことを表す。ほかの mikan が録画を引き継いだので、書き込みをやめる。
var ErrLeaseLost = errors.New("the recording assignment lease was lost")

// Assignment は、mikan が取った割り振り。
type Assignment struct {
	ID            uuid.UUID
	ReservationID uuid.UUID
	// Priority は、チューナーサーバーに渡す優先度。
	Priority int
	// Generation は、取ったときの世代番号。
	Generation int64
}

// Session は、1 つの割り振りの録画を行い、録画を終えたら返る。
// ctx は、リースを失ったとき (原因は ErrLeaseLost) と、mikan を止めるときに終わる。そのときは、書き込みをやめてすぐに返す。
type Session func(ctx context.Context, assignment *Assignment) error

// WorkerOptions は Worker の設定。0 の項目は既定値にする。
type WorkerOptions struct {
	LeaseTTL        time.Duration
	RenewInterval   time.Duration
	AcquireInterval time.Duration
	// PollOnly は、変更の通知を LISTEN で受けずに、ポーリングだけで知るかどうか。
	PollOnly     bool
	PollInterval time.Duration
}

// Worker は、mikan で、割り振りを取り、リースを延長しながら Session で録画する (設計書 1.5)。
// 複数の mikan が動いていても、1 つの割り振りを取れるのは 1 台だけである。
type Worker struct {
	pool    *pgxpool.Pool
	clock   clock.Clock
	holder  uuid.UUID
	logger  *slog.Logger
	session Session
	opts    WorkerOptions
}

// NewWorker は Worker を返す。holder は、この mikan のインスタンスの ID。opts は nil でもよい。
func NewWorker(pool *pgxpool.Pool, clk clock.Clock, holder uuid.UUID, logger *slog.Logger, session Session, opts *WorkerOptions) *Worker {
	o := WorkerOptions{
		LeaseTTL: DefaultLeaseTTL, RenewInterval: DefaultRenewInterval, AcquireInterval: DefaultAcquireInterval,
		PollOnly: false, PollInterval: notify.DefaultPollInterval,
	}
	if opts != nil {
		o.PollOnly = opts.PollOnly
		if opts.LeaseTTL > 0 {
			o.LeaseTTL = opts.LeaseTTL
		}
		if opts.RenewInterval > 0 {
			o.RenewInterval = opts.RenewInterval
		}
		if opts.AcquireInterval > 0 {
			o.AcquireInterval = opts.AcquireInterval
		}
		if opts.PollInterval > 0 {
			o.PollInterval = opts.PollInterval
		}
	}

	return &Worker{pool: pool, clock: clk, holder: holder, logger: logger, session: session, opts: o}
}

// Run は、ctx が終わるまで、割り振りが変わったときと AcquireInterval ごとに、取れる割り振りをすべて取って録画する。
// ctx が終わったら、録画中の Session が返るのを待つ。止めた録画の割り振りは、終わったことにせず、ほかの mikan か、再起動した mikan に任せる。
func (w *Worker) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	defer wg.Wait()

	return notify.Loop(ctx, w.pool, w.logger, &notify.LoopOptions{
		Channels: []string{NotifyChannel},
		Debounce: 0,
		Interval: w.opts.AcquireInterval,
		Watcher:  notify.WatcherOptions{UseListen: !w.opts.PollOnly, PollInterval: w.opts.PollInterval},
	}, func(ctx context.Context) {
		for {
			assignment, ok, err := w.acquire(ctx)
			if err != nil {
				if ctx.Err() == nil {
					w.logger.WarnContext(ctx, "failed to acquire a recording assignment", slog.Any("error", err))
				}

				return
			}
			if !ok {
				return
			}
			wg.Go(func() {
				w.record(ctx, assignment)
			})
		}
	})
}

// acquire は、取れる割り振りを 1 つ取る。なければ false を返す。
func (w *Worker) acquire(ctx context.Context) (*Assignment, bool, error) {
	now := w.clock.Now()
	row, err := sqlc.New(w.pool).AcquireRecordingAssignment(ctx, sqlc.AcquireRecordingAssignmentParams{
		Now: now, Holder: &w.holder, ExpiresAt: new(now.Add(w.opts.LeaseTTL)),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to acquire a recording assignment: %w", err)
	}

	return &Assignment{ID: row.ID, ReservationID: row.ReservationID, Priority: int(row.Priority), Generation: row.Generation}, true, nil
}

// record は、リースを延長しながら Session で録画し、録画を終えたら割り振りを終わったことにする。
func (w *Worker) record(ctx context.Context, assignment *Assignment) {
	logger := w.logger.With(slog.String("assignment_id", assignment.ID.String()), slog.String("reservation_id", assignment.ReservationID.String()),
		slog.Int64("generation", assignment.Generation))
	sessionCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		w.renew(sessionCtx, cancel, assignment, logger)
	}()

	err := w.session(sessionCtx, assignment)
	cancel(nil)
	<-renewDone
	switch {
	case errors.Is(context.Cause(sessionCtx), ErrLeaseLost):
		logger.WarnContext(ctx, "stopped recording; another mikan took over the assignment")

		return
	case ctx.Err() != nil:
		// 止めるときは、割り振りを残し、ほかの mikan か、再起動した mikan に続きを任せる (設計書 5.4)。
		return
	case err != nil:
		logger.WarnContext(ctx, "recording session failed", slog.Any("error", err))
	}
	n, err := sqlc.New(w.pool).FinishRecordingAssignment(ctx, sqlc.FinishRecordingAssignmentParams{
		Now: w.clock.Now(), ID: assignment.ID, Generation: assignment.Generation, Holder: &w.holder,
	})
	if err != nil {
		logger.WarnContext(ctx, "failed to finish the recording assignment", slog.Any("error", err))

		return
	}
	if n == 0 {
		logger.WarnContext(ctx, "the recording assignment was taken over before it finished")
	}
}

// renew は、ctx が終わるまで、RenewInterval ごとにリースを延長する。
// ほかの mikan が取ったか、延長できないままリースが切れたら、ErrLeaseLost で録画を止める。
func (w *Worker) renew(ctx context.Context, cancel context.CancelCauseFunc, assignment *Assignment, logger *slog.Logger) {
	ticker := time.NewTicker(w.opts.RenewInterval)
	defer ticker.Stop()
	expiresAt := w.clock.Now().Add(w.opts.LeaseTTL)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		now := w.clock.Now()
		n, err := sqlc.New(w.pool).RenewRecordingAssignment(ctx, sqlc.RenewRecordingAssignmentParams{
			ExpiresAt: new(now.Add(w.opts.LeaseTTL)), Now: now, ID: assignment.ID, Generation: assignment.Generation, Holder: &w.holder,
		})
		switch {
		case err != nil && ctx.Err() != nil:
			return
		case err != nil:
			logger.WarnContext(ctx, "failed to renew the recording assignment lease", slog.Any("error", err))
			// DB に届かない間も、リースが切れるまでは書き続ける。切れたら、ほかの mikan に任せる。
			if !now.Before(expiresAt) {
				cancel(ErrLeaseLost)

				return
			}
		case n == 0:
			cancel(ErrLeaseLost)

			return
		default:
			expiresAt = now.Add(w.opts.LeaseTTL)
		}
	}
}
