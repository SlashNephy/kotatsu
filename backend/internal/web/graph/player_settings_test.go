package graph_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/usersettings"
)

type playerSettings struct {
	JikkyoEnabled   bool `json:"jikkyoEnabled"`
	CaptionsEnabled bool `json:"captionsEnabled"`
}

// TestPlayerSettings は、プレイヤーの設定を、省いた項目を変えずに変えられ、ほかの端末に知らせることを確かめる。
func TestPlayerSettings(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	get := func() playerSettings {
		var got struct {
			PlayerSettings playerSettings `json:"playerSettings"`
		}
		require.Empty(t, f.query(t, `{ playerSettings { jikkyoEnabled captionsEnabled } }`, nil, &got))

		return got.PlayerSettings
	}
	update := func(input map[string]any) playerSettings {
		var got struct {
			UpdatePlayerSettings playerSettings `json:"updatePlayerSettings"`
		}
		require.Empty(t, f.query(t, `mutation($input: PlayerSettingsInput!) {
			updatePlayerSettings(input: $input) { jikkyoEnabled captionsEnabled }
		}`, map[string]any{varInput: input}, &got))

		return got.UpdatePlayerSettings
	}
	version := func() int64 {
		v, err := usersettings.Version(t.Context(), f.pool, f.userID)
		require.NoError(t, err)

		return v
	}

	tests := []struct {
		name  string
		input map[string]any
		want  playerSettings
	}{
		{name: "実況だけを切ると、字幕は変えない", input: map[string]any{"jikkyoEnabled": false}, want: playerSettings{JikkyoEnabled: false, CaptionsEnabled: true}},
		{name: "字幕だけを切ると、実況は変えない", input: map[string]any{"captionsEnabled": false}, want: playerSettings{JikkyoEnabled: false, CaptionsEnabled: false}},
		{name: "両方を入れる", input: map[string]any{"jikkyoEnabled": true, "captionsEnabled": true}, want: playerSettings{JikkyoEnabled: true, CaptionsEnabled: true}},
	}
	// 一度も変えていなければ、どちらも出す。
	assert.Equal(t, playerSettings{JikkyoEnabled: true, CaptionsEnabled: true}, get())
	// 順に変えていくので、並行に動かさない。
	for _, tt := range tests {
		before := version()
		assert.Equal(t, tt.want, update(tt.input), tt.name)
		assert.Equal(t, tt.want, get(), tt.name)
		// ほかの端末に viewerSettingsChanged で知らせるよう、設定の版数を上げる。
		assert.Greater(t, version(), before, tt.name)
	}
}

type mutedKeyword struct {
	Text  string `json:"text"`
	Match string `json:"match"`
}

type commentSettings struct {
	Opacity       float64        `json:"opacity"`
	Speed         float64        `json:"speed"`
	Scale         float64        `json:"scale"`
	ShowAll       bool           `json:"showAll"`
	MutedKeywords []mutedKeyword `json:"mutedKeywords"`
	MutedUsers    []string       `json:"mutedUsers"`
}

const commentSettingsFields = `comments { opacity speed scale showAll mutedKeywords { text match } mutedUsers }`

// TestCommentSettings は、実況のコメントの設定を、既定値から置き換えられ、ほかの項目を変えず、範囲の外は断ることを確かめる (#488)。
func TestCommentSettings(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	get := func() commentSettings {
		var got struct {
			PlayerSettings struct {
				Comments commentSettings `json:"comments"`
			} `json:"playerSettings"`
		}
		require.Empty(t, f.query(t, `{ playerSettings { `+commentSettingsFields+` } }`, nil, &got))

		return got.PlayerSettings.Comments
	}
	type updated struct {
		UpdatePlayerSettings struct {
			CaptionsEnabled bool            `json:"captionsEnabled"`
			Comments        commentSettings `json:"comments"`
		} `json:"updatePlayerSettings"`
	}
	const mutation = `mutation($input: PlayerSettingsInput!) { updatePlayerSettings(input: $input) { captionsEnabled ` + commentSettingsFields + ` } }`
	valid := commentSettings{
		Opacity: 0.5, Speed: 1.5, Scale: 1.2, ShowAll: false,
		MutedKeywords: []mutedKeyword{{Text: "ネタバレ", Match: "PARTIAL"}, {Text: "^w+$", Match: "REGEX"}},
		MutedUsers:    []string{"a:x"},
	}

	// 一度も変えていなければ、既定値を返す。
	assert.Equal(t, commentSettings{Opacity: 1, Speed: 1, Scale: 1, ShowAll: true, MutedKeywords: []mutedKeyword{}, MutedUsers: []string{}}, get())

	var got updated
	require.Empty(t, f.query(t, mutation, map[string]any{varInput: map[string]any{"comments": valid}}, &got))
	assert.True(t, got.UpdatePlayerSettings.CaptionsEnabled, "the other settings are kept")
	assert.Equal(t, valid, got.UpdatePlayerSettings.Comments)
	assert.Equal(t, valid, get())

	// コメントの設定を省いて変えても、コメントの設定は残す。
	require.Empty(t, f.query(t, mutation, map[string]any{varInput: struct {
		CaptionsEnabled bool `json:"captionsEnabled"`
	}{CaptionsEnabled: true}}, &got))
	assert.Equal(t, valid, got.UpdatePlayerSettings.Comments)

	tests := []struct {
		name   string
		change func(s *commentSettings)
	}{
		{name: "不透明度が範囲の外", change: func(s *commentSettings) { s.Opacity = 0 }},
		{name: "速さが範囲の外", change: func(s *commentSettings) { s.Speed = 3 }},
		{name: "大きさが範囲の外", change: func(s *commentSettings) { s.Scale = 0.1 }},
		{name: "空のキーワード", change: func(s *commentSettings) { s.MutedKeywords = []mutedKeyword{{Text: "", Match: "EXACT"}} }},
		{name: "長すぎるユーザー ID", change: func(s *commentSettings) { s.MutedUsers = []string{strings.Repeat("a", 65)} }},
	}
	// 同じユーザーの設定を順に変えるので、並行に動かさない。
	for _, tt := range tests {
		input := valid
		tt.change(&input)
		var ignored struct{}
		assert.Equal(t, []string{codeBadUserInput}, f.query(t, mutation, map[string]any{varInput: map[string]any{"comments": input}}, &ignored), tt.name)
	}
	assert.Equal(t, valid, get(), "rejected changes are not saved")
}

// captionSettings は、字幕の設定 (#491)。
type captionSettings struct {
	SuperimposeLive     bool     `json:"superimposeLive"`
	SuperimposeRecorded bool     `json:"superimposeRecorded"`
	Stroke              bool     `json:"stroke"`
	BackgroundOpacity   *float64 `json:"backgroundOpacity"`
}

const captionSettingsFields = `captions { superimposeLive superimposeRecorded stroke backgroundOpacity }`

// TestCaptionSettings は、字幕の設定を、既定値から変えて残し、範囲の外の値を断ることを確かめる (#491)。
func TestCaptionSettings(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	get := func() captionSettings {
		var got struct {
			PlayerSettings struct {
				Captions captionSettings `json:"captions"`
			} `json:"playerSettings"`
		}
		require.Empty(t, f.query(t, `{ playerSettings { `+captionSettingsFields+` } }`, nil, &got))

		return got.PlayerSettings.Captions
	}
	const mutation = `mutation($input: PlayerSettingsInput!) { updatePlayerSettings(input: $input) { ` + captionSettingsFields + ` } }`

	// 一度も変えていなければ、文字スーパーをライブでも録画でも出し、見た目は放送の指定どおりにする。
	assert.Equal(t, captionSettings{SuperimposeLive: true, SuperimposeRecorded: true, Stroke: false, BackgroundOpacity: nil}, get())

	valid := captionSettings{SuperimposeLive: true, SuperimposeRecorded: false, Stroke: true, BackgroundOpacity: new(0.5)}
	var ignored struct{}
	require.Empty(t, f.query(t, mutation, map[string]any{varInput: map[string]any{"captions": valid}}, &ignored))
	assert.Equal(t, valid, get())

	// ほかの設定を変えても、字幕の設定は残す。
	require.Empty(t, f.query(t, mutation, map[string]any{varInput: struct {
		JikkyoEnabled bool `json:"jikkyoEnabled"`
	}{JikkyoEnabled: false}}, &ignored))
	assert.Equal(t, valid, get())

	tests := []struct {
		name    string
		opacity float64
	}{
		{name: "背景の不透明度が 1 を超える", opacity: 1.5},
		{name: "背景の不透明度が負", opacity: -0.1},
	}
	// 同じユーザーの設定を順に変えるので、並行に動かさない。
	for _, tt := range tests {
		invalid := valid
		invalid.BackgroundOpacity = new(tt.opacity)
		assert.Equal(t, []string{codeBadUserInput}, f.query(t, mutation, map[string]any{varInput: map[string]any{"captions": invalid}}, &ignored), tt.name)
	}
	assert.Equal(t, valid, get(), "rejected changes are not saved")
}
