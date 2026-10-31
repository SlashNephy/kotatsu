package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/generated"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// DeleteRecording is the resolver for the deleteRecording field.
func (r *mutationResolver) DeleteRecording(ctx context.Context, id string) (string, error) {
	return r.deleteRecording(ctx, id)
}

// DeleteRecordings is the resolver for the deleteRecordings field.
func (r *mutationResolver) DeleteRecordings(ctx context.Context, ids []string) (*model.BulkOperationPayload, error) {
	return r.deleteRecordings(ctx, ids)
}

// SetRecordingProtected is the resolver for the setRecordingProtected field.
func (r *mutationResolver) SetRecordingProtected(ctx context.Context, id string, protected bool) (*model.Recording, error) {
	return r.setRecordingProtected(ctx, id, protected)
}

// SetRecordingTags is the resolver for the setRecordingTags field.
func (r *mutationResolver) SetRecordingTags(ctx context.Context, id string, tags []string) (*model.Recording, error) {
	return r.setRecordingTags(ctx, id, tags)
}

// SaveWatchPosition is the resolver for the saveWatchPosition field.
func (r *mutationResolver) SaveWatchPosition(ctx context.Context, id string, positionMs int64) (*model.Recording, error) {
	return r.saveWatchPosition(ctx, id, positionMs)
}

// RegenerateRecordingImages is the resolver for the regenerateRecordingImages field.
func (r *mutationResolver) RegenerateRecordingImages(ctx context.Context, id string) (bool, error) {
	return r.regenerateRecordingImages(ctx, id)
}

// ReanalyzeRecording is the resolver for the reanalyzeRecording field.
func (r *mutationResolver) ReanalyzeRecording(ctx context.Context, id string) (bool, error) {
	return r.reanalyzeRecording(ctx, id)
}

// Recordings is the resolver for the recordings field.
func (r *queryResolver) Recordings(ctx context.Context, first int, after *string, filter *model.RecordingFilter, order model.RecordingOrder) (*model.RecordingConnection, error) {
	return r.listRecordings(ctx, first, after, filter, order)
}

// ContinueWatching is the resolver for the continueWatching field.
func (r *queryResolver) ContinueWatching(ctx context.Context, first int) ([]*model.Recording, error) {
	return r.continueWatching(ctx, first)
}

// Tags is the resolver for the tags field.
func (r *queryResolver) Tags(ctx context.Context) ([]*model.Tag, error) {
	return r.listTags(ctx)
}

// Service is the resolver for the service field.
func (r *recordingResolver) Service(ctx context.Context, obj *model.Recording) (*model.Service, error) {
	row, err := loadersFrom(ctx).Services.Load(ctx, obj.ServiceKey)
	if err != nil {
		return nil, err
	}

	return toService(row), nil
}

// StreamQuality is the resolver for the streamQuality field.
func (r *recordingResolver) StreamQuality(ctx context.Context, obj *model.Recording) ([]*model.StreamQuality, error) {
	return r.streamQuality(ctx, obj)
}

// VideoFormat is the resolver for the videoFormat field.
func (r *recordingResolver) VideoFormat(ctx context.Context, obj *model.Recording) (model.VideoFormat, error) {
	return r.recordingVideoFormat(ctx, obj)
}

// Rule is the resolver for the rule field.
func (r *recordingResolver) Rule(ctx context.Context, obj *model.Recording) (*model.Rule, error) {
	return r.recordingRule(ctx, obj)
}

// LiveStats is the resolver for the liveStats field.
func (r *recordingResolver) LiveStats(ctx context.Context, obj *model.Recording) (*model.RecordingLiveStats, error) {
	return r.recordingLiveStats(ctx, obj)
}

// WatchFrame is the resolver for the watchFrame field.
func (r *recordingResolver) WatchFrame(ctx context.Context, obj *model.Recording) (*model.PreviewFrame, error) {
	return r.watchFrame(ctx, obj), nil
}

// Recording returns generated.RecordingResolver implementation.
func (r *Resolver) Recording() generated.RecordingResolver { return &recordingResolver{r} }

type recordingResolver struct{ *Resolver }
