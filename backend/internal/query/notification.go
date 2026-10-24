package query

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// NotificationListQuery は、通知の一覧の条件。
type NotificationListQuery struct {
	UnreadOnly bool
	// Before は、これより前 (古い) の通知だけを返すためのカーソル。nil なら最も新しい通知から返す。
	Before *domain.NotificationPosition
	// Limit は、返す件数の上限。1 以上にする。
	Limit int32
}

// ListNotifications は、ユーザーの通知を、新しい順に返す。
func (q *Queries) ListNotifications(ctx context.Context, userID uuid.UUID, query *NotificationListQuery) ([]*domain.Notification, error) {
	if query.Limit < 1 {
		return nil, domain.ErrInvalidNotificationLimit
	}
	params := sqlc.ListNotificationsParams{UserID: userID, UnreadOnly: query.UnreadOnly, BeforeAt: nil, BeforeID: nil, MaxCount: query.Limit}
	if query.Before != nil {
		params.BeforeAt, params.BeforeID = &query.Before.CreatedAt, &query.Before.ID
	}
	rows, err := sqlc.New(q.pool).ListNotifications(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list notifications: %w", err)
	}
	notifications := make([]*domain.Notification, 0, len(rows))
	for i := range rows {
		payload := map[string]any{}
		if err := json.Unmarshal(rows[i].NotificationEvent.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to decode the notification payload: %w", err)
		}
		notifications = append(notifications, &domain.Notification{
			ID:         rows[i].Notification.ID,
			Kind:       domain.NotificationKind(rows[i].NotificationEvent.Kind),
			OccurredAt: rows[i].NotificationEvent.OccurredAt,
			Payload:    payload,
			CreatedAt:  rows[i].Notification.CreatedAt,
			ReadAt:     rows[i].Notification.ReadAt,
		})
	}

	return notifications, nil
}

// UnreadNotificationCount は、ユーザーの未読の通知の数を返す。
func (q *Queries) UnreadNotificationCount(ctx context.Context, userID uuid.UUID) (int, error) {
	n, err := sqlc.New(q.pool).CountUnreadNotifications(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to count unread notifications: %w", err)
	}

	return int(n), nil
}

// NotificationPreferences は、ユーザーの通知設定を、すべての種類について返す。設定していない種類は、どの通知先でも受け取る。
func (q *Queries) NotificationPreferences(ctx context.Context, userID uuid.UUID) ([]*domain.NotificationPreference, error) {
	rows, err := sqlc.New(q.pool).ListNotificationPreferences(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification preferences: %w", err)
	}
	saved := make(map[string]sqlc.NotificationPreference, len(rows))
	for _, row := range rows {
		saved[row.Kind] = row
	}
	preferences := make([]*domain.NotificationPreference, 0, len(domain.NotificationKinds()))
	for _, kind := range domain.NotificationKinds() {
		preference := &domain.NotificationPreference{Kind: kind, InApp: true, WebPush: true, RuleReservations: true}
		if row, ok := saved[string(kind)]; ok {
			preference.InApp, preference.WebPush, preference.RuleReservations = row.InApp, row.WebPush, row.RuleReservations
		}
		preferences = append(preferences, preference)
	}

	return preferences, nil
}
