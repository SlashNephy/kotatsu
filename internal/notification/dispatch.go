package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
)

const (
	// DispatchInterval は、配っていないイベントを配る間隔 (付録 A)。
	DispatchInterval = 10 * time.Second
	// dispatchBatchSize は、1 つのトランザクションで配るイベントの数の上限。
	dispatchBatchSize = 100
)

// Dispatcher は、配っていないイベントを、受け取るユーザーごとの画面内の通知に配る。
// 複数の nekonote が同時に動いても、行のロックを飛ばすので、同じイベントを二度配らない。
type Dispatcher struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewDispatcher は Dispatcher を返す。
func NewDispatcher(pool *pgxpool.Pool, clk clock.Clock) *Dispatcher {
	return &Dispatcher{pool: pool, clock: clk}
}

// DispatchOnce は、配っていないイベントがなくなるまで配り、配ったイベントの数を返す。
func (d *Dispatcher) DispatchOnce(ctx context.Context) (int, error) {
	total := 0
	for {
		n, err := d.dispatchBatch(ctx)
		total += n
		if err != nil {
			return total, err
		}
		if n < dispatchBatchSize {
			return total, nil
		}
	}
}

func (d *Dispatcher) dispatchBatch(ctx context.Context) (int, error) {
	dispatched := 0
	err := pgx.BeginFunc(ctx, d.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		events, err := q.ListPendingNotificationEvents(ctx, dispatchBatchSize)
		if err != nil {
			return fmt.Errorf("failed to list pending notification events: %w", err)
		}
		now := d.clock.Now()
		// 受け取るユーザーは種類ごとに決まるので、同じ種類のイベントでは引き直さない。
		recipientsByKind := map[string][]uuid.UUID{}
		for i := range events {
			recipients, ok := recipientsByKind[events[i].Kind]
			if !ok {
				recipients, err = q.ListNotificationRecipients(ctx, events[i].Kind)
				if err != nil {
					return fmt.Errorf("failed to list notification recipients: %w", err)
				}
				recipientsByKind[events[i].Kind] = recipients
			}
			if len(recipients) > 0 {
				ids := make([]uuid.UUID, 0, len(recipients))
				for range recipients {
					ids = append(ids, id.New())
				}
				err := q.InsertNotifications(ctx, sqlc.InsertNotificationsParams{Ids: ids, UserIds: recipients, EventID: events[i].ID, Now: now})
				if err != nil {
					return fmt.Errorf("failed to save notifications: %w", err)
				}
			}
			if err := q.MarkNotificationEventDispatched(ctx, sqlc.MarkNotificationEventDispatchedParams{ID: events[i].ID, Now: &now}); err != nil {
				return fmt.Errorf("failed to mark the notification event dispatched: %w", err)
			}
		}
		dispatched = len(events)

		return nil
	})
	if err != nil {
		return 0, err
	}

	return dispatched, nil
}

// DispatchArgs は、配っていないイベントを配るジョブの引数。
type DispatchArgs struct{}

var _ river.JobArgs = new(DispatchArgs)

func (DispatchArgs) Kind() string { return "notification_dispatch" }

// DispatchWorker は、配っていないイベントを配るジョブを実行する。
type DispatchWorker struct {
	river.WorkerDefaults[DispatchArgs]

	dispatcher *Dispatcher
}

var _ river.Worker[DispatchArgs] = new(DispatchWorker)

// NewDispatchWorker は DispatchWorker を返す。
func NewDispatchWorker(dispatcher *Dispatcher) *DispatchWorker {
	return &DispatchWorker{WorkerDefaults: river.WorkerDefaults[DispatchArgs]{}, dispatcher: dispatcher}
}

func (w *DispatchWorker) Work(ctx context.Context, _ *river.Job[DispatchArgs]) error {
	dispatched, err := w.dispatcher.DispatchOnce(ctx)
	if err != nil {
		return err
	}
	if err := river.RecordOutput(ctx, map[string]int{"dispatched": dispatched}); err != nil {
		return fmt.Errorf("failed to record the job output: %w", err)
	}

	return nil
}

// DispatchPeriodicJob は、配っていないイベントを DispatchInterval ごとに配る定期ジョブを返す。
func DispatchPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(DispatchInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return DispatchArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "notification_dispatch", RunOnStart: true},
	)
}
