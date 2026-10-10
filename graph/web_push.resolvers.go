package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/graph/model"
)

// SubscribeWebPush is the resolver for the subscribeWebPush field.
func (r *mutationResolver) SubscribeWebPush(ctx context.Context, input model.WebPushSubscriptionInput) (*model.WebPushSubscription, error) {
	return r.subscribeWebPush(ctx, &input)
}

// UnsubscribeWebPush is the resolver for the unsubscribeWebPush field.
func (r *mutationResolver) UnsubscribeWebPush(ctx context.Context, endpoint string) (bool, error) {
	return r.unsubscribeWebPush(ctx, endpoint)
}

// WebPushPublicKey is the resolver for the webPushPublicKey field.
func (r *queryResolver) WebPushPublicKey(ctx context.Context) (string, error) {
	return r.webPushPublicKey(ctx)
}

// WebPushSubscriptions is the resolver for the webPushSubscriptions field.
func (r *queryResolver) WebPushSubscriptions(ctx context.Context) ([]*model.WebPushSubscription, error) {
	return r.webPushSubscriptions(ctx)
}
