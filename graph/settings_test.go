package graph_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/reserve"
)

const (
	fieldDefaults  = "recordingDefaults"
	fieldDays      = "archiveRetentionDays"
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
	var unstamped struct {
		SystemSettings struct {
			UpdatedAt *time.Time `json:"updatedAt"`
		} `json:"systemSettings"`
	}
	require.Empty(t, f.query(t, `{ systemSettings { updatedAt } }`, nil, &unstamped))
	assert.Nil(t, unstamped.SystemSettings.UpdatedAt, "never changed")
	var got struct {
		SystemSettings systemSettings `json:"systemSettings"`
	}
	viewerFixture := f.as(t, "viewer", auth.RoleViewer)
	require.Empty(t, viewerFixture.query(t, settingsQuery, nil, &got))
	assert.Equal(t, defaultSystemSettings(), got.SystemSettings)

	change := map[string]any{varInput: map[string]any{fieldDefaults: map[string]any{"startMarginSeconds": 30, "poolId": poolMain}, fieldDays: 0}}
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
		{fieldDays: -1},
		{fieldDefaults: map[string]any{"fileNameTemplate": "{{.Unknown}}"}},
		{fieldDefaults: map[string]any{"priority": 11}},
	} {
		assert.Equal(t, []string{codeBadUserInput}, admin.query(t, updateSettings, map[string]any{varInput: input}, nil), "%v", input)
	}
	require.Empty(t, f.query(t, settingsQuery, nil, &got))
	assert.Equal(t, want, got.SystemSettings)

	// 省いた項目は変えない。プールは空にすると、自動で選ぶのに戻る。
	var partial struct {
		UpdateSystemSettings systemSettings `json:"updateSystemSettings"`
	}
	require.Empty(t, admin.query(t, updateSettings, map[string]any{varInput: map[string]any{fieldDefaults: map[string]any{"poolId": ""}}}, &partial))
	want.RecordingDefaults.PoolID = nil
	assert.Equal(t, want, partial.UpdateSystemSettings)
	require.Empty(t, admin.query(t, updateSettings, map[string]any{varInput: map[string]any{fieldDays: 30}}, &partial))
	want.ArchiveRetentionDays = 30
	assert.Equal(t, want, partial.UpdateSystemSettings)

	var stamped struct {
		SystemSettings struct {
			UpdatedAt *time.Time `json:"updatedAt"`
		} `json:"systemSettings"`
	}
	require.Empty(t, f.query(t, `{ systemSettings { updatedAt } }`, nil, &stamped))
	require.NotNil(t, stamped.SystemSettings.UpdatedAt)
	assert.True(t, stamped.SystemSettings.UpdatedAt.Equal(f.clock.Now()))

	// 設定を変えるのは重要な操作なので、直前の再認証を求める。
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	assert.Equal(t, []string{codeReauthRequired}, admin.query(t, updateSettings, change, nil))
}

const fieldSortOrder = "sortOrder"

const updateChannelType = `mutation($type: String!, $input: ChannelTypeInput!) {
	updateChannelType(type: $type, input: $input) { type displayName sortOrder } }`

type channelType struct {
	Type        string `json:"type"`
	DisplayName string `json:"displayName"`
	SortOrder   int    `json:"sortOrder"`
}

// TestUpdateChannelType は、管理者だけがチャンネル種別の表示名と並び順を変えられ、変えた順に並ぶことを確かめる (設計書 3.1、11.2)。
func TestUpdateChannelType(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	list := func() []channelType {
		var got struct {
			ChannelTypes []channelType `json:"channelTypes"`
		}
		require.Empty(t, f.query(t, `{ channelTypes { type displayName sortOrder } }`, nil, &got))

		return got.ChannelTypes
	}
	before := list()
	require.NotEmpty(t, before)
	last := before[len(before)-1]

	variables := func(channel string, input map[string]any) map[string]any {
		return map[string]any{"type": channel, varInput: input}
	}
	// 最後の種別を、表示名を変えて先頭に動かす。表示名の前後の空白は落とす。
	change := variables(last.Type, map[string]any{fieldDisplay: " ちでじ ", fieldSortOrder: 0})
	assert.Equal(t, []string{codeForbidden}, f.query(t, updateChannelType, change, nil), "only administrators can change channel types")

	admin := f.as(t, adminName, auth.RoleAdmin)
	var updated struct {
		UpdateChannelType channelType `json:"updateChannelType"`
	}
	require.Empty(t, admin.query(t, updateChannelType, change, &updated))
	want := channelType{Type: last.Type, DisplayName: "ちでじ", SortOrder: 0}
	assert.Equal(t, want, updated.UpdateChannelType)
	assert.Equal(t, want, list()[0])

	// 省いた項目は変えない。
	require.Empty(t, admin.query(t, updateChannelType, variables(last.Type, map[string]any{fieldSortOrder: 10000}), &updated))
	want.SortOrder = 10000
	assert.Equal(t, want, updated.UpdateChannelType)

	var action string
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT action FROM audit_events WHERE target_type = 'channel_type' AND target_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT 1", last.Type).Scan(&action))
	assert.Equal(t, "channel_type.updated", action)

	// 表示を変えるのは管理者の設定の変更なので、直前の再認証を求める。
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	assert.Equal(t, []string{codeReauthRequired}, admin.query(t, updateChannelType, change, nil))
}

// TestUpdateChannelTypeRejectsInvalidInput は、正しくない値と、ない種別を拒み、保存しないことを確かめる。
func TestUpdateChannelTypeRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	admin := f.as(t, adminName, auth.RoleAdmin)
	const query = `{ channelTypes { type displayName sortOrder } }`
	var before struct {
		ChannelTypes []channelType `json:"channelTypes"`
	}
	require.Empty(t, f.query(t, query, nil, &before))
	require.NotEmpty(t, before.ChannelTypes)
	target := before.ChannelTypes[0].Type

	tests := []struct {
		name    string
		channel string
		input   map[string]any
		want    string
	}{
		{name: "空の表示名", channel: target, input: map[string]any{fieldDisplay: " "}, want: codeBadUserInput},
		{name: "長すぎる表示名", channel: target, input: map[string]any{fieldDisplay: strings.Repeat("あ", 33)}, want: codeBadUserInput},
		{name: "負の並び順", channel: target, input: map[string]any{fieldSortOrder: -1}, want: codeBadUserInput},
		{name: "上限を超える並び順", channel: target, input: map[string]any{fieldSortOrder: 10001}, want: codeBadUserInput},
		{name: "ない種別", channel: "UNKNOWN", input: map[string]any{fieldSortOrder: 1}, want: codeNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			variables := map[string]any{"type": tt.channel, varInput: tt.input}
			assert.Equal(t, []string{tt.want}, admin.query(t, updateChannelType, variables, nil))
			var after struct {
				ChannelTypes []channelType `json:"channelTypes"`
			}
			require.Empty(t, f.query(t, query, nil, &after))
			assert.Equal(t, before.ChannelTypes, after.ChannelTypes, "rejected changes are not saved")
		})
	}
}

const (
	poolMain     = "main"
	poolSub      = "sub"
	mountMain    = "/mnt/main"
	mountSub     = "/mnt/sub"
	fieldMinFree = "minFreeBytes"
)

const updateStoragePool = `mutation($id: String!, $input: StoragePoolInput!) {
	updateStoragePool(id: $id, input: $input) { id path priority minFreeBytes freeBytes } }`

type storagePool struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	Priority     int    `json:"priority"`
	MinFreeBytes int    `json:"minFreeBytes"`
	FreeBytes    *int   `json:"freeBytes"`
}

// TestUpdateStoragePool は、管理者だけが保存先プールの優先順位と容量の閾値を変えられ、システムの状態で読めることを確かめる (設計書 5.6、6.1)。
func TestUpdateStoragePool(t *testing.T) {
	t.Parallel()

	f := newFixture(t, &graph.Options{
		Pools:     map[string]string{poolMain: mountMain, poolSub: mountSub},
		FreeSpace: func(path string) (uint64, error) { return map[string]uint64{mountMain: 100 << 30}[path], nil },
	})
	pools := func(user *fixture) []storagePool {
		var got struct {
			SystemStatus struct {
				Pools []storagePool `json:"pools"`
			} `json:"systemStatus"`
		}
		require.Empty(t, user.query(t, `{ systemStatus { pools { id path priority minFreeBytes freeBytes } } }`, nil, &got))

		return got.SystemStatus.Pools
	}
	admin := f.as(t, adminName, auth.RoleAdmin)
	// 設定していないプールは、優先順位 0、容量の閾値 0 で、録画を消さない。
	assert.Equal(t, []storagePool{
		{ID: poolMain, Path: mountMain, Priority: 0, MinFreeBytes: 0, FreeBytes: new(100 << 30)},
		{ID: poolSub, Path: mountSub, Priority: 0, MinFreeBytes: 0, FreeBytes: new(0)},
	}, pools(admin))

	change := map[string]any{"id": poolMain, varInput: map[string]any{varPriority: 10, fieldMinFree: 50 << 30}}
	assert.Equal(t, []string{codeForbidden}, f.query(t, updateStoragePool, change, nil), "only administrators can change storage pools")
	var updated struct {
		UpdateStoragePool storagePool `json:"updateStoragePool"`
	}
	require.Empty(t, admin.query(t, updateStoragePool, change, &updated))
	want := storagePool{ID: poolMain, Path: mountMain, Priority: 10, MinFreeBytes: 50 << 30, FreeBytes: new(100 << 30)}
	assert.Equal(t, want, updated.UpdateStoragePool)
	assert.Equal(t, want, pools(admin)[0])

	// 省いた項目は変えない。
	require.Empty(t, admin.query(t, updateStoragePool, map[string]any{"id": poolMain, varInput: map[string]any{fieldMinFree: 0}}, &updated))
	want.MinFreeBytes = 0
	assert.Equal(t, want, updated.UpdateStoragePool)

	var action string
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT action FROM audit_events WHERE target_type = 'storage_pool' AND target_id = 'main' ORDER BY occurred_at DESC, id DESC LIMIT 1").Scan(&action))
	assert.Equal(t, "storage_pool.updated", action)

	tests := []struct {
		name  string
		id    string
		input map[string]any
		want  string
	}{
		{name: "マウントされていないプール", id: "elsewhere", input: map[string]any{varPriority: 1}, want: codeNotFound},
		{name: "優先順位が範囲の外", id: poolMain, input: map[string]any{varPriority: 1001}, want: codeBadUserInput},
		{name: "負の容量の閾値", id: poolMain, input: map[string]any{fieldMinFree: -1}, want: codeBadUserInput},
	}
	for _, tt := range tests {
		assert.Equal(t, []string{tt.want}, admin.query(t, updateStoragePool, map[string]any{"id": tt.id, varInput: tt.input}, nil), tt.name)
	}
	assert.Equal(t, want, pools(admin)[0], "rejected changes are not saved")

	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	assert.Equal(t, []string{codeReauthRequired}, admin.query(t, updateStoragePool, change, nil))
}
