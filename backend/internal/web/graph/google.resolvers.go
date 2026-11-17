package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/usecase/google"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// UpdateGoogleApp is the resolver for the updateGoogleApp field.
func (r *mutationResolver) UpdateGoogleApp(ctx context.Context, input model.GoogleAppInput) (*model.GoogleIntegration, error) {
	_, actor, err := r.recentAdministrator(ctx, "only administrators can change the Google OAuth client")
	if err != nil {
		return nil, err
	}
	service, err := r.googleService()
	if err != nil {
		return nil, err
	}
	if err := service.UpdateApp(ctx, r.db, actor, &google.AppChanges{ClientID: input.ClientID, ClientSecret: input.ClientSecret}); err != nil {
		return nil, googleError(err)
	}

	return r.googleIntegration(ctx)
}

// BeginGoogleDriveConnection is the resolver for the beginGoogleDriveConnection field.
func (r *mutationResolver) BeginGoogleDriveConnection(ctx context.Context) (string, error) {
	principal, _, err := r.recentAdministrator(ctx, "only administrators can connect Google Drive")
	if err != nil {
		return "", err
	}
	service, err := r.googleService()
	if err != nil {
		return "", err
	}
	url, err := service.BeginDrive(ctx, r.db, principal, requestOrigin(ctx))
	if err != nil {
		return "", googleError(err)
	}

	return url, nil
}

// DisconnectGoogleDrive is the resolver for the disconnectGoogleDrive field.
func (r *mutationResolver) DisconnectGoogleDrive(ctx context.Context) (*model.GoogleIntegration, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can disconnect Google Drive")
	if err != nil {
		return nil, err
	}
	service, err := r.googleService()
	if err != nil {
		return nil, err
	}
	if err := service.DisconnectDrive(ctx, r.db, actor); err != nil {
		return nil, googleError(err)
	}

	return r.googleIntegration(ctx)
}

// GoogleIntegration is the resolver for the googleIntegration field.
func (r *queryResolver) GoogleIntegration(ctx context.Context) (*model.GoogleIntegration, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}

	return r.googleIntegration(ctx)
}
