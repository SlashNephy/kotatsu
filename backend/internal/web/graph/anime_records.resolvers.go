package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// UpdateAnimeRecordApp is the resolver for the updateAnimeRecordApp field.
func (r *mutationResolver) UpdateAnimeRecordApp(ctx context.Context, destination model.AnimeRecordDestination, input model.AnimeRecordAppInput) (*model.AnimeRecordApp, error) {
	_, actor, err := r.recentAdministrator(ctx, "only administrators can change the anime record apps")
	if err != nil {
		return nil, err
	}
	service, err := r.animeRecordService()
	if err != nil {
		return nil, err
	}
	name := destinationName(destination)
	if err := service.UpdateApp(ctx, r.db, actor, name, &animerecord.AppChanges{ClientID: input.ClientID, ClientSecret: input.ClientSecret}); err != nil {
		return nil, animeRecordError(err)
	}
	apps, err := r.animeRecordApps(ctx)
	if err != nil {
		return nil, err
	}
	for _, app := range apps {
		if app.Destination == destination {
			return app, nil
		}
	}

	return nil, NewError(CodeNotFound, "destination not found")
}

// BeginAnimeRecordConnection is the resolver for the beginAnimeRecordConnection field.
func (r *mutationResolver) BeginAnimeRecordConnection(ctx context.Context, destination model.AnimeRecordDestination) (string, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return "", err
	}
	service, err := r.animeRecordService()
	if err != nil {
		return "", err
	}
	url, err := service.Begin(ctx, r.db, principal, destinationName(destination), requestOrigin(ctx))
	if err != nil {
		return "", animeRecordError(err)
	}

	return url, nil
}

// DisconnectAnimeRecord is the resolver for the disconnectAnimeRecord field.
func (r *mutationResolver) DisconnectAnimeRecord(ctx context.Context, destination model.AnimeRecordDestination) (*model.AnimeRecordSettings, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	service, err := r.animeRecordService()
	if err != nil {
		return nil, err
	}
	if err := service.Disconnect(ctx, r.db, actorOf(ctx, principal), principal.UserID, destinationName(destination)); err != nil {
		return nil, animeRecordError(err)
	}

	return r.animeRecordSettings(ctx, principal.UserID)
}

// UpdateAnimeRecordSettings is the resolver for the updateAnimeRecordSettings field.
func (r *mutationResolver) UpdateAnimeRecordSettings(ctx context.Context, input model.AnimeRecordSettingsInput) (*model.AnimeRecordSettings, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	service, err := r.animeRecordService()
	if err != nil {
		return nil, err
	}
	current, err := animerecord.LoadSettings(ctx, r.db, principal.UserID)
	if err != nil {
		return nil, err
	}
	notifyWishlist := current.NotifyWishlist
	if input.NotifyWishlist != nil {
		notifyWishlist = *input.NotifyWishlist
	}
	if _, err := service.SaveSettings(ctx, r.db, principal.UserID, &animerecord.UserSettings{
		ThresholdPercent: input.ThresholdPercent,
		Rules: &animerecord.StatusRules{
			WatchingOnFirst: input.WatchingOnFirst, WatchingOnEpisode: input.WatchingOnEpisode, WatchedOnLast: input.WatchedOnLast,
		},
		NotifyWishlist: notifyWishlist,
	}); err != nil {
		return nil, animeRecordError(err)
	}

	return r.animeRecordSettings(ctx, principal.UserID)
}

// ReportAnimePlayback is the resolver for the reportAnimePlayback field.
func (r *mutationResolver) ReportAnimePlayback(ctx context.Context, serviceID *string, recordingID *string, intervals []*model.PlaybackIntervalInput) (bool, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return false, err
	}
	target, err := toPlaybackTarget(serviceID, recordingID)
	if err != nil {
		return false, err
	}
	converted, err := toIntervals(intervals)
	if err != nil {
		return false, err
	}
	if r.animeRecords == nil {
		return false, nil
	}
	tracked, err := r.animeRecords.Report(ctx, r.db, r.jobs, principal.UserID, target, converted)
	if err != nil {
		return false, animeRecordError(err)
	}

	return tracked, nil
}

// AnimeRecordApps is the resolver for the animeRecordApps field.
func (r *queryResolver) AnimeRecordApps(ctx context.Context) ([]*model.AnimeRecordApp, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}

	return r.animeRecordApps(ctx)
}

// AnimeRecordSettings is the resolver for the animeRecordSettings field.
func (r *queryResolver) AnimeRecordSettings(ctx context.Context) (*model.AnimeRecordSettings, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}

	return r.animeRecordSettings(ctx, principal.UserID)
}
