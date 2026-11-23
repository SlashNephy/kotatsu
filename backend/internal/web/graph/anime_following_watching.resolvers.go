package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// CreateAnimeFollowingWatchingRule is the resolver for the createAnimeFollowingWatchingRule field.
func (r *mutationResolver) CreateAnimeFollowingWatchingRule(ctx context.Context, tid int) (*model.Rule, error) {
	return r.createAnimeFollowingWatchingRule(ctx, tid)
}

// AnimeFollowingWatching is the resolver for the animeFollowingWatching field.
func (r *queryResolver) AnimeFollowingWatching(ctx context.Context) (*model.AnimeFollowingWatching, error) {
	return r.animeFollowingWatching(ctx)
}
