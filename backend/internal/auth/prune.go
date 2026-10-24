package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

const (
	// sessionPruneInterval は、使えなくなったセッションを消す間隔。
	sessionPruneInterval = 24 * time.Hour
	// sessionPruneBatchSize は、1 回の DELETE で消すセッションの数の上限。
	sessionPruneBatchSize = 1000
)

// PruneSessions は、使えなくなったセッション (失効させた、絶対的な期限を過ぎた、無操作の期限を過ぎた) を、
// sessionPruneBatchSize 件ずつ区切って消し、消した数を返す。
func PruneSessions(ctx context.Context, db sqlc.DBTX, clk clock.Clock) (int64, error) {
	now := clk.Now()
	var total int64
	for {
		deleted, err := sqlc.New(db).DeleteStaleSessions(ctx, sqlc.DeleteStaleSessionsParams{
			Now: now, IdleSince: now.Add(-DefaultIdleTimeout), MaxCount: sessionPruneBatchSize,
		})
		if err != nil {
			return total, fmt.Errorf("failed to prune sessions: %w", err)
		}
		total += deleted
		if deleted < sessionPruneBatchSize {
			return total, nil
		}
	}
}

// PruneSessionsArgs は、使えなくなったセッションを消すジョブの引数。
type PruneSessionsArgs struct{}

var _ river.JobArgs = new(PruneSessionsArgs)

func (PruneSessionsArgs) Kind() string { return "session_prune" }

// PruneSessionsWorker は、使えなくなったセッションを消すジョブを実行する。
type PruneSessionsWorker struct {
	river.WorkerDefaults[PruneSessionsArgs]

	db    sqlc.DBTX
	clock clock.Clock
}

var _ river.Worker[PruneSessionsArgs] = new(PruneSessionsWorker)

// NewPruneSessionsWorker は PruneSessionsWorker を作る。
func NewPruneSessionsWorker(db sqlc.DBTX, clk clock.Clock) *PruneSessionsWorker {
	return &PruneSessionsWorker{WorkerDefaults: river.WorkerDefaults[PruneSessionsArgs]{}, db: db, clock: clk}
}

func (w *PruneSessionsWorker) Work(ctx context.Context, _ *river.Job[PruneSessionsArgs]) error {
	deleted, err := PruneSessions(ctx, w.db, w.clock)
	if err != nil {
		return err
	}
	// 消した数を、River のジョブの記録に残す。
	if err := river.RecordOutput(ctx, map[string]int64{"deleted": deleted}); err != nil {
		return fmt.Errorf("failed to record the job output: %w", err)
	}

	return nil
}

// PruneSessionsPeriodicJob は、使えなくなったセッションを 1 日ごとに消す定期ジョブを返す。
// nekonote が起動したときにも 1 回消す。
func PruneSessionsPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(sessionPruneInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return PruneSessionsArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "session_prune", RunOnStart: true},
	)
}
