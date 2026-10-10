package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/graph/model"
)

// DataChanged is the resolver for the dataChanged field.
func (r *subscriptionResolver) DataChanged(ctx context.Context) (<-chan *model.DataChange, error) {
	return r.subscribeDataChanges(ctx)
}
