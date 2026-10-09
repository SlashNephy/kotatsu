package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// pruneInterval は、保持期間を過ぎた記録を消す間隔。
const pruneInterval = 24 * time.Hour

// Prune は、保持期間 retention を過ぎた記録を消し、消した数を返す。
func Prune(ctx context.Context, db sqlc.DBTX, clk clock.Clock, retention time.Duration) (int64, error) {
	deleted, err := sqlc.New(db).DeleteAuditEventsBefore(ctx, clk.Now().Add(-retention))
	if err != nil {
		return 0, fmt.Errorf("failed to prune audit events: %w", err)
	}

	return deleted, nil
}

// PruneArgs は、保持期間を過ぎた記録を消すジョブの引数。
type PruneArgs struct{}

var _ river.JobArgs = new(PruneArgs)

func (PruneArgs) Kind() string { return "audit_prune" }

// PruneWorker は、保持期間を過ぎた記録を消すジョブを実行する。
type PruneWorker struct {
	river.WorkerDefaults[PruneArgs]

	db        sqlc.DBTX
	clock     clock.Clock
	retention time.Duration
}

var _ river.Worker[PruneArgs] = new(PruneWorker)

// NewPruneWorker は PruneWorker を作る。
func NewPruneWorker(db sqlc.DBTX, clk clock.Clock, retention time.Duration) *PruneWorker {
	return &PruneWorker{WorkerDefaults: river.WorkerDefaults[PruneArgs]{}, db: db, clock: clk, retention: retention}
}

func (w *PruneWorker) Work(ctx context.Context, _ *river.Job[PruneArgs]) error {
	deleted, err := Prune(ctx, w.db, w.clock, w.retention)
	if err != nil {
		return err
	}
	// 消した数を、River のジョブの記録に残す。
	if err := river.RecordOutput(ctx, map[string]int64{"deleted": deleted}); err != nil {
		return fmt.Errorf("failed to record the job output: %w", err)
	}

	return nil
}

// PrunePeriodicJob は、保持期間を過ぎた記録を 1 日ごとに消す定期ジョブを返す。
// nekonote が起動したときにも 1 回消す。
func PrunePeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(pruneInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return PruneArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "audit_prune", RunOnStart: true},
	)
}
