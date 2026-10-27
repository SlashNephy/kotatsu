package graph

import (
	"context"
	"errors"

	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// systemSettingsAdministrator は、全体の設定を変える管理者を確かめ、操作者を返す。
// MCP のクライアントとの連携を変えるときだけ、外部のサービスに読ませるかどうかを決める操作なので、直近 10 分以内の再認証を求める。
// 録画やルールの既定値、保持期間は、第三者に影響しないので求めない (設計書 9.5)。
func (r *Resolver) systemSettingsAdministrator(ctx context.Context, input model.SystemSettingsInput) (*auth.Principal, *audit.Actor, error) {
	const message = "only administrators can change the system settings"
	if input.McpEnabled != nil {
		return r.recentAdministrator(ctx, message)
	}

	return r.administratorActor(ctx, message)
}

// systemSettingsOf は、全体の設定を GraphQL の形にする。
func systemSettingsOf(system *settings.System) *model.SystemSettings {
	recording := &model.RecordingSettings{
		PoolID:             nil,
		FileNameTemplate:   system.Recording.FileNameTemplate,
		StartMarginSeconds: system.Recording.StartMarginSeconds,
		EndMarginSeconds:   system.Recording.EndMarginSeconds,
		Priority:           system.Recording.Priority,
		JoinSegments:       system.Recording.JoinSegments,
	}
	if system.Recording.PoolID != "" {
		recording.PoolID = new(system.Recording.PoolID)
	}

	return &model.SystemSettings{
		RecordingDefaults:         recording,
		ArchiveRetentionDays:      system.ArchiveRetentionDays,
		NotificationRetentionDays: system.NotificationRetentionDays,
		RuleDefaults: &model.RuleDefaults{
			AvoidDuplicates:     system.Rule.AvoidDuplicates,
			DuplicateWindowDays: system.Rule.DuplicateWindowDays,
			FollowEventRelay:    system.Rule.FollowEventRelay,
		},
		McpEnabled: system.MCPEnabled,
		UpdatedAt:  system.UpdatedAt,
	}
}

// toRuleChanges は、ルールの既定値の入力を、変える項目にする。
func toRuleChanges(input *model.RuleDefaultsInput) *settings.RuleChanges {
	if input == nil {
		return nil
	}

	return &settings.RuleChanges{
		AvoidDuplicates:          input.AvoidDuplicates,
		DuplicateWindowDays:      input.DuplicateWindowDays,
		ClearDuplicateWindowDays: input.ClearDuplicateWindowDays,
		FollowEventRelay:         input.FollowEventRelay,
	}
}

// settingsError は、設定のエラーを、エラーコードを持つエラーにする。
func settingsError(err error) error {
	if errors.Is(err, settings.ErrInvalidSettings) {
		return NewError(CodeBadUserInput, err.Error())
	}
	if errors.Is(err, settings.ErrChannelTypeNotFound) {
		return NewError(CodeNotFound, "channel type not found")
	}
	if errors.Is(err, settings.ErrServiceNotFound) {
		return NewError(CodeNotFound, "service not found")
	}

	return err
}
