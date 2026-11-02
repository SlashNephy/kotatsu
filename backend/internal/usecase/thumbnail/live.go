package thumbnail

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// LiveInterval は、録画中の録画の代表画像を作り直す間隔 (#1236、付録 A)。録り始めてからこの長さがたった録画から作る。
const LiveInterval = 5 * time.Minute

// LiveArgs は、録画中の録画の代表画像を作り直すジョブを入れる、定期ジョブの引数。
type LiveArgs struct{}

var _ river.JobArgs = new(LiveArgs)

func (LiveArgs) Kind() string { return "thumbnail_live" }

// LiveWorker は、録画中の録画ごとに、今の様子の代表画像を作るジョブ (GenerateArgs.Live) を入れる。
// 同じ録画のジョブを待っている間は、重ねて入れない (GenerateArgs.InsertOpts)。
type LiveWorker struct {
	river.WorkerDefaults[LiveArgs]

	db       Beginner
	inserter Inserter
	clock    clock.Clock
}

var _ river.Worker[LiveArgs] = new(LiveWorker)

// NewLiveWorker は LiveWorker を返す。
func NewLiveWorker(db Beginner, inserter Inserter, clk clock.Clock) *LiveWorker {
	return &LiveWorker{WorkerDefaults: river.WorkerDefaults[LiveArgs]{}, db: db, inserter: inserter, clock: clk}
}

func (w *LiveWorker) Work(ctx context.Context, _ *river.Job[LiveArgs]) error {
	return pgx.BeginFunc(ctx, w.db, func(tx pgx.Tx) error {
		ids, err := sqlc.New(tx).ListLiveThumbnailRecordings(ctx, w.clock.Now().Add(-LiveInterval))
		if err != nil {
			return fmt.Errorf("failed to list the recordings being recorded: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}
		params := make([]river.InsertManyParams, 0, len(ids))
		for _, id := range ids {
			params = append(params, river.InsertManyParams{Args: GenerateArgs{RecordingID: id, Live: true}, InsertOpts: nil})
		}
		if _, err := w.inserter.InsertManyTx(ctx, tx, params); err != nil {
			return fmt.Errorf("failed to insert the live thumbnail jobs: %w", err)
		}

		return nil
	})
}

// LivePeriodicJob は、録画中の録画の代表画像を、LiveInterval ごとに作り直す定期ジョブを返す。
func LivePeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(LiveInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return LiveArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "thumbnail_live", RunOnStart: false},
	)
}
