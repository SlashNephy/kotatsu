package reserve_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
)

// subServiceID は、こたつテスト1 と同じチャンネルのサブチャンネル。イベントリレーの引き継ぎ先に使う。
const subServiceID = 0x0401

func (f *fixture) follower() *reserve.Follower {
	return reserve.NewFollower(f.pool, func(context.Context, pgx.Tx) error { return nil }, slog.New(slog.DiscardHandler), f.reservations, nil)
}

// addRelayingProgram は、こたつテスト1 の番組 (eventId 6) を、start から 30 分、サブチャンネルの eventId 7 に引き継ぐ番組にする。
func (f *fixture) addRelayingProgram(start time.Time) {
	f.fake.AddProgram(&faketuner.Program{
		ID: faketuner.ProgramID(grNetworkID, grServiceID, futureEvent), EventID: futureEvent, ServiceID: grServiceID, NetworkID: grNetworkID,
		StartAt: start.UnixMilli(), Duration: (30 * time.Minute).Milliseconds(), IsFree: true,
		Name: "こたつテスト1 テスト番組 6", Description: "偽のチューナーサーバーが合成した番組です。",
		RelatedItems: []*faketuner.RelatedItem{{Type: "relay", NetworkID: 0, ServiceID: subServiceID, EventID: 7}},
	})
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

			assert.Equal(t, &reserve.FollowResult{Updated: tt.wantUpdated, Relayed: 0, Removed: 0}, f.follow(t))
			got, err := query.New(f.pool).GetReservation(t.Context(), reservation.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStart, got.StartAt)
			assert.Equal(t, tt.wantEnd, got.EndAt)

			// 同じ EPG に何度追従しても、予約は変わらない。
			assert.Equal(t, &reserve.FollowResult{Updated: 0, Relayed: 0, Removed: 0}, f.follow(t))
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
			in.Overrides = &domain.RecordingOverrides{Priority: new(3)}
			original, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, in)
			require.NoError(t, err)
			if tt.disabled {
				_, err := f.reservations.Update(t.Context(), f.pool, f.user, original.ID, &reserve.Update{Enabled: new(false)})
				require.NoError(t, err)
			}

			// こたつテスト1 の eventId 6 が、サブチャンネルの eventId 7 に引き継がれる。
			start := time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC)
			f.addRelayingProgram(start)
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

// TestFollowerRelaysAfterTheEnd は、引き継ぎ先の番組が、元の番組の終わりの後に EPG に出ても、引き継ぎ先の予約を作ることを確かめる (#639)。
// 延長で臨時のサブチャンネルへ移るときは、移る先の番組が、移った時点で初めて EPG に出ることがある。
func TestFollowerRelaysAfterTheEnd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// after は、元の番組 (13:30〜14:00) の予定の終わりから、引き継ぎ先の番組が EPG に出るまでの時間。
		after       time.Duration
		wantRelayed int
	}{
		{name: "終わりの直後に出たら、引き継ぐ", after: 10 * time.Minute, wantRelayed: 1},
		{name: "終わってから 1 時間を過ぎて出たら、引き継がない", after: 90 * time.Minute, wantRelayed: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			original, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, programInput())
			require.NoError(t, err)
			plannedEnd := time.Date(2026, 10, 10, 14, 0, 15, 0, time.UTC)
			_, err = f.pool.Exec(t.Context(), "UPDATE reservations SET assignment_state = 'ok', planned_start_at = $2, planned_end_at = $3 WHERE id = $1",
				original.ID, time.Date(2026, 10, 10, 13, 29, 45, 0, time.UTC), plannedEnd)
			require.NoError(t, err)
			// 引き継ぎ先の番組は、まだ EPG にない。
			f.addRelayingProgram(time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC))
			relayedID := faketuner.ProgramID(grNetworkID, subServiceID, 7)
			require.NoError(t, f.fake.ForgetProgram(relayedID))
			f.syncEPG(t)
			assert.Zero(t, f.follow(t).Relayed)

			// 予定の終わりを過ぎてから、サブチャンネルで放送が始まり、引き継ぎ先の番組が EPG に出た。
			f.clock.Advance(plannedEnd.Add(tt.after).Sub(f.clock.Now()))
			f.fake.AddProgram(&faketuner.Program{
				ID: relayedID, EventID: 7, ServiceID: subServiceID, NetworkID: grNetworkID,
				StartAt: time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC).UnixMilli(), Duration: 0, IsFree: true,
				Name: "こたつテスト1 テスト番組 6", Description: "偽のチューナーサーバーが合成した番組です。",
			})
			f.syncEPG(t)

			assert.Equal(t, tt.wantRelayed, f.follow(t).Relayed)
			var count int
			require.NoError(t, f.pool.QueryRow(t.Context(),
				"SELECT count(*) FROM reservations WHERE relayed_from = $1", original.ID).Scan(&count))
			assert.Equal(t, tt.wantRelayed, count)
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
	f.addRelayingProgram(start)
	f.syncEPG(t)

	assert.Equal(t, 1, f.follow(t).Relayed)
	var priority int
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT (settings ->> 'priority')::int FROM reservations WHERE relayed_from = $1", original.ID).Scan(&priority))
	assert.Equal(t, 4, priority)
}

// TestFollowerRemovesFinishedReservations は、放送が終わって猶予が過ぎ、録画も終わった予約を消すことを確かめる。
func TestFollowerRemovesFinishedReservations(t *testing.T) {
	t.Parallel()

	// recording は、録画中の割り振りと録画の状態。
	const recording = "recording"

	tests := []struct {
		name string
		// elapsed は、予約を作ってから追従するまでに進める時間。予約の番組は 13:30〜14:00 で、今は 12:10。
		elapsed time.Duration
		// assignment は、録画の割り振りの状態。空なら割り振りを作らない。
		assignment string
		// recording は、録画の行の状態。空なら録画の行を作らない。
		recording   string
		wantRemoved int
	}{
		{name: "after the grace", elapsed: 3 * time.Hour, assignment: "", recording: "", wantRemoved: 1},
		{name: "after the grace with a completed recording", elapsed: 3 * time.Hour, assignment: "finished", recording: "completed", wantRemoved: 1},
		{name: "within the grace", elapsed: 2 * time.Hour, assignment: "", recording: "", wantRemoved: 0},
		{name: "not ended", elapsed: 0, assignment: "", recording: "", wantRemoved: 0},
		{name: "still recording", elapsed: 3 * time.Hour, assignment: recording, recording: recording, wantRemoved: 0},
		{name: "waiting for a handover", elapsed: 3 * time.Hour, assignment: "handover", recording: recording, wantRemoved: 0},
		{name: "recording without an assignment", elapsed: 3 * time.Hour, assignment: "", recording: recording, wantRemoved: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservation, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, programInput())
			require.NoError(t, err)
			if tt.assignment != "" {
				_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_assignments (id, reservation_id, priority, state, generation, created_at, updated_at)
					VALUES ($1, $2, 0, $3, 1, $4, $4)`, id.New(), reservation.ID, tt.assignment, testNow())
				require.NoError(t, err)
			}
			if tt.recording != "" {
				_, err = f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
					state, recorded_from, protected, created_at, updated_at)
					VALUES ($1, $2, $3, $4, '録画', '', $5, $6, $7, $5, false, $5, $5)`,
					id.New(), reservation.ID, serviceID(), futureEvent, reservation.StartAt, reservation.EndAt, tt.recording)
				require.NoError(t, err)
			}
			f.clock.Advance(tt.elapsed)

			assert.Equal(t, tt.wantRemoved, f.follow(t).Removed)
			_, err = query.New(f.pool).GetReservation(t.Context(), reservation.ID)
			if tt.wantRemoved > 0 {
				require.ErrorIs(t, err, domain.ErrReservationNotFound)
				// 消したことを監査ログに残す。録画は残る。
				assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'reservation.delete' AND target_id = $1", reservation.ID.String()))
				if tt.recording != "" {
					assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM recordings WHERE event_id = $1 AND reservation_id IS NULL", futureEvent))
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestFollowerReleasesReusedEventIDs は、放送が終わった予約の EID が別の番組に再利用されたとき、
// 古い予約を新しい番組に書き換えず、新しい番組を予約できるようにすることを確かめる (#510)。
func TestFollowerReleasesReusedEventIDs(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	old, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, programInput())
	require.NoError(t, err)

	// 番組 (13:30〜14:00) の放送が終わってしばらく経ってから、同じ EID が 1 週間後の別の番組に使われる。
	f.clock.Advance(3 * time.Hour)
	reused := time.Date(2026, 10, 17, 21, 0, 0, 0, time.UTC)
	require.NoError(t, f.fake.RescheduleProgram(faketuner.ProgramID(grNetworkID, grServiceID, futureEvent), reused, time.Hour))
	f.syncEPG(t)

	assert.Equal(t, &reserve.FollowResult{Updated: 0, Relayed: 0, Removed: 1}, f.follow(t))
	_, err = query.New(f.pool).GetReservation(t.Context(), old.ID)
	require.ErrorIs(t, err, domain.ErrReservationNotFound)

	reservation, created, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, programInput())
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotEqual(t, old.ID, reservation.ID)
	assert.Equal(t, reused, reservation.StartAt)
}

// TestFollowerDoesNotRewriteEndedReservations は、猶予の間に EID が再利用されても、終わった予約を新しい番組に書き換えないことを確かめる。
// 同じ放送の終わりが延びたときは、予約の終わりが過ぎていても追従する。
func TestFollowerDoesNotRewriteEndedReservations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		change      func(t *testing.T, fake *faketuner.Server)
		wantUpdated int
		wantStart   time.Time
		wantEnd     time.Time
	}{
		{
			name: "reused",
			change: func(t *testing.T, fake *faketuner.Server) {
				t.Helper()
				require.NoError(t, fake.RescheduleProgram(faketuner.ProgramID(grNetworkID, grServiceID, futureEvent),
					time.Date(2026, 10, 17, 21, 0, 0, 0, time.UTC), time.Hour))
			},
			wantUpdated: 0,
			wantStart:   time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC),
		},
		{
			name: "extended",
			change: func(t *testing.T, fake *faketuner.Server) {
				t.Helper()
				require.NoError(t, fake.ExtendProgram(faketuner.ProgramID(grNetworkID, grServiceID, futureEvent), 20*time.Minute))
			},
			wantUpdated: 1,
			wantStart:   time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 10, 10, 14, 20, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservation, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, programInput())
			require.NoError(t, err)
			// 予約の終わり (14:00) を 5 分過ぎた。猶予の間なので、予約はまだ消さない。
			f.clock.Advance(time.Hour + 55*time.Minute)
			tt.change(t, f.fake)
			f.syncEPG(t)

			result := f.follow(t)
			assert.Equal(t, tt.wantUpdated, result.Updated)
			assert.Zero(t, result.Removed)
			got, err := query.New(f.pool).GetReservation(t.Context(), reservation.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStart, got.StartAt)
			assert.Equal(t, tt.wantEnd, got.EndAt)
		})
	}
}

// TestFollowerKeepsExtendedReservations は、割り当ての予定の終わりより後まで放送が延びた予約を、延びた終わりまで消さないことを確かめる。
func TestFollowerKeepsExtendedReservations(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservation, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, programInput())
	require.NoError(t, err)
	// 割り当ては、番組 (13:30〜14:00) の終わりにマージンを足した時刻を予定の終わりにした。
	_, err = f.pool.Exec(t.Context(), "UPDATE reservations SET assignment_state = 'ok', planned_start_at = $2, planned_end_at = $3 WHERE id = $1",
		reservation.ID, time.Date(2026, 10, 10, 13, 29, 45, 0, time.UTC), time.Date(2026, 10, 10, 14, 0, 15, 0, time.UTC))
	require.NoError(t, err)
	// 放送が 15:30 まで延びた。
	require.NoError(t, f.fake.ExtendProgram(faketuner.ProgramID(grNetworkID, grServiceID, futureEvent), 90*time.Minute))
	f.syncEPG(t)
	assert.Equal(t, 1, f.follow(t).Updated)

	// 割り当ての予定の終わりからは 1 時間が過ぎたが、延びた終わりからは過ぎていない。
	f.clock.Advance(3 * time.Hour)
	assert.Zero(t, f.follow(t).Removed)
	_, err = query.New(f.pool).GetReservation(t.Context(), reservation.ID)
	require.NoError(t, err)
}

// TestReservationsOfUndecidedPrograms は、終了時刻が未定の番組からも、引き継ぎと手動で予約を作れることを確かめる (#521)。
// Mahiron は未定の長さを 0 で返すので、番組の終わりを写すと、予約の始まりと終わりが同じになり、制約に違反していた。
// 予約の終わりは、始まりから 1 時間にしておき、終了時刻が決まったら追従で直す。
func TestReservationsOfUndecidedPrograms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// duration は、チューナーサーバーが返す未定の番組の長さ。Mahiron は 0、Mirakurun は 1 ミリ秒。
		duration time.Duration
	}{
		{name: "Mahiron", duration: 0},
		{name: "Mirakurun", duration: time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			original, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, programInput())
			require.NoError(t, err)
			// 引き継ぎ先 (サブチャンネルの eventId 7) は、終了時刻が未定。
			start := time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC)
			relayedStart := time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)
			f.addRelayingProgram(start)
			require.NoError(t, f.fake.RescheduleProgram(faketuner.ProgramID(grNetworkID, subServiceID, 7), relayedStart, tt.duration))
			f.syncEPG(t)

			assert.Equal(t, 1, f.follow(t).Relayed)
			var startAt, endAt time.Time
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT start_at, end_at FROM reservations WHERE relayed_from = $1", original.ID).
				Scan(&startAt, &endAt))
			assert.Equal(t, relayedStart, startAt)
			assert.Equal(t, relayedStart.Add(time.Hour), endAt)

			// 手動でも予約できる。
			_, err = f.pool.Exec(t.Context(), "DELETE FROM reservations WHERE relayed_from = $1", original.ID)
			require.NoError(t, err)
			reservation, created, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, &reserve.ProgramInput{
				ServiceID: faketuner.ServiceItemID(grNetworkID, subServiceID), EventID: 7, FollowEventRelay: true, Overrides: nil,
			})
			require.NoError(t, err)
			assert.True(t, created)
			assert.Equal(t, relayedStart.Add(time.Hour), reservation.EndAt)

			// 終了時刻が決まったら、追従で予約の終わりを直す。
			require.NoError(t, f.fake.RescheduleProgram(faketuner.ProgramID(grNetworkID, subServiceID, 7), relayedStart, 30*time.Minute))
			f.syncEPG(t)
			f.follow(t)
			got, err := query.New(f.pool).GetReservation(t.Context(), reservation.ID)
			require.NoError(t, err)
			assert.Equal(t, relayedStart.Add(30*time.Minute), got.EndAt)
		})
	}
}

// TestDeletingRelayedReservationDisablesIt は、イベントリレーで引き継いだ予約を消す操作では、予約を消さずに無効にし、
// 追従で作り直さないことを確かめる (#522)。消すと、引き継ぎ元がある限り、次の追従でまた作られていた。
func TestDeletingRelayedReservationDisablesIt(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	original, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user, programInput())
	require.NoError(t, err)
	f.addRelayingProgram(time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC))
	f.syncEPG(t)
	require.Equal(t, 1, f.follow(t).Relayed)
	var relayedID uuid.UUID
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT id FROM reservations WHERE relayed_from = $1", original.ID).Scan(&relayedID))

	require.NoError(t, f.reservations.Delete(t.Context(), f.pool, f.user, relayedID))
	assert.Zero(t, f.follow(t).Relayed)
	got, err := query.New(f.pool).GetReservation(t.Context(), relayedID)
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM reservations WHERE relayed_from = $1", original.ID))
}
