// Package capacity は、保存先プールの容量を管理する (設計書 6.1)。
//
// nekonote の定期ジョブが、各プールの空き容量を調べる。空き容量が閾値を下回ったら、保護されていない古い録画から消し、
// 空き容量の不足を通知する。録画を消しても、重複の回避に使う録画履歴は残す。
package capacity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/library"
	"github.com/SlashNephy/kotatsu/internal/notification"
)

const (
	// CheckInterval は、空き容量を調べる間隔 (付録 A)。
	CheckInterval = 5 * time.Minute
	// maxDeletionsPerPool は、1 回の確認で 1 つのプールから消す録画の数の上限。容量の計算を誤っても、録画を一度に消しすぎないようにする。
	maxDeletionsPerPool = 50
	// auditRole は、容量の管理で録画を消す操作者のロール。
	auditRole = "nekonote"
	// segmentKind は、録画ファイル (セグメント) の種類。
	segmentKind = "segment"
)

// Manager は、保存先プールの容量を管理する。
type Manager struct {
	pool    *pgxpool.Pool
	clock   clock.Clock
	store   *kago.Store
	library *library.Library
	logger  *slog.Logger
}

// NewManager は Manager を返す。store は、このインスタンスにマウントされたプール。
func NewManager(pool *pgxpool.Pool, clk clock.Clock, store *kago.Store, recorder audit.Recorder, logger *slog.Logger) *Manager {
	return &Manager{pool: pool, clock: clk, store: store, library: library.New(clk, recorder, store, logger), logger: logger}
}

// PoolResult は、1 つのプールの確認の結果。
type PoolResult struct {
	PoolID string
	// FreeBytes は、確認を終えたときの空き容量。
	FreeBytes uint64
	// Deleted は、消した録画の数。
	Deleted int
	// Low は、録画を消しても、空き容量が閾値を下回ったままかどうか。
	Low bool
}

// RunOnce は、容量の閾値を設定した、このインスタンスにマウントされたプールを、1 回ずつ確かめる。
func (m *Manager) RunOnce(ctx context.Context) ([]*PoolResult, error) {
	rows, err := sqlc.New(m.pool).ListStoragePools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list storage pools: %w", err)
	}
	mounted := m.store.PoolIDs()
	results := []*PoolResult{}
	errs := []error{}
	for i := range rows {
		row := &rows[i]
		if row.MinFreeBytes <= 0 || !slices.Contains(mounted, row.ID) {
			continue
		}
		result, err := m.check(ctx, row, mounted)
		if err != nil {
			errs = append(errs, fmt.Errorf("pool %s: %w", row.ID, err))

			continue
		}
		results = append(results, result)
	}

	return results, errors.Join(errs...)
}

// check は、プールの空き容量が閾値を下回っていれば、保護されていない古い録画から消し、空き容量の不足を通知する。
func (m *Manager) check(ctx context.Context, row *sqlc.StoragePool, mounted []string) (*PoolResult, error) {
	threshold := uint64(max(row.MinFreeBytes, 0))
	free, err := m.store.PoolFreeSpace(row.ID)
	if err != nil {
		return nil, err
	}
	result := &PoolResult{PoolID: row.ID, FreeBytes: free, Deleted: 0, Low: false}
	if free >= threshold {
		return result, m.report(ctx, row, result)
	}
	candidates, err := sqlc.New(m.pool).ListDeletableRecordings(ctx, sqlc.ListDeletableRecordingsParams{
		PoolID: row.ID, Mounted: mounted, MaxCount: maxDeletionsPerPool,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deletable recordings: %w", err)
	}
	for i := 0; i < len(candidates) && free < threshold; i++ {
		// 録画ファイルが 1 つも見つからない録画は、消しても空きが増えない。プールの実体がマウントされず、
		// 空のディレクトリを測っているときに、録画の記録を消し続けないよう、消さない (#513)。
		found, err := m.hasFiles(ctx, candidates[i].ID)
		if err != nil {
			return nil, err
		}
		if !found {
			m.logger.WarnContext(ctx, "skipped a recording whose files are not found in the storage pool; the pool may not be mounted",
				slog.String("recording_id", candidates[i].ID.String()), slog.String("pool_id", row.ID))

			continue
		}
		deleted, err := m.delete(ctx, candidates[i].ID, row.ID)
		if err != nil {
			return nil, err
		}
		if !deleted {
			continue
		}
		result.Deleted++
		before := free
		if free, err = m.store.PoolFreeSpace(row.ID); err != nil {
			return nil, err
		}
		// 消しても空きが増えなければ (スナップショットや、ファイルを開いたままの視聴者)、録画を次々に消さないよう、この回はやめる (#513)。
		if free <= before {
			m.logger.WarnContext(ctx, "the free space did not grow after deleting a recording; stopped deleting for now",
				slog.String("pool_id", row.ID), slog.Uint64("free_bytes", free))

			break
		}
	}
	result.FreeBytes = free
	result.Low = free < threshold

	return result, m.report(ctx, row, result)
}

// delete は、録画を消す (library.Delete)。候補に選んだ後に保護されたか、録画を始めたか、ほかで消されたなら、消さずに false を返す。
func (m *Manager) delete(ctx context.Context, recordingID uuid.UUID, poolID string) (bool, error) {
	err := m.library.Delete(ctx, m.pool, audit.System(auditRole), recordingID, "storage_low")
	if errors.Is(err, library.ErrProtected) || errors.Is(err, library.ErrRecording) || errors.Is(err, library.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	m.logger.InfoContext(ctx, "deleted a recording to free up the storage pool",
		slog.String("recording_id", recordingID.String()), slog.String("pool_id", poolID))

	return true, nil
}

// report は、録画を消したか、消しても空き容量が閾値を下回ったままになったときに、空き容量の不足を通知する。
// 下回ったままの間は、確かめるたびに通知しないよう、下回り始めたときだけ通知する。
func (m *Manager) report(ctx context.Context, row *sqlc.StoragePool, result *PoolResult) error {
	return pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		newlyLow := false
		if result.Low {
			n, err := q.MarkStoragePoolLow(ctx, sqlc.MarkStoragePoolLowParams{Now: new(m.clock.Now()), ID: row.ID})
			if err != nil {
				return fmt.Errorf("failed to record the low storage: %w", err)
			}
			newlyLow = n > 0
		} else if err := q.ClearStoragePoolLow(ctx, row.ID); err != nil {
			return fmt.Errorf("failed to clear the low storage: %w", err)
		}
		if result.Deleted == 0 && !newlyLow {
			return nil
		}

		return notification.Emit(ctx, tx, m.clock, notification.KindStorageLow, map[string]any{
			"poolId": row.ID, "freeBytes": result.FreeBytes, "minFreeBytes": row.MinFreeBytes,
			"deletedRecordings": result.Deleted, "stillLow": result.Low,
		})
	})
}

// CheckArgs は、空き容量を調べるジョブの引数。
type CheckArgs struct{}

var _ river.JobArgs = new(CheckArgs)

func (CheckArgs) Kind() string { return "storage_capacity_check" }

// CheckWorker は、空き容量を調べるジョブを実行する。
type CheckWorker struct {
	river.WorkerDefaults[CheckArgs]

	manager *Manager
}

var _ river.Worker[CheckArgs] = new(CheckWorker)

// NewCheckWorker は CheckWorker を返す。
func NewCheckWorker(manager *Manager) *CheckWorker {
	return &CheckWorker{WorkerDefaults: river.WorkerDefaults[CheckArgs]{}, manager: manager}
}

func (w *CheckWorker) Work(ctx context.Context, _ *river.Job[CheckArgs]) error {
	results, err := w.manager.RunOnce(ctx)
	deleted := 0
	for _, result := range results {
		deleted += result.Deleted
	}
	if recordErr := river.RecordOutput(ctx, map[string]int{"pools": len(results), "deleted": deleted}); recordErr != nil {
		return errors.Join(err, fmt.Errorf("failed to record the job output: %w", recordErr))
	}

	return err
}

// CheckPeriodicJob は、空き容量を CheckInterval ごとに調べる定期ジョブを返す。
func CheckPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(CheckInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return CheckArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "storage_capacity_check", RunOnStart: true},
	)
}

// hasFiles は、録画の録画ファイル (セグメント) のうち、マウントしたプールに見つかるものがあるかを返す。
func (m *Manager) hasFiles(ctx context.Context, recordingID uuid.UUID) (bool, error) {
	files, err := sqlc.New(m.pool).ListRecordingFiles(ctx, recordingID)
	if err != nil {
		return false, fmt.Errorf("failed to list the recording files: %w", err)
	}
	for i := range files {
		if files[i].Kind != segmentKind {
			continue
		}
		path, err := m.store.Path(files[i].PoolID, files[i].Path)
		if err != nil {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return true, nil
		}
	}

	return false, nil
}
