package recording

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/notify"
	"github.com/SlashNephy/kotatsu/internal/registry"
)

const (
	// DefaultLeaseTTL と DefaultRenewInterval は、mikan の割り振りのリースの有効期間と延長の間隔 (付録 A)。
	DefaultLeaseTTL      = 10 * time.Second
	DefaultRenewInterval = 3 * time.Second
	// DefaultAcquireInterval は、変更の通知がなくても、取れる割り振りを探す間隔。リースの切れた割り振りに気づくのに使う。
	DefaultAcquireInterval = 5 * time.Second
	// DefaultHandoverTimeout は、止まる mikan が、ほかの mikan がストリームを開くのを待つ時間の上限 (設計書 5.4)。
	// Kubernetes の既定の猶予 (30 秒) に収まるようにする。
	DefaultHandoverTimeout = 20 * time.Second
	// mikanRole は、録画のロールの名前。引き継ぐ mikan がいるかを、インスタンスの登録簿で調べるのに使う。
	mikanRole = "mikan"
)

var (
	// ErrLeaseLost は、割り振りのリースを失ったことを表す。ほかの mikan が録画を引き継いだので、書き込みをやめる。
	ErrLeaseLost = errors.New("the recording assignment lease was lost")
	// ErrHandedOver は、止まる mikan が、ほかの mikan にストリームを開いてもらい、録画を引き継いだことを表す (設計書 5.4)。
	ErrHandedOver = errors.New("the recording was handed over to another mikan")
	// ErrStopped は、mikan を止めるので、録画をやめたことを表す。引き継ぐ mikan がいなかったか、引き継ぎを待ちきれなかった。
	ErrStopped = errors.New("the mikan is stopping")
)

// Assignment は、mikan が取った割り振り。
type Assignment struct {
	ID            uuid.UUID
	ReservationID uuid.UUID
	// Priority は、チューナーサーバーに渡す優先度。
	Priority int
	// Generation は、取ったときの世代番号。
	Generation int64
	// HandedOver は、止まる mikan から引き継いだかどうか。引き継いだなら、古い mikan がまだ書いているので、録画は途切れていない。
	HandedOver bool
	// PreviousLeaseExpiresAt は、前の mikan のリースの期限。落ちた mikan の代わりに取ったとき、途切れた区間の始まりを見積もるのに使う。
	PreviousLeaseExpiresAt *time.Time
}

// Session は、1 つの割り振りの録画を行い、録画を終えたら返る。
// ctx は、リースを失ったとき (原因は ErrLeaseLost) と、mikan を止めるときに終わる。そのときは、書き込みをやめてすぐに返す。
type Session func(ctx context.Context, assignment *Assignment) error

// WorkerOptions は Worker の設定。0 の項目は既定値にする。
type WorkerOptions struct {
	LeaseTTL        time.Duration
	RenewInterval   time.Duration
	AcquireInterval time.Duration
	HandoverTimeout time.Duration
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

	mu     sync.Mutex
	active map[uuid.UUID]*activeSession
}

// NewWorker は Worker を返す。holder は、この mikan のインスタンスの ID。opts は nil でもよい。
func NewWorker(pool *pgxpool.Pool, clk clock.Clock, holder uuid.UUID, logger *slog.Logger, session Session, opts *WorkerOptions) *Worker {
	o := WorkerOptions{
		LeaseTTL: DefaultLeaseTTL, RenewInterval: DefaultRenewInterval, AcquireInterval: DefaultAcquireInterval,
		HandoverTimeout: DefaultHandoverTimeout, PollOnly: false, PollInterval: notify.DefaultPollInterval,
	}
	if opts != nil {
		o = WorkerOptions{
			LeaseTTL:        orDefault(opts.LeaseTTL, o.LeaseTTL),
			RenewInterval:   orDefault(opts.RenewInterval, o.RenewInterval),
			AcquireInterval: orDefault(opts.AcquireInterval, o.AcquireInterval),
			HandoverTimeout: orDefault(opts.HandoverTimeout, o.HandoverTimeout),
			PollOnly:        opts.PollOnly,
			PollInterval:    orDefault(opts.PollInterval, o.PollInterval),
		}
	}

	return &Worker{pool: pool, clock: clk, holder: holder, logger: logger, session: session, opts: o, mu: sync.Mutex{}, active: map[uuid.UUID]*activeSession{}}
}

// activeSession は、録画している割り振りと、その Session を止める関数。
type activeSession struct {
	assignment *Assignment
	cancel     context.CancelCauseFunc
	// handingOver は、引き継ぎ待ちかどうか。引き継ぎ待ちの間は、世代が進んでリースを延長できなくても、ストリームを閉じない。
	handingOver atomic.Bool
	// done は、Session が返ったら閉じる。引き継ぎを待つ間に録画を終えたら、待つのをやめる。
	done chan struct{}
}

// Run は、ctx が終わるまで、割り振りが変わったときと AcquireInterval ごとに、取れる割り振りをすべて取って録画する。
//
// ctx が終わったら、録画を引き継ぐ (設計書 5.4)。ほかの mikan が動いていれば、割り振りに引き継ぎ待ちの印を付け、
// ほかの mikan がストリームを開くまで録り続けてから止める。いなければ、割り振りを手放してすぐに止め、再起動した mikan に続きを任せる。
// どちらも、割り振りは終わったことにしない。Session がすべて返るまで待つ。
func (w *Worker) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	// Session は、ctx が終わっても、引き継ぎを終えるまで録り続けるので、ctx とは別に止める。
	sessionsCtx, stopSessions := context.WithCancel(context.WithoutCancel(ctx))
	defer stopSessions()

	err := notify.Loop(ctx, w.pool, w.logger, &notify.LoopOptions{
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
			// 止めるときに引き継ぎから漏れないよう、Session を始める前に、録画している割り振りに載せる。
			active, sessionCtx := w.register(sessionsCtx, assignment)
			wg.Go(func() {
				w.record(sessionCtx, active)
			})
		}
	})
	w.handOverAll(sessionsCtx, &wg)
	wg.Wait()

	return err
}

// handOverAll は、録画しているすべての割り振りを、ほかの mikan に引き継ぐか、手放す。
func (w *Worker) handOverAll(ctx context.Context, wg *sync.WaitGroup) {
	w.mu.Lock()
	sessions := make([]*activeSession, 0, len(w.active))
	for _, active := range w.active {
		sessions = append(sessions, active)
	}
	w.mu.Unlock()
	if len(sessions) == 0 {
		return
	}
	peers, err := w.peers(ctx)
	if err != nil {
		w.logger.WarnContext(ctx, "failed to find another mikan; stopping without handover", slog.Any("error", err))
	}
	for _, active := range sessions {
		wg.Go(func() {
			w.handOver(ctx, active, peers > 0)
		})
	}
}

// peers は、動いているほかの mikan の数を返す。
func (w *Worker) peers(ctx context.Context) (int, error) {
	instances, err := sqlc.New(w.pool).ListAliveInstances(ctx, sqlc.ListAliveInstancesParams{
		Role: mikanRole, AliveSince: w.clock.Now().Add(-registry.AliveTTL),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to list alive mikan instances: %w", err)
	}
	n := 0
	for i := range instances {
		if instances[i].ID != w.holder {
			n++
		}
	}

	return n, nil
}

// handOver は、1 つの割り振りを、ほかの mikan に引き継ぐ。引き継げなければ、割り振りを手放して止める。
func (w *Worker) handOver(ctx context.Context, active *activeSession, hasPeers bool) {
	a := active.assignment
	logger := w.logger.With(slog.String("assignment_id", a.ID.String()), slog.Int64("generation", a.Generation))
	if hasPeers {
		marked, err := w.markHandover(ctx, a)
		if err != nil {
			logger.WarnContext(ctx, "failed to mark the recording for handover", slog.Any("error", err))
		}
		if marked {
			active.handingOver.Store(true)
			logger.InfoContext(ctx, "waiting for another mikan to take over the recording")
			if w.waitForSuccessor(ctx, active) {
				logger.InfoContext(ctx, "another mikan took over the recording")
				active.cancel(ErrHandedOver)

				return
			}
			logger.WarnContext(ctx, "no mikan took over the recording in time")
		}
	}
	// 引き継ぐ mikan がいなければ、録画を止め、再起動した mikan がすぐに取れるよう、割り振りを手放す。
	// 先に手放すと、止めるまでの間にリースの延長に失敗し、リースを失ったものとして止まるので、止めてから手放す。
	active.cancel(ErrStopped)
	<-active.done
	err := sqlc.New(w.pool).ReleaseRecordingAssignment(ctx, sqlc.ReleaseRecordingAssignmentParams{
		Now: w.clock.Now(), ID: a.ID, Generation: a.Generation, Holder: &w.holder,
	})
	if err != nil {
		logger.WarnContext(ctx, "failed to release the recording assignment", slog.Any("error", err))
	}
}

// markHandover は、割り振りに引き継ぎ待ちの印を付け、ほかの mikan に知らせる。
func (w *Worker) markHandover(ctx context.Context, a *Assignment) (bool, error) {
	marked := false
	err := pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		n, err := sqlc.New(tx).MarkRecordingHandover(ctx, sqlc.MarkRecordingHandoverParams{
			Now: w.clock.Now(), ID: a.ID, Generation: a.Generation, Holder: &w.holder,
		})
		if err != nil {
			return fmt.Errorf("failed to mark the recording for handover: %w", err)
		}
		if n == 0 {
			return nil
		}
		marked = true
		_, err = notify.Publish(ctx, tx, w.clock, NotifyChannel)

		return err
	})

	return marked, err
}

// waitForSuccessor は、ほかの mikan が割り振りを取ってストリームを開くまで、HandoverTimeout の間待つ。開いたら true を返す。
// 待つ間に録画を終えたら、引き継ぐものがないので true を返す。
func (w *Worker) waitForSuccessor(ctx context.Context, active *activeSession) bool {
	a := active.assignment
	timeout := time.NewTimer(w.opts.HandoverTimeout)
	defer timeout.Stop()
	ticker := time.NewTicker(w.opts.RenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-timeout.C:
			return false
		case <-active.done:
			return true
		case <-ticker.C:
		}
		stream, err := sqlc.New(w.pool).GetRecordingAssignmentStream(ctx, a.ID)
		if err != nil {
			w.logger.WarnContext(ctx, "failed to check the handover", slog.Any("error", err))

			continue
		}
		if stream.StreamGeneration > a.Generation {
			return true
		}
	}
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

	// 引き継ぎ待ちでも、古い mikan のリースが切れていれば、引き継ぎの最中に落ちたので、落ちた mikan の代わりとして扱う。
	handedOver := row.PreviousState == string(AssignmentHandover) && row.PreviousLeaseExpiresAt != nil && row.PreviousLeaseExpiresAt.After(now)

	return &Assignment{
		ID: row.ID, ReservationID: row.ReservationID, Priority: int(row.Priority), Generation: row.Generation,
		HandedOver: handedOver, PreviousLeaseExpiresAt: row.PreviousLeaseExpiresAt,
	}, true, nil
}

// register は、割り振りを、録画している割り振りに載せ、その Session の context を返す。
func (w *Worker) register(ctx context.Context, assignment *Assignment) (*activeSession, context.Context) {
	sessionCtx, cancel := context.WithCancelCause(ctx)
	active := &activeSession{assignment: assignment, cancel: cancel, handingOver: atomic.Bool{}, done: make(chan struct{})}
	w.mu.Lock()
	w.active[assignment.ID] = active
	w.mu.Unlock()

	return active, sessionCtx
}

// record は、リースを延長しながら Session で録画し、録画を終えたら割り振りを終わったことにする。
// sessionCtx は、register が返した Session の context。
func (w *Worker) record(sessionCtx context.Context, active *activeSession) {
	assignment, cancel := active.assignment, active.cancel
	// Session を止めた後も、割り振りを終わったことにするため、止まらない context を使う。
	ctx := context.WithoutCancel(sessionCtx)
	logger := w.logger.With(slog.String("assignment_id", assignment.ID.String()), slog.String("reservation_id", assignment.ReservationID.String()),
		slog.Int64("generation", assignment.Generation))
	defer cancel(nil)
	defer func() {
		w.mu.Lock()
		delete(w.active, assignment.ID)
		w.mu.Unlock()
		close(active.done)
	}()
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		w.renew(sessionCtx, active, logger)
	}()

	err := w.session(sessionCtx, assignment)
	cancel(nil)
	<-renewDone
	switch cause := context.Cause(sessionCtx); {
	case errors.Is(cause, ErrLeaseLost):
		logger.WarnContext(ctx, "stopped recording; another mikan took over the assignment")

		return
	case errors.Is(cause, ErrHandedOver), errors.Is(cause, ErrStopped):
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
func (w *Worker) renew(ctx context.Context, active *activeSession, logger *slog.Logger) {
	assignment, cancel := active.assignment, active.cancel
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
		case n == 0 && active.handingOver.Load():
			// 引き継ぎ待ちの間に世代が進んだのは、ほかの mikan が取ったからである。ストリームを開くまで録り続ける。
			return
		case n == 0 && w.assignmentDeleted(ctx, assignment.ID, logger):
			// 録画中に予約が消され、割り振りも一緒に消えた。録画は、予約を読み直したときに取り消しとして終える。
			logger.InfoContext(ctx, "the recording assignment was deleted with its reservation")

			return
		case n == 0:
			cancel(ErrLeaseLost)

			return
		default:
			expiresAt = now.Add(w.opts.LeaseTTL)
		}
	}
}

// assignmentDeleted は、割り振りが DB から消えたかどうかを返す。確かめられなければ、消えていないとみなす。
func (w *Worker) assignmentDeleted(ctx context.Context, id uuid.UUID, logger *slog.Logger) bool {
	exists, err := sqlc.New(w.pool).RecordingAssignmentExists(ctx, id)
	if err != nil {
		logger.WarnContext(ctx, "failed to check the recording assignment", slog.Any("error", err))

		return false
	}

	return !exists
}

// orDefault は、v が 0 以下なら d を返す。
func orDefault(v, d time.Duration) time.Duration {
	if v > 0 {
		return v
	}

	return d
}
