package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// ViewerEvents is the resolver for the viewerEvents field.
func (r *subscriptionResolver) ViewerEvents(ctx context.Context) (<-chan *model.ViewerEvent, error) {
	return r.subscribeViewerEvents(ctx)
}

// DataChanged is the resolver for the dataChanged field.
func (r *subscriptionResolver) DataChanged(ctx context.Context) (<-chan *model.DataChange, error) {
	return r.subscribeDataChanges(ctx)
}

// UnreadNotificationCount is the resolver for the unreadNotificationCount field.
func (r *subscriptionResolver) UnreadNotificationCount(ctx context.Context) (<-chan int, error) {
	return r.subscribeUnreadNotificationCount(ctx)
}

// ViewerSettingsChanged is the resolver for the viewerSettingsChanged field.
func (r *subscriptionResolver) ViewerSettingsChanged(ctx context.Context) (<-chan int, error) {
	return r.subscribeViewerSettings(ctx)
}
