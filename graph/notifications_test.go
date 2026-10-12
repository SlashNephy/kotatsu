package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/notification"
)

const (
	listNotifications = `query($first: Int!, $after: String, $unreadOnly: Boolean!) {
		notifications(first: $first, after: $after, unreadOnly: $unreadOnly) {
			edges { node { id kind payload message readAt } } pageInfo { hasNextPage endCursor } } }`
	unreadCount         = `{ unreadNotificationCount }`
	markRead            = `mutation($ids: [ID!]!) { markNotificationsRead(ids: $ids) }`
	markAllRead         = `mutation { markAllNotificationsRead }`
	listPreferences     = `{ notificationPreferences { kind inApp webPush } }`
	updatePreference    = `mutation($input: NotificationPreferenceInput!) { updateNotificationPreference(input: $input) { kind inApp webPush } }`
	kindStorageLow      = "STORAGE_LOW"
	kindRecordingFailed = "RECORDING_FAILED"
	kindRecordingDone   = "RECORDING_COMPLETED"
	kindRecordingBegan  = "RECORDING_STARTED"
	varIDs              = "ids"
	varUnreadOnly       = "unreadOnly"
)

type notificationNode struct {
	ID      string     `json:"id"`
	Kind    string     `json:"kind"`
	Payload string     `json:"payload"`
	Message string     `json:"message"`
	ReadAt  *time.Time `json:"readAt"`
}

type notificationPage struct {
	Notifications struct {
		Edges []struct {
			Node notificationNode `json:"node"`
		} `json:"edges"`
		PageInfo struct {
			HasNextPage bool    `json:"hasNextPage"`
			EndCursor   *string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"notifications"`
}

func (p *notificationPage) nodes() []notificationNode {
	nodes := make([]notificationNode, 0, len(p.Notifications.Edges))
	for _, edge := range p.Notifications.Edges {
		nodes = append(nodes, edge.Node)
	}

	return nodes
}

func (p *notificationPage) kinds() []string {
	kinds := make([]string, 0, len(p.Notifications.Edges))
	for _, node := range p.nodes() {
		kinds = append(kinds, node.Kind)
	}

	return kinds
}

// notify は、イベントを記録して、受け取るユーザーに配る。作った日時で並ぶよう、配るたびに時計を進める。
func (f *fixture) notify(t *testing.T, kind notification.Kind, payload map[string]any) {
	t.Helper()

	require.NoError(t, notification.Emit(t.Context(), f.pool, f.clock, kind, payload))
	_, err := notification.NewDispatcher(f.pool, f.clock).DispatchOnce(t.Context())
	require.NoError(t, err)
	f.clock.Advance(time.Second)
}

func (f *fixture) notifications(t *testing.T, first int, after *string, unreadOnly bool) *notificationPage {
	t.Helper()

	var page notificationPage
	require.Empty(t, f.query(t, listNotifications, map[string]any{varFirst: first, varAfter: after, varUnreadOnly: unreadOnly}, &page))

	return &page
}

func (f *fixture) unreadCount(t *testing.T) int {
	t.Helper()

	var got struct {
		UnreadNotificationCount int `json:"unreadNotificationCount"`
	}
	require.Empty(t, f.query(t, unreadCount, nil, &got))

	return got.UnreadNotificationCount
}

// TestNotifications は、ログイン中のユーザーが自分の通知を読み、既読にできることを確かめる (設計書 6.2)。
func TestNotifications(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	// 視聴者も通知を受け取る。
	other := f.as(t, "viewer", auth.RoleViewer)
	f.notify(t, notification.KindRecordingStarted, map[string]any{varName: tagKotatsu})
	f.notify(t, notification.KindRecordingCompleted, nil)
	f.notify(t, notification.KindRecordingFailed, nil)

	all := f.notifications(t, 10, nil, false)
	assert.Equal(t, []string{kindRecordingFailed, kindRecordingDone, kindRecordingBegan}, all.kinds())
	assert.JSONEq(t, `{"name": "`+tagKotatsu+`"}`, all.nodes()[2].Payload)
	assert.Equal(t, 3, f.unreadCount(t))

	// 2 件ずつページを送る。
	first := f.notifications(t, 2, nil, false)
	assert.True(t, first.Notifications.PageInfo.HasNextPage)
	next := f.notifications(t, 2, first.Notifications.PageInfo.EndCursor, false)
	assert.Equal(t, []string{kindRecordingBegan}, next.kinds())
	assert.False(t, next.Notifications.PageInfo.HasNextPage)

	// ほかのユーザーの通知は既読にできない。
	others := other.notifications(t, 1, nil, false)
	var marked struct {
		MarkNotificationsRead int `json:"markNotificationsRead"`
	}
	require.Empty(t, f.query(t, markRead, map[string]any{varIDs: []string{all.nodes()[0].ID, others.nodes()[0].ID}}, &marked))
	assert.Equal(t, 1, marked.MarkNotificationsRead)
	unread := f.notifications(t, 10, nil, true)
	assert.Equal(t, []string{kindRecordingDone, kindRecordingBegan}, unread.kinds())
	assert.NotNil(t, f.notifications(t, 1, nil, false).nodes()[0].ReadAt)

	var markedAll struct {
		MarkAllNotificationsRead int `json:"markAllNotificationsRead"`
	}
	require.Empty(t, f.query(t, markAllRead, nil, &markedAll))
	assert.Equal(t, 2, markedAll.MarkAllNotificationsRead)
	assert.Zero(t, f.unreadCount(t))
	assert.Equal(t, 3, other.unreadCount(t), "the other user's notifications stay unread")
}

func TestNotificationsRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	tests := []struct {
		name      string
		query     string
		variables map[string]any
	}{
		{name: "first が 0", query: listNotifications, variables: map[string]any{varFirst: 0, varUnreadOnly: false}},
		{name: "first が上限を超える", query: listNotifications, variables: map[string]any{varFirst: 201, varUnreadOnly: false}},
		{name: "壊れたカーソル", query: listNotifications, variables: map[string]any{varFirst: 1, varAfter: "broken", varUnreadOnly: false}},
		{name: "通知でない ID", query: markRead, variables: map[string]any{varIDs: []string{graph.GlobalID("User", f.userID.String())}}},
		{name: "既読にする数が上限を超える", query: markRead, variables: map[string]any{varIDs: make([]string, 201)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []string{codeBadUserInput}, f.query(t, tt.query, tt.variables, nil))
		})
	}
}

// TestNotificationPreferences は、ユーザーが種類ごとの通知設定を変え、画面内の通知を切った種類は届かないことを確かめる。
func TestNotificationPreferences(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	type preference struct {
		Kind    string `json:"kind"`
		InApp   bool   `json:"inApp"`
		WebPush bool   `json:"webPush"`
	}
	list := func() []preference {
		var got struct {
			NotificationPreferences []preference `json:"notificationPreferences"`
		}
		require.Empty(t, f.query(t, listPreferences, nil, &got))

		return got.NotificationPreferences
	}
	want := []preference{
		{Kind: kindRecordingBegan, InApp: true, WebPush: true},
		{Kind: kindRecordingDone, InApp: true, WebPush: true},
		{Kind: kindRecordingFailed, InApp: true, WebPush: true},
		{Kind: "RESERVATION_CONFLICT", InApp: true, WebPush: true},
		{Kind: kindStorageLow, InApp: true, WebPush: true},
		{Kind: "TUNER_SERVER_UNREACHABLE", InApp: true, WebPush: true},
	}
	assert.Equal(t, want, list())

	var updated struct {
		UpdateNotificationPreference preference `json:"updateNotificationPreference"`
	}
	input := map[string]any{"kind": kindStorageLow, "inApp": false, "webPush": true}
	require.Empty(t, f.query(t, updatePreference, map[string]any{varInput: input}, &updated))
	assert.Equal(t, preference{Kind: kindStorageLow, InApp: false, WebPush: true}, updated.UpdateNotificationPreference)
	want[4].InApp = false
	assert.Equal(t, want, list())

	f.notify(t, notification.KindStorageLow, nil)
	f.notify(t, notification.KindRecordingFailed, nil)
	assert.Equal(t, []string{kindRecordingFailed}, f.notifications(t, 10, nil, false).kinds())
}

// TestNotificationMessage は、通知に、Web Push と同じ文を、表示のタイムゾーンで付けることを確かめる。
func TestNotificationMessage(t *testing.T) {
	t.Parallel()

	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	tests := []struct {
		name    string
		kind    notification.Kind
		payload map[string]any
		want    string
	}{
		{name: "番組名を添える", kind: notification.KindRecordingStarted, payload: map[string]any{varName: tagKotatsu}, want: "「" + tagKotatsu + "」の録画を始めました"},
		{
			name: "競合した予約の日時を、表示のタイムゾーンで書く", kind: notification.KindReservationConflict,
			payload: map[string]any{"startAt": "2026-10-10T12:00:00Z", "serviceName": "NHK総合", varName: tagKotatsu},
			want:    "チューナーが足りず、録画できない見込みの予約があります。10/10（土）21:00 NHK総合「" + tagKotatsu + "」",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, &graph.Options{Location: tokyo})
			f.notify(t, tt.kind, tt.payload)
			nodes := f.notifications(t, 1, nil, false).nodes()
			require.Len(t, nodes, 1)
			assert.Equal(t, tt.want, nodes[0].Message)
		})
	}
}
