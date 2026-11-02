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

// UpdateAnimeImportToken is the resolver for the updateAnimeImportToken field.
func (r *mutationResolver) UpdateAnimeImportToken(ctx context.Context, annictToken string) (*model.AnimeImportToken, error) {
	_, actor, err := r.recentAdministrator(ctx, "only administrators can change the anime import token")
	if err != nil {
		return nil, err
	}
	service, err := r.animeRecordService()
	if err != nil {
		return nil, err
	}
	if err := service.UpdateImportToken(ctx, r.db, actor, annictToken); err != nil {
		return nil, animeRecordError(err)
	}

	return r.animeImportToken(ctx)
}

// AnimeEpisodes is the resolver for the animeEpisodes field.
func (r *programResolver) AnimeEpisodes(ctx context.Context, obj *model.Program) ([]*model.AnimeEpisode, error) {
	episodes, err := anime.ProgramEpisodes(ctx, r.db, obj.ServiceKey, obj.StartAt)
	if err != nil {
		return nil, err
	}

	return animeEpisodesOf(episodes), nil
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

// AnimeImportToken is the resolver for the animeImportToken field.
func (r *queryResolver) AnimeImportToken(ctx context.Context) (*model.AnimeImportToken, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}

	return r.animeImportToken(ctx)
}

// AnimeEpisodes is the resolver for the animeEpisodes field.
func (r *recordingResolver) AnimeEpisodes(ctx context.Context, obj *model.Recording) ([]*model.AnimeEpisode, error) {
	recordingID, err := parseRecordingID(obj.ID)
	if err != nil {
		return nil, err
	}
	episodes, err := anime.RecordingEpisodes(ctx, r.db, recordingID)
	if err != nil {
		return nil, err
	}

	return animeEpisodesOf(episodes), nil
}
