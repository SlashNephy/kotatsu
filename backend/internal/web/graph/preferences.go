package graph

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"unicode/utf8"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	// maxPreferenceValueLength は、端末をまたいで同期する設定の値の長さの上限 (文字)。キーの割り当てなど、いちばん長いものでも十分に収まる。
	maxPreferenceValueLength = 16384
)

// isSyncedPreference は、key が、端末をまたいで同期する画面の設定のキーかどうかを返す (#1862、設計書 9.1)。
// 機器や回線によって変わる設定 (音量、画質、拡大など) は、端末ごとに保存するので入れない。
// 画面の frontend/src/settings/syncedPreferences.ts と同じものにする。
func isSyncedPreference(key string) bool {
	switch key {
	case "kotatsu.appIcon",
		"kotatsu.anime.streamingServices",
		"kotatsu.capture.fileNameTemplate",
		"kotatsu.guide.appearance",
		"kotatsu.guide.extendedHours",
		"kotatsu.player.audioLanguage",
		"kotatsu.player.autoPictureInPicture",
		"kotatsu.player.cmAutoSkip",
		"kotatsu.player.commentOffsetMs",
		"kotatsu.player.gamepadMap",
		"kotatsu.player.keymap",
		"kotatsu.player.miniPlayer",
		"kotatsu.player.mouseActions",
		"kotatsu.player.resetAudioDelayOnChannelChange",
		"kotatsu.player.swipeControls",
		"kotatsu.rules.collapsedGroups":
		return true
	}

	return false
}

// preferencesInput は、同期する設定の変更を確認して、置き換えるものと消すキーに分ける。許していないキーと、長すぎる値は断る。
func preferencesInput(input []*model.PreferenceInput) (map[string]string, []string, error) {
	set, unset := map[string]string{}, []string{}
	for _, preference := range input {
		if !isSyncedPreference(preference.Key) {
			return nil, nil, NewError(CodeBadUserInput, fmt.Sprintf("preference %q is not synced", preference.Key))
		}
		if preference.Value == nil {
			unset = append(unset, preference.Key)
			delete(set, preference.Key)

			continue
		}
		if utf8.RuneCountInString(*preference.Value) > maxPreferenceValueLength {
			return nil, nil, NewError(CodeBadUserInput, fmt.Sprintf("preference %q must be at most %d characters", preference.Key, maxPreferenceValueLength))
		}
		set[preference.Key] = *preference.Value
		unset = slices.DeleteFunc(unset, func(key string) bool { return key == preference.Key })
	}

	return set, unset, nil
}

// decodePreferences は、DB の同期する設定の JSON を、キーの順に並べる。今は同期しないキー (同期をやめたもの) は返さない。
func decodePreferences(raw []byte) ([]*model.Preference, error) {
	values := map[string]string{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, fmt.Errorf("failed to decode the preferences: %w", err)
		}
	}
	preferences := make([]*model.Preference, 0, len(values))
	for _, key := range slices.Sorted(maps.Keys(values)) {
		if isSyncedPreference(key) {
			preferences = append(preferences, &model.Preference{Key: key, Value: values[key]})
		}
	}

	return preferences, nil
}
