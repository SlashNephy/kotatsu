package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// RefreshAnimeFollowingViewings is the resolver for the refreshAnimeFollowingViewings field.
func (r *mutationResolver) RefreshAnimeFollowingViewings(ctx context.Context, tid int) (bool, error) {
	return r.refreshAnimeFollowingViewings(ctx, tid)
}

// AnimeFollowingViewings is the resolver for the animeFollowingViewings field.
func (r *queryResolver) AnimeFollowingViewings(ctx context.Context, tid int) (*model.AnimeFollowingViewings, error) {
	return r.animeFollowingViewings(ctx, tid)
}
