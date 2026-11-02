package rule

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/usecase/usersettings"
)

// applyWishlistWatch は、見たいリストの作品の放送を通知するユーザー (#1234) の視聴予約を、見たいリストに合わせて付け外しする。
//
//   - 見たいリストの作品の、これからの本放送のうち、見終えた話数より後の話ごとに最も早い放送に、視聴予約を付ける。
//     視聴ルールと同じく、手で外した番組と、通知を送る時刻を過ぎた番組には付けない。
//   - 見たいリストの通知が付けた視聴予約のうち、もう当たらなくなったもの (作品が見たいリストから外れた、見終えた、通知をやめた) は、
//     放送の前なら消す。手で付けたものと、視聴ルールが付けたものは消さない。
//   - 管理者がアニメの連携を無効にしていれば、通知をやめたものとして扱う。
func (e *Engine) applyWishlistWatch(ctx context.Context, tx pgx.Tx, now time.Time, result *Result) error {
	q := sqlc.New(tx)
	system, err := settings.Load(ctx, tx)
	if err != nil {
		return err
	}
	users, err := q.ListWishlistWatchUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list the users notified of their wishlists: %w", err)
	}
	for _, user := range users {
		serviceIDs, eventIDs := []int64{}, []int64{}
		if user.NotifyWishlist && system.AnimeLinkingEnabled {
			candidates, err := q.ListWishlistWatchCandidates(ctx, sqlc.ListWishlistWatchCandidatesParams{UserID: user.UserID, Now: now})
			if err != nil {
				return fmt.Errorf("failed to list the wishlist broadcasts of user %s: %w", user.UserID, err)
			}
			for _, candidate := range candidates {
				serviceIDs, eventIDs = append(serviceIDs, candidate.ServiceID), append(eventIDs, candidate.EventID)
			}
		}
		deleted, err := q.DeleteStaleWishlistWatchReservations(ctx, sqlc.DeleteStaleWishlistWatchReservationsParams{
			UserID: user.UserID, Now: now, ServiceIds: serviceIDs, EventIds: eventIDs,
		})
		if err != nil {
			return fmt.Errorf("failed to delete the stale wishlist watch reservations of user %s: %w", user.UserID, err)
		}
		var created int64
		if len(serviceIDs) > 0 {
			// 照合に時間がかかる間に、通知のジョブが通知していることがあるので、付ける直前の時刻で確かめる。
			created, err = q.InsertWishlistWatchReservations(ctx, sqlc.InsertWishlistWatchReservationsParams{
				UserID: user.UserID, Now: e.clock.Now(), ServiceIds: serviceIDs, EventIds: eventIDs,
			})
			if err != nil {
				return fmt.Errorf("failed to create the wishlist watch reservations of user %s: %w", user.UserID, err)
			}
		}
		result.WishlistCreated += int(created)
		result.WishlistDeleted += int(deleted)
		if created > 0 || deleted > 0 {
			if err := usersettings.Touch(ctx, tx, e.clock, user.UserID); err != nil {
				return err
			}
		}
	}

	return nil
}
