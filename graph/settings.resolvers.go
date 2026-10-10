package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/settings"
)

// UpdateSystemSettings is the resolver for the updateSystemSettings field.
func (r *mutationResolver) UpdateSystemSettings(ctx context.Context, input model.SystemSettingsInput) (*model.SystemSettings, error) {
	_, actor, err := r.recentAdministrator(ctx, "only administrators can change the system settings")
	if err != nil {
		return nil, err
	}
	updated, err := r.settings.Update(ctx, r.db, actor, &settings.Changes{
		Recording:            toOverrides(input.RecordingDefaults),
		ArchiveRetentionDays: input.ArchiveRetentionDays,
	})
	if err != nil {
		return nil, settingsError(err)
	}

	return systemSettingsOf(updated), nil
}

// SystemSettings is the resolver for the systemSettings field.
func (r *queryResolver) SystemSettings(ctx context.Context) (*model.SystemSettings, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	system, err := settings.Load(ctx, r.db)
	if err != nil {
		return nil, err
	}

	return systemSettingsOf(system), nil
}
