package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

const (
	// pruneInterval は、保持期間を過ぎた通知を消す間隔。
	pruneInterval = 24 * time.Hour
	// pruneBatchSize は、1 回の DELETE で消す通知のイベントの数の上限。長く溜まった後でも、ほかの書き込みを待たせない。
	pruneBatchSize = 1000
)

// Prune は、配り終えた通知のイベントのうち、保持期間 retention を過ぎたものを、pruneBatchSize 件ずつ区切って消し、消した数を返す。
// ユーザーごとの通知も一緒に消える (既読か未読かにかかわらない)。retention が負なら、無期限として何も消さない (#551)。
func Prune(ctx context.Context, db sqlc.DBTX, clk clock.Clock, retention time.Duration) (int64, error) {
	if retention < 0 {
		return 0, nil
	}
	before := clk.Now().Add(-retention)
	var total int64
	for {
		deleted, err := sqlc.New(db).DeleteNotificationEventsBefore(ctx, sqlc.DeleteNotificationEventsBeforeParams{
			Before: before, MaxCount: pruneBatchSize,
		})
		if err != nil {
			return total, fmt.Errorf("failed to prune notification events: %w", err)
		}
		total += deleted
		if deleted < pruneBatchSize {
			return total, nil
		}
	}
}

// PruneArgs は、保持期間を過ぎた通知を消すジョブの引数。
type PruneArgs struct{}

var _ river.JobArgs = new(PruneArgs)

func (PruneArgs) Kind() string { return "notification_prune" }

// PruneWorker は、保持期間を過ぎた通知を消すジョブを実行する。
type PruneWorker struct {
	river.WorkerDefaults[PruneArgs]

	db    sqlc.DBTX
	clock clock.Clock
	// retention は、通知の保持期間を返す。画面から変えられるので、ジョブのたびに読む。無期限なら負の値を返す。
	retention func(ctx context.Context) (time.Duration, error)
}

var _ river.Worker[PruneArgs] = new(PruneWorker)

// NewPruneWorker は PruneWorker を作る。retention は、通知の保持期間を返す。
func NewPruneWorker(db sqlc.DBTX, clk clock.Clock, retention func(ctx context.Context) (time.Duration, error)) *PruneWorker {
	return &PruneWorker{WorkerDefaults: river.WorkerDefaults[PruneArgs]{}, db: db, clock: clk, retention: retention}
}

func (w *PruneWorker) Work(ctx context.Context, _ *river.Job[PruneArgs]) error {
	retention, err := w.retention(ctx)
	if err != nil {
		return err
	}
	deleted, err := Prune(ctx, w.db, w.clock, retention)
	if err != nil {
		return err
	}
	if err := river.RecordOutput(ctx, map[string]int64{"deleted": deleted}); err != nil {
		return fmt.Errorf("failed to record the job output: %w", err)
	}

	return nil
}

// PrunePeriodicJob は、保持期間を過ぎた通知を 1 日ごとに消す定期ジョブを返す。nekonote が起動したときにも 1 回消す。
func PrunePeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(pruneInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return PruneArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "notification_prune", RunOnStart: true},
	)
}
