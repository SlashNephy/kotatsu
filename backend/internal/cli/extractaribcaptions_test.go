package cli_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
)

// stateCompleted は、録り終えた録画の状態。
const stateCompleted = "completed"

// TestAdminExtractARIBCaptions は、kotatsu admin extract-arib-captions が、ARIB 字幕の文を取り出していない録画と、
// 取り出し方の古いバージョンで取り出した録画だけを選び、--all では今のバージョンで取り出した録画も選ぶことと、
// 録画中の録画と、TS のセグメントのない録画を選ばないことと、待っているジョブの録画には重ねて投入しないことを確認する (#2218)。
// 取り出し方の今のバージョンは 1。
func TestAdminExtractARIBCaptions(t *testing.T) {
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

	base := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES (3273601024, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $1, $1)`, base)
	require.NoError(t, err)
	// 録画ごとに、状態、TS のセグメントがあるか、前に取り出したときの取り出し方のバージョン (0 なら取り出していない)。
	// fileOf は、録画のファイルの種類と大きさ。
	type fileOf struct {
		kind string
		size int
	}
	recordings := []struct {
		name    string
		state   string
		segment bool
		version int
		files   []fileOf
	}{
		{name: "取り出していない", state: stateCompleted, segment: true, version: 0},
		{name: "古いバージョン", state: stateCompleted, segment: true, version: -1},
		{name: "今のバージョン", state: stateCompleted, segment: true, version: 1},
		{name: "録画中", state: "recording", segment: true, version: 0},
		{name: "セグメントがない", state: stateCompleted, segment: false, version: 0},
		{name: "一部だけ録った", state: "partial", segment: true, version: 0},
		// 大きさが 0 のセグメントと、セグメントでないファイルだけを持つ録画は、読むものがないので選ばない。
		{name: "空のセグメントだけ", state: stateCompleted, segment: false, version: 0, files: []fileOf{{kind: "segment", size: 0}}},
		{name: "セグメントでないファイルだけ", state: stateCompleted, segment: false, version: 0, files: []fileOf{{kind: "other", size: 188}}},
	}
	ids := make([]string, len(recordings))
	for i, recording := range recordings {
		recordingID := id.New()
		ids[i] = recordingID.String()
		startAt := base.Add(time.Duration(i) * time.Hour)
		_, err := pool.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
			VALUES ($1, 3273601024, $2, '', $3, $3, $4, $3, $3, $3)`, recordingID, recording.name, startAt, recording.state)
		require.NoError(t, err)
		if recording.segment {
			_, err := pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
				VALUES ($1, $2, 'segment', 'main', $3, 188, $4, $4, $4)`, id.New(), recordingID, fmt.Sprintf("%d.m2ts", i), startAt)
			require.NoError(t, err)
		}
		for j, file := range recording.files {
			_, err := pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
				VALUES ($1, $2, $3, 'main', $4, $5, $6, $6, $6)`, id.New(), recordingID, file.kind, fmt.Sprintf("%d-%d.bin", i, j), file.size, startAt)
			require.NoError(t, err)
		}
		if recording.version != 0 {
			_, err := pool.Exec(t.Context(), `INSERT INTO recording_arib_captions (recording_id, extractor_version, extracted_at, cues, search_grams)
				VALUES ($1, $2, $3, '[]', '{}')`, recordingID, recording.version, startAt)
			require.NoError(t, err)
		}
	}
	line := func(i int, previous string) string {
		return fmt.Sprintf("%s\t%s\t%s\t%s\n", base.Add(time.Duration(i)*time.Hour).Format(time.RFC3339), ids[i], previous, recordings[i].name)
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "取り出していない録画と古いバージョンの録画を並べる",
			args: []string{flagDryRun},
			want: "3 recordings would be extracted by version 1\n" + line(0, "none") + line(1, "v-1") + line(5, "none"),
		},
		{
			name: "今のバージョンで取り出した録画も並べる",
			args: []string{flagDryRun, "--all"},
			want: "4 recordings would be extracted by version 1\n" + line(0, "none") + line(1, "v-1") + line(2, "v1") + line(5, "none"),
		},
		{name: "投入する", args: nil, want: "queued 3 of 3 recordings (0 were already queued)\n"},
		{name: "待っている録画には重ねない", args: nil, want: "queued 0 of 3 recordings (3 were already queued)\n"},
	}
	// 投入したジョブが残るので、順に動かす。
	for _, tt := range tests {
		var out bytes.Buffer
		cmd := cli.NewCommand(opts)
		cmd.SetArgs(append([]string{subcommandAdmin, "extract-arib-captions"}, tt.args...))
		cmd.SetOut(&out)
		require.NoError(t, cmd.ExecuteContext(t.Context()), tt.name)
		assert.Equal(t, tt.want, out.String(), tt.name)
	}

	// 新しく録った録画のジョブを待たせないよう、優先度を下げて (4)、録画を終えたときと同じキューに入れる。
	rows, err := pool.Query(t.Context(), "SELECT DISTINCT queue, priority FROM river_job WHERE kind = 'recording_arib_captions'")
	require.NoError(t, err)
	defer rows.Close()
	type placement struct {
		queue    string
		priority int
	}
	got := []placement{}
	for rows.Next() {
		var p placement
		require.NoError(t, rows.Scan(&p.queue, &p.priority))
		got = append(got, p)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []placement{{queue: "media", priority: 4}}, got)
}
