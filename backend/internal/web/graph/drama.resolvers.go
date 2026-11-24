package graph

import (
	"context"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/usecase/drama"
	"github.com/SlashNephy/kotatsu/internal/web/graph/generated"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// UpdateDramaLinkingToken is the resolver for the updateDramaLinkingToken field.
func (r *mutationResolver) UpdateDramaLinkingToken(ctx context.Context, token string) (*model.DramaLinkingStatus, error) {
	_, actor, err := r.recentAdministrator(ctx, "only administrators can change the TMDB token")
	if err != nil {
		return nil, err
	}
	tokens, err := r.dramaTokenStore()
	if err != nil {
		return nil, err
	}
	if err := tokens.Update(ctx, r.db, actor, token); err != nil {
		return nil, dramaError(err)
	}
	// 入れたトークンで、6 時間ごとの照合し直しを待たずに照合する。連携が無効なら、ジョブは何もしない。
	if strings.TrimSpace(token) != "" {
		r.startDramaLinking(ctx)
	}

	return r.dramaLinking(ctx)
}

// UpdateDramaTitleOverride is the resolver for the updateDramaTitleOverride field.
func (r *mutationResolver) UpdateDramaTitleOverride(ctx context.Context, input model.DramaTitleOverrideInput) ([]*model.DramaTitleOverride, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can change drama title overrides")
	if err != nil {
		return nil, err
	}
	titleKey, err := dramaTitleKeyOf(&input)
	if err != nil {
		return nil, err
	}
	change := &drama.OverrideChange{SeriesID: nil, Exclude: input.Exclude, Reset: input.Reset}
	if input.TmdbSeriesID != nil {
		id, err := tmdbSeriesIDOf(*input.TmdbSeriesID)
		if err != nil {
			return nil, err
		}
		change.SeriesID = new(id)
	}
	if err := r.dramaOverrides.Update(ctx, r.db, actor, titleKey, change); err != nil {
		return nil, dramaError(err)
	}

	return r.dramaTitleOverrides(ctx)
}

// DramaEpisode is the resolver for the dramaEpisode field.
func (r *programResolver) DramaEpisode(ctx context.Context, obj *model.Program) (*model.DramaEpisode, error) {
	episode, found, err := drama.ProgramEpisode(ctx, r.db, obj.ServiceKey, obj.StartAt)
	if err != nil || !found {
		return nil, err
	}

	return dramaEpisodeOf(episode), nil
}

// DramaLinking is the resolver for the dramaLinking field.
func (r *queryResolver) DramaLinking(ctx context.Context) (*model.DramaLinkingStatus, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}

	return r.dramaLinking(ctx)
}

// DramaTitleOverrides is the resolver for the dramaTitleOverrides field.
func (r *queryResolver) DramaTitleOverrides(ctx context.Context) ([]*model.DramaTitleOverride, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}

	return r.dramaTitleOverrides(ctx)
}

// DramaEpisode is the resolver for the dramaEpisode field.
func (r *recordingResolver) DramaEpisode(ctx context.Context, obj *model.Recording) (*model.DramaEpisode, error) {
	recordingID, err := parseRecordingID(obj.ID)
	if err != nil {
		return nil, err
	}
	episode, found, err := drama.RecordingEpisode(ctx, r.db, recordingID)
	if err != nil || !found {
		return nil, err
	}

	return dramaEpisodeOf(episode), nil
}

// DramaEpisode returns generated.DramaEpisodeResolver implementation.
func (r *Resolver) DramaEpisode() generated.DramaEpisodeResolver { return &dramaEpisodeResolver{r} }

// DramaSeries returns generated.DramaSeriesResolver implementation.
func (r *Resolver) DramaSeries() generated.DramaSeriesResolver { return &dramaSeriesResolver{r} }

type (
	dramaEpisodeResolver struct{ *Resolver }
	dramaSeriesResolver  struct{ *Resolver }
)
