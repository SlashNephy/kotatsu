package library

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

const (
	// pruneProgressInterval は、保持期間を過ぎた再生位置を消す間隔。
	pruneProgressInterval = 24 * time.Hour
	// pruneProgressBatchSize は、1 回の DELETE で消す再生位置の数の上限。長く溜まった後でも、ほかの書き込みを待たせない。
	pruneProgressBatchSize = 1000
)

// PruneProgress は、最後に更新してから retention より経った再生位置を、pruneProgressBatchSize 件ずつ区切って消し、消した数を返す。
// retention が負なら、無期限として何も消さない (#1366)。
func PruneProgress(ctx context.Context, db sqlc.DBTX, clk clock.Clock, retention time.Duration) (int64, error) {
	if retention < 0 {
		return 0, nil
	}
	before := clk.Now().Add(-retention)
	var total int64
	for {
		deleted, err := sqlc.New(db).DeleteWatchProgressBefore(ctx, sqlc.DeleteWatchProgressBeforeParams{Before: before, MaxCount: pruneProgressBatchSize})
		if err != nil {
			return total, fmt.Errorf("failed to prune the watch progress: %w", err)
		}
		total += deleted
		if deleted < pruneProgressBatchSize {
			return total, nil
		}
	}
}

// PruneProgressArgs は、保持期間を過ぎた再生位置を消すジョブの引数。
type PruneProgressArgs struct{}

var _ river.JobArgs = new(PruneProgressArgs)

func (PruneProgressArgs) Kind() string { return "watch_progress_prune" }

// PruneProgressWorker は、保持期間を過ぎた再生位置を消すジョブを実行する。
type PruneProgressWorker struct {
	river.WorkerDefaults[PruneProgressArgs]

	db    sqlc.DBTX
	clock clock.Clock
	// retention は、再生位置の保持期間を返す。画面から変えられるので、ジョブのたびに読む。無期限なら負の値を返す。
	retention func(ctx context.Context) (time.Duration, error)
}

var _ river.Worker[PruneProgressArgs] = new(PruneProgressWorker)

// NewPruneProgressWorker は PruneProgressWorker を作る。
func NewPruneProgressWorker(db sqlc.DBTX, clk clock.Clock, retention func(ctx context.Context) (time.Duration, error)) *PruneProgressWorker {
	return &PruneProgressWorker{WorkerDefaults: river.WorkerDefaults[PruneProgressArgs]{}, db: db, clock: clk, retention: retention}
}

func (w *PruneProgressWorker) Work(ctx context.Context, _ *river.Job[PruneProgressArgs]) error {
	retention, err := w.retention(ctx)
	if err != nil {
		return err
	}
	deleted, err := PruneProgress(ctx, w.db, w.clock, retention)
	if err != nil {
		return err
	}
	if err := river.RecordOutput(ctx, map[string]int64{"deleted": deleted}); err != nil {
		return fmt.Errorf("failed to record the job output: %w", err)
	}

	return nil
}

// PruneProgressPeriodicJob は、保持期間を過ぎた再生位置を 1 日ごとに消す定期ジョブを返す。nekonote が起動したときにも 1 回消す。
func PruneProgressPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(pruneProgressInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return PruneProgressArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "watch_progress_prune", RunOnStart: true},
	)
}
