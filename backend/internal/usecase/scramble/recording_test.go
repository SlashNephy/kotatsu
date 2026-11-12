package scramble_test

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/scramble"
)

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	return pool
}

func addUser(t *testing.T, pool *pgxpool.Pool, name, role string, disabled bool, now time.Time) uuid.UUID {
	t.Helper()

	userID := id.New()
	var disabledAt *time.Time
	if disabled {
		disabledAt = &now
	}
	_, err := pool.Exec(t.Context(), `INSERT INTO users (id, username, display_name, role, created_at, updated_at, disabled_at)
		VALUES ($1, $2, $2, $3, $4, $4, $5)`, userID, name, role, now, disabledAt)
	require.NoError(t, err)

	return userID
}

// alertEvents は、管理者への知らせのイベントの宛先を、記録した順に返す。
func alertEvents(t *testing.T, pool *pgxpool.Pool) [][]uuid.UUID {
	t.Helper()

	rows, err := pool.Query(t.Context(), "SELECT audience FROM notification_events WHERE kind = $1 ORDER BY occurred_at, id",
		string(domain.NotificationKindScrambledRecordings))
	require.NoError(t, err)
	audiences, err := pgx.CollectRows(rows, pgx.RowTo[[]uuid.UUID])
	require.NoError(t, err)

	return audiences
}

// TestAlert は、判定できた録画が続けてスクランブルされたままだったら、管理者にだけ、間隔を空けて知らせることを確かめる (設計書 6.2、#1517)。
func TestAlert(t *testing.T) {
	t.Parallel()

	scrambled := ts.RecordingScramble{Judged: true, Ratio: 1, Warning: true}
	decrypted := ts.RecordingScramble{Judged: true, Ratio: 0, Warning: false}
	unjudged := ts.RecordingScramble{Judged: false, Ratio: 0, Warning: false}
	type step struct {
		result ts.RecordingScramble
		// after は、この録画を終える前に進める時間。
		after time.Duration
	}
	tests := []struct {
		name  string
		steps []step
		// want は、知らせた数。
		want int
	}{
		{name: "one scrambled recording", steps: []step{{scrambled, 0}}, want: 0},
		{name: "two scrambled recordings in a row", steps: []step{{scrambled, 0}, {scrambled, time.Hour}}, want: 1},
		{name: "a decrypted recording resets the count", steps: []step{{scrambled, 0}, {decrypted, time.Hour}, {scrambled, time.Hour}}, want: 0},
		{name: "an unjudged recording keeps the count", steps: []step{{scrambled, 0}, {unjudged, time.Hour}, {scrambled, time.Hour}}, want: 1},
		{
			name:  "within the interval",
			steps: []step{{scrambled, 0}, {scrambled, time.Hour}, {scrambled, time.Hour}, {scrambled, scramble.AlertInterval - 2*time.Hour}},
			want:  1,
		},
		{
			name:  "after the interval",
			steps: []step{{scrambled, 0}, {scrambled, time.Hour}, {scrambled, scramble.AlertInterval}},
			want:  2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := newPool(t)
			clk := clocktest.NewFake(time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC))
			admin := addUser(t, pool, "admin", "admin", false, clk.Now())
			addUser(t, pool, "disabled", "admin", true, clk.Now())
			addUser(t, pool, "member", "member", false, clk.Now())
			for _, s := range tt.steps {
				clk.Advance(s.after)
				err := pgx.BeginFunc(t.Context(), pool, func(tx pgx.Tx) error {
					return scramble.Alert(t.Context(), tx, clk, s.result, map[string]any{"name": "番組"})
				})
				require.NoError(t, err)
			}

			audiences := alertEvents(t, pool)
			assert.Len(t, audiences, tt.want)
			for _, audience := range audiences {
				// 有効な管理者にだけ届ける。
				assert.Equal(t, []uuid.UUID{admin}, audience)
			}
		})
	}
}

// TestJudge は、セグメントの PID の集計から判定した結果を、録画に記録することを確かめる。
func TestJudge(t *testing.T) {
	t.Parallel()

	quality := func(scrambled int64) []byte {
		encoded, err := json.Marshal([]ts.PIDQuality{{
			PID: 0x0111, Kind: ts.PIDKindVideo, Packets: ts.MinRecordingScramblePackets, Drops: 0, Errors: 0, Scrambled: scrambled,
			Payloads: new(int64(ts.MinRecordingScramblePackets)), ScrambledPayloads: new(scrambled), StreamType: nil, ComponentTag: nil,
		}})
		require.NoError(t, err)

		return encoded
	}
	tests := []struct {
		name        string
		segments    [][]byte
		wantRatio   *float64
		wantWarning bool
	}{
		{name: "scrambled", segments: [][]byte{quality(ts.MinRecordingScramblePackets)}, wantRatio: new(1.0), wantWarning: true},
		{name: "decrypted", segments: [][]byte{quality(0)}, wantRatio: new(0.0), wantWarning: false},
		{name: "no PID stats", segments: [][]byte{nil}, wantRatio: nil, wantWarning: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := newPool(t)
			now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
			recordingID := id.New()
			_, err := pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
			require.NoError(t, err)
			_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
				VALUES (1, 1, 1, 'service', 'service', 1, 'GR', '27', $1, $1)`, now)
			require.NoError(t, err)
			_, err = pool.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from,
				created_at, updated_at, scramble_warning, scrambled_ratio)
				VALUES ($1, 1, 'name', '', $2, $2, 'completed', $2, $2, $2, true, 0.5)`, recordingID, now)
			require.NoError(t, err)
			for i, stats := range tt.segments {
				_, err := pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at,
					pid_stats, created_at) VALUES ($1, $2, 'segment', 'pool', $3, 1, $4, $4, $5, $4)`,
					id.New(), recordingID, string(rune('a'+i))+".ts", now, stats)
				require.NoError(t, err)
			}

			err = pgx.BeginFunc(t.Context(), pool, func(tx pgx.Tx) error {
				_, err := scramble.Judge(t.Context(), sqlc.New(tx), recordingID, now)

				return err
			})
			require.NoError(t, err)
			var warning bool
			var ratio *float64
			require.NoError(t, pool.QueryRow(t.Context(), "SELECT scramble_warning, scrambled_ratio FROM recordings WHERE id = $1", recordingID).
				Scan(&warning, &ratio))
			assert.Equal(t, tt.wantWarning, warning)
			assert.Equal(t, tt.wantRatio, ratio)
		})
	}
}
