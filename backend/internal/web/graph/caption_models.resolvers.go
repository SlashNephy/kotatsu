package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/usecase/captionmodel"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// DownloadCaptionModel is the resolver for the downloadCaptionModel field.
func (r *mutationResolver) DownloadCaptionModel(ctx context.Context, id string) (*model.CaptionModelSlot, error) {
	return r.downloadCaptionModel(ctx, id)
}

// CancelCaptionModelDownload is the resolver for the cancelCaptionModelDownload field.
func (r *mutationResolver) CancelCaptionModelDownload(ctx context.Context, kind captionmodel.Kind) (*model.CaptionModelSlot, error) {
	return r.cancelCaptionModelDownload(ctx, kind)
}

// DeleteCaptionModel is the resolver for the deleteCaptionModel field.
func (r *mutationResolver) DeleteCaptionModel(ctx context.Context, kind captionmodel.Kind) (*model.CaptionModelSlot, error) {
	return r.deleteCaptionModel(ctx, kind)
}

// CaptionModels is the resolver for the captionModels field.
func (r *queryResolver) CaptionModels(ctx context.Context) ([]*model.CaptionModelSlot, error) {
	return r.captionModelSlots(ctx)
}
