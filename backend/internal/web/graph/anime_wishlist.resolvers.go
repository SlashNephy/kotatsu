package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// RefreshAnimeWishlist is the resolver for the refreshAnimeWishlist field.
func (r *mutationResolver) RefreshAnimeWishlist(ctx context.Context) (bool, error) {
	return r.refreshAnimeWishlist(ctx)
}

// CreateAnimeWishlistRule is the resolver for the createAnimeWishlistRule field.
func (r *mutationResolver) CreateAnimeWishlistRule(ctx context.Context, tid int) (*model.Rule, error) {
	return r.createAnimeWishlistRule(ctx, tid)
}

// AnimeWishlist is the resolver for the animeWishlist field.
func (r *queryResolver) AnimeWishlist(ctx context.Context) (*model.AnimeWishlist, error) {
	wishlist, _, err := r.animeWishlist(ctx)

	return wishlist, err
}
