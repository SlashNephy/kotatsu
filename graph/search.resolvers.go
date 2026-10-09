package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/graph/model"
)

// SearchPrograms is the resolver for the searchPrograms field.
func (r *queryResolver) SearchPrograms(ctx context.Context, conditions model.ProgramConditionsInput, archive *model.ArchiveRangeInput, first int, after *string) (*model.ProgramConnection, error) {
	return r.searchPrograms(ctx, &conditions, archive, first, after)
}
