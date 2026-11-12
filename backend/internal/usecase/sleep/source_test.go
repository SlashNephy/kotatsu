package sleep_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/sleep"
)

// TestDBSource は、録っている割り振り、動いている後処理のジョブ、録画する予定の予約と次の予約の開始を、DB から読むことを確かめる。
func TestDBSource(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	logger := slog.New(slog.DiscardHandler)
	require.NoError(t, database.Migrate(t.Context(), logger, pool))
	require.NoError(t, jobs.Migrate(t.Context(), logger, pool))
	now := time.Date(2026, 11, 12, 3, 0, 0, 0, time.UTC)
	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(t.Context(), sql, args...)
		require.NoError(t, err)
	}
	exec("INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	for _, sid := range []int{1024, 1025, 1026} {
		exec(`INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
			VALUES ($1, 32736, $2, 'テスト', 'テスト', 1, 'GR', '27', $3, $3)`, 3273600000+sid, sid, now)
	}
	// reserve は、録画する予定の時刻が plannedStart から plannedEnd の予約を足し、その ID を返す。
	reserve := func(enabled bool, state string, plannedStart, plannedEnd *time.Time) string {
		t.Helper()
		reservationID := id.New()
		exec(`INSERT INTO reservations (id, kind, service_id, event_id, start_at, end_at, name, description, extended, genres,
			enabled, follow_event_relay, settings, created_at, updated_at, assignment_state, planned_start_at, planned_end_at)
			VALUES ($1, 'time', 3273601024, NULL, $2, $3, '', '', '{}', '[]', $4, false, '{}', $2, $2, $5, $6, $7)`,
			reservationID, now.Add(-time.Hour), now.Add(3*time.Hour), enabled, state, plannedStart, plannedEnd)

		return reservationID.String()
	}
	// 録画する予定の時刻に入っている予約。1 つは mikan が録っていて、1 つは引き継ぎを待っていて、1 つはまだ取っていない。
	recording := reserve(true, "ok", new(now.Add(-time.Hour)), new(now.Add(time.Hour)))
	handover := reserve(true, "ok", new(now.Add(-time.Hour)), new(now.Add(time.Hour)))
	reserve(true, "conflict", new(now.Add(-time.Minute)), new(now.Add(time.Hour)))
	// 終わった予約、無効な予約、まだ計算していない予約は、どちらにも数えない。
	reserve(true, "ok", new(now.Add(-2*time.Hour)), new(now.Add(-time.Hour)))
	reserve(false, "ok", new(now.Add(-time.Minute)), new(now.Add(time.Hour)))
	reserve(false, "ok", new(now.Add(10*time.Minute)), new(now.Add(time.Hour)))
	reserve(true, "pending", nil, nil)
	// 次の予約は、開始マージンを前の録画に譲った後の、録画する予定の開始で見る。
	reserve(true, "partial", new(now.Add(2*time.Hour)), new(now.Add(3*time.Hour)))
	reserve(true, "ok", new(now.Add(30*time.Minute)), new(now.Add(time.Hour)))
	// 録っている割り振りと、引き継ぎ待ちの割り振りを数える。待っている割り振りと、終わった割り振りは数えない。
	for state, reservationID := range map[string]string{
		"recording": recording,
		"handover":  handover,
		"waiting":   reserve(true, "ok", new(now.Add(time.Hour)), new(now.Add(2*time.Hour))),
		"finished":  reserve(true, "ok", new(now.Add(-3*time.Hour)), new(now.Add(-2*time.Hour))),
	} {
		exec(`INSERT INTO recording_assignments (id, reservation_id, priority, state, generation, created_at, updated_at)
			VALUES ($1, $2, 2, $3, 1, $4, $4)`, id.New(), reservationID, state, now)
	}
	for sid, state := range map[int64]string{3273601024: "handover", 3273601025: "waiting", 3273601026: "recording"} {
		exec(`INSERT INTO timeshift_channels (service_id, retention_seconds, created_at, updated_at) VALUES ($1, 7200, $2, $2)`, sid, now)
		exec(`INSERT INTO timeshift_assignments (id, service_id, priority, state, generation, created_at, updated_at)
			VALUES ($1, $2, 0, $3, 1, $4, $4)`, id.New(), sid, state, now)
	}
	// 動いている後処理のジョブだけを数える。待っているジョブと、ほかの種類のジョブは数えない。
	exec(`INSERT INTO river_job (args, kind, max_attempts, queue, state) VALUES
		('{}', 'recording_transcode', 1, 'transcode', 'running'),
		('{}', 'recording_transcode', 1, 'transcode', 'available'),
		('{}', 'recording_cm_detect', 1, 'cm', 'running'),
		('{}', 'thumbnail_generate', 1, 'media', 'running')`)

	source := sleep.NewDBSource(pool, []string{"recording_transcode", "recording_cm_detect"}, func() int64 { return 3 })
	got, err := source.Activity(t.Context(), now)
	require.NoError(t, err)
	assert.Equal(t, &sleep.Activity{Recordings: 4, Encodings: 2, Viewers: 3, Airing: 3, NextStart: now.Add(30 * time.Minute)}, got)

	// 次の予約がなければ、ゼロにする。
	later, err := source.Activity(t.Context(), now.Add(5*time.Hour))
	require.NoError(t, err)
	assert.True(t, later.NextStart.IsZero())
}
