package cli_test

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
)

// TestAdminReanalyze は、kotatsu admin reanalyze が、生成物のない録画の解析のジョブを投入し、
// 待っているジョブの録画には重ねて投入しないことと、--dry-run では数えるだけのことを確かめる (#897)。
func TestAdminReanalyze(t *testing.T) {
	t.Parallel()

	dbConfig := databasetest.New(t)
	env := testEnv()
	env["KOTATSU_DATABASE_URL"] = dbConfig.URL.Value()
	env["KOTATSU_STORAGE_POOLS"] = "main=" + t.TempDir()
	env["KOTATSU_STORAGE_DATA_DIR"] = t.TempDir()
	opts := cli.DefaultOptions(slog.New(slog.DiscardHandler))
	opts.LookupEnv = func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	}
	migrateCmd := cli.NewCommand(opts)
	migrateCmd.SetArgs([]string{subcommandMigrate})
	require.NoError(t, migrateCmd.ExecuteContext(t.Context()))
	pool, err := database.Open(t.Context(), dbConfig)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	now := time.Now()
	recordingID := id.New()
	for _, sql := range []struct {
		query string
		args  []any
	}{
		{"INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)", nil},
		{`INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
			VALUES (3273601024, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $1, $1)`, []any{now}},
		{`INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
			VALUES ($1, 3273601024, '録画', '', $2, $2, 'completed', $2, $2, $2)`, []any{recordingID, now}},
		{`INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
			VALUES ($1, $2, 'segment', 'main', '録画.m2ts', 1, $3, $3, $3)`, []any{id.New(), recordingID, now}},
	} {
		_, err := pool.Exec(t.Context(), sql.query, sql.args...)
		require.NoError(t, err)
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "dry run", args: []string{"--dry-run"}, want: "1 of 1 recordings would be reanalyzed\n"},
		{name: "queue", args: nil, want: "queued 1 of 1 recordings (0 were already queued)\n"},
		{name: "already queued", args: nil, want: "queued 0 of 1 recordings (1 were already queued)\n"},
	}
	// 投入したジョブが残るので、順に動かす。
	for _, tt := range tests {
		var out bytes.Buffer
		cmd := cli.NewCommand(opts)
		cmd.SetArgs(append([]string{"admin", "reanalyze"}, tt.args...))
		cmd.SetOut(&out)
		require.NoError(t, cmd.ExecuteContext(t.Context()), tt.name)
		assert.Equal(t, tt.want, out.String(), tt.name)
	}
}
