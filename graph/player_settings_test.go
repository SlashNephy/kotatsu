package graph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usersettings"
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
