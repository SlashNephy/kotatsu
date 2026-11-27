package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// ExtractARIBCaptions is the resolver for the extractARIBCaptions field.
func (r *mutationResolver) ExtractARIBCaptions(ctx context.Context) (int, error) {
	return r.extractARIBCaptions(ctx)
}

// AribCaptionBackfill is the resolver for the aribCaptionBackfill field.
func (r *queryResolver) AribCaptionBackfill(ctx context.Context) (*model.ARIBCaptionBackfill, error) {
	return r.aribCaptionBackfill(ctx)
}
