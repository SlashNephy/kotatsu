package oauth

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

const (
	// pruneInterval は、期限を過ぎた認可コードとトークンを消す間隔。
	pruneInterval = 24 * time.Hour
	// pruneBatchSize は、1 回の DELETE で消す行の数の上限。
	pruneBatchSize = 1000
)

// Prune は、期限を過ぎた認可コードとトークンと、トークンがなくなった連携を、pruneBatchSize 件ずつ区切って消し、消した数を返す。
// 使い済みのリフレッシュトークンも、使い回しを見つけるため、期限までは残しておく。
func Prune(ctx context.Context, db sqlc.DBTX, clk clock.Clock) (int64, error) {
	now := clk.Now()
	q := sqlc.New(db)
	steps := []func() (int64, error){
		func() (int64, error) {
			return q.DeleteExpiredOAuthAuthorizationCodes(ctx, sqlc.DeleteExpiredOAuthAuthorizationCodesParams{Now: now, MaxCount: pruneBatchSize})
		},
		func() (int64, error) {
			return q.DeleteExpiredOAuthTokens(ctx, sqlc.DeleteExpiredOAuthTokensParams{Now: now, MaxCount: pruneBatchSize})
		},
		// 引き換えた直後の連携は、トークンを作るのと同じトランザクションで作るので、トークンがないことはない。念のため、作ってから 1 日は残す。
		func() (int64, error) {
			return q.DeleteOAuthGrantsWithoutTokens(ctx, sqlc.DeleteOAuthGrantsWithoutTokensParams{
				CreatedBefore: now.Add(-pruneInterval), MaxCount: pruneBatchSize,
			})
		},
	}
	var total int64
	for _, step := range steps {
		for {
			deleted, err := step()
			if err != nil {
				return total, fmt.Errorf("failed to prune OAuth records: %w", err)
			}
			total += deleted
			if deleted < pruneBatchSize {
				break
			}
		}
	}

	return total, nil
}

// PruneArgs は、期限を過ぎた認可コードとトークンを消すジョブの引数。
type PruneArgs struct{}

var _ river.JobArgs = new(PruneArgs)

func (PruneArgs) Kind() string { return "oauth_prune" }

// PruneWorker は、期限を過ぎた認可コードとトークンを消すジョブを実行する。
type PruneWorker struct {
	river.WorkerDefaults[PruneArgs]

	db    sqlc.DBTX
	clock clock.Clock
}

var _ river.Worker[PruneArgs] = new(PruneWorker)

// NewPruneWorker は PruneWorker を作る。
func NewPruneWorker(db sqlc.DBTX, clk clock.Clock) *PruneWorker {
	return &PruneWorker{WorkerDefaults: river.WorkerDefaults[PruneArgs]{}, db: db, clock: clk}
}

func (w *PruneWorker) Work(ctx context.Context, _ *river.Job[PruneArgs]) error {
	deleted, err := Prune(ctx, w.db, w.clock)
	if err != nil {
		return err
	}
	if err := river.RecordOutput(ctx, map[string]int64{"deleted": deleted}); err != nil {
		return fmt.Errorf("failed to record the job output: %w", err)
	}

	return nil
}

// PrunePeriodicJob は、期限を過ぎた認可コードとトークンを 1 日ごとに消す定期ジョブを返す。nekonote が起動したときにも 1 回消す。
func PrunePeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(pruneInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return PruneArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "oauth_prune", RunOnStart: true},
	)
}
