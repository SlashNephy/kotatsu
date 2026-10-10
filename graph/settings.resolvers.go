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

// UpdateChannelType is the resolver for the updateChannelType field.
func (r *mutationResolver) UpdateChannelType(ctx context.Context, typeArg string, input model.ChannelTypeInput) (*model.ChannelType, error) {
	_, actor, err := r.recentAdministrator(ctx, "only administrators can change channel types")
	if err != nil {
		return nil, err
	}
	updated, err := r.settings.UpdateChannelType(ctx, r.db, actor, typeArg, &settings.ChannelTypeChanges{
		DisplayName: input.DisplayName,
		SortOrder:   input.SortOrder,
	})
	if err != nil {
		return nil, settingsError(err)
	}

	return toChannelType(updated), nil
}

// UpdateStoragePool is the resolver for the updateStoragePool field.
func (r *mutationResolver) UpdateStoragePool(ctx context.Context, id string, input model.StoragePoolInput) (*model.StoragePool, error) {
	_, actor, err := r.recentAdministrator(ctx, "only administrators can change storage pools")
	if err != nil {
		return nil, err
	}
	if _, ok := r.pools[id]; !ok {
		return nil, NewError(CodeNotFound, "storage pool not found")
	}
	updated, err := r.settings.UpdateStoragePool(ctx, r.db, actor, id, &settings.StoragePoolChanges{Priority: input.Priority, MinFreeBytes: input.MinFreeBytes})
	if err != nil {
		return nil, settingsError(err)
	}

	return r.storagePoolOf(id, updated), nil
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
