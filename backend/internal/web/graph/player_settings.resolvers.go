package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// UpdatePlayerSettings is the resolver for the updatePlayerSettings field.
func (r *mutationResolver) UpdatePlayerSettings(ctx context.Context, input model.PlayerSettingsInput) (*model.PlayerSettings, error) {
	return r.updatePlayerSettings(ctx, &input)
}

// PlayerSettings is the resolver for the playerSettings field.
func (r *queryResolver) PlayerSettings(ctx context.Context) (*model.PlayerSettings, error) {
	return r.playerSettings(ctx)
}
