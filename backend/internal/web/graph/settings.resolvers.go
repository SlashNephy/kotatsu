package graph

import (
	"context"
	"fmt"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/web/graph/generated"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// UpdateSystemSettings is the resolver for the updateSystemSettings field.
func (r *mutationResolver) UpdateSystemSettings(ctx context.Context, input model.SystemSettingsInput) (*model.SystemSettings, error) {
	_, actor, err := r.systemSettingsAdministrator(ctx, input)
	if err != nil {
		return nil, err
	}
	// neko は、この設定のままサービスを割り振るので、設定していない構成で、録れない割り振りを作らないようにする。
	if input.TimeshiftWhileWatching != nil && *input.TimeshiftWhileWatching && !r.store.HasTimeshiftDir() {
		return nil, NewError(CodeConflict, "timeshift is not configured on this instance; set storage.timeshift_dir")
	}
	// nil のままなら、外字の置き換えの表は変えない。
	var replacements map[string]*string
	if input.DrcsReplacements != nil {
		if replacements, err = toDRCSReplacements(input.DrcsReplacements); err != nil {
			return nil, err
		}
	}
	// nil のままなら、実況のチャンネルの対応は変えない (#1869)。
	var jikkyoChannels map[string]*string
	if input.JikkyoChannels != nil {
		if jikkyoChannels, err = toJikkyoChannels(input.JikkyoChannels); err != nil {
			return nil, err
		}
	}
	updated, err := r.settings.Update(ctx, r.db, actor, &settings.Changes{
		Recording:                  toOverrides(input.RecordingDefaults),
		ArchiveRetentionDays:       input.ArchiveRetentionDays,
		NotificationRetentionDays:  input.NotificationRetentionDays,
		WatchProgressRetentionDays: input.WatchProgressRetentionDays,
		NetworkActivityEnabled:     input.NetworkActivityEnabled, NetworkActivityRetentionDays: input.NetworkActivityRetentionDays,
		Rule:                      toRuleChanges(input.RuleDefaults),
		MCPEnabled:                input.McpEnabled,
		DataBroadcastIPEnabled:    input.DataBroadcastIPEnabled,
		ExternalPlayerURLsEnabled: input.ExternalPlayerUrlsEnabled,
		IPTVFeedsEnabled:          input.IptvFeedsEnabled,
		RecordingDownloadsEnabled: input.RecordingDownloadsEnabled,
		TvtplayChaptersEnabled:    input.TvtplayChaptersEnabled,
		CMDetector:                cmDetectorName(input.CMDetector),
		EPGStationAPIEnabled:      input.EpgstationAPIEnabled,
		AnimeLinkingEnabled:       input.AnimeLinkingEnabled,
		DramaLinkingEnabled:       input.DramaLinkingEnabled,
		CommentSources:            optionalCommentSources(input.CommentSources),
		DRCSReplacements:          replacements,
		TimeshiftWhileWatching:    input.TimeshiftWhileWatching, TimeshiftWhileWatchingRetentionSeconds: input.TimeshiftWhileWatchingRetentionSeconds,
		RuleUnmatchedGraceMinutes: input.RuleUnmatchedGraceMinutes,
		AuditRetentionDays:        input.AuditRetentionDays, AuditClientInfoRetentionDays: input.AuditClientInfoRetentionDays,
		HIBPEnabled:              input.HibpEnabled,
		WebPushSubject:           input.WebPushSubject,
		ThumbnailPreviewsEnabled: input.ThumbnailPreviewsEnabled,
		SleepControlEnabled:      input.SleepControlEnabled,
		CaptionThreads:           input.CaptionThreads,
		JikkyoChannels:           jikkyoChannels,
	})
	if err != nil {
		return nil, settingsError(err)
	}
	// 実況のチャンネルの対応を変えたら、このインスタンスでは、覚えた対応を待たずに使う。ほかのインスタンスは、少しの間で読み直す。
	if jikkyoChannels != nil && r.jikkyoTable != nil {
		r.jikkyoTable.Invalidate()
	}
	// 連携を有効にしたら、1 時間ごとの確かめを待たずに、連携先の情報を取り込み始める (設計書 3.8)。
	if updated.AnimeLinkingEnabled && input.AnimeLinkingEnabled != nil {
		r.startAnimeLinking(ctx)
	}
	// TMDb との連携を有効にしたら、6 時間ごとの照合し直しを待たずに、これから 1 週間の番組を照合する (設計書 3.9)。
	if updated.DramaLinkingEnabled && input.DramaLinkingEnabled != nil {
		r.startDramaLinking(ctx)
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
	rows, err := r.queries.ListGuideServices(ctx, query.ListGuideServicesParams{IncludeHidden: true, IncludeGone: true, MediaServiceTypes: []int64{}, IDs: []int64{serviceID}, Now: r.clock.Now()})
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

// TranscodePresets is the resolver for the transcodePresets field.
func (r *queryResolver) TranscodePresets(ctx context.Context) ([]string, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}

	return domain.TranscodePresets(), nil
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

// WebPushSubject is the resolver for the webPushSubject field.
func (r *systemSettingsResolver) WebPushSubject(ctx context.Context, obj *model.SystemSettings) (*string, error) {
	// 管理者の連絡先を含みうるので、管理者のほかには FORBIDDEN を返す (#1869)。値は systemSettingsOf が入れておく。
	if _, _, err := r.administratorActor(ctx, "only administrators can read the web push subject"); err != nil {
		return nil, err
	}

	return obj.WebPushSubject, nil
}

// SystemSettings returns generated.SystemSettingsResolver implementation.
func (r *Resolver) SystemSettings() generated.SystemSettingsResolver {
	return &systemSettingsResolver{r}
}

type systemSettingsResolver struct{ *Resolver }
