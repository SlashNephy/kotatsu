package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/graph/model"
)

// DeleteOrphanFiles is the resolver for the deleteOrphanFiles field.
func (r *mutationResolver) DeleteOrphanFiles(ctx context.Context, files []*model.OrphanFileInput) (*model.DeleteOrphanFilesPayload, error) {
	return r.deleteOrphanFiles(ctx, files)
}

// OrphanFiles is the resolver for the orphanFiles field.
func (r *queryResolver) OrphanFiles(ctx context.Context) ([]*model.OrphanFile, error) {
	return r.orphanFiles(ctx)
}
