package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/notification"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	// notificationType は、Notification の ID に付ける型の名前。
	notificationType = "Notification"
	// maxNotifications は、通知を 1 回に返す件数と、1 回に既読にする件数の上限。
	maxNotifications = 200
)

// notificationCursor は、通知のページを送るカーソル。最後に返した通知を作った日時と ID。
type notificationCursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        uuid.UUID `json:"id"`
}

// listNotifications は、ログイン中のユーザーの通知を、新しい順に first 件まで返す。
func (r *Resolver) listNotifications(ctx context.Context, first int, after *string, unreadOnly bool) (*model.NotificationConnection, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if first < 1 || first > maxNotifications {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("first must be between 1 and %d", maxNotifications))
	}
	// 次のページがあるかを知るため、1 件多く読む。
	query := &notification.ListQuery{UnreadOnly: unreadOnly, Before: nil, Limit: int32(first + 1)}
	if after != nil {
		var cursor notificationCursor
		if err := DecodeCursor(*after, &cursor); err != nil {
			return nil, NewError(CodeBadUserInput, "invalid cursor")
		}
		query.Before = &notification.Position{CreatedAt: cursor.CreatedAt, ID: cursor.ID}
	}

	notifications, err := r.inbox.List(ctx, r.db, principal.UserID, query)
	if err != nil {
		return nil, err
	}
	hasNext := len(notifications) > first
	notifications = notifications[:min(len(notifications), first)]
	edges := make([]*model.NotificationEdge, 0, len(notifications))
	for _, n := range notifications {
		cursor, err := EncodeCursor(notificationCursor{CreatedAt: n.CreatedAt, ID: n.ID})
		if err != nil {
			return nil, err
		}
		node, err := notificationOf(n, r.location)
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.NotificationEdge{Cursor: cursor, Node: node})
	}
	info := &model.PageInfo{HasNextPage: hasNext, HasPreviousPage: after != nil, StartCursor: nil, EndCursor: nil}
	if len(edges) > 0 {
		info.StartCursor, info.EndCursor = &edges[0].Cursor, &edges[len(edges)-1].Cursor
	}

	return &model.NotificationConnection{Edges: edges, PageInfo: info}, nil
}

// unreadNotificationCount は、ログイン中のユーザーの未読の通知の数を返す。
func (r *Resolver) unreadNotificationCount(ctx context.Context) (int, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return 0, err
	}

	return r.inbox.UnreadCount(ctx, r.db, principal.UserID)
}

// markNotificationsRead は、ログイン中のユーザーの通知 ids を既読にする。
func (r *Resolver) markNotificationsRead(ctx context.Context, ids []string) (int, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return 0, err
	}
	if len(ids) > maxNotifications {
		return 0, NewError(CodeBadUserInput, fmt.Sprintf("at most %d notifications can be marked read at once", maxNotifications))
	}
	parsed := make([]uuid.UUID, 0, len(ids))
	for _, globalID := range ids {
		notificationID, err := parseTypedID(globalID, notificationType, "invalid notification ID")
		if err != nil {
			return 0, err
		}
		parsed = append(parsed, notificationID)
	}

	return r.inbox.MarkRead(ctx, r.db, principal.UserID, parsed)
}

// markAllNotificationsRead は、ログイン中のユーザーの未読の通知をすべて既読にする。
func (r *Resolver) markAllNotificationsRead(ctx context.Context) (int, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return 0, err
	}

	return r.inbox.MarkAllRead(ctx, r.db, principal.UserID)
}

// notificationPreferences は、ログイン中のユーザーの通知設定を返す。
func (r *Resolver) notificationPreferences(ctx context.Context) ([]*model.NotificationPreference, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	preferences, err := r.inbox.Preferences(ctx, r.db, principal.UserID)
	if err != nil {
		return nil, err
	}
	result := make([]*model.NotificationPreference, 0, len(preferences))
	for _, preference := range preferences {
		result = append(result, preferenceOf(preference))
	}

	return result, nil
}

// updateNotificationPreference は、ログイン中のユーザーの、イベントの種類ごとの通知設定を変える。
func (r *Resolver) updateNotificationPreference(ctx context.Context, input *model.NotificationPreferenceInput) (*model.NotificationPreference, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	kind, err := notification.ParseKind(strings.ToLower(string(input.Kind)))
	if err != nil {
		return nil, NewError(CodeBadUserInput, "unknown notification kind")
	}
	preference := &notification.Preference{Kind: kind, InApp: input.InApp, WebPush: input.WebPush, RuleReservations: true}
	if input.RuleReservations != nil {
		preference.RuleReservations = *input.RuleReservations
	} else {
		current, err := r.inbox.Preferences(ctx, r.db, principal.UserID)
		if err != nil {
			return nil, err
		}
		for _, p := range current {
			if p.Kind == kind {
				preference.RuleReservations = p.RuleReservations
			}
		}
	}
	if err := r.inbox.SetPreference(ctx, r.db, principal.UserID, preference); err != nil {
		return nil, err
	}

	return preferenceOf(preference), nil
}

// notificationOf は、通知を GraphQL の型にする。文は、Web Push と同じく、表示のタイムゾーン location で書く。
func notificationOf(n *notification.Notification, location *time.Location) (*model.Notification, error) {
	payload, err := json.Marshal(n.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the notification payload: %w", err)
	}

	return &model.Notification{
		ID:         GlobalID(notificationType, n.ID.String()),
		Kind:       kindOf(n.Kind),
		OccurredAt: n.OccurredAt,
		Payload:    string(payload),
		Message:    notification.Message(n.Kind, n.Payload, location),
		URL:        nonEmpty(notification.URL(n.Payload)),
		ReadAt:     n.ReadAt,
	}, nil
}

func preferenceOf(preference *notification.Preference) *model.NotificationPreference {
	return &model.NotificationPreference{
		Kind: kindOf(preference.Kind), InApp: preference.InApp, WebPush: preference.WebPush, RuleReservations: preference.RuleReservations,
	}
}

func kindOf(kind notification.Kind) model.NotificationKind {
	return model.NotificationKind(strings.ToUpper(string(kind)))
}

// nonEmpty は、空文字列を nil にする。
func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
