package graph

import (
	"context"
	"fmt"

	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// UpdateSystemSettings is the resolver for the updateSystemSettings field.
func (r *mutationResolver) UpdateSystemSettings(ctx context.Context, input model.SystemSettingsInput) (*model.SystemSettings, error) {
	_, actor, err := r.systemSettingsAdministrator(ctx, input)
	if err != nil {
		return nil, err
	}
	// nil のままなら、外字の置き換えの表は変えない。
	var replacements map[string]*string
	if input.DrcsReplacements != nil {
		if replacements, err = toDRCSReplacements(input.DrcsReplacements); err != nil {
			return nil, err
		}
	}
	updated, err := r.settings.Update(ctx, r.db, actor, &settings.Changes{
		Recording:                 toOverrides(input.RecordingDefaults),
		ArchiveRetentionDays:      input.ArchiveRetentionDays,
		NotificationRetentionDays: input.NotificationRetentionDays,
		Rule:                      toRuleChanges(input.RuleDefaults),
		MCPEnabled:                input.McpEnabled,
		DRCSReplacements:          replacements,
	})
	if err != nil {
		return nil, settingsError(err)
	}

	return systemSettingsOf(updated), nil
}

// UpdateChannelType is the resolver for the updateChannelType field.
func (r *mutationResolver) UpdateChannelType(ctx context.Context, typeArg string, input model.ChannelTypeInput) (*model.ChannelType, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can change channel types")
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

// UpdateService is the resolver for the updateService field.
func (r *mutationResolver) UpdateService(ctx context.Context, id string, input model.ServiceInput) (*model.Service, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can change services")
	if err != nil {
		return nil, err
	}
	serviceID, err := parseServiceGlobalID(id)
	if err != nil {
		return nil, err
	}
	err = r.settings.UpdateService(ctx, r.db, actor, serviceID, &settings.ServiceChanges{
		Hidden: input.Hidden, SortOrder: input.SortOrder, ResetSortOrder: input.ResetSortOrder,
	})
	if err != nil {
		return nil, settingsError(err)
	}
	rows, err := r.queries.ListGuideServices(ctx, query.ListGuideServicesParams{IncludeHidden: true, MediaServiceTypes: []int64{}, IDs: []int64{serviceID}})
	if err != nil {
		return nil, fmt.Errorf("failed to read the service: %w", err)
	}
	if len(rows) == 0 {
		return nil, NewError(CodeNotFound, "service not found")
	}

	return toService(&rows[0]), nil
}

// UpdateStoragePool is the resolver for the updateStoragePool field.
func (r *mutationResolver) UpdateStoragePool(ctx context.Context, id string, input model.StoragePoolInput) (*model.StoragePool, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can change storage pools")
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

// DrcsReplacements is the resolver for the drcsReplacements field.
func (r *queryResolver) DrcsReplacements(ctx context.Context) ([]*model.DrcsReplacement, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	system, err := settings.Load(ctx, r.db)
	if err != nil {
		return nil, err
	}

	return drcsReplacementsOf(system.DRCSReplacements), nil
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
