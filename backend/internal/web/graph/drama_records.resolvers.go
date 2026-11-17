package graph

import (
	"context"
	"errors"

	"github.com/SlashNephy/kotatsu/internal/usecase/dramalibrary"
	"github.com/SlashNephy/kotatsu/internal/usecase/dramarecord"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// UpdateDramaRecordApp is the resolver for the updateDramaRecordApp field.
func (r *mutationResolver) UpdateDramaRecordApp(ctx context.Context, destination model.DramaRecordDestination, input model.DramaRecordAppInput) (*model.DramaRecordApp, error) {
	_, actor, err := r.recentAdministrator(ctx, "only administrators can change the drama record apps")
	if err != nil {
		return nil, err
	}
	service, err := r.dramaRecordService()
	if err != nil {
		return nil, err
	}
	if err := service.UpdateApp(ctx, r.db, actor, dramaDestinationName(destination), &dramarecord.AppChanges{
		ClientID: input.ClientID, ClientSecret: input.ClientSecret,
	}); err != nil {
		return nil, dramaRecordError(err)
	}
	apps, err := r.dramaRecordApps(ctx)
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

// BeginDramaRecordConnection is the resolver for the beginDramaRecordConnection field.
func (r *mutationResolver) BeginDramaRecordConnection(ctx context.Context, destination model.DramaRecordDestination) (string, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return "", err
	}
	service, err := r.dramaRecordService()
	if err != nil {
		return "", err
	}
	url, err := service.Begin(ctx, r.db, principal, dramaDestinationName(destination), requestOrigin(ctx))
	if err != nil {
		return "", dramaRecordError(err)
	}

	return url, nil
}

// DisconnectDramaRecord is the resolver for the disconnectDramaRecord field.
func (r *mutationResolver) DisconnectDramaRecord(ctx context.Context, destination model.DramaRecordDestination) (*model.DramaRecordSettings, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	service, err := r.dramaRecordService()
	if err != nil {
		return nil, err
	}
	if err := service.Disconnect(ctx, r.db, actorOf(ctx, principal), principal.UserID, dramaDestinationName(destination)); err != nil {
		return nil, dramaRecordError(err)
	}

	return r.dramaRecordSettings(ctx, principal.UserID)
}

// ImportDramaWatchlist is the resolver for the importDramaWatchlist field.
func (r *mutationResolver) ImportDramaWatchlist(ctx context.Context, force bool) (bool, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return false, err
	}
	service, err := r.dramaRecordService()
	if err != nil {
		return false, err
	}
	enqueued, err := service.RequestWatchlistImport(ctx, r.db, r.jobs, principal.UserID, force)
	if err != nil {
		return false, dramaRecordError(err)
	}

	return enqueued, nil
}

// UpdateDramaRecordSettings is the resolver for the updateDramaRecordSettings field.
func (r *mutationResolver) UpdateDramaRecordSettings(ctx context.Context, input model.DramaRecordSettingsInput) (*model.DramaRecordSettings, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := dramalibrary.SaveThresholdPercent(ctx, r.db, r.clock, principal.UserID, input.ThresholdPercent); err != nil {
		if errors.Is(err, dramalibrary.ErrInvalidSettings) {
			return nil, NewError(CodeBadUserInput, err.Error())
		}

		return nil, err
	}

	return r.dramaRecordSettings(ctx, principal.UserID)
}

// DramaRecordApps is the resolver for the dramaRecordApps field.
func (r *queryResolver) DramaRecordApps(ctx context.Context) ([]*model.DramaRecordApp, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}

	return r.dramaRecordApps(ctx)
}

// DramaRecordSettings is the resolver for the dramaRecordSettings field.
func (r *queryResolver) DramaRecordSettings(ctx context.Context) (*model.DramaRecordSettings, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}

	return r.dramaRecordSettings(ctx, principal.UserID)
}
