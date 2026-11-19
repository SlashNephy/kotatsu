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

const (
	// subcommandAdmin と flagDryRun は、管理用のサブコマンドと、数えるだけにする引数。
	subcommandAdmin = "admin"
	flagDryRun      = "--dry-run"
	// detectorFFmpeg と detectorLogo は、CM の区間の見分け方の名前。
	detectorFFmpeg = "ffmpeg"
	detectorLogo   = "logo"
)

// TestAdminRedetectCM は、kotatsu admin redetect-cm が、最後に見分けた CM の区間が今の見分け方の古い版の録画だけを選び、
// --all-detectors ではほかの見分け方で見分けた録画も選ぶことと、待っているジョブの録画には重ねて投入しないことを確かめる (#1814)。
// 見分け方は、設定の行がないので、既定の FFmpegDetector (版 2) になる。
func TestAdminRedetectCM(t *testing.T) {
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

	base := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	// detection は、録画の CM の区間を見分けた見分け方と版。
	type detection struct {
		detector string
		version  int
	}
	// 録画ごとに、見分けた見分け方と版を、見分けた順に並べる。
	recordings := []struct {
		name       string
		detections []detection
		material   bool
	}{
		{name: "古い版", detections: []detection{{detectorFFmpeg, 1}}, material: true},
		{name: "今の版", detections: []detection{{detectorFFmpeg, 2}}},
		{name: "ほかの見分け方", detections: []detection{{detectorLogo, 1}}},
		// 古い版の後に、ほかの見分け方で見分け直した。最後の結果はほかの見分け方のもの。
		{name: "見分け方を替えた", detections: []detection{{detectorFFmpeg, 1}, {detectorLogo, 1}}},
	}
	ids := make([]string, len(recordings))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES (3273601024, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $1, $1)`, base)
	require.NoError(t, err)
	for i, recording := range recordings {
		recordingID := id.New()
		ids[i] = recordingID.String()
		startAt := base.Add(time.Duration(i) * time.Hour)
		_, err := pool.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
			VALUES ($1, 3273601024, $2, '', $3, $3, 'completed', $3, $3, $3)`, recordingID, recording.name, startAt)
		require.NoError(t, err)
		for j, detection := range recording.detections {
			_, err := pool.Exec(t.Context(), `INSERT INTO recording_cm_detections (recording_id, detector, detector_version, detected_at)
				VALUES ($1, $2, $3, $4)`, recordingID, detection.detector, detection.version, startAt.Add(time.Duration(j+1)*time.Minute))
			require.NoError(t, err)
		}
		if recording.material {
			// 今の形式の版 (5) の材料。
			_, err := pool.Exec(t.Context(), `INSERT INTO recording_cm_materials (recording_id, detector, format_version, data, created_at)
				VALUES ($1, 'ffmpeg', 5, '\x00', $2)`, recordingID, startAt)
			require.NoError(t, err)
		}
	}
	line := func(i int, detector string, version int, how string) string {
		return fmt.Sprintf("%s\t%s\t%s v%d\t%s\t%s\n",
			base.Add(time.Duration(i)*time.Hour).Format(time.RFC3339), ids[i], detector, version, how, recordings[i].name)
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "古い版だけを並べる",
			args: []string{flagDryRun},
			want: "1 recordings would be redetected by ffmpeg version 2 (1 from the materials, 0 by reading the recordings)\n" +
				line(0, detectorFFmpeg, 1, "materials"),
		},
		{
			name: "ほかの見分け方で最後に見分けた録画も並べる",
			args: []string{flagDryRun, "--all-detectors"},
			want: "3 recordings would be redetected by ffmpeg version 2 (1 from the materials, 2 by reading the recordings)\n" +
				line(0, detectorFFmpeg, 1, "materials") + line(2, detectorLogo, 1, "read") + line(3, detectorLogo, 1, "read"),
		},
		{name: "投入する", args: nil, want: "queued 1 of 1 recordings (0 were already queued)\n"},
		{name: "待っている録画には重ねない", args: nil, want: "queued 0 of 1 recordings (1 were already queued)\n"},
	}
	// 投入したジョブが残るので、順に動かす。
	for _, tt := range tests {
		var out bytes.Buffer
		cmd := cli.NewCommand(opts)
		cmd.SetArgs(append([]string{subcommandAdmin, "redetect-cm"}, tt.args...))
		cmd.SetOut(&out)
		require.NoError(t, cmd.ExecuteContext(t.Context()), tt.name)
		assert.Equal(t, tt.want, out.String(), tt.name)
	}
}

// TestAdminRedetectCMWithLogo は、管理者がロゴの見分け方を選んでいれば、その版で古さを決めることと、
// 材料が今の形式の版でなければ、読み直す録画として数えることを確かめる (#1814)。ロゴの見分け方の今の版は 5、材料の形式の今の版は 5。
func TestAdminRedetectCMWithLogo(t *testing.T) {
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

	base := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	for _, sql := range []string{
		"INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)",
		`INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
			VALUES (3273601024, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', '2026-11-15T12:00:00Z', '2026-11-15T12:00:00Z')`,
		`INSERT INTO system_settings (id, recording_defaults, archive_retention_days, cm_detector, updated_at)
			VALUES (1, '{}', 0, 'logo', '2026-11-15T12:00:00Z')`,
	} {
		_, err := pool.Exec(t.Context(), sql)
		require.NoError(t, err)
	}
	// 録画ごとに、ロゴで見分けた版と、残した材料の形式の版 (0 なら材料なし)。
	recordings := []struct {
		name           string
		version        int
		materialFormat int
	}{
		{name: "今の形式の材料", version: 3, materialFormat: 5},
		{name: "古い形式の材料", version: 3, materialFormat: 4},
		{name: "今の版", version: 5, materialFormat: 5},
	}
	ids := make([]string, len(recordings))
	for i, recording := range recordings {
		recordingID := id.New()
		ids[i] = recordingID.String()
		startAt := base.Add(time.Duration(i) * time.Hour)
		_, err := pool.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
			VALUES ($1, 3273601024, $2, '', $3, $3, 'completed', $3, $3, $3)`, recordingID, recording.name, startAt)
		require.NoError(t, err)
		_, err = pool.Exec(t.Context(), `INSERT INTO recording_cm_detections (recording_id, detector, detector_version, detected_at)
			VALUES ($1, $2, $3, $4)`, recordingID, detectorLogo, recording.version, startAt)
		require.NoError(t, err)
		if recording.materialFormat != 0 {
			_, err := pool.Exec(t.Context(), `INSERT INTO recording_cm_materials (recording_id, detector, format_version, data, created_at)
				VALUES ($1, $2, $3, '\x00', $4)`, recordingID, detectorLogo, recording.materialFormat, startAt)
			require.NoError(t, err)
		}
	}

	var out bytes.Buffer
	cmd := cli.NewCommand(opts)
	cmd.SetArgs([]string{subcommandAdmin, "redetect-cm", flagDryRun})
	cmd.SetOut(&out)
	require.NoError(t, cmd.ExecuteContext(t.Context()))
	assert.Equal(t,
		"2 recordings would be redetected by logo version 5 (1 from the materials, 1 by reading the recordings)\n"+
			fmt.Sprintf("%s\t%s\tlogo v3\tmaterials\t今の形式の材料\n", base.Format(time.RFC3339), ids[0])+
			fmt.Sprintf("%s\t%s\tlogo v3\tread\t古い形式の材料\n", base.Add(time.Hour).Format(time.RFC3339), ids[1]),
		out.String())
}
