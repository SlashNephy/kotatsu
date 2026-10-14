package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/graph/model"
)

// CleanUpMissingRecordings is the resolver for the cleanUpMissingRecordings field.
func (r *mutationResolver) CleanUpMissingRecordings(ctx context.Context, ids []string) (*model.CleanUpMissingRecordingsPayload, error) {
	return r.cleanUpMissingRecordings(ctx, ids)
}

// MissingRecordings is the resolver for the missingRecordings field.
func (r *queryResolver) MissingRecordings(ctx context.Context) ([]*model.MissingRecording, error) {
	return r.missingRecordings(ctx)
}
