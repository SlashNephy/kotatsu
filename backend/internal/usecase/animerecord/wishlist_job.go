package animerecord

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
)

const (
	// wishlistCheckInterval は、見たいリストを取り込み直す時期が来たかを確かめる間隔 (付録 A)。
	// 取り込み直すのは、取り込めてから WishlistRefreshInterval が過ぎた連携と、取り込めなかったときは問い合わせ直す日時を過ぎた連携だけである。
	// Retry-After で示された長さに近く問い合わせ直せるよう、問い合わせ直す間隔より短くする。DB を読むだけなので軽い。
	wishlistCheckInterval = 10 * time.Minute
	// wishlistTimeout は、見たいリストを取り込むジョブの時間の上限。連携ごとに、ページの数だけ、間を空けて問い合わせる。
	wishlistTimeout = 15 * time.Minute
)

var _ river.JobArgsWithInsertOpts = new(WishlistArgs)

func (WishlistArgs) Kind() string { return "anime_wishlist_import" }

// InsertOpts は、同じ引数のジョブを、終わるまでは 1 つにする。取り込み直しを何度押しても、問い合わせを重ねない。
// 取り込めなかった連携は、River では送り直さず、問い合わせ直す日時の後の定期ジョブに任せる。
func (WishlistArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable, rivertype.JobStateRunning, rivertype.JobStateScheduled,
	}}}
}

// WishlistWorker は、見たいリストを取り込むジョブを実行する。
type WishlistWorker struct {
	river.WorkerDefaults[WishlistArgs]

	service *Service
	db      Database
}

var _ river.Worker[WishlistArgs] = new(WishlistWorker)

// NewWishlistWorker は WishlistWorker を作る。
func NewWishlistWorker(service *Service, db Database) *WishlistWorker {
	return &WishlistWorker{WorkerDefaults: river.WorkerDefaults[WishlistArgs]{}, service: service, db: db}
}

// Timeout は、見たいリストを取り込むジョブの時間の上限を返す。
func (w *WishlistWorker) Timeout(*river.Job[WishlistArgs]) time.Duration {
	return wishlistTimeout
}

func (w *WishlistWorker) Work(ctx context.Context, job *river.Job[WishlistArgs]) error {
	if job.Args.UserID != nil {
		return w.service.ImportUserWishlists(ctx, w.db, *job.Args.UserID)
	}

	return w.service.ImportDueWishlists(ctx, w.db)
}

// WishlistPeriodicJob は、見たいリストを取り込み直す時期が来たかを、10 分ごとに確かめる定期ジョブを返す。
func WishlistPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(wishlistCheckInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return WishlistArgs{UserID: nil}, nil
		},
		&river.PeriodicJobOpts{ID: "anime_wishlist_import", RunOnStart: true},
	)
}

// EnqueueWishlistImport は、ユーザー userID の見たいリストを、今すぐ取り込み直すジョブを tx で入れる。inserter が nil なら何もしない。
func EnqueueWishlistImport(ctx context.Context, tx pgx.Tx, inserter anime.Inserter, userID uuid.UUID) error {
	if inserter == nil {
		return nil
	}
	if _, err := inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: WishlistArgs{UserID: &userID}, InsertOpts: nil}}); err != nil {
		return fmt.Errorf("failed to enqueue the wishlist import: %w", err)
	}

	return nil
}

// RefreshWishlist は、ユーザー userID の見たいリストを、今すぐ取り込み直すジョブを入れる。
func (s *Service) RefreshWishlist(ctx context.Context, db Database, inserter anime.Inserter, userID uuid.UUID) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		return EnqueueWishlistImport(ctx, tx, inserter, userID)
	})
}
