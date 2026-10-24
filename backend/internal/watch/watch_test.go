package watch_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/watch"
)

const (
	grNetworkID = 0x7FE0
	grServiceID = 0x0400
)

// testNow は、偽チューナーの EPG を取り込む時刻。番組は 11:00 から 30 分ごとで、eventId 4 は 12:30、5 は 13:00 に始まる。
func testNow() time.Time {
	return time.Date(2026, 10, 10, 12, 10, 0, 0, time.UTC)
}

func at(hour, minute int) time.Time {
	return time.Date(2026, 10, 10, hour, minute, 0, 0, time.UTC)
}

// serviceID は、こたつテスト1 の Mirakurun のサービス ID。
func serviceID() int64 {
	return faketuner.ServiceItemID(grNetworkID, grServiceID)
}

type fixture struct {
	pool  *pgxpool.Pool
	clock *clocktest.Fake
	// users は、名前ごとのユーザー。bob は、放送の 30 分前に通知を受け取る設定にしてある。
	users    map[string]uuid.UUID
	notifier *watch.Notifier
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(testNow())

	server := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(testNow(), nil)))
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)
	_, err = epg.NewReconciler(pool, client, clk, func(context.Context, pgx.Tx) error { return nil }).SyncAll(t.Context())
	require.NoError(t, err)

	users := map[string]uuid.UUID{}
	for _, name := range []string{"alice", "bob"} {
		user, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{
			Username: name, DisplayName: name, Role: auth.RoleViewer, PasswordHash: nil,
		})
		require.NoError(t, err)
		users[name] = user.ID
	}
	_, err = watch.SetReminderMinutes(t.Context(), pool, clk, users["bob"], 30)
	require.NoError(t, err)
	liveURL := func(id int64) string { return fmt.Sprintf("/live/%d", id) }

	return &fixture{pool: pool, clock: clk, users: users, notifier: watch.NewNotifier(pool, clk, liveURL)}
}

func (f *fixture) reserve(t *testing.T, user string, eventID int64) {
	t.Helper()

	require.NoError(t, watch.Reserve(t.Context(), f.pool, f.clock, f.users[user], watch.ProgramKey{ServiceID: serviceID(), EventID: eventID}))
}

// sent は、記録した視聴予約の通知の、受け取るユーザーの名前と payload を、記録した順に返す。
func (f *fixture) sent(t *testing.T) []sentEvent {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), `SELECT audience, payload FROM notification_events WHERE kind = 'watch_reminder' ORDER BY occurred_at, id`)
	require.NoError(t, err)
	names := map[uuid.UUID]string{}
	for name, id := range f.users {
		names[id] = name
	}
	events := []sentEvent{}
	for rows.Next() {
		var audience []uuid.UUID
		var raw []byte
		require.NoError(t, rows.Scan(&audience, &raw))
		require.Len(t, audience, 1, "a watch reminder is sent only to the user who reserved it")
		var payload struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		require.NoError(t, json.Unmarshal(raw, &payload))
		events = append(events, sentEvent{User: names[audience[0]], Name: payload.Name, URL: payload.URL})
	}
	require.NoError(t, rows.Err())

	return events
}

type sentEvent struct {
	User string
	Name string
	URL  string
}

// step は、時刻 now に Notify を呼んだときに、増える通知と、片付ける視聴予約の数。
type step struct {
	now        time.Time
	wantSent   []sentEvent
	wantPruned int64
}

func reminder(user string, eventID int) sentEvent {
	return sentEvent{User: user, Name: fmt.Sprintf("こたつテスト1 テスト番組 %d", eventID), URL: fmt.Sprintf("/live/%d", serviceID())}
}

// TestNotify は、視聴予約の通知を、通知を送る時刻を過ぎてから放送が終わるまでの間に一度だけ送り、
// 放送が終わった番組と EPG から消えた番組の視聴予約は、送らずに片付けることを確かめる (設計書 4.8)。
func TestNotify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, f *fixture)
		steps []step
	}{
		{
			name: "既定の 5 分前に、一度だけ送る",
			setup: func(t *testing.T, f *fixture) {
				t.Helper()
				f.reserve(t, "alice", 4)
			},
			steps: []step{
				{now: at(12, 24), wantSent: []sentEvent{}},
				{now: at(12, 25), wantSent: []sentEvent{reminder("alice", 4)}},
				{now: at(12, 26), wantSent: []sentEvent{}},
			},
		},
		{
			name: "ユーザーの設定の分だけ前に送る",
			setup: func(t *testing.T, f *fixture) {
				t.Helper()
				f.reserve(t, "bob", 5)
			},
			steps: []step{
				{now: at(12, 29), wantSent: []sentEvent{}},
				{now: at(12, 30), wantSent: []sentEvent{reminder("bob", 5)}},
			},
		},
		{
			name: "同じ番組を予約したユーザーそれぞれに、それぞれの時刻で送る",
			setup: func(t *testing.T, f *fixture) {
				t.Helper()
				f.reserve(t, "alice", 5)
				f.reserve(t, "bob", 5)
			},
			steps: []step{
				{now: at(12, 30), wantSent: []sentEvent{reminder("bob", 5)}},
				{now: at(12, 55), wantSent: []sentEvent{reminder("alice", 5)}},
			},
		},
		{
			name: "送る時刻を過ぎてから付けたら、すぐに送る",
			setup: func(t *testing.T, f *fixture) {
				t.Helper()
				f.clock.Advance(at(12, 28).Sub(f.clock.Now()))
				f.reserve(t, "alice", 4)
			},
			steps: []step{{now: at(12, 28), wantSent: []sentEvent{reminder("alice", 4)}}},
		},
		{
			name: "ジョブが遅れても、放送が終わる前なら送る",
			setup: func(t *testing.T, f *fixture) {
				t.Helper()
				f.reserve(t, "alice", 4)
			},
			steps: []step{{now: at(12, 45), wantSent: []sentEvent{reminder("alice", 4)}}},
		},
		{
			name: "放送が終わった番組は、送らずに片付ける",
			setup: func(t *testing.T, f *fixture) {
				t.Helper()
				f.reserve(t, "alice", 4)
			},
			steps: []step{{now: at(13, 0), wantSent: []sentEvent{}, wantPruned: 1}},
		},
		{
			name: "EPG から消えた番組は、送らずに片付ける",
			setup: func(t *testing.T, f *fixture) {
				t.Helper()
				f.reserve(t, "alice", 5)
				_, err := f.pool.Exec(t.Context(), `DELETE FROM programs WHERE service_id = $1 AND event_id = 5`, serviceID())
				require.NoError(t, err)
			},
			steps: []step{{now: at(12, 10), wantSent: []sentEvent{}, wantPruned: 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			tt.setup(t, f)
			before := 0
			for _, s := range tt.steps {
				f.clock.Advance(s.now.Sub(f.clock.Now()))
				notified, pruned, err := f.notifier.Notify(t.Context())
				require.NoError(t, err)
				all := f.sent(t)
				assert.Equal(t, s.wantSent, all[before:], s.now.Format(time.TimeOnly))
				assert.Equal(t, len(s.wantSent), notified, s.now.Format(time.TimeOnly))
				assert.Equal(t, s.wantPruned, pruned, s.now.Format(time.TimeOnly))
				before = len(all)
			}
		})
	}
}

// TestReserve は、放送が始まった番組と EPG にない番組には、視聴予約を付けられないことを確かめる。
func TestReserve(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	tests := []struct {
		name    string
		eventID int64
		wantErr error
	}{
		{name: "これから放送する番組", eventID: 5, wantErr: nil},
		{name: "付けてある番組にもう一度付ける", eventID: 5, wantErr: nil},
		{name: "放送中の番組", eventID: 3, wantErr: watch.ErrProgramUnavailable},
		{name: "EPG にない番組", eventID: 9999, wantErr: watch.ErrProgramUnavailable},
	}
	// 同じ番組に二度付けるので、順に動かす。
	for _, tt := range tests {
		err := watch.Reserve(t.Context(), f.pool, f.clock, f.users["alice"], watch.ProgramKey{ServiceID: serviceID(), EventID: tt.eventID})
		if tt.wantErr == nil {
			require.NoError(t, err, tt.name)
		} else {
			require.ErrorIs(t, err, tt.wantErr, tt.name)
		}
	}
}
