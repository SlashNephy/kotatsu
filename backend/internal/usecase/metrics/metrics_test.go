package metrics_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/metrics"
)

// TestDBCollector は、DB の録画、予約、割り振り、チューナー、保存先プール、ジョブの状態を、メトリクスとして返すことを確かめる (設計書 11.3)。
func TestDBCollector(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	logger := slog.New(slog.DiscardHandler)
	require.NoError(t, database.Migrate(t.Context(), logger, pool))
	require.NoError(t, jobs.Migrate(t.Context(), logger, pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	now := clk.Now()

	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(t.Context(), sql, args...)
		require.NoError(t, err)
	}
	exec("INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	exec(`INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES (3273601024, 32736, 1024, 'テスト', 'テスト', 1, 'GR', '27', $1, $1)`, now)
	recording := func(state string, reason *string, drops int) {
		t.Helper()
		exec(`INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
			state, failure_reason, protected, drops, errors, scrambled, recorded_from, created_at, updated_at)
			VALUES ($1, NULL, 3273601024, NULL, '', '', $2, $2, $3, $4, false, $5, 1, 0, $2, $2, $2)`, id.New(), now, state, reason, drops)
	}
	recording("recording", nil, 3)
	recording("recording", nil, 4)
	recording("completed", nil, 0)
	recording("failed", new("tuner_unavailable"), 0)
	recording("failed", new("cancelled"), 0)
	recording("failed", new("cancelled"), 0)
	// 失敗の累計 (#960)。マイグレーションが、理由ごとに 0 件の行を作っておく。録画を消しても減らないので、録画の数とは合わない。
	exec(`UPDATE recording_failure_counts SET count = 5, last_failed_at = $1 WHERE reason = 'cancelled'`, now.Add(-time.Hour))
	tuner := func(index int, available, fault bool) {
		t.Helper()
		exec(`INSERT INTO tuners (tuner_index, name, types, is_available, is_remote, is_fault, updated_at)
			VALUES ($1, 'tuner', '{GR}', $2, false, $3, $4)`, index, available, fault, now)
	}
	tuner(0, true, false)
	tuner(1, true, false)
	tuner(2, false, false)
	tuner(3, true, true)
	reservation := id.New()
	exec(`INSERT INTO reservations (id, kind, service_id, event_id, start_at, end_at, name, description, extended, genres,
		enabled, follow_event_relay, settings, created_at, updated_at)
		VALUES ($1, 'time', 3273601024, NULL, $2, $3, '', '', '{}', '[]', true, false, '{}', $2, $2)`, reservation, now, now.Add(time.Hour))
	// 予約。上の時刻予約は、まだ割り当てていない (pending)。一部のみ (partial) の予約はないので、0 件になる。
	reserve := func(enabled bool, state string, end time.Time, plannedEnd *time.Time) {
		t.Helper()
		exec(`INSERT INTO reservations (id, kind, service_id, event_id, start_at, end_at, name, description, extended, genres,
			enabled, follow_event_relay, settings, created_at, updated_at, assignment_state, planned_end_at)
			VALUES ($1, 'time', 3273601024, NULL, $2, $3, '', '', '{}', '[]', $4, false, '{}', $2, $2, $5, $6)`,
			id.New(), now.Add(-2*time.Hour), end, enabled, state, plannedEnd)
	}
	reserve(true, "ok", now.Add(time.Hour), new(now.Add(time.Hour)))
	reserve(true, "conflict", now.Add(time.Hour), new(now.Add(time.Hour)))
	// 予定の終わりは、マージンで番組の終わりより後になる。番組が終わっても、予定の終わりまでは数える。
	reserve(true, "conflict", now.Add(-time.Minute), new(now.Add(time.Minute)))
	// 無効な予約は、割り当ての結果が残っていても、無効として数える。
	reserve(false, "conflict", now.Add(time.Hour), new(now.Add(time.Hour)))
	// 予定の終わりが過ぎた予約は、猶予の間は表に残っていても数えない。無効な予約も同じ。
	reserve(true, "ok", now.Add(time.Hour), new(now.Add(-time.Minute)))
	reserve(true, "partial", now.Add(-time.Minute), new(now.Add(-time.Minute)))
	reserve(false, "pending", now.Add(-time.Minute), nil)
	exec(`INSERT INTO recording_assignments (id, reservation_id, priority, state, generation, created_at, updated_at)
		VALUES ($1, $2, 2, 'recording', 1, $3, $3)`, id.New(), reservation, now)
	// 保存先プール (#957)。測っていないプールは出さない。
	exec(`INSERT INTO storage_pools (id, priority, min_free_bytes, updated_at, total_bytes, free_bytes, measured_at)
		VALUES ('main', 0, 300, $1, 1000, 400, $2)`, now, now.Add(-time.Minute))
	exec(`INSERT INTO storage_pools (id, priority, min_free_bytes, updated_at) VALUES ('unmeasured', 0, 100, $1)`, now)
	exec(`INSERT INTO river_job (args, kind, max_attempts, queue, state) VALUES ('{}', 'test', 1, 'default', 'available'), ('{}', 'test', 1, 'default', 'available')`)

	collector := metrics.NewDBCollector(pool, clk, logger)
	const common = `
# HELP kotatsu_recordings Number of recordings by state.
# TYPE kotatsu_recordings gauge
kotatsu_recordings{state="completed"} 1
kotatsu_recordings{state="failed"} 3
kotatsu_recordings{state="recording"} 2
# HELP kotatsu_recording_failures Number of failed recordings by failure reason.
# TYPE kotatsu_recording_failures gauge
kotatsu_recording_failures{reason="cancelled"} 2
kotatsu_recording_failures{reason="tuner_unavailable"} 1
# HELP kotatsu_recording_failures_total Number of recordings that have failed, by failure reason. Deleting recordings does not decrease it.
# TYPE kotatsu_recording_failures_total counter
kotatsu_recording_failures_total{reason="cancelled"} 5
kotatsu_recording_failures_total{reason="program_not_found"} 0
kotatsu_recording_failures_total{reason="recorder_unavailable"} 0
kotatsu_recording_failures_total{reason="storage_full"} 0
kotatsu_recording_failures_total{reason="storage_unavailable"} 0
kotatsu_recording_failures_total{reason="stream_interrupted"} 0
kotatsu_recording_failures_total{reason="tuner_server_unreachable"} 0
kotatsu_recording_failures_total{reason="tuner_unavailable"} 0
# HELP kotatsu_recording_last_failure_timestamp_seconds Time when a recording last failed, by failure reason.
# TYPE kotatsu_recording_last_failure_timestamp_seconds gauge
kotatsu_recording_last_failure_timestamp_seconds{reason="cancelled"} 1.79163e+09
# HELP kotatsu_recording_active_packets Dropped, errored, and scrambled packets of the recordings in progress.
# TYPE kotatsu_recording_active_packets gauge
kotatsu_recording_active_packets{kind="drop"} 7
kotatsu_recording_active_packets{kind="error"} 2
kotatsu_recording_active_packets{kind="scrambled"} 0
# HELP kotatsu_reservations Number of reservations that have not ended, by assignment state, or disabled.
# TYPE kotatsu_reservations gauge
kotatsu_reservations{state="conflict"} 2
kotatsu_reservations{state="disabled"} 1
kotatsu_reservations{state="ok"} 1
kotatsu_reservations{state="partial"} 0
kotatsu_reservations{state="pending"} 1
# HELP kotatsu_recording_assignments Number of recording assignments by state.
# TYPE kotatsu_recording_assignments gauge
kotatsu_recording_assignments{state="recording"} 1
# HELP kotatsu_tuners Number of tuners of the tuner server by state.
# TYPE kotatsu_tuners gauge
kotatsu_tuners{state="available"} 2
kotatsu_tuners{state="fault"} 1
kotatsu_tuners{state="unavailable"} 1
# HELP kotatsu_storage_pool_bytes Size and available space of the storage pools, as last measured by the capacity check.
# TYPE kotatsu_storage_pool_bytes gauge
kotatsu_storage_pool_bytes{kind="available",pool="main"} 400
kotatsu_storage_pool_bytes{kind="total",pool="main"} 1000
# HELP kotatsu_storage_pool_threshold_bytes Free space that the capacity check keeps in the storage pools by deleting old recordings. 0 means it deletes nothing.
# TYPE kotatsu_storage_pool_threshold_bytes gauge
kotatsu_storage_pool_threshold_bytes{pool="main"} 300
# HELP kotatsu_storage_pool_last_measured_timestamp_seconds Time when the capacity check last measured the storage pools.
# TYPE kotatsu_storage_pool_last_measured_timestamp_seconds gauge
kotatsu_storage_pool_last_measured_timestamp_seconds{pool="main"} 1.79163354e+09
# HELP kotatsu_river_jobs Number of River jobs waiting or running, by queue and state.
# TYPE kotatsu_river_jobs gauge
kotatsu_river_jobs{queue="default",state="available"} 2
# HELP kotatsu_metrics_collection_errors Number of metric groups that could not be read from the database in this scrape.
# TYPE kotatsu_metrics_collection_errors gauge
kotatsu_metrics_collection_errors 0
`
	names := []string{
		"kotatsu_recordings", "kotatsu_recording_failures", "kotatsu_recording_failures_total", "kotatsu_recording_last_failure_timestamp_seconds",
		"kotatsu_recording_active_packets", "kotatsu_reservations", "kotatsu_recording_assignments", "kotatsu_tuners",
		"kotatsu_storage_pool_bytes", "kotatsu_storage_pool_threshold_bytes", "kotatsu_storage_pool_last_measured_timestamp_seconds", "kotatsu_river_jobs", "kotatsu_metrics_collection_errors",
	}
	require.NoError(t, testutil.CollectAndCompare(collector, strings.NewReader(common), names...))
	// neko のリースを持っているかは、インスタンスの値なので、クラスターの値を出す DBCollector では出さない (#962)。
	assert.Zero(t, testutil.CollectAndCount(collector, "kotatsu_neko_leader"))

	// 失敗の累計の作成の時刻は、どのインスタンスでも同じ値にする。リーダーが替わっても、OTLP の受け手がカウンターのリセットとみなさないようにする (#962)。
	registry := prometheus.NewPedanticRegistry()
	require.NoError(t, registry.Register(collector))
	families, err := registry.Gather()
	require.NoError(t, err)
	var created []time.Time
	for _, family := range families {
		if family.GetName() != "kotatsu_recording_failures_total" {
			continue
		}
		for _, m := range family.GetMetric() {
			created = append(created, m.GetCounter().GetCreatedTimestamp().AsTime())
		}
	}
	require.NotEmpty(t, created)
	for _, at := range created {
		assert.True(t, at.Equal(metrics.FailureCountsCreated()), at)
	}
}

// TestLeaderCollector は、このインスタンスが neko のリースを持っているかを、メトリクスとして返すことを確かめる。
func TestLeaderCollector(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	logger := slog.New(slog.DiscardHandler)
	require.NoError(t, database.Migrate(t.Context(), logger, pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	now := clk.Now()
	instanceID, other := id.New(), id.New()
	collector := metrics.NewLeaderCollector(pool, clk, instanceID, logger)
	leader := func(value string) string {
		return "# HELP kotatsu_neko_leader Whether this instance holds the neko lease (1) or not (0).\n" +
			"# TYPE kotatsu_neko_leader gauge\nkotatsu_neko_leader " + value + "\n"
	}

	tests := []struct {
		name   string
		holder *uuid.UUID
		want   string
	}{
		{name: "リースを持っていない", holder: nil, want: leader("0")},
		{name: "ほかのインスタンスが持っている", holder: &other, want: leader("0")},
		{name: "このインスタンスが持っている", holder: &instanceID, want: leader("1")},
	}
	for _, tt := range tests {
		// リースの行を変えるので、順に動かす。
		_, err := pool.Exec(t.Context(), "DELETE FROM leases")
		require.NoError(t, err)
		if tt.holder != nil {
			_, err = pool.Exec(t.Context(), `INSERT INTO leases (name, holder, generation, acquired_at, renewed_at, expires_at) VALUES ('neko', $1, 1, $2, $2, $3)`,
				*tt.holder, now, now.Add(time.Minute))
			require.NoError(t, err)
		}
		require.NoError(t, testutil.CollectAndCompare(collector, strings.NewReader(tt.want), "kotatsu_neko_leader"), tt.name)
	}
}
