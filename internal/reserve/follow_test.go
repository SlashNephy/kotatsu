package reserve_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
)

// subServiceID は、こたつテスト1 と同じチャンネルのサブチャンネル。イベントリレーの引き継ぎ先に使う。
const subServiceID = 0x0401

func (f *fixture) follower() *reserve.Follower {
	return reserve.NewFollower(f.pool, func(context.Context, pgx.Tx) error { return nil }, slog.New(slog.DiscardHandler), f.reservations, nil)
}

func (f *fixture) follow(t *testing.T) *reserve.FollowResult {
	t.Helper()

	result, err := f.follower().RunOnce(t.Context())
	require.NoError(t, err)

	return result
}

// syncEPG は、偽チューナーサーバーの今の EPG を DB に取り込む。
func (f *fixture) syncEPG(t *testing.T) {
	t.Helper()

	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
}

// TestFollowerUpdatesRescheduledPrograms は、放送時間の変更が、予約の番組情報と時刻に反映されることを確かめる。
func TestFollowerUpdatesRescheduledPrograms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// change は、番組 (eventId 6、13:30〜14:00) を変える。
		change      func(t *testing.T, fake *faketuner.Server)
		wantUpdated int
		wantStart   time.Time
		wantEnd     time.Time
	}{
		{
			name: "rescheduled",
			change: func(t *testing.T, fake *faketuner.Server) {
				t.Helper()
				require.NoError(t, fake.RescheduleProgram(faketuner.ProgramID(grNetworkID, grServiceID, futureEvent), testNow().Add(3*time.Hour), time.Hour))
			},
			wantUpdated: 1,
			wantStart:   testNow().Add(3 * time.Hour),
			wantEnd:     testNow().Add(4 * time.Hour),
		},
		{
			name: "extended",
			change: func(t *testing.T, fake *faketuner.Server) {
				t.Helper()
				require.NoError(t, fake.ExtendProgram(faketuner.ProgramID(grNetworkID, grServiceID, futureEvent), 10*time.Minute))
			},
			wantUpdated: 1,
			wantStart:   time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 10, 10, 14, 10, 0, 0, time.UTC),
		},
		{
			name: "delayed with an undecided end",
			change: func(t *testing.T, fake *faketuner.Server) {
				t.Helper()
				// 前の番組が延びて開始が 40 分遅れ、終了時刻は未定。予約の長さ (30 分) を保ったまま後ろにずらす。
				require.NoError(t, fake.RescheduleProgram(faketuner.ProgramID(grNetworkID, grServiceID, futureEvent),
					time.Date(2026, 10, 10, 14, 10, 0, 0, time.UTC), time.Millisecond))
			},
			wantUpdated: 1,
			wantStart:   time.Date(2026, 10, 10, 14, 10, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 10, 10, 14, 40, 0, 0, time.UTC),
		},
		{
			name: "end becomes undecided",
			change: func(t *testing.T, fake *faketuner.Server) {
				t.Helper()
				// 延長中で終了時刻が未定のとき、Mirakurun は長さを 1 ミリ秒にして返す。予約の終了時刻は縮めない。
				require.NoError(t, fake.RescheduleProgram(faketuner.ProgramID(grNetworkID, grServiceID, futureEvent),
					time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC), time.Millisecond))
			},
			wantUpdated: 0,
			wantStart:   time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservation, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, programInput())
			require.NoError(t, err)
			tt.change(t, f.fake)
			f.syncEPG(t)

			assert.Equal(t, &reserve.FollowResult{Updated: tt.wantUpdated, Relayed: 0}, f.follow(t))
			got, err := f.reservations.Get(t.Context(), f.pool, reservation.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStart, got.StartAt)
			assert.Equal(t, tt.wantEnd, got.EndAt)

			// 同じ EPG に何度追従しても、予約は変わらない。
			assert.Equal(t, &reserve.FollowResult{Updated: 0, Relayed: 0}, f.follow(t))
		})
	}
}

// TestFollowerCreatesRelayedReservations は、イベントリレーの引き継ぎ先の予約を、追従が有効な予約にだけ作ることを確かめる。
func TestFollowerCreatesRelayedReservations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		follow      bool
		disabled    bool
		wantRelayed int
	}{
		{name: "following", follow: true, disabled: false, wantRelayed: 1},
		{name: "not following", follow: false, disabled: false, wantRelayed: 0},
		{name: "disabled", follow: true, disabled: true, wantRelayed: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			in := programInput()
			in.FollowEventRelay = tt.follow
			in.Overrides = &reserve.Overrides{Priority: new(3)}
			original, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, in)
			require.NoError(t, err)
			if tt.disabled {
				_, err := f.reservations.Update(t.Context(), f.pool, f.user, original.ID, &reserve.Update{Enabled: new(false)})
				require.NoError(t, err)
			}

			// こたつテスト1 の eventId 6 が、サブチャンネルの eventId 7 に引き継がれる。
			start := time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC)
			f.fake.AddProgram(&faketuner.Program{
				ID: faketuner.ProgramID(grNetworkID, grServiceID, futureEvent), EventID: futureEvent, ServiceID: grServiceID, NetworkID: grNetworkID,
				StartAt: start.UnixMilli(), Duration: (30 * time.Minute).Milliseconds(), IsFree: true,
				Name: "こたつテスト1 テスト番組 6", Description: "偽のチューナーサーバーが合成した番組です。",
				RelatedItems: []*faketuner.RelatedItem{{Type: "relay", NetworkID: 0, ServiceID: subServiceID, EventID: 7}},
			})
			f.syncEPG(t)

			assert.Equal(t, tt.wantRelayed, f.follow(t).Relayed)
			// 何度追従しても、引き継ぎ先の予約は 1 件のまま。
			assert.Zero(t, f.follow(t).Relayed)

			var count int
			require.NoError(t, f.pool.QueryRow(t.Context(),
				"SELECT count(*) FROM reservations WHERE service_id = $1 AND event_id = 7 AND relayed_from = $2 AND kind = 'program'",
				faketuner.ServiceItemID(grNetworkID, subServiceID), original.ID).Scan(&count))
			assert.Equal(t, tt.wantRelayed, count)
			if tt.wantRelayed == 0 {
				return
			}
			var priority int
			require.NoError(t, f.pool.QueryRow(t.Context(),
				"SELECT (settings ->> 'priority')::int FROM reservations WHERE relayed_from = $1", original.ID).Scan(&priority))
			// 引き継ぎ先は、元の予約の録画設定を引き継ぐ。
			assert.Equal(t, 3, priority)
		})
	}
}

// TestFollowerRelaysRuleSettings は、ルール予約を引き継ぐとき、一致したルールの録画設定を引き継ぐことを確かめる。
func TestFollowerRelaysRuleSettings(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	original, _, err := f.reservations.CreateRule(t.Context(), f.pool, f.user, serviceID(), futureEvent, true)
	require.NoError(t, err)
	ruleID := id.New()
	_, err = f.pool.Exec(t.Context(),
		`INSERT INTO rules (id, name, memo, enabled, conditions, settings, avoid_duplicates, follow_event_relay, created_at, updated_at)
		VALUES ($1, 'ルール', '', true, '{"version":1,"conditions":{}}', '{"priority":4}', false, true, $2, $2)`, ruleID, testNow())
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "INSERT INTO reservation_rules (reservation_id, rule_id) VALUES ($1, $2)", original.ID, ruleID)
	require.NoError(t, err)

	start := time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC)
	f.fake.AddProgram(&faketuner.Program{
		ID: faketuner.ProgramID(grNetworkID, grServiceID, futureEvent), EventID: futureEvent, ServiceID: grServiceID, NetworkID: grNetworkID,
		StartAt: start.UnixMilli(), Duration: (30 * time.Minute).Milliseconds(), IsFree: true,
		Name: "こたつテスト1 テスト番組 6", Description: "偽のチューナーサーバーが合成した番組です。",
		RelatedItems: []*faketuner.RelatedItem{{Type: "relay", NetworkID: 0, ServiceID: subServiceID, EventID: 7}},
	})
	f.syncEPG(t)

	assert.Equal(t, 1, f.follow(t).Relayed)
	var priority int
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT (settings ->> 'priority')::int FROM reservations WHERE relayed_from = $1", original.ID).Scan(&priority))
	assert.Equal(t, 4, priority)
}
