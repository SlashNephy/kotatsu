package cli_test

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/epgstation/epgstationtest"
)

// epgstationSeed は、2026-10-10 21:00 (JST) を今として、取り込む物と取り込まない物を一通り入れる。
func epgstationSeed() []string {
	return []string{
		`INSERT INTO recorded_tag (id, name, halfWidthName, color) VALUES (1, 'アニメ', 'アニメ', '#ff0000')`,
		`INSERT INTO rule (id, keyword, parentDirectoryName) VALUES (1, 'ニュース', 'anime')`,
		// 番組予約、時刻予約、開始済みの番組予約。
		`INSERT INTO reserve (id, updateTime, programId, channelId, channel, channelType, startAt, endAt)
		VALUES (1, 1, 1, 3273601024, '27', 'GR', 1791637200000, 1791639000000)`,
		`INSERT INTO reserve (id, updateTime, isTimeSpecified, channelId, channel, channelType, startAt, endAt)
		VALUES (2, 1, 1, 3273601024, '27', 'GR', 1791637200000, 1791639000000)`,
		`INSERT INTO reserve (id, updateTime, programId, channelId, channel, channelType, startAt, endAt)
		VALUES (3, 1, 2, 3273601024, '27', 'GR', 1791630000000, 1791631800000)`,
		`INSERT INTO drop_log_file (id, errorCnt, dropCnt, scramblingCnt, filePath) VALUES (1, 0, 0, 0, '1.log')`,
		`INSERT INTO recorded (id, channelId, startAt, endAt, duration, name, halfWidthName, isRecording, dropLogFileId)
		VALUES (1, 3273601024, 1791626400000, 1791628200000, 1800000, 'a', 'a', 0, 1)`,
		`INSERT INTO recorded (id, channelId, startAt, endAt, duration, name, halfWidthName, isRecording)
		VALUES (2, 3273601024, 1791633000000, 1791634800000, 1800000, 'b', 'b', 1)`,
		`INSERT INTO video_file (id, parentDirectoryName, filePath, type, name, size, recordedId)
		VALUES (1, 'recorded', 'a.m2ts', 'ts', 'TS', 1, 1), (2, 'encoded', 'a.mp4', 'encoded', 'H.264', 1, 1)`,
		`INSERT INTO recorded_history (id, name, channelId, endAt) VALUES (1, 'a', 3273601024, 1791628200000)`,
	}
}

// poolFlag は、録画の保存先ディレクトリにプールを対応付けるフラグ。
const (
	poolFlag = "--pool"
	// mapRecordedToMain は、EPGStation の既定の保存先ディレクトリをプール main に対応付ける。
	mapRecordedToMain = "recorded=main"
)

func TestImportEPGStationDryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr string
	}{
		{
			name: "lists directories without a pool",
			args: []string{poolFlag, mapRecordedToMain},
			want: `EPGStation import plan (dry run)

Rules                      1
Manual reserves (program)  1
Manual reserves (time)     1
Recordings                 1
  TS files                 1
  Other files              0
  Drop logs                1
Recorded histories         1
Tags                       1

Skipped
  Manual reserves already started  1
  Recordings in progress           1

Recorded directories without a storage pool (map them with --pool NAME=POOL_ID)
  NAME     FILES  RULES  RESERVES
  anime    0      1      0
  encoded  1      0      0
`,
			wantErr: "",
		},
		{
			name: "every directory is mapped",
			args: []string{poolFlag, mapRecordedToMain, poolFlag, "encoded=main", poolFlag, "anime=archive"},
			want: `EPGStation import plan (dry run)

Rules                      1
Manual reserves (program)  1
Manual reserves (time)     1
Recordings                 1
  TS files                 1
  Other files              1
  Drop logs                1
Recorded histories         1
Tags                       1

Skipped
  Manual reserves already started  1
  Recordings in progress           1
`,
			wantErr: "",
		},
		{
			name:    "pool is not configured",
			args:    []string{poolFlag, "recorded=missing"},
			want:    "",
			wantErr: `invalid --pool "recorded=missing": pool "missing" is not configured (configured pools: archive, main)`,
		},
		{
			name:    "pool mapping without a pool ID",
			args:    []string{poolFlag, "recorded"},
			want:    "",
			wantErr: `invalid --pool "recorded": must be NAME=POOL_ID`,
		},
		{
			name:    "directory is mapped twice",
			args:    []string{poolFlag, mapRecordedToMain, poolFlag, "recorded=archive"},
			want:    "",
			wantErr: `invalid --pool "recorded=archive": directory "recorded" is mapped more than once`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := epgstationtest.New(t, epgstationtest.SQLite)
			epgstationtest.Exec(t, db.Writer, epgstationSeed()...)

			var out bytes.Buffer
			cmd := cli.NewCommand(importOptions(t))
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"import", "epgstation", "--dry-run", "--source", db.URL}, tt.args...))
			err := cmd.ExecuteContext(t.Context())
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, out.String())
		})
	}
}

func TestImportEPGStationRequiresDryRun(t *testing.T) {
	t.Parallel()

	db := epgstationtest.New(t, epgstationtest.SQLite)
	cmd := cli.NewCommand(importOptions(t))
	cmd.SetArgs([]string{"import", "epgstation", "--source", db.URL})
	require.EqualError(t, cmd.ExecuteContext(t.Context()), "importing is not implemented yet; run with --dry-run to see the plan")
}

// importOptions は、保存先プールを 2 つ設定し、今を 2026-10-10 21:00 (JST) にした Options を返す。
func importOptions(t *testing.T) *cli.Options {
	t.Helper()

	env := testEnv()
	env["KOTATSU_STORAGE_POOLS"] = "main=" + t.TempDir() + ",archive=" + t.TempDir()
	opts := testOptions(slog.New(slog.DiscardHandler), cli.DefaultRoles(), func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	})
	opts.Clock = clocktest.NewFake(time.UnixMilli(1791633600000))

	return opts
}
