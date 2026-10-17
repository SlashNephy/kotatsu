package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/usersettings"
)

const (
	setWatchReservation = `mutation($program: ID!, $reserved: Boolean!) {
		setWatchReservation(program: $program, reserved: $reserved) { eventId watchReserved }
	}`
	varReserved = "reserved"
)

type watchReservedProgram struct {
	EventID       int  `json:"eventId"`
	WatchReserved bool `json:"watchReserved"`
}

type watchReservationResult struct {
	Program struct {
		EventID int `json:"eventId"`
	} `json:"program"`
	NotifyAt time.Time `json:"notifyAt"`
}

// setWatch は、eventId の番組の視聴予約を付け外しし、エラーコードを返す。
func (f *fixture) setWatch(t *testing.T, eventID int, reserved bool) (watchReservedProgram, []string) {
	t.Helper()

	var got struct {
		SetWatchReservation watchReservedProgram `json:"setWatchReservation"`
	}
	codes := f.query(t, setWatchReservation, map[string]any{varProgram: programID(eventID), varReserved: reserved}, &got)

	return got.SetWatchReservation, codes
}

func (f *fixture) watchReservations(t *testing.T) []watchReservationResult {
	t.Helper()

	var got struct {
		WatchReservations []watchReservationResult `json:"watchReservations"`
	}
	require.Empty(t, f.query(t, `{ watchReservations { program { eventId } notifyAt } }`, nil, &got))

	return got.WatchReservations
}

func (f *fixture) programWatchReserved(t *testing.T, eventID int) bool {
	t.Helper()

	var got struct {
		Node watchReservedProgram `json:"node"`
	}
	require.Empty(t, f.query(t, `query($id: ID!) { node(id: $id) { ... on Program { eventId watchReserved } } }`,
		map[string]any{"id": programID(eventID)}, &got))

	return got.Node.WatchReserved
}

// TestWatchReservations は、視聴予約を付け外しでき、一覧に放送の早い順に出て、ほかの端末に知らせることを確かめる (設計書 4.8)。
func TestWatchReservations(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	version := func() int64 {
		v, err := usersettings.Version(t.Context(), f.pool, f.userID)
		require.NoError(t, err)

		return v
	}

	// 偽チューナーの EPG は 11:00 から 30 分ごとで、今は 12:10。eventId 5 は 13:00、6 は 13:30 に始まる。
	assert.False(t, f.programWatchReserved(t, 5))
	before := version()
	for _, eventID := range []int{6, 5} {
		got, codes := f.setWatch(t, eventID, true)
		require.Empty(t, codes)
		assert.Equal(t, watchReservedProgram{EventID: eventID, WatchReserved: true}, got)
	}
	assert.Greater(t, version(), before, "the other devices are told to reload")
	assert.True(t, f.programWatchReserved(t, 5))
	// 付けてある番組にもう一度付けても、そのまま返す。
	got, codes := f.setWatch(t, 5, true)
	require.Empty(t, codes)
	assert.True(t, got.WatchReserved)

	// 放送の早い順に並び、既定では放送の 5 分前に通知する。
	list := f.watchReservations(t)
	require.Len(t, list, 2)
	assert.Equal(t, []int{5, 6}, []int{list[0].Program.EventID, list[1].Program.EventID})
	assert.Equal(t, time.Date(2026, 10, 10, 12, 55, 0, 0, time.UTC), list[0].NotifyAt)

	// ほかのユーザーの視聴予約は見えない。
	assert.Empty(t, f.as(t, "other", auth.RoleMember).watchReservations(t))

	// 外すと一覧から消える。付けていない番組から外しても、そのまま返す。
	for range 2 {
		got, codes := f.setWatch(t, 6, false)
		require.Empty(t, codes)
		assert.Equal(t, watchReservedProgram{EventID: 6, WatchReserved: false}, got)
	}
	list = f.watchReservations(t)
	require.Len(t, list, 1)
	assert.Equal(t, 5, list[0].Program.EventID)
}

// TestSetWatchReservationRejects は、視聴予約を付けられない番組を BAD_USER_INPUT で拒むことを確かめる。
func TestSetWatchReservationRejects(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)

	tests := []struct {
		name    string
		eventID int
	}{
		// 12:00 から 12:30 の番組は、12:10 の今、放送中。
		{name: "放送中の番組", eventID: 3},
		{name: "放送が終わった番組", eventID: 1},
		{name: "EPG にない番組", eventID: 9999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, codes := f.setWatch(t, tt.eventID, true)
			assert.Equal(t, []string{codeBadUserInput}, codes)
		})
	}
}

// TestWatchReservationsForViewers は、閲覧だけのユーザーも、視聴予約を使えることを確かめる。録画の予約と違い、チューナーを使わないため。
func TestWatchReservationsForViewers(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	v := f.as(t, "viewer", auth.RoleViewer)

	got, codes := v.setWatch(t, 5, true)
	require.Empty(t, codes)
	assert.True(t, got.WatchReserved)
	assert.Len(t, v.watchReservations(t), 1)
}

// TestWatchReminderSettings は、通知を放送の何分前に送るかを 0 から 60 分の範囲で変えられ、通知の時刻に反映されることを確かめる。
func TestWatchReminderSettings(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	_, codes := f.setWatch(t, 5, true)
	require.Empty(t, codes)

	var got struct {
		WatchReminderSettings struct {
			MinutesBefore int `json:"minutesBefore"`
		} `json:"watchReminderSettings"`
	}
	require.Empty(t, f.query(t, `{ watchReminderSettings { minutesBefore } }`, nil, &got))
	assert.Equal(t, 5, got.WatchReminderSettings.MinutesBefore)

	tests := []struct {
		name      string
		minutes   int
		wantCodes []string
		// wantNotifyAt は、13:00 に始まる番組の通知の時刻。
		wantNotifyAt time.Time
	}{
		{name: "30 分前", minutes: 30, wantCodes: []string{}, wantNotifyAt: time.Date(2026, 10, 10, 12, 30, 0, 0, time.UTC)},
		{name: "放送の開始と同時", minutes: 0, wantCodes: []string{}, wantNotifyAt: time.Date(2026, 10, 10, 13, 0, 0, 0, time.UTC)},
		{name: "60 分前", minutes: 60, wantCodes: []string{}, wantNotifyAt: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)},
		// 範囲の外は拒み、前の設定のままにする。
		{name: "61 分前は拒む", minutes: 61, wantCodes: []string{codeBadUserInput}, wantNotifyAt: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)},
		{name: "負の値は拒む", minutes: -1, wantCodes: []string{codeBadUserInput}, wantNotifyAt: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)},
	}
	// 順に変えていくので、並行に動かさない。
	for _, tt := range tests {
		codes := f.query(t, `mutation($minutes: Int!) { updateWatchReminderSettings(minutesBefore: $minutes) { minutesBefore } }`,
			map[string]any{"minutes": tt.minutes}, nil)
		assert.Equal(t, tt.wantCodes, codes, tt.name)
		list := f.watchReservations(t)
		require.Len(t, list, 1, tt.name)
		assert.Equal(t, tt.wantNotifyAt, list[0].NotifyAt, tt.name)
	}
}

// TestWatchReservedInGuide は、番組表で多くの番組をまとめて引いても、ログイン中のユーザーが視聴予約した番組だけが印付きになることを確かめる。
func TestWatchReservedInGuide(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	for _, eventID := range []int{5, 7} {
		_, codes := f.setWatch(t, eventID, true)
		require.Empty(t, codes)
	}
	// ほかのユーザーの視聴予約は、印にしない。
	_, codes := f.as(t, "other", auth.RoleMember).setWatch(t, 6, true)
	require.Empty(t, codes)

	var guide struct {
		Guide []struct {
			Service struct {
				Name string `json:"name"`
			} `json:"service"`
			Programs []watchReservedProgram `json:"programs"`
		} `json:"guide"`
	}
	require.Empty(t, f.query(t, `{ guide(from: "2026-10-10T12:00:00Z", to: "2026-10-10T15:00:00Z") {
		service { name } programs { eventId watchReserved }
	} }`, nil, &guide))
	reserved := map[string][]int{}
	programs := 0
	for _, row := range guide.Guide {
		for _, p := range row.Programs {
			programs++
			if p.WatchReserved {
				reserved[row.Service.Name] = append(reserved[row.Service.Name], p.EventID)
			}
		}
	}
	assert.Greater(t, programs, 10, "the guide loads many programs at once")
	assert.Equal(t, map[string][]int{"こたつテスト1": {5, 7}}, reserved)
}
