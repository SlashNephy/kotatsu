package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/graph/model"
)

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
