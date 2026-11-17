package graph

import (
	"context"
	"errors"

	"github.com/SlashNephy/kotatsu/internal/lib/secretbox"
	"github.com/SlashNephy/kotatsu/internal/usecase/google"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// errNoGoogle は、Google の連携を扱う部品を渡していない (アプリの秘密キーがない) ことを表す。
var errNoGoogle = errors.New("the Google integration is not configured")

// googleService は、Google の連携を扱う部品を返す。渡していなければ、内部のエラーにする。
func (r *Resolver) googleService() (*google.Service, error) {
	if r.google == nil {
		return nil, errNoGoogle
	}

	return r.google, nil
}

// googleIntegration は、Google の OAuth クライアントと、Google Drive の接続を返す。
func (r *Resolver) googleIntegration(ctx context.Context) (*model.GoogleIntegration, error) {
	service, err := r.googleService()
	if err != nil {
		return nil, err
	}
	app, err := service.App(ctx, r.db)
	if err != nil {
		return nil, err
	}
	result := &model.GoogleIntegration{
		ClientID: nonEmpty(app.ClientID), Configured: app.Configured, RedirectURI: google.RedirectURI(requestOrigin(ctx)),
		UpdatedAt: app.UpdatedAt, Drive: nil, Folders: []*model.GoogleDriveFolder{},
	}
	folders, err := service.DriveFolders(ctx, r.db)
	if err != nil {
		return nil, err
	}
	for _, folder := range folders {
		result.Folders = append(result.Folders, &model.GoogleDriveFolder{ID: folder.ID, LibraryName: folder.Library, Name: folder.Name, AddedAt: folder.AddedAt})
	}
	drive, err := service.Drive(ctx, r.db)
	switch {
	case errors.Is(err, google.ErrNotConnected):
	case err != nil:
		return nil, err
	default:
		result.Drive = &model.GoogleDriveConnection{AccountName: drive.AccountName, ConnectedAt: drive.ConnectedAt, RevokedAt: drive.RevokedAt}
	}

	return result, nil
}

// googleError は、Google の連携のエラーを、GraphQL のエラーにする。
func googleError(err error) error {
	switch {
	case errors.Is(err, google.ErrInvalidApp), errors.Is(err, google.ErrNotAFolder), errors.Is(err, google.ErrInvalidLibraryName):
		return NewError(CodeBadUserInput, err.Error())
	case errors.Is(err, google.ErrUnavailable):
		return NewError(CodeConflict, "register the Google OAuth client first")
	case errors.Is(err, google.ErrSessionRequired):
		return NewError(CodeForbidden, "a browser session is required to connect")
	case errors.Is(err, google.ErrNotConnected):
		return NewError(CodeNotFound, "Google Drive is not connected")
	case errors.Is(err, google.ErrFolderNotSelected):
		return NewError(CodeNotFound, "the folder is not selected")
	case errors.Is(err, secretbox.ErrDecrypt):
		// アプリの秘密キーを変えると、保存したシークレットやトークンを読めなくなる。中身は返さない。
		return NewError(CodeConflict, "the saved secret cannot be decrypted; register it again")
	default:
		return err
	}
}
