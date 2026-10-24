package graph_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	fieldDefaults = "recordingDefaults"
	fieldDays     = "archiveRetentionDays"
	// fieldNotificationDays は、通知の保持期間の項目 (#551)。
	fieldNotificationDays = "notificationRetentionDays"
	settingsQuery         = `{ systemSettings { recordingDefaults { poolId fileNameTemplate startMarginSeconds endMarginSeconds priority } archiveRetentionDays notificationRetentionDays mcpEnabled } }`
	updateSettings        = `mutation($input: SystemSettingsInput!) { updateSystemSettings(input: $input) {
		recordingDefaults { poolId fileNameTemplate startMarginSeconds endMarginSeconds priority } archiveRetentionDays notificationRetentionDays mcpEnabled } }`
)

type recordingSettings struct {
	PoolID             *string `json:"poolId"`
	FileNameTemplate   string  `json:"fileNameTemplate"`
	StartMarginSeconds int     `json:"startMarginSeconds"`
	EndMarginSeconds   int     `json:"endMarginSeconds"`
	Priority           int     `json:"priority"`
}

type systemSettings struct {
	RecordingDefaults         recordingSettings `json:"recordingDefaults"`
	ArchiveRetentionDays      int               `json:"archiveRetentionDays"`
	NotificationRetentionDays int               `json:"notificationRetentionDays"`
	MCPEnabled                bool              `json:"mcpEnabled"`
}

func defaultSystemSettings() systemSettings {
	return systemSettings{
		RecordingDefaults: recordingSettings{
			PoolID: nil, FileNameTemplate: reserve.DefaultFileNameTemplate,
			StartMarginSeconds: reserve.DefaultMarginSeconds, EndMarginSeconds: reserve.DefaultMarginSeconds, Priority: reserve.DefaultPriority,
		},
		ArchiveRetentionDays:      90,
		NotificationRetentionDays: 90,
		MCPEnabled:                false,
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
		{fieldNotificationDays: -1},
		{fieldNotificationDays: 3651},
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
	// 通知の保持期間も変えられ、ほかの項目は変えない (#551)。
	require.Empty(t, admin.query(t, updateSettings, map[string]any{varInput: map[string]any{fieldNotificationDays: 30}}, &partial))
	want.NotificationRetentionDays = 30
	assert.Equal(t, want, partial.UpdateSystemSettings)
	// MCP は、既定では使わせず、管理者が有効にする (#963)。
	require.Empty(t, admin.query(t, updateSettings, map[string]any{varInput: map[string]any{"mcpEnabled": true}}, &partial))
	want.MCPEnabled = true
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
	updateStoragePool(id: $id, input: $input) { id path priority minFreeBytes freeBytes totalBytes } }`

type storagePool struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	Priority     int    `json:"priority"`
	MinFreeBytes int    `json:"minFreeBytes"`
	FreeBytes    *int   `json:"freeBytes"`
	TotalBytes   *int   `json:"totalBytes"`
}

// TestUpdateStoragePool は、管理者だけが保存先プールの優先順位と容量の閾値を変えられ、システムの状態で読めることを確かめる (設計書 5.6、6.1)。
func TestUpdateStoragePool(t *testing.T) {
	t.Parallel()

	f := newFixture(t, &graph.Options{
		Pools:     map[string]string{poolMain: mountMain, poolSub: mountSub},
		FreeSpace: func(path string) (uint64, error) { return map[string]uint64{mountMain: 100 << 30}[path], nil },
		// sub の容量は調べられない。
		TotalSpace: func(path string) (uint64, error) {
			if path != mountMain {
				return 0, errors.ErrUnsupported
			}

			return 400 << 30, nil
		},
	})
	pools := func(user *fixture) []storagePool {
		var got struct {
			SystemStatus struct {
				Pools []storagePool `json:"pools"`
			} `json:"systemStatus"`
		}
		require.Empty(t, user.query(t, `{ systemStatus { pools { id path priority minFreeBytes freeBytes totalBytes } } }`, nil, &got))

		return got.SystemStatus.Pools
	}
	admin := f.as(t, adminName, auth.RoleAdmin)
	// 設定していないプールは、優先順位 0、容量の閾値 0 で、録画を消さない。
	assert.Equal(t, []storagePool{
		{ID: poolMain, Path: mountMain, Priority: 0, MinFreeBytes: 0, FreeBytes: new(100 << 30), TotalBytes: new(400 << 30)},
		{ID: poolSub, Path: mountSub, Priority: 0, MinFreeBytes: 0, FreeBytes: new(0), TotalBytes: nil},
	}, pools(admin))

	change := map[string]any{"id": poolMain, varInput: map[string]any{varPriority: 10, fieldMinFree: 50 << 30}}
	assert.Equal(t, []string{codeForbidden}, f.query(t, updateStoragePool, change, nil), "only administrators can change storage pools")
	var updated struct {
		UpdateStoragePool storagePool `json:"updateStoragePool"`
	}
	require.Empty(t, admin.query(t, updateStoragePool, change, &updated))
	want := storagePool{ID: poolMain, Path: mountMain, Priority: 10, MinFreeBytes: 50 << 30, FreeBytes: new(100 << 30), TotalBytes: new(400 << 30)}
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

// fieldHidden は、ServiceInput の非表示の印。
const fieldHidden = "hidden"

const updateService = `mutation($id: ID!, $input: ServiceInput!) { updateService(id: $id, input: $input) { id name hidden sortOrder } }`

type serviceRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Hidden    bool   `json:"hidden"`
	SortOrder *int   `json:"sortOrder"`
}

// TestUpdateService は、管理者だけがサービスの非表示の印と並び順の上書きを変えられ、EPG の同期で上書きされないことを確かめる (#309)。
func TestUpdateService(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	list := func(includeHidden bool) []serviceRow {
		var got struct {
			Services []serviceRow `json:"services"`
		}
		require.Empty(t, f.query(t, `query($all: Boolean!) { services(includeHidden: $all) { id name hidden sortOrder } }`,
			map[string]any{"all": includeHidden}, &got))

		return got.Services
	}
	before := list(false)
	require.Greater(t, len(before), 1)
	// 並びは、チャンネル種別の順が先に来るので、先頭の種別の中で、最後のサービスを動かす。
	var types struct {
		Services []struct {
			ChannelType struct {
				Type string `json:"type"`
			} `json:"channelType"`
		} `json:"services"`
	}
	require.Empty(t, f.query(t, `{ services { channelType { type } } }`, nil, &types))
	lastIndex := 0
	for i, service := range types.Services {
		if service.ChannelType.Type == types.Services[0].ChannelType.Type {
			lastIndex = i
		}
	}
	require.Positive(t, lastIndex)
	last := before[lastIndex]
	variables := func(input map[string]any) map[string]any {
		return map[string]any{"id": last.ID, varInput: input}
	}

	// 最後のサービスを、並び順を上書きして先頭に動かす。
	change := variables(map[string]any{fieldSortOrder: 0})
	assert.Equal(t, []string{codeForbidden}, f.query(t, updateService, change, nil), "only administrators can change services")
	admin := f.as(t, adminName, auth.RoleAdmin)
	var updated struct {
		UpdateService serviceRow `json:"updateService"`
	}
	require.Empty(t, admin.query(t, updateService, change, &updated))
	assert.Equal(t, serviceRow{ID: last.ID, Name: last.Name, Hidden: false, SortOrder: new(0)}, updated.UpdateService)
	assert.Equal(t, last.ID, list(false)[0].ID)

	// 非表示にすると、番組表のサービスから外れる。省いた並び順は変えない。
	require.Empty(t, admin.query(t, updateService, variables(map[string]any{fieldHidden: true}), &updated))
	assert.Equal(t, serviceRow{ID: last.ID, Name: last.Name, Hidden: true, SortOrder: new(0)}, updated.UpdateService)
	assert.NotContains(t, list(false), updated.UpdateService)
	assert.Contains(t, list(true), updated.UpdateService)

	// EPG を同期し直しても、管理者が変えた値は残る。
	f.syncEPG(t)
	assert.Contains(t, list(true), updated.UpdateService)

	// 並び順の上書きを消すと、元の並びに戻る。
	require.Empty(t, admin.query(t, updateService, variables(map[string]any{fieldHidden: false, "resetSortOrder": true}), &updated))
	assert.Equal(t, last, updated.UpdateService)
	assert.Equal(t, before, list(false))

	var action string
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT action FROM audit_events WHERE target_type = 'service' ORDER BY occurred_at DESC, id DESC LIMIT 1").Scan(&action))
	assert.Equal(t, "service.updated", action)

	// 表示を変えるのは管理者の設定の変更なので、直前の再認証を求める。
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	assert.Equal(t, []string{codeReauthRequired}, admin.query(t, updateService, change, nil))
}

// TestUpdateServiceRejectsInvalidInput は、正しくない値と、ないサービスを拒み、保存しないことを確かめる。
func TestUpdateServiceRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	admin := f.as(t, adminName, auth.RoleAdmin)
	const query = `{ services(includeHidden: true) { id name hidden sortOrder } }`
	var before struct {
		Services []serviceRow `json:"services"`
	}
	require.Empty(t, f.query(t, query, nil, &before))
	require.NotEmpty(t, before.Services)
	target := before.Services[0].ID

	tests := []struct {
		name  string
		id    string
		input map[string]any
		want  string
	}{
		{name: "負の並び順", id: target, input: map[string]any{fieldSortOrder: -1}, want: codeBadUserInput},
		{name: "上限を超える並び順", id: target, input: map[string]any{fieldSortOrder: 10001}, want: codeBadUserInput},
		{name: "並び順の上書きと消去を一緒に指定する", id: target, input: map[string]any{fieldSortOrder: 1, "resetSortOrder": true}, want: codeBadUserInput},
		{name: "ないサービス", id: graph.GlobalID("Service", "999999"), input: map[string]any{fieldHidden: true}, want: codeNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []string{tt.want}, admin.query(t, updateService, map[string]any{"id": tt.id, varInput: tt.input}, nil))
			var after struct {
				Services []serviceRow `json:"services"`
			}
			require.Empty(t, f.query(t, query, nil, &after))
			assert.Equal(t, before.Services, after.Services, "rejected changes are not saved")
		})
	}
}
