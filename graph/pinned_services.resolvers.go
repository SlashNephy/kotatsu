package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/graph/model"
)

// SetServicePinned is the resolver for the setServicePinned field.
func (r *mutationResolver) SetServicePinned(ctx context.Context, id string, pinned bool) ([]*model.Service, error) {
	return r.setServicePinned(ctx, id, pinned)
}

// PinnedServices is the resolver for the pinnedServices field.
func (r *queryResolver) PinnedServices(ctx context.Context) ([]*model.Service, error) {
	return r.pinnedServices(ctx)
}
