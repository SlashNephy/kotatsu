// Package networkactivity は、外部との通信の記録 (#1369、設計書 11.8) を DB に書き、古いものを消す。
package networkactivity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
)

const pruneInterval = 24 * time.Hour

// Store は、netactivity の集計を DB に書く。
type Store struct {
	db sqlc.DBTX
}

var _ netactivity.Store = new(Store)

// NewStore は Store を返す。
func NewStore(db sqlc.DBTX) *Store {
	return &Store{db: db}
}

// Enabled は、記録が有効かを返す。設定の行がなければ、既定どおり無効。
func (s *Store) Enabled(ctx context.Context) (bool, error) {
	row, err := sqlc.New(s.db).GetSystemSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read the system settings: %w", err)
	}

	return row.NetworkActivityEnabled, nil
}

// Save は、集計を足し込む。
func (s *Store) Save(ctx context.Context, buckets []netactivity.Bucket) error {
	params := sqlc.AddNetworkActivityParams{
		Hours: make([]time.Time, 0, len(buckets)), Purposes: make([]string, 0, len(buckets)), Hosts: make([]string, 0, len(buckets)),
		Initiators: make([]string, 0, len(buckets)), Successes: make([]bool, 0, len(buckets)), Counts: make([]int64, 0, len(buckets)),
		SentBytesList: make([]int64, 0, len(buckets)), ReceivedBytesList: make([]int64, 0, len(buckets)),
	}
	for _, bucket := range buckets {
		params.Hours = append(params.Hours, bucket.Hour)
		params.Purposes = append(params.Purposes, string(bucket.Purpose))
		params.Hosts = append(params.Hosts, bucket.Host)
		params.Initiators = append(params.Initiators, string(bucket.Initiator))
		params.Successes = append(params.Successes, bucket.Success)
		params.Counts = append(params.Counts, bucket.Count)
		params.SentBytesList = append(params.SentBytesList, bucket.SentBytes)
		params.ReceivedBytesList = append(params.ReceivedBytesList, bucket.ReceivedBytes)
	}
	if err := sqlc.New(s.db).AddNetworkActivity(ctx, params); err != nil {
		return fmt.Errorf("failed to save the network activity: %w", err)
	}

	return nil
}

// Prune は、保持期間 retention を過ぎた記録を消し、消した数を返す。
func Prune(ctx context.Context, db sqlc.DBTX, clk clock.Clock, retention time.Duration) (int64, error) {
	deleted, err := sqlc.New(db).DeleteNetworkActivityBefore(ctx, clk.Now().Add(-retention))
	if err != nil {
		return 0, fmt.Errorf("failed to prune the network activity: %w", err)
	}

	return deleted, nil
}

// PruneArgs は、保持期間を過ぎた記録を消すジョブの引数。
type PruneArgs struct{}

var _ river.JobArgs = new(PruneArgs)

func (PruneArgs) Kind() string { return "network_activity_prune" }

// PruneWorker は、保持期間を過ぎた記録を消すジョブを実行する。
type PruneWorker struct {
	river.WorkerDefaults[PruneArgs]

	db    sqlc.DBTX
	clock clock.Clock
	// retention は、記録の保持期間を返す。画面から変えられるので、ジョブのたびに読む。
	retention func(ctx context.Context) (time.Duration, error)
}

var _ river.Worker[PruneArgs] = new(PruneWorker)

// NewPruneWorker は PruneWorker を作る。
func NewPruneWorker(db sqlc.DBTX, clk clock.Clock, retention func(ctx context.Context) (time.Duration, error)) *PruneWorker {
	return &PruneWorker{WorkerDefaults: river.WorkerDefaults[PruneArgs]{}, db: db, clock: clk, retention: retention}
}

// Work は、保持期間を過ぎた記録を消す。
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

// PrunePeriodicJob は、保持期間を過ぎた記録を 1 日ごとに消す定期ジョブを返す。nekonote が起動したときにも 1 回消す。
func PrunePeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(pruneInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return PruneArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "network_activity_prune", RunOnStart: true},
	)
}
