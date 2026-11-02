package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// AnimeLibrary is the resolver for the animeLibrary field.
func (r *queryResolver) AnimeLibrary(ctx context.Context) (*model.AnimeLibrary, error) {
	return r.animeLibrary(ctx)
}
