package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/graph/model"
)

// RenameRecordingFiles is the resolver for the renameRecordingFiles field.
func (r *mutationResolver) RenameRecordingFiles(ctx context.Context, ids []string) (*model.RenameRecordingFilesPayload, error) {
	return r.renameRecordingFiles(ctx, ids)
}

// RecordingRenames is the resolver for the recordingRenames field.
func (r *queryResolver) RecordingRenames(ctx context.Context) ([]*model.RecordingRename, error) {
	return r.recordingRenames(ctx)
}
