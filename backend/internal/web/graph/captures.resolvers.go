package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// DeleteCapture is the resolver for the deleteCapture field.
func (r *mutationResolver) DeleteCapture(ctx context.Context, id string) (string, error) {
	return r.deleteCapture(ctx, id)
}

// Captures is the resolver for the captures field.
func (r *queryResolver) Captures(ctx context.Context, first int, after *string, keyword *string) (*model.CaptureConnection, error) {
	return r.listCaptures(ctx, first, after, keyword)
}

// CapturesAvailable is the resolver for the capturesAvailable field.
func (r *queryResolver) CapturesAvailable(ctx context.Context) (bool, error) {
	return r.capturesAvailable(ctx)
}
