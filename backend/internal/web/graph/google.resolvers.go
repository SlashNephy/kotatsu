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

// AddGoogleDriveFolder is the resolver for the addGoogleDriveFolder field.
func (r *mutationResolver) AddGoogleDriveFolder(ctx context.Context, id string, libraryName string) (*model.GoogleIntegration, error) {
	_, actor, err := r.recentAdministrator(ctx, "only administrators can choose the Google Drive folders")
	if err != nil {
		return nil, err
	}
	service, err := r.googleService()
	if err != nil {
		return nil, err
	}
	if err := service.AddDriveFolder(ctx, r.db, actor, id, libraryName); err != nil {
		return nil, googleError(err)
	}

	return r.googleIntegration(ctx)
}

// RenameGoogleDriveFolder is the resolver for the renameGoogleDriveFolder field.
func (r *mutationResolver) RenameGoogleDriveFolder(ctx context.Context, id string, libraryName string) (*model.GoogleIntegration, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can rename the Google Drive libraries")
	if err != nil {
		return nil, err
	}
	service, err := r.googleService()
	if err != nil {
		return nil, err
	}
	if err := service.RenameDriveFolder(ctx, r.db, actor, id, libraryName); err != nil {
		return nil, googleError(err)
	}

	return r.googleIntegration(ctx)
}

// RemoveGoogleDriveFolder is the resolver for the removeGoogleDriveFolder field.
func (r *mutationResolver) RemoveGoogleDriveFolder(ctx context.Context, id string) (*model.GoogleIntegration, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can choose the Google Drive folders")
	if err != nil {
		return nil, err
	}
	service, err := r.googleService()
	if err != nil {
		return nil, err
	}
	if err := service.RemoveDriveFolder(ctx, r.db, actor, id); err != nil {
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

// GoogleDriveFolderChoices is the resolver for the googleDriveFolderChoices field.
func (r *queryResolver) GoogleDriveFolderChoices(ctx context.Context, parentID *string) ([]*model.GoogleDriveFolderChoice, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}
	service, err := r.googleService()
	if err != nil {
		return nil, err
	}
	parent := ""
	if parentID != nil {
		parent = *parentID
	}
	choices, err := service.BrowseDriveFolders(ctx, r.db, parent)
	if err != nil {
		return nil, googleError(err)
	}
	result := make([]*model.GoogleDriveFolderChoice, 0, len(choices))
	for _, choice := range choices {
		result = append(result, &model.GoogleDriveFolderChoice{ID: choice.ID, Name: choice.Name})
	}

	return result, nil
}
