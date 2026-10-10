package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/reserve"
)

const (
	fieldDefaults  = "recordingDefaults"
	settingsQuery  = `{ systemSettings { recordingDefaults { poolId fileNameTemplate startMarginSeconds endMarginSeconds priority } archiveRetentionDays } }`
	updateSettings = `mutation($input: SystemSettingsInput!) { updateSystemSettings(input: $input) {
		recordingDefaults { poolId fileNameTemplate startMarginSeconds endMarginSeconds priority } archiveRetentionDays } }`
)

type recordingSettings struct {
	PoolID             *string `json:"poolId"`
	FileNameTemplate   string  `json:"fileNameTemplate"`
	StartMarginSeconds int     `json:"startMarginSeconds"`
	EndMarginSeconds   int     `json:"endMarginSeconds"`
	Priority           int     `json:"priority"`
}

type systemSettings struct {
	RecordingDefaults    recordingSettings `json:"recordingDefaults"`
	ArchiveRetentionDays int               `json:"archiveRetentionDays"`
}

func defaultSystemSettings() systemSettings {
	return systemSettings{
		RecordingDefaults: recordingSettings{
			PoolID: nil, FileNameTemplate: reserve.DefaultFileNameTemplate,
			StartMarginSeconds: reserve.DefaultMarginSeconds, EndMarginSeconds: reserve.DefaultMarginSeconds, Priority: reserve.DefaultPriority,
		},
		ArchiveRetentionDays: 90,
	}
}

// TestSystemSettings は、誰でも全体の設定を読め、管理者だけが変えられることを確かめる (設計書 9.7、11.2)。
func TestSystemSettings(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	var got struct {
		SystemSettings systemSettings `json:"systemSettings"`
	}
	viewerFixture := f.as(t, "viewer", auth.RoleViewer)
	require.Empty(t, viewerFixture.query(t, settingsQuery, nil, &got))
	assert.Equal(t, defaultSystemSettings(), got.SystemSettings)

	change := map[string]any{varInput: map[string]any{fieldDefaults: map[string]any{"startMarginSeconds": 30, "poolId": "main"}, "archiveRetentionDays": 0}}
	for _, user := range []*fixture{f, viewerFixture} {
		assert.Equal(t, []string{codeForbidden}, user.query(t, updateSettings, change, nil), "only administrators can change the settings")
	}

	admin := f.as(t, adminName, auth.RoleAdmin)
	var updated struct {
		UpdateSystemSettings systemSettings `json:"updateSystemSettings"`
	}
	require.Empty(t, admin.query(t, updateSettings, change, &updated))
	want := defaultSystemSettings()
	want.RecordingDefaults.StartMarginSeconds = 30
	want.RecordingDefaults.PoolID = new("main")
	want.ArchiveRetentionDays = 0
	assert.Equal(t, want, updated.UpdateSystemSettings)
	require.Empty(t, f.query(t, settingsQuery, nil, &got))
	assert.Equal(t, want, got.SystemSettings, "members read the changed settings")

	// 正しくない値は、BAD_USER_INPUT を返し、保存しない。
	for _, input := range []map[string]any{
		{"archiveRetentionDays": -1},
		{fieldDefaults: map[string]any{"fileNameTemplate": "{{.Unknown}}"}},
		{fieldDefaults: map[string]any{"priority": 11}},
	} {
		assert.Equal(t, []string{codeBadUserInput}, admin.query(t, updateSettings, map[string]any{varInput: input}, nil), "%v", input)
	}
	require.Empty(t, f.query(t, settingsQuery, nil, &got))
	assert.Equal(t, want, got.SystemSettings)

	// 設定を変えるのは重要な操作なので、直前の再認証を求める。
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	assert.Equal(t, []string{codeReauthRequired}, admin.query(t, updateSettings, change, nil))
}
