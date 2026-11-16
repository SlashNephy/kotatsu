package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// SaveLibraryWatchPosition is the resolver for the saveLibraryWatchPosition field.
func (r *mutationResolver) SaveLibraryWatchPosition(ctx context.Context, id string, positionMs int64) (*model.LibraryFile, error) {
	return r.saveLibraryWatchPosition(ctx, id, positionMs)
}

// Libraries is the resolver for the libraries field.
func (r *queryResolver) Libraries(ctx context.Context) ([]*model.Library, error) {
	return r.librariesList(ctx)
}

// LibraryDirectory is the resolver for the libraryDirectory field.
func (r *queryResolver) LibraryDirectory(ctx context.Context, library string, path string) (*model.LibraryDirectory, error) {
	return r.libraryDirectory(ctx, library, path)
}

// LibraryFile is the resolver for the libraryFile field.
func (r *queryResolver) LibraryFile(ctx context.Context, library string, path string) (*model.LibraryFile, error) {
	return r.libraryFile(ctx, library, path)
}
