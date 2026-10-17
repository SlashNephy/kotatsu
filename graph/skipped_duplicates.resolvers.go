package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/graph/model"
)

// SkippedDuplicates is the resolver for the skippedDuplicates field.
func (r *queryResolver) SkippedDuplicates(ctx context.Context) ([]*model.SkippedDuplicate, error) {
	return r.skippedDuplicates(ctx)
}
