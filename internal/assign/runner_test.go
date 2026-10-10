package assign_test

import (
	"context"
	"errors"
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
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/tunerstate"
)

const grNetworkID = 0x7FE0

// grService は、偽チューナーサーバーの地上波の 3 つのチャンネルの、i 番目のサービスの ID を返す。
// 地上波のチューナーは 2 本ある。
func grService(i int) int64 {
	serviceIDs := []int{0x0400, 0x0408, 0x0410}

	return faketuner.ServiceItemID(grNetworkID+i, serviceIDs[i])
}

func testNow() time.Time {
	return time.Date(2026, 10, 10, 12, 10, 0, 0, time.UTC)
}

func noGuard(context.Context, pgx.Tx) error { return nil }

type fixture struct {
	pool         *pgxpool.Pool
	clock        *clocktest.Fake
	reservations *reserve.Reservations
	runner       *assign.Runner
}

// newFixture は、偽チューナーサーバーの EPG とチューナーの構成を、12:10 の時点で取り込んだ DB を用意する。
// 番組は 11:00 から 30 分ごとに並び、eventId 6 は 13:30 に始まる。
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
		runner:       assign.NewRunner(pool, clk, noGuard, slog.New(slog.DiscardHandler), assign.FixedDefaults(reserve.DefaultSettings()), nil),
	}
}

// reserve は、地上波の i 番目のサービスの、13:30 に始まる番組 (eventId 6) を予約する。
func (f *fixture) reserve(t *testing.T, service int, overrides *reserve.Overrides) uuid.UUID {
	t.Helper()

	reservation, _, err := f.reservations.CreateProgram(t.Context(), f.pool, audit.System("test"), &reserve.ProgramInput{
		ServiceID: grService(service), EventID: 6, FollowEventRelay: true, Overrides: overrides,
	})
	require.NoError(t, err)

	return reservation.ID
}

type stored struct {
	state        string
	tuner        *int64
	plannedStart *time.Time
}

func (f *fixture) stored(t *testing.T, reservationID uuid.UUID) stored {
	t.Helper()

	var s stored
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT assignment_state, assigned_tuner, planned_start_at FROM reservations WHERE id = $1", reservationID).
		Scan(&s.state, &s.tuner, &s.plannedStart))

	return s
}

func (f *fixture) version(t *testing.T, channel string) int64 {
	t.Helper()

	var version int64
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT coalesce((SELECT version FROM change_versions WHERE channel = $1), 0)", channel).Scan(&version))

	return version
}

// notifiedConflicts は、競合を通知したイベントの、予約の ID を返す。
func (f *fixture) notifiedConflicts(t *testing.T) []uuid.UUID {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT (payload->>'reservationId')::uuid FROM notification_events WHERE kind = 'reservation_conflict'")
	require.NoError(t, err)
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	require.NoError(t, err)

	return ids
}

func (f *fixture) runOnce(t *testing.T) *assign.Result {
	t.Helper()

	result, err := f.runner.RunOnce(t.Context())
	require.NoError(t, err)

	return result
}

// TestRunOnceStoresAssignments は、割り当ての結果を DB に書き、新たな競合だけを通知することを確かめる。
func TestRunOnceStoresAssignments(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	// 地上波のチューナーは 2 本なので、同じ時刻の 3 つのチャンネルのうち、ID の最も大きい予約が競合する。
	ids := []uuid.UUID{f.reserve(t, 0, nil), f.reserve(t, 1, nil), f.reserve(t, 2, nil)}
	conflicted := ids[0]
	for _, id := range ids[1:] {
		if string(id[:]) > string(conflicted[:]) {
			conflicted = id
		}
	}

	result := f.runOnce(t)
	assert.Equal(t, 3, result.Changed)
	assert.Equal(t, []uuid.UUID{conflicted}, result.NewConflicts)
	assert.Equal(t, []uuid.UUID{conflicted}, f.notifiedConflicts(t))
	assert.Positive(t, f.version(t, assign.ConflictChannel))
	for _, id := range ids {
		want := string(assign.StateOK)
		if id == conflicted {
			want = string(assign.StateConflict)
		}
		assert.Equal(t, want, f.stored(t, id).state)
	}

	// 何も変わらなければ書かず、続いている競合は新たな競合として通知しない。
	conflicts := f.version(t, assign.ConflictChannel)
	assert.Equal(t, &assign.Result{Changed: 0, NewConflicts: []uuid.UUID{}}, f.runOnce(t))
	assert.Equal(t, conflicts, f.version(t, assign.ConflictChannel))
	assert.Equal(t, []uuid.UUID{conflicted}, f.notifiedConflicts(t))

	// 1 件を無効にすると、競合していた予約にチューナーが空く。無効にした予約の結果は消す。
	disabled := ids[0]
	if disabled == conflicted {
		disabled = ids[1]
	}
	_, err := f.reservations.Update(t.Context(), f.pool, audit.System("test"), disabled, &reserve.Update{Enabled: new(false)})
	require.NoError(t, err)
	f.runOnce(t)
	assert.Equal(t, string(assign.StateOK), f.stored(t, conflicted).state)
	assert.Equal(t, stored{state: "pending", tuner: nil, plannedStart: nil}, f.stored(t, disabled))
}

// TestRunOnceUsesRecordingSettings は、録画設定のマージンを、全体の既定値に予約の上書きを重ねて使うことを確かめる。
func TestRunOnceUsesRecordingSettings(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	withDefault := f.reserve(t, 0, nil)
	withOverride := f.reserve(t, 1, &reserve.Overrides{StartMarginSeconds: new(60)})
	f.runOnce(t)

	start := time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC)
	assert.Equal(t, start.Add(-reserve.DefaultMarginSeconds*time.Second), f.stored(t, withDefault).plannedStart.UTC())
	assert.Equal(t, start.Add(-time.Minute), f.stored(t, withOverride).plannedStart.UTC())
}

// TestRunOnceReadsDefaultsEachTime は、録画設定の全体の既定値を、割り当てのたびに引くことを確かめる。
// 画面で既定値を変えたら、次の割り当てから反映する (設計書 11.2)。
func TestRunOnceReadsDefaultsEachTime(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	margin := reserve.DefaultMarginSeconds
	var loadErr error
	f.runner = assign.NewRunner(f.pool, f.clock, noGuard, slog.New(slog.DiscardHandler), func(context.Context, sqlc.DBTX) (*reserve.Settings, error) {
		if loadErr != nil {
			return nil, loadErr
		}
		settings := reserve.DefaultSettings()
		settings.StartMarginSeconds = margin

		return settings, nil
	}, nil)
	reservationID := f.reserve(t, 0, nil)
	start := time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC)
	f.runOnce(t)
	assert.Equal(t, start.Add(-reserve.DefaultMarginSeconds*time.Second), f.stored(t, reservationID).plannedStart.UTC())

	margin = 120
	f.runOnce(t)
	assert.Equal(t, start.Add(-2*time.Minute), f.stored(t, reservationID).plannedStart.UTC())

	// 既定値を引けなければ、割り当てを変えずにエラーを返す。
	loadErr = errors.New("load failed")
	_, err := f.runner.RunOnce(t.Context())
	require.ErrorIs(t, err, loadErr)
	assert.Equal(t, start.Add(-2*time.Minute), f.stored(t, reservationID).plannedStart.UTC())
}

// TestRunOnceKeepsRecordingReservations は、録画が始まった予約を、後から優先度の高い予約が来ても動かさないことを確かめる。
func TestRunOnceKeepsRecordingReservations(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recording := f.reserve(t, 0, nil)
	f.runOnce(t)
	before := f.stored(t, recording)
	require.NotNil(t, before.tuner)

	// 録画が始まった後に、同じ時刻の優先度の高い予約が 2 件増える。
	f.clock.Advance(80 * time.Minute)
	high := &reserve.Overrides{Priority: new(10)}
	others := []uuid.UUID{f.reserve(t, 1, high), f.reserve(t, 2, high)}
	result := f.runOnce(t)

	assert.Equal(t, before, f.stored(t, recording))
	assert.Len(t, result.NewConflicts, 1)
	assert.Contains(t, others, result.NewConflicts[0])
}

// TestRunRecomputesOnReservationChanges は、予約が変わると、その通知を受けて計算し直すことを確かめる。
func TestRunRecomputesOnReservationChanges(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	runner := assign.NewRunner(f.pool, f.clock, noGuard, slog.New(slog.DiscardHandler), assign.FixedDefaults(reserve.DefaultSettings()),
		&assign.RunnerOptions{Debounce: 10 * time.Millisecond, Interval: time.Hour, PollOnly: false, PollInterval: 0})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})

	// 最初の予約が割り当てられるのを待ち、始めたときの計算が終わったことを確かめる。
	first := f.reserve(t, 0, nil)
	require.Eventually(t, func() bool {
		return f.stored(t, first).state == string(assign.StateOK)
	}, 10*time.Second, 20*time.Millisecond)

	// 定期の計算は 1 時間後なので、次の予約が割り当てられるのは、予約の変更の通知を受けたときだけである。
	second := f.reserve(t, 1, nil)
	require.Eventually(t, func() bool {
		return f.stored(t, second).state == string(assign.StateOK)
	}, 10*time.Second, 20*time.Millisecond)
}
