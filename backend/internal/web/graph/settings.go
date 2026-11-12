package graph

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// systemSettingsAdministrator は、全体の設定を変える管理者を確かめ、操作者を返す。
// MCP のクライアントとの連携、データ放送の通信の中継 (#501)、しょぼいカレンダー、Annict、AniList との連携、ニコニコ実況を変えるときだけ、
// 外部のサービスに読ませたり、受信機の情報を外部のサーバーへ送らせたり、外部へ問い合わせ始めたりするかどうかを決める操作なので、
// 直近 10 分以内の再認証を求める。録画やルールの既定値、保持期間は、第三者に影響しないので求めない (設計書 9.5)。
func (r *Resolver) systemSettingsAdministrator(ctx context.Context, input model.SystemSettingsInput) (*auth.Principal, *audit.Actor, error) {
	const message = "only administrators can change the system settings"
	if input.McpEnabled != nil || input.DataBroadcastIPEnabled != nil || input.AnimeLinkingEnabled != nil || input.CommentSources != nil {
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
		DetectCM:           system.Recording.DetectCM,
		TranscodePreset:    nil,
		TranscodeFrameRate: system.Recording.TranscodeFrameRate,
	}
	if system.Recording.PoolID != "" {
		recording.PoolID = new(system.Recording.PoolID)
	}
	if system.Recording.TranscodePreset != "" {
		recording.TranscodePreset = new(system.Recording.TranscodePreset)
	}

	return &model.SystemSettings{
		RecordingDefaults:          recording,
		ArchiveRetentionDays:       system.ArchiveRetentionDays,
		NotificationRetentionDays:  system.NotificationRetentionDays,
		WatchProgressRetentionDays: system.WatchProgressRetentionDays,
		NetworkActivityEnabled:     system.NetworkActivityEnabled, NetworkActivityRetentionDays: system.NetworkActivityRetentionDays,
		RuleDefaults: &model.RuleDefaults{
			AvoidDuplicates:     system.Rule.AvoidDuplicates,
			DuplicateWindowDays: system.Rule.DuplicateWindowDays,
			FollowEventRelay:    system.Rule.FollowEventRelay,
		},
		McpEnabled:             system.MCPEnabled,
		DataBroadcastIPEnabled: system.DataBroadcastIPEnabled,
		AnimeLinkingEnabled:    system.AnimeLinkingEnabled,
		JikkyoEnabled:          slices.Contains(system.CommentSources, domain.CommentSourceNXJikkyo),
		CommentSources:         commentSourcesOf(system.CommentSources),
		DrcsReplacements:       drcsReplacementsOf(system.DRCSReplacements),
		TimeshiftWhileWatching: system.TimeshiftWhileWatching, TimeshiftWhileWatchingRetentionSeconds: system.TimeshiftWhileWatchingRetentionSeconds,
		RuleUnmatchedGraceMinutes: system.RuleUnmatchedGraceMinutes,
		UpdatedAt:                 system.UpdatedAt,
	}
}

// drcsReplacementsOf は、外字の置き換えの表を、MD5 の順に並べた GraphQL の形にする (#825)。
func drcsReplacementsOf(replacements map[string]string) []*model.DrcsReplacement {
	result := make([]*model.DrcsReplacement, 0, len(replacements))
	for _, md5 := range slices.Sorted(maps.Keys(replacements)) {
		result = append(result, &model.DrcsReplacement{Md5: md5, Text: replacements[md5]})
	}

	return result
}

// toDRCSReplacements は、外字の置き換えの表の変更の入力を、MD5 から文字 (nil なら消す) への表にする。
// 同じ MD5 が 2 度出てきたら、どちらを採るか決められないので拒む。
func toDRCSReplacements(input []*model.DrcsReplacementInput) (map[string]*string, error) {
	replacements := make(map[string]*string, len(input))
	for _, row := range input {
		key := strings.ToLower(row.Md5)
		if _, ok := replacements[key]; ok {
			return nil, NewError(CodeBadUserInput, "drcsReplacements has a duplicated MD5: "+key)
		}
		replacements[key] = row.Text
	}

	return replacements, nil
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

// commentSourcesOf は、DB の実況のコメントのソースを GraphQL の形にする。知らないソースは除く。
func commentSourcesOf(sources []string) []model.CommentSource {
	converted := make([]model.CommentSource, 0, len(sources))
	for _, source := range sources {
		if value := model.CommentSource(strings.ToUpper(source)); value.IsValid() {
			converted = append(converted, value)
		}
	}

	return converted
}

// commentSourcesInput は、GraphQL の実況のコメントのソースを DB の形にする。重複は除く。省いた項目 (nil) は nil のままにする。
func commentSourcesInput(sources []model.CommentSource) []string {
	if sources == nil {
		return nil
	}
	converted := make([]string, 0, len(sources))
	for _, source := range sources {
		if value := strings.ToLower(string(source)); !slices.Contains(converted, value) {
			converted = append(converted, value)
		}
	}

	return converted
}

// optionalCommentSources は、全体の設定の変更の形にする。省いた項目 (nil) は変えない。
func optionalCommentSources(sources []model.CommentSource) *[]string {
	converted := commentSourcesInput(sources)
	if converted == nil {
		return nil
	}

	return &converted
}
