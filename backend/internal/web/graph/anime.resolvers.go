package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// UpdateAnimeChannel is the resolver for the updateAnimeChannel field.
func (r *mutationResolver) UpdateAnimeChannel(ctx context.Context, serviceID string, input model.AnimeChannelInput) (*model.AnimeChannel, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can change anime channels")
	if err != nil {
		return nil, err
	}
	id, err := parseServiceGlobalID(serviceID)
	if err != nil {
		return nil, err
	}
	changes, err := toAnimeChannelChanges(&input)
	if err != nil {
		return nil, err
	}
	if err := r.animeChannels.Update(ctx, r.db, actor, id, changes); err != nil {
		return nil, animeError(err)
	}

	return r.animeChannel(ctx, id)
}

// AnimeChannels is the resolver for the animeChannels field.
func (r *queryResolver) AnimeChannels(ctx context.Context) ([]*model.AnimeChannel, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}
	channels, err := anime.ListChannels(ctx, r.db)
	if err != nil {
		return nil, err
	}
	result := make([]*model.AnimeChannel, 0, len(channels))
	for _, channel := range channels {
		result = append(result, animeChannelOf(channel))
	}

	return result, nil
}

// AnimeImports is the resolver for the animeImports field.
func (r *queryResolver) AnimeImports(ctx context.Context) ([]*model.AnimeImport, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}

	return r.animeImports(ctx)
}
