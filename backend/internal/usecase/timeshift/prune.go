package timeshift

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// PruneInterval は、保持時間を過ぎたチャンクを捨てる間隔 (付録 A)。
const PruneInterval = time.Minute

// PruneArgs は、保持時間を過ぎたチャンクを捨てるジョブの引数。
type PruneArgs struct{}

var _ river.JobArgs = new(PruneArgs)

func (PruneArgs) Kind() string { return "timeshift_prune" }

// PruneWorker は、保持時間を過ぎたチャンクを捨てるジョブを実行する。
// チャンクのファイル、索引、DB の行の順に消し、再生できる範囲の外になった番組の行も消す。
type PruneWorker struct {
	river.WorkerDefaults[PruneArgs]

	pool   *pgxpool.Pool
	clock  clock.Clock
	store  *kago.Store
	logger *slog.Logger
}

var _ river.Worker[PruneArgs] = new(PruneWorker)

// NewPruneWorker は PruneWorker を返す。store は、このインスタンスにマウントされたプール。
func NewPruneWorker(pool *pgxpool.Pool, clk clock.Clock, store *kago.Store, logger *slog.Logger) *PruneWorker {
	return &PruneWorker{WorkerDefaults: river.WorkerDefaults[PruneArgs]{}, pool: pool, clock: clk, store: store, logger: logger}
}

func (w *PruneWorker) Work(ctx context.Context, _ *river.Job[PruneArgs]) error {
	pruned, err := w.RunOnce(ctx)
	if recordErr := river.RecordOutput(ctx, map[string]int{"pruned": pruned}); recordErr != nil {
		return errors.Join(err, fmt.Errorf("failed to record the job output: %w", recordErr))
	}

	return err
}

// RunOnce は、保持時間を過ぎたチャンクを捨て、捨てたチャンクの数を返す。
// タイムシフトのディレクトリを設定していなければ、何もしない。
func (w *PruneWorker) RunOnce(ctx context.Context) (int, error) {
	if !w.store.HasTimeshiftDir() {
		return 0, nil
	}
	now := w.clock.Now()
	q := sqlc.New(w.pool)
	expired, err := q.ListExpiredTimeshiftChunks(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("failed to list expired timeshift chunks: %w", err)
	}
	channels, err := q.ListTimeshiftChannels(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list timeshift channels: %w", err)
	}
	// 番組の行は、チャンクを捨てたサービスのほか、設定のあるサービスでも整理する。
	services := map[int64]struct{}{}
	for i := range channels {
		services[channels[i].ServiceID] = struct{}{}
	}
	pruned := 0
	errs := []error{}
	for i := range expired {
		chunk := &expired[i]
		services[chunk.ServiceID] = struct{}{}
		// ファイルがもうなくても、行は消す。ファイルを消せなければ、行を残して次の周期でやり直す。
		if err := w.store.Remove(kago.TimeshiftPoolID, chunk.Path, chunk.ID); err != nil {
			errs = append(errs, fmt.Errorf("chunk %s: %w", chunk.ID, err))

			continue
		}
		if _, err := q.DeleteTimeshiftChunk(ctx, chunk.ID); err != nil {
			errs = append(errs, fmt.Errorf("chunk %s: failed to delete the row: %w", chunk.ID, err))

			continue
		}
		pruned++
	}
	for serviceID := range services {
		if err := w.pruneProgramsOf(ctx, q, serviceID, now); err != nil {
			errs = append(errs, fmt.Errorf("service %d: %w", serviceID, err))
		}
	}
	if pruned > 0 {
		w.logger.InfoContext(ctx, "pruned timeshift chunks", slog.Int("count", pruned))
	}

	return pruned, errors.Join(errs...)
}

// pruneProgramsOf は、サービスの残っているチャンクの最も早い始まりより前に終わった番組を消す。
// チャンクが 1 つも残っていなければ、今より前に終わった番組を消す。
func (w *PruneWorker) pruneProgramsOf(ctx context.Context, q *sqlc.Queries, serviceID int64, now time.Time) error {
	before, err := q.GetOldestTimeshiftChunkStart(ctx, serviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		before = now
	} else if err != nil {
		return fmt.Errorf("failed to get the oldest timeshift chunk: %w", err)
	}
	if _, err := q.DeleteTimeshiftProgramsBefore(ctx, sqlc.DeleteTimeshiftProgramsBeforeParams{ServiceID: serviceID, Before: before}); err != nil {
		return fmt.Errorf("failed to delete old timeshift programs: %w", err)
	}

	return nil
}

// PrunePeriodicJob は、保持時間を過ぎたチャンクを PruneInterval ごとに捨てる定期ジョブを返す。
func PrunePeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(PruneInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return PruneArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "timeshift_prune", RunOnStart: true},
	)
}
