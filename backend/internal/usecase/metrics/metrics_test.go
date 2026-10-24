package metrics_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/metrics"
)

// TestDBCollector は、DB の録画、割り振り、チューナー、リース、ジョブの状態を、メトリクスとして返すことを確かめる (設計書 11.3)。
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
	instanceID, other := id.New(), id.New()

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
	exec(`INSERT INTO recording_assignments (id, reservation_id, priority, state, generation, created_at, updated_at)
		VALUES ($1, $2, 2, 'recording', 1, $3, $3)`, id.New(), reservation, now)
	exec(`INSERT INTO river_job (args, kind, max_attempts, queue, state) VALUES ('{}', 'test', 1, 'default', 'available'), ('{}', 'test', 1, 'default', 'available')`)

	collector := metrics.NewDBCollector(pool, clk, instanceID, logger)
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
# HELP kotatsu_recording_active_packets Dropped, errored, and scrambled packets of the recordings in progress.
# TYPE kotatsu_recording_active_packets gauge
kotatsu_recording_active_packets{kind="drop"} 7
kotatsu_recording_active_packets{kind="error"} 2
kotatsu_recording_active_packets{kind="scrambled"} 0
# HELP kotatsu_recording_assignments Number of recording assignments by state.
# TYPE kotatsu_recording_assignments gauge
kotatsu_recording_assignments{state="recording"} 1
# HELP kotatsu_tuners Number of tuners of the tuner server by state.
# TYPE kotatsu_tuners gauge
kotatsu_tuners{state="available"} 2
kotatsu_tuners{state="fault"} 1
kotatsu_tuners{state="unavailable"} 1
# HELP kotatsu_river_jobs Number of River jobs waiting or running, by queue and state.
# TYPE kotatsu_river_jobs gauge
kotatsu_river_jobs{queue="default",state="available"} 2
# HELP kotatsu_metrics_collection_errors Number of metric groups that could not be read from the database in this scrape.
# TYPE kotatsu_metrics_collection_errors gauge
kotatsu_metrics_collection_errors 0
`
	names := []string{
		"kotatsu_recordings", "kotatsu_recording_failures", "kotatsu_recording_active_packets", "kotatsu_recording_assignments", "kotatsu_tuners",
		"kotatsu_river_jobs", "kotatsu_metrics_collection_errors", "kotatsu_neko_leader",
	}
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
		exec("DELETE FROM leases")
		if tt.holder != nil {
			exec(`INSERT INTO leases (name, holder, generation, acquired_at, renewed_at, expires_at) VALUES ('neko', $1, 1, $2, $2, $3)`,
				*tt.holder, now, now.Add(time.Minute))
		}
		require.NoError(t, testutil.CollectAndCompare(collector, strings.NewReader(common+tt.want), names...), tt.name)
	}
}
