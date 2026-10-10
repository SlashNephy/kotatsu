package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// Notification は、ユーザーに届いた画面内の通知。
type Notification struct {
	ID         uuid.UUID
	Kind       Kind
	OccurredAt time.Time
	Payload    map[string]any
	CreatedAt  time.Time
	// ReadAt は、既読にした日時。未読なら nil。
	ReadAt *time.Time
}

// Position は、通知の一覧での位置。通知を作った日時と ID で並べる。
type Position struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// ListQuery は、通知の一覧の条件。
type ListQuery struct {
	UnreadOnly bool
	// Before は、これより前 (古い) の通知だけを返すためのカーソル。nil なら最も新しい通知から返す。
	Before *Position
	// Limit は、返す件数の上限。1 以上にする。
	Limit int32
}

// ErrInvalidLimit は、一覧の件数の上限が 1 未満であることを表す。
var ErrInvalidLimit = errors.New("the limit must be positive")

// Inbox は、ユーザーの画面内の通知と、通知設定を扱う。
type Inbox struct {
	clock clock.Clock
}

// NewInbox は Inbox を返す。
func NewInbox(clk clock.Clock) *Inbox {
	return &Inbox{clock: clk}
}

// List は、ユーザーの通知を、新しい順に返す。
func (b *Inbox) List(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, query *ListQuery) ([]*Notification, error) {
	if query.Limit < 1 {
		return nil, ErrInvalidLimit
	}
	params := sqlc.ListNotificationsParams{UserID: userID, UnreadOnly: query.UnreadOnly, BeforeAt: nil, BeforeID: nil, MaxCount: query.Limit}
	if query.Before != nil {
		params.BeforeAt, params.BeforeID = &query.Before.CreatedAt, &query.Before.ID
	}
	rows, err := sqlc.New(db).ListNotifications(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list notifications: %w", err)
	}
	notifications := make([]*Notification, 0, len(rows))
	for i := range rows {
		payload := map[string]any{}
		if err := json.Unmarshal(rows[i].NotificationEvent.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to decode the notification payload: %w", err)
		}
		notifications = append(notifications, &Notification{
			ID:         rows[i].Notification.ID,
			Kind:       Kind(rows[i].NotificationEvent.Kind),
			OccurredAt: rows[i].NotificationEvent.OccurredAt,
			Payload:    payload,
			CreatedAt:  rows[i].Notification.CreatedAt,
			ReadAt:     rows[i].Notification.ReadAt,
		})
	}

	return notifications, nil
}

// UnreadCount は、ユーザーの未読の通知の数を返す。
func (b *Inbox) UnreadCount(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) (int, error) {
	n, err := sqlc.New(db).CountUnreadNotifications(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to count unread notifications: %w", err)
	}

	return int(n), nil
}

// MarkRead は、ユーザーの通知 ids を既読にし、既読にした数を返す。ほかのユーザーの通知や、既読の通知は飛ばす。
func (b *Inbox) MarkRead(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, ids []uuid.UUID) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	n, err := sqlc.New(db).MarkNotificationsRead(ctx, sqlc.MarkNotificationsReadParams{UserID: userID, Ids: ids, Now: new(b.clock.Now())})
	if err != nil {
		return 0, fmt.Errorf("failed to mark notifications read: %w", err)
	}

	return int(n), nil
}

// MarkAllRead は、ユーザーの未読の通知をすべて既読にし、既読にした数を返す。
func (b *Inbox) MarkAllRead(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) (int, error) {
	n, err := sqlc.New(db).MarkAllNotificationsRead(ctx, sqlc.MarkAllNotificationsReadParams{UserID: userID, Now: new(b.clock.Now())})
	if err != nil {
		return 0, fmt.Errorf("failed to mark notifications read: %w", err)
	}

	return int(n), nil
}

// Preference は、イベントの種類ごとの通知設定。
type Preference struct {
	Kind    Kind
	InApp   bool
	WebPush bool
}

// Preferences は、ユーザーの通知設定を、すべての種類について返す。設定していない種類は、どの通知先でも受け取る。
func (b *Inbox) Preferences(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) ([]*Preference, error) {
	rows, err := sqlc.New(db).ListNotificationPreferences(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification preferences: %w", err)
	}
	saved := make(map[string]sqlc.NotificationPreference, len(rows))
	for _, row := range rows {
		saved[row.Kind] = row
	}
	preferences := make([]*Preference, 0, len(Kinds()))
	for _, kind := range Kinds() {
		preference := &Preference{Kind: kind, InApp: true, WebPush: true}
		if row, ok := saved[string(kind)]; ok {
			preference.InApp, preference.WebPush = row.InApp, row.WebPush
		}
		preferences = append(preferences, preference)
	}

	return preferences, nil
}

// SetPreference は、ユーザーのイベントの種類 kind の通知設定を変える。
func (b *Inbox) SetPreference(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, preference *Preference) error {
	if _, err := ParseKind(string(preference.Kind)); err != nil {
		return err
	}
	err := sqlc.New(db).UpsertNotificationPreference(ctx, sqlc.UpsertNotificationPreferenceParams{
		UserID: userID, Kind: string(preference.Kind), InApp: preference.InApp, WebPush: preference.WebPush, Now: b.clock.Now(),
	})
	if err != nil {
		return fmt.Errorf("failed to save the notification preference: %w", err)
	}

	return nil
}
