package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// CreateStreamURL is the resolver for the createStreamURL field.
func (r *mutationResolver) CreateStreamURL(ctx context.Context, target string) (*model.StreamURL, error) {
	return r.createStreamURL(ctx, target)
}

// Transcoding is the resolver for the transcoding field.
func (r *queryResolver) Transcoding(ctx context.Context) (*model.Transcoding, error) {
	return r.transcoding(ctx)
}

// LiveStreamRates is the resolver for the liveStreamRates field.
func (r *subscriptionResolver) LiveStreamRates(ctx context.Context, service string) (<-chan *model.LiveStreamRates, error) {
	return r.subscribeLiveStreamRates(ctx, service)
}
