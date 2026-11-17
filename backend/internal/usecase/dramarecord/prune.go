package dramarecord

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// pruneInterval は、期限の切れた認可の途中の状態を消す間隔。
const pruneInterval = 24 * time.Hour

// PruneArgs は、期限の切れた認可の途中の状態を消すジョブの引数。
type PruneArgs struct{}

var _ river.JobArgs = new(PruneArgs)

func (PruneArgs) Kind() string { return "drama_record_prune" }

// PruneWorker は、期限の切れた認可の途中の状態を消すジョブを実行する。認可を始めたまま戻ってこなかった行を残さない。
type PruneWorker struct {
	river.WorkerDefaults[PruneArgs]

	service *Service
	db      Database
}

var _ river.Worker[PruneArgs] = new(PruneWorker)

// NewPruneWorker は PruneWorker を作る。
func NewPruneWorker(service *Service, db Database) *PruneWorker {
	return &PruneWorker{WorkerDefaults: river.WorkerDefaults[PruneArgs]{}, service: service, db: db}
}

func (w *PruneWorker) Work(ctx context.Context, _ *river.Job[PruneArgs]) error {
	if _, err := sqlc.New(w.db).DeleteExpiredDramaRecordAuthorizations(ctx, w.service.clock.Now()); err != nil {
		return fmt.Errorf("failed to delete expired drama record authorizations: %w", err)
	}

	return nil
}

// PrunePeriodicJob は、期限の切れた認可の途中の状態を 1 日ごとに消す定期ジョブを返す。
func PrunePeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(pruneInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return PruneArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "drama_record_prune", RunOnStart: false},
	)
}
