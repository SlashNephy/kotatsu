package recording_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/assign"
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/recording"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
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

	server := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(testNow(), nil)))
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)
	clk := clocktest.NewFake(testNow())
	_, err = epg.NewReconciler(pool, client, clk, noGuard).SyncAll(t.Context())
	require.NoError(t, err)
	require.NoError(t, tunerstate.NewMonitor(pool, client, clk, noGuard, slog.New(slog.DiscardHandler), nil).Check(t.Context()))

	return &fixture{
		pool:         pool,
		clock:        clk,
		reservations: reserve.NewReservations(clk, audit.NewRecorder(clk)),
		assigner:     assign.NewRunner(pool, clk, noGuard, slog.New(slog.DiscardHandler), nil, nil),
		scheduler:    recording.NewScheduler(pool, clk, noGuard, slog.New(slog.DiscardHandler), nil),
	}
}

// reserve は、地上波の i 番目のサービスの、13:30 に始まる番組を予約し、チューナーを割り当てる。
func (f *fixture) reserve(t *testing.T, service int) uuid.UUID {
	t.Helper()

	reservation, _, err := f.reservations.CreateProgram(t.Context(), f.pool, audit.System("test"), &reserve.ProgramInput{
		ServiceID: grService(service), EventID: 6, FollowEventRelay: true, Overrides: nil,
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
	}{
		{name: "予約を無効にした", change: func(t *testing.T, f *fixture, reservationID uuid.UUID) {
			t.Helper()

			_, err := f.reservations.Update(t.Context(), f.pool, audit.System("test"), reservationID, &reserve.Update{Enabled: new(false)})
			require.NoError(t, err)
		}},
		{name: "予定の開始が後に動いた", change: func(t *testing.T, f *fixture, reservationID uuid.UUID) {
			t.Helper()

			_, err := f.pool.Exec(t.Context(), "UPDATE reservations SET planned_start_at = planned_start_at + interval '1 hour' WHERE id = $1", reservationID)
			require.NoError(t, err)
		}},
		{name: "mikan が取らないまま予定の終わりが過ぎた", change: func(t *testing.T, f *fixture, _ uuid.UUID) {
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
			assert.Equal(t, &recording.ScheduleResult{Created: 0, Deleted: 1}, f.schedule(t))
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
	assert.Equal(t, &recording.ScheduleResult{Created: 0, Deleted: 0}, f.schedule(t))
}
