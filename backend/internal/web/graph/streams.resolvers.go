package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// CreateStreamURL is the resolver for the createStreamURL field.
func (r *mutationResolver) CreateStreamURL(ctx context.Context, target string) (*model.StreamURL, error) {
	return r.createStreamURL(ctx, target)
}

// RevokeStreamURL is the resolver for the revokeStreamUrl field.
func (r *mutationResolver) RevokeStreamURL(ctx context.Context, id string) (bool, error) {
	return r.revokeStreamURL(ctx, id)
}

// Transcoding is the resolver for the transcoding field.
func (r *queryResolver) Transcoding(ctx context.Context) (*model.Transcoding, error) {
	return r.transcoding(ctx)
}

// IssuedStreamUrls is the resolver for the issuedStreamUrls field.
func (r *queryResolver) IssuedStreamUrls(ctx context.Context) ([]*model.IssuedStreamURL, error) {
	return r.issuedStreamURLs(ctx)
}

// LiveStreamRates is the resolver for the liveStreamRates field.
func (r *subscriptionResolver) LiveStreamRates(ctx context.Context, service string) (<-chan *model.LiveStreamRates, error) {
	return r.subscribeLiveStreamRates(ctx, service)
}
