package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// FileNameTemplateHelp is the resolver for the fileNameTemplateHelp field.
func (r *queryResolver) FileNameTemplateHelp(ctx context.Context) (*model.FileNameTemplateHelp, error) {
	return r.fileNameTemplateHelp(ctx)
}

// PreviewFileName is the resolver for the previewFileName field.
func (r *queryResolver) PreviewFileName(ctx context.Context, template string, program *string) (*model.FileNamePreview, error) {
	return r.previewFileName(ctx, template, program)
}
