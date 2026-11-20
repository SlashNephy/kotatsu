package timeshift_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
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

const (
	grNetworkID = 0x7FE0
	// bonDriverA は、EDCB のチューナーの名前 (BonDriver)。
	bonDriverA = "BonDriver_A.so"
)

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

	return newEnvWith(t, nil)
}

// newEnvWith は、タイムシフトの割り振りの設定を opts にした env を返す。
func newEnvWith(t *testing.T, opts *timeshift.SchedulerOptions) *env {
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
		scheduler:    timeshift.NewScheduler(pool, clk, noGuard, slog.New(slog.DiscardHandler), opts),
		q:            sqlc.New(pool),
	}
}

func (e *env) configure(t *testing.T, service int) {
	t.Helper()

	e.configureService(t, grService(service))
}

// configureService は、サービス serviceID をタイムシフトで録るチャンネルにする。
func (e *env) configureService(t *testing.T, serviceID int64) {
	t.Helper()

	require.NoError(t, e.q.UpsertTimeshiftChannel(t.Context(), sqlc.UpsertTimeshiftChannelParams{
		ServiceID: serviceID, RetentionSeconds: 7200, Now: e.clk.Now(), UpdatedBy: nil,
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
	// 地上波のチューナー 2 本と 3 チャンネルで、2 チャンネルを録る。チャンネルは番号の若い順に優先する。
	// タイムシフトで録らない地上波のチャンネルが残るうちは、番組表の取得にチューナーを 1 本残す。
	runSteps(t, e, []step{
		{
			name:        "チャンネルを設定すると、番組表の取得に 1 本残して、割り振りを書く",
			prepare:     func() { e.configure(t, 0); e.configure(t, 1) },
			wantCreated: 1, wantDeleted: 0, wantChanged: true,
			want: []int64{grService(0)},
		},
		{
			name:        "変わらなければ、書き直さず、通知しない",
			prepare:     func() {},
			wantCreated: 0, wantDeleted: 0, wantChanged: false,
			want: []int64{grService(0)},
		},
		{
			name:        "予約が割り振りの前に入るまでは、割り振りを消さない",
			prepare:     func() { e.reserve(t, 2) },
			wantCreated: 0, wantDeleted: 0, wantChanged: false,
			want: []int64{grService(0)},
		},
		{
			// 録画の割り振りを書く 2 分前に、見直しの間隔の分だけ余裕を持たせて、先に外す。
			// 予約が 1 本を使い、残る 1 本は、録らない 2 番目のチャンネルの番組表の取得に残す。
			name:        "予約の開始が録画の割り振りの時刻の間隔の分前に入ると、割り振りを消す",
			prepare:     func() { e.clk.Set(time.Date(2026, 10, 10, 13, 27, 55, 0, time.UTC)) },
			wantCreated: 0, wantDeleted: 1, wantChanged: true,
			want: []int64{},
		},
		{
			name:        "予約の録画が始まってからも、チューナーは使われているので、外した割り振りを戻さない",
			prepare:     func() { e.clk.Set(time.Date(2026, 10, 10, 13, 35, 0, 0, time.UTC)) },
			wantCreated: 0, wantDeleted: 0, wantChanged: false,
			want: []int64{},
		},
		{
			name:        "予約の録画が終わると、空いたチューナーで録り直す",
			prepare:     func() { e.clk.Set(time.Date(2026, 10, 10, 14, 30, 0, 0, time.UTC)) },
			wantCreated: 1, wantDeleted: 0, wantChanged: true,
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

// TestSchedulerIgnoresGoneServiceChannels は、上流から消えて 24 時間たったサービスのチャンネルは、番組表の取得に
// チューナーを残す理由にしないことを確かめる (#1973)。チューナーサーバーは、消えたサービスの番組表を取らない。
// 地上波のチューナー 2 本と 3 チャンネルで、2 チャンネルを録る。残る 3 番目のチャンネルのサービスを、上流から消えたことにする。
func TestSchedulerIgnoresGoneServiceChannels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		goneFor time.Duration
		want    []int64
	}{
		{name: "消えて 24 時間たったら、残さずに 2 本とも使う", goneFor: 25 * time.Hour, want: []int64{grService(0), grService(1)}},
		{name: "消えて 24 時間たつまでは、番組表の取得に 1 本残す", goneFor: 23 * time.Hour, want: []int64{grService(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newEnv(t)
			e.configure(t, 0)
			e.configure(t, 1)
			_, err := e.pool.Exec(t.Context(), "UPDATE services SET gone_at = $2 WHERE id = $1", grService(2), e.clk.Now().Add(-tt.goneFor))
			require.NoError(t, err)

			_, err = e.scheduler.RunOnce(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tt.want, assignedServices(t, e.pool))
		})
	}
}

func TestSchedulerIgnoresConflictedReservation(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	runSteps(t, e, []step{
		{
			name:        "チャンネルを設定すると、番組表の取得に 1 本残して、割り振りを書く",
			prepare:     func() { e.configure(t, 0); e.configure(t, 1) },
			wantCreated: 1, wantDeleted: 0, wantChanged: true,
			want: []int64{grService(0)},
		},
		{
			// チューナーが割り当てられなかった予約は、チューナーを使わないので、タイムシフトを外さない。
			// 競合した予約は、チューナーサーバーで優先度により空きチューナーかタイムシフトのチューナーを取る (設計書 5.7)。
			name: "競合した予約が割り振りの時間に入っても、割り振りを消さない",
			prepare: func() {
				e.reserve(t, 2)
				tag, err := e.pool.Exec(t.Context(), "UPDATE reservations SET assigned_tuner = NULL, assignment_state = 'conflict'")
				require.NoError(t, err)
				require.EqualValues(t, 1, tag.RowsAffected())
				e.clk.Set(time.Date(2026, 10, 10, 13, 29, 0, 0, time.UTC))
			},
			wantCreated: 0, wantDeleted: 0, wantChanged: false,
			want: []int64{grService(0)},
		},
	})
}

// TestSchedulerSkipsOccupiedTuners は、チューナーサーバー自身の予約 (EDCB の予約) が、今から先回りの時間までに使うチューナーを、
// タイムシフトに使わないことを確かめる (#1536)。地上波のチューナーは 2 本で、1 本は番組表の取得に残す。
func TestSchedulerSkipsOccupiedTuners(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	occupy := func(from, to time.Time) {
		_, err := e.pool.Exec(t.Context(), "DELETE FROM tuner_occupancies")
		require.NoError(t, err)
		_, err = e.pool.Exec(t.Context(), "INSERT INTO tuner_occupancies (tuner_index, start_at, end_at) SELECT min(tuner_index), $1, $2 FROM tuners", from, to)
		require.NoError(t, err)
	}
	runSteps(t, e, []step{
		{
			name:        "1 本を番組表の取得に残して、割り振りを書く",
			prepare:     func() { e.configure(t, 0) },
			wantCreated: 1, wantDeleted: 0, wantChanged: true,
			want: []int64{grService(0)},
		},
		{
			// 残る 1 本は番組表の取得に残すので、タイムシフトを外す。
			name:        "EDCB の予約が今チューナーを使うなら、そのチューナーを使わない",
			prepare:     func() { occupy(e.clk.Now().Add(-time.Minute), e.clk.Now().Add(time.Hour)) },
			wantCreated: 0, wantDeleted: 1, wantChanged: true,
			want: []int64{},
		},
		{
			name:        "EDCB の予約が先回りの時間より後なら、チューナーを使う",
			prepare:     func() { occupy(e.clk.Now().Add(time.Hour), e.clk.Now().Add(2*time.Hour)) },
			wantCreated: 1, wantDeleted: 0, wantChanged: true,
			want: []int64{grService(0)},
		},
	})
}

// TestSchedulerReplacesFinishedAssignment は、終わったことになったタイムシフトの割り振りを、ないものとして扱うことを確かめる。
// 終わった割り振りはどの mikan も取らないので、残すとそのチャンネルを録れなくなる (#1055)。
func TestSchedulerReplacesFinishedAssignment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// configured は、終わった割り振りのチャンネルを、タイムシフトで録るよう設定しておくかどうか。
		configured  bool
		wantCreated int
		wantDeleted int
		want        []int64
	}{
		{name: "録るチャンネルなら、消して書き直す", configured: true, wantCreated: 1, wantDeleted: 1, want: []int64{grService(0)}},
		{name: "設定のないチャンネルなら、消す", configured: false, wantCreated: 0, wantDeleted: 1, want: []int64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newEnv(t)
			if tt.configured {
				e.configure(t, 0)
			}
			finishedID := id.New()
			require.NoError(t, e.q.InsertTimeshiftAssignment(t.Context(), sqlc.InsertTimeshiftAssignmentParams{
				ID: finishedID, ServiceID: grService(0), Priority: timeshift.Priority, Now: e.clk.Now(),
			}))
			tag, err := e.pool.Exec(t.Context(), "UPDATE timeshift_assignments SET state = 'finished' WHERE id = $1", finishedID)
			require.NoError(t, err)
			require.EqualValues(t, 1, tag.RowsAffected())
			before := channelVersion(t, e.pool)

			result, err := e.scheduler.RunOnce(t.Context())
			require.NoError(t, err)

			assert.Equal(t, tt.wantCreated, result.Created)
			assert.Equal(t, tt.wantDeleted, result.Deleted)
			assert.Greater(t, channelVersion(t, e.pool), before, "mikan is notified")
			rows, err := e.q.ListTimeshiftAssignments(t.Context())
			require.NoError(t, err)
			got := make([]int64, 0, len(rows))
			for _, row := range rows {
				got = append(got, row.ServiceID)
				assert.NotEqual(t, finishedID, row.ID, "the finished assignment is gone")
				assert.Equal(t, domain.RecordingAssignmentStateWaiting, row.State, "a new assignment is waiting for mikan")
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// fakeCapture は、EPG の取得の時刻の前後を from から to までとする、偽の EPG の取得の設定。
type fakeCapture struct {
	known      bool
	from, to   time.Time
	bonDrivers map[string]tunerserver.EPGCaptureBonDriver
	err        error
}

func (f *fakeCapture) EPGCapture(_ context.Context, at time.Time) (*tunerserver.EPGCapture, error) {
	if f.err != nil {
		return nil, f.err
	}

	return &tunerserver.EPGCapture{
		Known: f.known, Near: !at.Before(f.from) && at.Before(f.to), AnyBonDriver: false, BonDrivers: f.bonDrivers,
		DefaultBonDriver: tunerserver.EPGCaptureBonDriver{Disabled: false, Tuners: 0},
	}, nil
}

var _ tunerserver.EPGCaptureReporter = new(fakeCapture)

// unknownCapture は、EpgTimerSrv.ini を読めず、EPG の取得の時刻も数も分からない EDCB の設定。
func unknownCapture() *fakeCapture {
	return &fakeCapture{known: false, from: time.Time{}, to: time.Time{}, bonDrivers: map[string]tunerserver.EPGCaptureBonDriver{}, err: nil}
}

// edcbOptions は、EDCB のときのタイムシフトの割り振りの設定 (#1856)。
func edcbOptions(capture tunerserver.EPGCaptureReporter) *timeshift.SchedulerOptions {
	return &timeshift.SchedulerOptions{Interval: 0, Lead: 0, PollOnly: false, PollInterval: 0, ShareByService: true, EPGCapture: capture}
}

// TestSchedulerShareUnit は、予約と同じチャンネルのサービスを、予約の録画と相乗りさせるかを確かめる。
// Mirakurun 互換のサーバーではチャンネルが、EDCB ではサービスが相乗りの単位になる (#1856)。
// 偽チューナーサーバーの BS と CS のチューナーは故障にし、同じ BonDriver の地上波のチューナー 2 本だけにする。1 番目のチャンネルには、サービスが 2 つある。
func TestSchedulerShareUnit(t *testing.T) {
	t.Parallel()

	main := grService(0)
	sub := faketuner.ServiceItemID(grNetworkID, 0x0401)
	tests := []struct {
		name    string
		opts    *timeshift.SchedulerOptions
		service int64
		// wantBefore は、予約が先回りの時間に入る前に録るサービス。wantDuring は、入った後に録るサービス。
		wantBefore []int64
		wantDuring []int64
	}{
		{
			name: "Mirakurun では、予約と同じサービスを、チューナーを使わずに録る", opts: nil, service: main,
			wantBefore: []int64{main}, wantDuring: []int64{main},
		},
		{
			name: "Mirakurun では、予約と同じチャンネルの別のサービスも、チューナーを使わずに録る", opts: nil, service: sub,
			wantBefore: []int64{sub}, wantDuring: []int64{sub},
		},
		{
			name: "EDCB では、予約と同じサービスを、チューナーを使わずに録る", opts: edcbOptions(unknownCapture()), service: main,
			wantBefore: []int64{main}, wantDuring: []int64{main},
		},
		{
			// 予約が 1 本を使うと、空きチューナーは 1 本だけになり、EPG の取得に残す。
			name: "EDCB では、予約と同じチャンネルの別のサービスは、空きチューナーを使う", opts: edcbOptions(unknownCapture()), service: sub,
			wantBefore: []int64{sub}, wantDuring: []int64{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newEnvWith(t, tt.opts)
			// EDCB のチューナーは、同じ BonDriver の名前を持つ。
			_, err := e.pool.Exec(t.Context(), "UPDATE tuners SET name = $1, is_fault = ('BS' = ANY(types))", bonDriverA)
			require.NoError(t, err)
			e.configureService(t, tt.service)
			e.reserve(t, 0)
			_, err = e.scheduler.RunOnce(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tt.wantBefore, assignedServices(t, e.pool), "before the reservation")

			e.clk.Set(time.Date(2026, 10, 10, 13, 29, 0, 0, time.UTC))
			_, err = e.scheduler.RunOnce(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tt.wantDuring, assignedServices(t, e.pool), "during the reservation")
		})
	}
}

// TestSchedulerKeepsIdleTuner は、EDCB で EpgTimerSrv.ini を読めないとき、EPG の取得に、種別と録られるチャンネルを問わずに、
// BonDriver ごとに空きチューナーを 1 本、時刻を問わずに残すことを確かめる (#1856)。
// BonDriver は、EDCB のチューナーの名前で分かる。偽チューナーサーバーのチューナーは、地上波の 2 本と、BS と CS の 1 本。地上波の 3 チャンネルを録る。
func TestSchedulerKeepsIdleTuner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// grBonDriver と bsBonDriver は、地上波のチューナーと、BS と CS のチューナーの名前 (BonDriver)。
		grBonDriver, bsBonDriver string
		// noBS は、BS と CS のチューナーを故障にするかどうか。
		noBS bool
		want []int64
	}{
		{
			name:        "同じ BonDriver の BS と CS のチューナーが空いていれば、地上波のチューナーを 2 本とも使う",
			grBonDriver: bonDriverA, bsBonDriver: bonDriverA,
			want: []int64{grService(0), grService(1)},
		},
		{
			name:        "地上波のチューナーしかなければ、1 本を残す",
			grBonDriver: bonDriverA, bsBonDriver: bonDriverA, noBS: true,
			want: []int64{grService(0)},
		},
		{
			name:        "ほかの BonDriver のチューナーが空いていても、地上波の BonDriver の 1 本を残す",
			grBonDriver: bonDriverA, bsBonDriver: "BonDriver_B.so",
			want: []int64{grService(0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newEnvWith(t, edcbOptions(unknownCapture()))
			for i := range 3 {
				e.configure(t, i)
			}
			_, err := e.pool.Exec(t.Context(), "UPDATE tuners SET name = CASE WHEN 'BS' = ANY(types) THEN $2 ELSE $1 END, is_fault = ('BS' = ANY(types) AND $3)",
				tt.grBonDriver, tt.bsBonDriver, tt.noBS)
			require.NoError(t, err)

			_, err = e.scheduler.RunOnce(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tt.want, assignedServices(t, e.pool))
		})
	}
}

// TestSchedulerEPGCaptureWindow は、EDCB で、EPG の取得の時刻の前後だけ、BonDriver ごとに EPGCount の数の空きチューナーを残し、
// 前後を過ぎたら戻すことを確かめる (#1856)。偽チューナーサーバーの地上波のチューナー 2 本 (同じ BonDriver) で、地上波の 3 チャンネルを録る。
// BS と CS のチューナーは別の BonDriver。取得の時刻の前後は 12:20 から 12:27 まで。
func TestSchedulerEPGCaptureWindow(t *testing.T) {
	t.Parallel()

	at := func(minute int) time.Time { return time.Date(2026, 10, 10, 12, minute, 0, 0, time.UTC) }
	type step struct {
		at   time.Time
		want []int64
	}
	both := []int64{grService(0), grService(1)}
	tests := []struct {
		name    string
		capture *fakeCapture
		steps   []step
	}{
		{
			name: "EPGCount が 1 なら、取得の時刻の前後だけ 1 本を残す",
			capture: &fakeCapture{
				known: true, from: at(20), to: at(27), err: nil,
				bonDrivers: map[string]tunerserver.EPGCaptureBonDriver{strings.ToLower(bonDriverA): {Disabled: false, Tuners: 1}},
			},
			steps: []step{{at: at(10), want: both}, {at: at(20), want: []int64{grService(0)}}, {at: at(27), want: both}},
		},
		{
			// EPGCount の既定 (0) は、BonDriver のすべてのチューナーで取得する。
			name:    "EPGCount が 0 なら、取得の時刻の前後はすべて空ける",
			capture: &fakeCapture{known: true, from: at(20), to: at(27), bonDrivers: map[string]tunerserver.EPGCaptureBonDriver{}, err: nil},
			steps:   []step{{at: at(10), want: both}, {at: at(20), want: []int64{}}, {at: at(27), want: both}},
		},
		{
			name:    "EpgTimerSrv.ini を読めなければ、いつも 1 本を残す",
			capture: unknownCapture(),
			steps:   []step{{at: at(10), want: []int64{grService(0)}}, {at: at(20), want: []int64{grService(0)}}},
		},
		{
			name: "EDCB に届かなければ、読めないときと同じく 1 本を残す",
			capture: &fakeCapture{
				known: true, from: at(20), to: at(27), bonDrivers: map[string]tunerserver.EPGCaptureBonDriver{}, err: errors.New("unreachable"),
			},
			steps: []step{{at: at(10), want: []int64{grService(0)}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newEnvWith(t, edcbOptions(tt.capture))
			// 地上波のチューナーは bonDriverA で、BS と CS のチューナーは別の BonDriver。
			_, err := e.pool.Exec(t.Context(), "UPDATE tuners SET name = CASE WHEN 'BS' = ANY(types) THEN 'BonDriver_B.so' ELSE $1 END", bonDriverA)
			require.NoError(t, err)
			for i := range 3 {
				e.configure(t, i)
			}
			for _, st := range tt.steps {
				e.clk.Set(st.at)
				_, err := e.scheduler.RunOnce(t.Context())
				require.NoError(t, err)
				assert.Equal(t, st.want, assignedServices(t, e.pool), st.at)
			}
		})
	}
}
