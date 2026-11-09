package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// NetworkActivity is the resolver for the networkActivity field.
func (r *queryResolver) NetworkActivity(ctx context.Context) (*model.NetworkActivity, error) {
	return r.networkActivity(ctx)
}
