package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// pruneInterval は、保持期間を過ぎた記録を消す間隔。
const pruneInterval = 24 * time.Hour

// pruneBatchSize は、1 回の DELETE で消す記録の数の上限。
// 長く溜まった後に保持期間を短くしても、1 回のトランザクションで大量の行を消して、ほかの書き込みを待たせないようにする。
const pruneBatchSize = 1000

// Prune は、保持期間 retention を過ぎた記録を、pruneBatchSize 件ずつ区切って消し、消した数を返す。
func Prune(ctx context.Context, db sqlc.DBTX, clk clock.Clock, retention time.Duration) (int64, error) {
	before := clk.Now().Add(-retention)
	var total int64
	for {
		deleted, err := sqlc.New(db).DeleteAuditEventsBefore(ctx, sqlc.DeleteAuditEventsBeforeParams{Before: before, MaxCount: pruneBatchSize})
		if err != nil {
			return total, fmt.Errorf("failed to prune audit events: %w", err)
		}
		total += deleted
		if deleted < pruneBatchSize {
			return total, nil
		}
	}
}

// PruneClientInfo は、保持期間 retention を過ぎた記録の IP アドレスと User-Agent を、pruneBatchSize 件ずつ区切って空にし、空にした数を返す。
// 操作の記録そのものは Prune の保持期間まで残す (#1367)。
func PruneClientInfo(ctx context.Context, db sqlc.DBTX, clk clock.Clock, retention time.Duration) (int64, error) {
	before := clk.Now().Add(-retention)
	var total int64
	for {
		cleared, err := sqlc.New(db).ClearAuditEventClientInfoBefore(ctx, sqlc.ClearAuditEventClientInfoBeforeParams{Before: before, MaxCount: pruneBatchSize})
		if err != nil {
			return total, fmt.Errorf("failed to clear the client info of audit events: %w", err)
		}
		total += cleared
		if cleared < pruneBatchSize {
			return total, nil
		}
	}
}

// PruneArgs は、保持期間を過ぎた記録を消すジョブの引数。
type PruneArgs struct{}

var _ river.JobArgs = new(PruneArgs)

func (PruneArgs) Kind() string { return "audit_prune" }

// PruneWorker は、保持期間を過ぎた記録を消すジョブを実行する。
type PruneWorker struct {
	river.WorkerDefaults[PruneArgs]

	db                  sqlc.DBTX
	clock               clock.Clock
	retention           time.Duration
	clientInfoRetention time.Duration
}

var _ river.Worker[PruneArgs] = new(PruneWorker)

// NewPruneWorker は PruneWorker を作る。
func NewPruneWorker(db sqlc.DBTX, clk clock.Clock, retention, clientInfoRetention time.Duration) *PruneWorker {
	return &PruneWorker{WorkerDefaults: river.WorkerDefaults[PruneArgs]{}, db: db, clock: clk, retention: retention, clientInfoRetention: clientInfoRetention}
}

func (w *PruneWorker) Work(ctx context.Context, _ *river.Job[PruneArgs]) error {
	deleted, err := Prune(ctx, w.db, w.clock, w.retention)
	if err != nil {
		return err
	}
	cleared, err := PruneClientInfo(ctx, w.db, w.clock, w.clientInfoRetention)
	if err != nil {
		return err
	}
	// 消した数を、River のジョブの記録に残す。
	if err := river.RecordOutput(ctx, map[string]int64{"deleted": deleted, "client_info_cleared": cleared}); err != nil {
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
