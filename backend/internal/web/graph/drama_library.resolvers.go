package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// Watched is the resolver for the watched field.
func (r *dramaEpisodeResolver) Watched(ctx context.Context, obj *model.DramaEpisode) (bool, error) {
	return r.dramaEpisodeWatched(ctx, obj)
}

// WatchStatus is the resolver for the watchStatus field.
func (r *dramaSeriesResolver) WatchStatus(ctx context.Context, obj *model.DramaSeries) (*model.DramaWatchStatus, error) {
	return r.dramaSeriesWatchStatus(ctx, obj)
}

// SetDramaWatchStatus is the resolver for the setDramaWatchStatus field.
func (r *mutationResolver) SetDramaWatchStatus(ctx context.Context, seriesID int, status *model.DramaWatchStatus) (*model.DramaSeries, error) {
	return r.setDramaWatchStatus(ctx, seriesID, status)
}

// SetDramaEpisodeWatched is the resolver for the setDramaEpisodeWatched field.
func (r *mutationResolver) SetDramaEpisodeWatched(ctx context.Context, seriesID int, seasonNumber int, episodeNumber int, watched bool) (bool, error) {
	return r.setDramaEpisodeWatched(ctx, seriesID, seasonNumber, episodeNumber, watched)
}

// DramaLibrary is the resolver for the dramaLibrary field.
func (r *queryResolver) DramaLibrary(ctx context.Context) (*model.DramaLibrary, error) {
	return r.dramaLibrary(ctx)
}
