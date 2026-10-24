package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/usersettings"
)

// Inbox は、ユーザーの画面内の通知と、通知設定を扱う。
type Inbox struct {
	clock clock.Clock
}

// NewInbox は Inbox を返す。
func NewInbox(clk clock.Clock) *Inbox {
	return &Inbox{clock: clk}
}

// Database は、既読にするのに使う DB。既読にしたことを通知と一緒に書くため、トランザクションを始められる。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// MarkRead は、ユーザーの通知 ids を既読にし、既読にした数を返す。ほかのユーザーの通知や、既読の通知は飛ばす。
func (b *Inbox) MarkRead(ctx context.Context, db Database, userID uuid.UUID, ids []uuid.UUID) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	return b.markRead(ctx, db, func(q *sqlc.Queries, now *time.Time) (int64, error) {
		return q.MarkNotificationsRead(ctx, sqlc.MarkNotificationsReadParams{UserID: userID, Ids: ids, Now: now})
	})
}

// MarkAllRead は、ユーザーの未読の通知をすべて既読にし、既読にした数を返す。
func (b *Inbox) MarkAllRead(ctx context.Context, db Database, userID uuid.UUID) (int, error) {
	return b.markRead(ctx, db, func(q *sqlc.Queries, now *time.Time) (int64, error) {
		return q.MarkAllNotificationsRead(ctx, sqlc.MarkAllNotificationsReadParams{UserID: userID, Now: now})
	})
}

// SetPreference は、ユーザーのイベントの種類 kind の通知設定を変え、同じユーザーのほかの端末に、設定が変わったことを知らせる。
func (b *Inbox) SetPreference(ctx context.Context, db Database, userID uuid.UUID, preference *domain.NotificationPreference) error {
	if _, err := domain.ParseNotificationKind(string(preference.Kind)); err != nil {
		return err
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		err := sqlc.New(tx).UpsertNotificationPreference(ctx, sqlc.UpsertNotificationPreferenceParams{
			UserID: userID, Kind: string(preference.Kind), InApp: preference.InApp, WebPush: preference.WebPush,
			RuleReservations: preference.RuleReservations, Now: b.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to save the notification preference: %w", err)
		}

		return usersettings.Touch(ctx, tx, b.clock, userID)
	})
}

// markRead は、mark で既読にし、既読にした通知があれば、同じユーザーのほかの端末の未読の数が変わるよう、変更を通知する。
func (b *Inbox) markRead(ctx context.Context, db Database, mark func(q *sqlc.Queries, now *time.Time) (int64, error)) (int, error) {
	var marked int64
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		n, err := mark(sqlc.New(tx), new(b.clock.Now()))
		if err != nil {
			return fmt.Errorf("failed to mark notifications read: %w", err)
		}
		marked = n
		if n == 0 {
			return nil
		}
		if _, err := notify.Publish(ctx, tx, b.clock, NotifyChannel); err != nil {
			return fmt.Errorf("failed to publish the notification change: %w", err)
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	return int(marked), nil
}
