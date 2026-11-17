package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// UpdateBackupSettings is the resolver for the updateBackupSettings field.
func (r *mutationResolver) UpdateBackupSettings(ctx context.Context, input model.BackupSettingsInput) (*model.BackupStatus, error) {
	return r.updateBackupSettings(ctx, input)
}

// RunBackup is the resolver for the runBackup field.
func (r *mutationResolver) RunBackup(ctx context.Context) (*model.BackupStatus, error) {
	return r.runBackup(ctx)
}

// Backup is the resolver for the backup field.
func (r *queryResolver) Backup(ctx context.Context) (*model.BackupStatus, error) {
	return r.backupStatus(ctx)
}
