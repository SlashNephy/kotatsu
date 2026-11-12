package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// LogReport is the resolver for the logReport field.
func (r *queryResolver) LogReport(ctx context.Context) (*model.LogReport, error) {
	return r.logReport(ctx)
}
