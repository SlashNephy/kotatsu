package recording_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/assign"
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/notify"
	"github.com/SlashNephy/kotatsu/internal/recording"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/tunerstate"
)

const grNetworkID = 0x7FE0

// grService は、偽チューナーサーバーの地上波の 3 つのチャンネルの、i 番目のサービスの ID を返す。地上波のチューナーは 2 本ある。
func grService(i int) int64 {
	serviceIDs := []int{0x0400, 0x0408, 0x0410}

	return faketuner.ServiceItemID(grNetworkID+i, serviceIDs[i])
}

func testNow() time.Time {
	return time.Date(2026, 10, 10, 12, 10, 0, 0, time.UTC)
}

// programStart は、予約する番組 (eventId 6) の開始時刻。
func programStart() time.Time {
	return time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC)
}

func noGuard(context.Context, pgx.Tx) error { return nil }

type fixture struct {
	pool         *pgxpool.Pool
	tuner        *mirakurun.Client
	clock        *clocktest.Fake
	reservations *reserve.Reservations
	assigner     *assign.Runner
	scheduler    *recording.Scheduler
}

// newFixture は、偽チューナーサーバーの EPG とチューナーの構成を、12:10 の時点で取り込んだ DB を用意する。
func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	// 録画のテストのため、本物の TS を流す。
	stream, err := os.ReadFile("../../testdata/ts/test.ts")
	require.NoError(t, err)
	server := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(testNow(), stream)))
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)
	clk := clocktest.NewFake(testNow())
	_, err = epg.NewReconciler(pool, client, clk, noGuard).SyncAll(t.Context())
	require.NoError(t, err)
	require.NoError(t, tunerstate.NewMonitor(pool, client, clk, noGuard, slog.New(slog.DiscardHandler), nil).Check(t.Context()))

	return &fixture{
		pool:         pool,
		tuner:        client,
		clock:        clk,
		reservations: reserve.NewReservations(clk, audit.NewRecorder(clk)),
		assigner:     assign.NewRunner(pool, clk, noGuard, slog.New(slog.DiscardHandler), nil, nil),
		scheduler:    recording.NewScheduler(pool, clk, noGuard, slog.New(slog.DiscardHandler), nil),
	}
}

// reserve は、地上波の i 番目のサービスの、13:30 に始まる番組を予約し、チューナーを割り当てる。
func (f *fixture) reserve(t *testing.T, service int) uuid.UUID {
	t.Helper()

	return f.reserveEvent(t, service, 6)
}

// reserveEvent は、地上波の i 番目のサービスの、eventId の番組を予約し、チューナーを割り当てる。番組は 30 分ごとに並び、eventId 6 が 13:30 に始まる。
func (f *fixture) reserveEvent(t *testing.T, service int, eventID int64) uuid.UUID {
	t.Helper()

	reservation, _, err := f.reservations.CreateProgram(t.Context(), f.pool, audit.System("test"), &reserve.ProgramInput{
		ServiceID: grService(service), EventID: eventID, FollowEventRelay: true, Overrides: nil,
	})
	require.NoError(t, err)
	_, err = f.assigner.RunOnce(t.Context())
	require.NoError(t, err)

	return reservation.ID
}

func (f *fixture) schedule(t *testing.T) *recording.ScheduleResult {
	t.Helper()

	result, err := f.scheduler.RunOnce(t.Context())
	require.NoError(t, err)

	return result
}

// assigned は、割り振りのある予約と、その優先度を返す。
func (f *fixture) assigned(t *testing.T) map[uuid.UUID]int32 {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT reservation_id, priority FROM recording_assignments WHERE state = 'waiting' AND generation = 0 AND holder IS NULL")
	require.NoError(t, err)
	defer rows.Close()
	got := map[uuid.UUID]int32{}
	for rows.Next() {
		var (
			reservationID uuid.UUID
			priority      int32
		)
		require.NoError(t, rows.Scan(&reservationID, &priority))
		got[reservationID] = priority
	}
	require.NoError(t, rows.Err())

	return got
}

func (f *fixture) version(t *testing.T) int64 {
	t.Helper()

	var version int64
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT coalesce((SELECT version FROM change_versions WHERE channel = $1), 0)", recording.NotifyChannel).Scan(&version))

	return version
}

// recordingsVersion は、録画の変更の通知の版数を返す (#868)。
func (f *fixture) recordingsVersion(t *testing.T) int64 {
	t.Helper()

	var version int64
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT coalesce((SELECT version FROM change_versions WHERE channel = $1), 0)", notify.RecordingsChannel).Scan(&version))

	return version
}

// TestSchedulerWritesAssignmentsBeforeStart は、録画の予定の開始の 2 分前になった予約にだけ割り振りを書き、
// 競合した予約は優先度を下げ、書き直さないことを確かめる (設計書 1.5、1.6)。
func TestSchedulerWritesAssignmentsBeforeStart(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	// 地上波のチューナーは 2 本なので、3 つの予約のうち 1 つが競合する。
	ids := []uuid.UUID{f.reserve(t, 0), f.reserve(t, 1), f.reserve(t, 2)}

	// 予定の開始 (13:30 からマージンを引いた時刻) の 2 分前より前には書かない。
	f.clock.Advance(programStart().Add(-reserve.DefaultMarginSeconds*time.Second - recording.DefaultLead - time.Second).Sub(f.clock.Now()))
	assert.Equal(t, &recording.ScheduleResult{Created: 0, Deleted: 0}, f.schedule(t))
	assert.Zero(t, f.version(t))

	f.clock.Advance(time.Second)
	assert.Equal(t, &recording.ScheduleResult{Created: 3, Deleted: 0}, f.schedule(t))
	got := f.assigned(t)
	assert.Len(t, got, 3)
	priorities := map[int32]int{}
	for _, reservationID := range ids {
		priorities[got[reservationID]]++
	}
	assert.Equal(t, map[int32]int{recording.Priority: 2, recording.ConflictPriority: 1}, priorities)
	assert.Equal(t, int64(1), f.version(t), "mikan is notified")

	// 書いた割り振りは書き直さず、変わらなければ知らせない。
	assert.Equal(t, &recording.ScheduleResult{Created: 0, Deleted: 0}, f.schedule(t))
	assert.Equal(t, int64(1), f.version(t))
}

// TestSchedulerDeletesStaleAssignments は、まだ mikan が取っていない割り振りのうち、録画しなくなった予約のものを消し、
// mikan が取った割り振りは消さないことを確かめる。
func TestSchedulerDeletesStaleAssignments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(t *testing.T, f *fixture, reservationID uuid.UUID)
		// wantMissed は、mikan が取らなかったので、失敗として記録する録画の数。
		wantMissed int
	}{
		{name: "予約を無効にした", wantMissed: 0, change: func(t *testing.T, f *fixture, reservationID uuid.UUID) {
			t.Helper()

			_, err := f.reservations.Update(t.Context(), f.pool, audit.System("test"), reservationID, &reserve.Update{Enabled: new(false)})
			require.NoError(t, err)
		}},
		{name: "予定の開始が後に動いた", wantMissed: 0, change: func(t *testing.T, f *fixture, reservationID uuid.UUID) {
			t.Helper()

			_, err := f.pool.Exec(t.Context(), "UPDATE reservations SET planned_start_at = planned_start_at + interval '1 hour' WHERE id = $1", reservationID)
			require.NoError(t, err)
		}},
		{name: "mikan が取らないまま予定の終わりが過ぎた", wantMissed: 1, change: func(t *testing.T, f *fixture, _ uuid.UUID) {
			t.Helper()

			f.clock.Advance(time.Hour)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			waiting, taken := f.reserve(t, 0), f.reserve(t, 1)
			f.clock.Advance(programStart().Add(-time.Minute).Sub(f.clock.Now()))
			require.Equal(t, 2, f.schedule(t).Created)
			// 片方は、mikan が取ったものとして、録画中にする。
			_, err := f.pool.Exec(t.Context(), "UPDATE recording_assignments SET state = 'recording', generation = 1 WHERE reservation_id = $1", taken)
			require.NoError(t, err)

			tt.change(t, f, waiting)
			tt.change(t, f, taken)
			assert.Equal(t, &recording.ScheduleResult{Created: 0, Deleted: 1, Failed: 0, Missed: tt.wantMissed}, f.schedule(t))
			var left []uuid.UUID
			rows, err := f.pool.Query(t.Context(), "SELECT reservation_id FROM recording_assignments")
			require.NoError(t, err)
			left, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
			require.NoError(t, err)
			assert.Equal(t, []uuid.UUID{taken}, left, "the taken one is left to mikan")
		})
	}
}

// TestSchedulerSkipsFinishedReservations は、予定の終わりが過ぎた予約には割り振りを書かないことを確かめる。
func TestSchedulerSkipsFinishedReservations(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.reserve(t, 0)
	f.clock.Advance(programStart().Add(2 * time.Hour).Sub(f.clock.Now()))
	assert.Equal(t, &recording.ScheduleResult{Created: 0, Deleted: 0, Failed: 0, Missed: 0}, f.schedule(t))
}

// TestSchedulerFailsStuckRecordings は、予定の終わりから 10 分が過ぎても「録画中」のままで、録画している割り振りもない録画を、
// 失敗にして通知することを確かめる (#516)。録画している mikan がいなければ、ほかに録画を終える人がいない。
func TestSchedulerFailsStuckRecordings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// assignment は、録画の割り振りの状態。空なら割り振りがない (予約を消して CASCADE で消えたとき)。
		assignment string
		// elapsed は、番組の終わり (14:00) からの時間。
		elapsed  time.Duration
		wantFail bool
	}{
		{name: "no assignment", assignment: "", elapsed: 11 * time.Minute, wantFail: true},
		{name: "finished assignment", assignment: "finished", elapsed: 11 * time.Minute, wantFail: true},
		{name: "still recording", assignment: "recording", elapsed: 11 * time.Minute, wantFail: false},
		{name: "waiting for a handover", assignment: "handover", elapsed: 11 * time.Minute, wantFail: false},
		{name: "within the grace", assignment: "", elapsed: 9 * time.Minute, wantFail: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			recordingID := id.New()
			start := programStart()
			end := start.Add(30 * time.Minute)
			_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
				state, recorded_from, protected, created_at, updated_at)
				VALUES ($1, $2, $3, 6, '録画', '', $4, $5, 'recording', $4, false, $4, $4)`, recordingID, reservationID, grService(0), start, end)
			require.NoError(t, err)
			if tt.assignment != "" {
				_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_assignments (id, reservation_id, priority, state, generation, created_at, updated_at)
					VALUES ($1, $2, 0, $3, 1, $4, $4)`, id.New(), reservationID, tt.assignment, start)
				require.NoError(t, err)
			}
			f.clock.Advance(end.Add(tt.elapsed).Sub(f.clock.Now()))

			f.schedule(t)

			var state string
			var reason *string
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT state, failure_reason FROM recordings WHERE id = $1", recordingID).Scan(&state, &reason))
			var events int
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM notification_events WHERE kind = 'recording_failed'").Scan(&events))
			if !tt.wantFail {
				assert.Equal(t, "recording", state)
				assert.Zero(t, events)
				assert.Zero(t, f.recordingsVersion(t))

				return
			}
			assert.Equal(t, "failed", state)
			require.NotNil(t, reason)
			assert.Equal(t, "stream_interrupted", *reason)
			assert.Equal(t, 1, events)
			assert.EqualValues(t, 1, f.recordingsVersion(t))

			// 何度見直しても、通知は 1 回だけ。
			f.schedule(t)
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM notification_events WHERE kind = 'recording_failed'").Scan(&events))
			assert.Equal(t, 1, events)
		})
	}
}

// TestSchedulerRecordsMissedRecordings は、mikan が 1 台も取らないまま予定の終わりが過ぎた割り振りを、消す前に、
// 録画の失敗 (recorder_unavailable) として記録し、通知することを確かめる (#517)。記録しないと、録り逃したことに気づけない。
func TestSchedulerRecordsMissedRecordings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// prepare は、予定の終わりが過ぎる前に、予約や録画の状態を変える。
		prepare    func(t *testing.T, f *fixture, reservationID uuid.UUID)
		wantMissed int
	}{
		{name: "mikan が取らなかった", prepare: func(*testing.T, *fixture, uuid.UUID) {}, wantMissed: 1},
		{name: "予約を無効にした", prepare: func(t *testing.T, f *fixture, reservationID uuid.UUID) {
			t.Helper()

			_, err := f.reservations.Update(t.Context(), f.pool, audit.System("test"), reservationID, &reserve.Update{Enabled: new(false)})
			require.NoError(t, err)
		}, wantMissed: 0},
		{name: "録画の行がすでにある", prepare: func(t *testing.T, f *fixture, reservationID uuid.UUID) {
			t.Helper()

			_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
				state, failure_reason, recorded_from, protected, created_at, updated_at)
				VALUES ($1, $2, $3, 6, '録画', '', $4, $4, 'failed', 'tuner_unavailable', $4, false, $4, $4)`, id.New(), reservationID, grService(0), programStart())
			require.NoError(t, err)
		}, wantMissed: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			f.clock.Advance(programStart().Add(-time.Minute).Sub(f.clock.Now()))
			require.Equal(t, 1, f.schedule(t).Created)
			tt.prepare(t, f, reservationID)

			f.clock.Advance(time.Hour)
			assert.Equal(t, tt.wantMissed, f.schedule(t).Missed)
			var events int
			require.NoError(t, f.pool.QueryRow(t.Context(),
				"SELECT count(*) FROM notification_events WHERE kind = 'recording_failed' AND payload->>'reason' = 'recorder_unavailable'").Scan(&events))
			assert.Equal(t, tt.wantMissed, events)
			assert.EqualValues(t, tt.wantMissed, f.recordingsVersion(t))
			if tt.wantMissed == 0 {
				return
			}
			var state, reason, recordingID string
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT state, failure_reason, id FROM recordings WHERE reservation_id = $1", reservationID).
				Scan(&state, &reason, &recordingID))
			assert.Equal(t, "failed", state)
			assert.Equal(t, "recorder_unavailable", reason)
			var notified string
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT payload->>'recordingId' FROM notification_events WHERE kind = 'recording_failed'").Scan(&notified))
			assert.Equal(t, recordingID, notified)

			// 何度見直しても、記録と通知は 1 回だけ。
			assert.Zero(t, f.schedule(t).Missed)
		})
	}
}

// TestSchedulerFailsReleasedRecordingsWithTheirReason は、録画できずに手放されたまま予定の終わりが過ぎた録画を、
// 手放した mikan が残した理由で失敗にすることを確かめる (#524)。
func TestSchedulerFailsReleasedRecordingsWithTheirReason(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	f.clock.Advance(programStart().Add(-time.Minute).Sub(f.clock.Now()))
	require.Equal(t, 1, f.schedule(t).Created)
	// mikan が録画の行を作り、プールがないので理由を残して手放した。
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, failure_reason, recorded_from, protected, created_at, updated_at)
		VALUES ($1, $2, $3, 6, '録画', '', $4, $5, 'recording', 'storage_unavailable', $4, false, $4, $4)`,
		id.New(), reservationID, grService(0), programStart(), programStart().Add(30*time.Minute))
	require.NoError(t, err)

	f.clock.Advance(time.Hour)
	f.schedule(t)
	assert.Zero(t, f.count(t, "SELECT count(*) FROM recording_assignments WHERE reservation_id = $1", reservationID))
	var state, reason string
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT state, failure_reason FROM recordings WHERE reservation_id = $1", reservationID).Scan(&state, &reason))
	assert.Equal(t, "failed", state)
	assert.Equal(t, "storage_unavailable", reason)
}
