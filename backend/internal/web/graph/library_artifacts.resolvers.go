package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// ClearLibraryArtifacts is the resolver for the clearLibraryArtifacts field.
func (r *mutationResolver) ClearLibraryArtifacts(ctx context.Context) (*model.LibraryArtifacts, error) {
	return r.clearLibraryArtifacts(ctx)
}

// LibraryArtifacts is the resolver for the libraryArtifacts field.
func (r *queryResolver) LibraryArtifacts(ctx context.Context) (*model.LibraryArtifacts, error) {
	return r.libraryArtifacts(ctx)
}
