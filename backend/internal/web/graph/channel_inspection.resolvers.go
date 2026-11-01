package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// InspectChannel is the resolver for the inspectChannel field.
func (r *mutationResolver) InspectChannel(ctx context.Context, channelType string, channel string) (*model.ChannelInspection, error) {
	return r.inspectChannel(ctx, channelType, channel)
}
