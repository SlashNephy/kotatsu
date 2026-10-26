package timeshift

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

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
// タイムシフトから外したチャンネルに、録る mikan のいない書きかけのチャンクや放送中のままの番組が残っていれば、先に閉じる。
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
	errs := []error{}
	// 外したチャンネルの、録る mikan のいない書きかけを先に閉じる。閉じたチャンクは、設定がないので、この回で捨てる。
	abandoned, err := q.ListAbandonedTimeshiftServices(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list abandoned timeshift services: %w", err)
	}
	for _, serviceID := range abandoned {
		if err := closeStale(ctx, q, w.store, serviceID); err != nil {
			errs = append(errs, fmt.Errorf("service %d: %w", serviceID, err))

			continue
		}
		w.logger.InfoContext(ctx, "closed the abandoned timeshift chunks of a removed channel", slog.Int64("service_id", serviceID))
	}
	expired, err := q.ListExpiredTimeshiftChunks(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("failed to list expired timeshift chunks: %w", err)
	}
	pruned := 0
	for i := range expired {
		chunk := &expired[i]
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
	// 番組の行は、チャンクを捨てた後に、全サービスでまとめて整理する。外したチャンネルの行も残さない。
	if _, err := q.DeleteStaleTimeshiftPrograms(ctx, now); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete old timeshift programs: %w", err))
	}
	if pruned > 0 {
		w.logger.InfoContext(ctx, "pruned timeshift chunks", slog.Int("count", pruned))
	}

	return pruned, errors.Join(errs...)
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
