package timeshift_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/assign"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/epg"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
	"github.com/SlashNephy/kotatsu/internal/usecase/timeshift"
	"github.com/SlashNephy/kotatsu/internal/usecase/tunerstate"
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

func noGuard(context.Context, pgx.Tx) error { return nil }

func channelVersion(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()

	var version int64
	require.NoError(t, pool.QueryRow(t.Context(),
		"SELECT coalesce((SELECT version FROM change_versions WHERE channel = $1), 0)", timeshift.NotifyChannel).Scan(&version))

	return version
}

func assignedServices(t *testing.T, pool *pgxpool.Pool) []int64 {
	t.Helper()

	rows, err := sqlc.New(pool).ListTimeshiftAssignments(t.Context())
	require.NoError(t, err)
	got := make([]int64, 0, len(rows))
	for _, row := range rows {
		assert.EqualValues(t, timeshift.Priority, row.Priority)
		got = append(got, row.ServiceID)
	}

	return got
}

type env struct {
	pool         *pgxpool.Pool
	clk          *clocktest.Fake
	reservations *reserve.Reservations
	assigner     *assign.Runner
	scheduler    *timeshift.Scheduler
	q            *sqlc.Queries
}

func newEnv(t *testing.T) *env {
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

	return &env{
		pool:         pool,
		clk:          clk,
		reservations: reserve.NewReservations(clk, audit.NewRecorder(clk)),
		assigner:     assign.NewRunner(pool, clk, noGuard, slog.New(slog.DiscardHandler), nil, nil),
		scheduler:    timeshift.NewScheduler(pool, clk, noGuard, slog.New(slog.DiscardHandler), nil),
		q:            sqlc.New(pool),
	}
}

func (e *env) configure(t *testing.T, service int) {
	t.Helper()

	require.NoError(t, e.q.UpsertTimeshiftChannel(t.Context(), sqlc.UpsertTimeshiftChannelParams{
		ServiceID: grService(service), RetentionSeconds: 7200, Now: e.clk.Now(), UpdatedBy: nil,
	}))
}

// reserve は、地上波の i 番目のサービスの、13:30 に始まる番組を予約し、チューナーを割り当てる。
func (e *env) reserve(t *testing.T, service int) {
	t.Helper()

	_, _, err := e.reservations.CreateProgram(t.Context(), e.pool, audit.System("test"), &reserve.ProgramInput{
		ServiceID: grService(service), EventID: 6, FollowEventRelay: true, Overrides: nil,
	})
	require.NoError(t, err)
	_, err = e.assigner.RunOnce(t.Context())
	require.NoError(t, err)
}

type step struct {
	name        string
	prepare     func()
	wantCreated int
	wantDeleted int
	wantChanged bool
	want        []int64
}

// runSteps は、各段階を順に流す。各段階は前の段階の結果に続くので、サブテストにしない。
func runSteps(t *testing.T, e *env, steps []step) {
	t.Helper()

	for _, st := range steps {
		st.prepare()
		before := channelVersion(t, e.pool)

		result, err := e.scheduler.RunOnce(t.Context())
		require.NoError(t, err, st.name)

		assert.Equal(t, st.wantCreated, result.Created, st.name)
		assert.Equal(t, st.wantDeleted, result.Deleted, st.name)
		assert.Equal(t, st.want, assignedServices(t, e.pool), st.name)
		if st.wantChanged {
			assert.Greater(t, channelVersion(t, e.pool), before, st.name)
		} else {
			assert.Equal(t, before, channelVersion(t, e.pool), st.name)
		}
	}
}

func TestSchedulerRunOnce(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	// 地上波のチューナー 2 本で、2 チャンネルを録る。チャンネルは番号の若い順に優先する。
	runSteps(t, e, []step{
		{
			name:        "チャンネルを設定すると、割り振りを書く",
			prepare:     func() { e.configure(t, 0); e.configure(t, 1) },
			wantCreated: 2, wantDeleted: 0, wantChanged: true,
			want: []int64{grService(0), grService(1)},
		},
		{
			name:        "変わらなければ、書き直さず、通知しない",
			prepare:     func() {},
			wantCreated: 0, wantDeleted: 0, wantChanged: false,
			want: []int64{grService(0), grService(1)},
		},
		{
			name:        "予約が割り振りの前に入るまでは、割り振りを消さない",
			prepare:     func() { e.reserve(t, 2) },
			wantCreated: 0, wantDeleted: 0, wantChanged: false,
			want: []int64{grService(0), grService(1)},
		},
		{
			// 録画の割り振りを書く 2 分前に、見直しの間隔の分だけ余裕を持たせて、先に外す。
			name:        "予約の開始が録画の割り振りの時刻の間隔の分前に入ると、並びの後ろのチャンネルの割り振りを消す",
			prepare:     func() { e.clk.Set(time.Date(2026, 10, 10, 13, 27, 55, 0, time.UTC)) },
			wantCreated: 0, wantDeleted: 1, wantChanged: true,
			want: []int64{grService(0)},
		},
		{
			name:        "予約の録画が始まってからも、チューナーは使われているので、外した割り振りを戻さない",
			prepare:     func() { e.clk.Set(time.Date(2026, 10, 10, 13, 35, 0, 0, time.UTC)) },
			wantCreated: 0, wantDeleted: 0, wantChanged: false,
			want: []int64{grService(0)},
		},
		{
			name: "チャンネルを設定から外すと割り振りを消し、空いたチューナーで残りのチャンネルを録る",
			prepare: func() {
				rows, err := e.q.DeleteTimeshiftChannel(t.Context(), grService(0))
				require.NoError(t, err)
				require.EqualValues(t, 1, rows)
			},
			wantCreated: 1, wantDeleted: 1, wantChanged: true,
			want: []int64{grService(1)},
		},
	})
}

func TestSchedulerIgnoresConflictedReservation(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	runSteps(t, e, []step{
		{
			name:        "チャンネルを設定すると、割り振りを書く",
			prepare:     func() { e.configure(t, 0); e.configure(t, 1) },
			wantCreated: 2, wantDeleted: 0, wantChanged: true,
			want: []int64{grService(0), grService(1)},
		},
		{
			// チューナーが割り当てられなかった予約は、チューナーを使わないので、タイムシフトを外さない。
			name: "競合した予約が割り振りの時間に入っても、割り振りを消さない",
			prepare: func() {
				e.reserve(t, 2)
				tag, err := e.pool.Exec(t.Context(), "UPDATE reservations SET assigned_tuner = NULL, assignment_state = 'conflict'")
				require.NoError(t, err)
				require.EqualValues(t, 1, tag.RowsAffected())
				e.clk.Set(time.Date(2026, 10, 10, 13, 29, 0, 0, time.UTC))
			},
			wantCreated: 0, wantDeleted: 0, wantChanged: false,
			want: []int64{grService(0), grService(1)},
		},
	})
}
