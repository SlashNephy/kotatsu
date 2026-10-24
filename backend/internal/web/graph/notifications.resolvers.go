package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// MarkNotificationsRead is the resolver for the markNotificationsRead field.
func (r *mutationResolver) MarkNotificationsRead(ctx context.Context, ids []string) (int, error) {
	return r.markNotificationsRead(ctx, ids)
}

// MarkAllNotificationsRead is the resolver for the markAllNotificationsRead field.
func (r *mutationResolver) MarkAllNotificationsRead(ctx context.Context) (int, error) {
	return r.markAllNotificationsRead(ctx)
}

// UpdateNotificationPreference is the resolver for the updateNotificationPreference field.
func (r *mutationResolver) UpdateNotificationPreference(ctx context.Context, input model.NotificationPreferenceInput) (*model.NotificationPreference, error) {
	return r.updateNotificationPreference(ctx, &input)
}

// Notifications is the resolver for the notifications field.
func (r *queryResolver) Notifications(ctx context.Context, first int, after *string, unreadOnly bool) (*model.NotificationConnection, error) {
	return r.listNotifications(ctx, first, after, unreadOnly)
}

// UnreadNotificationCount is the resolver for the unreadNotificationCount field.
func (r *queryResolver) UnreadNotificationCount(ctx context.Context) (int, error) {
	return r.unreadNotificationCount(ctx)
}

// NotificationPreferences is the resolver for the notificationPreferences field.
func (r *queryResolver) NotificationPreferences(ctx context.Context) ([]*model.NotificationPreference, error) {
	return r.notificationPreferences(ctx)
}
