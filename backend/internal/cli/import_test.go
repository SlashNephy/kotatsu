package cli_test

import (
	"bytes"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/epgstationtest"
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
	// sourceFlag は、EPGStation の DB の URL を指定するフラグ。
	sourceFlag = "--source"
	// mapRecordedToMain は、EPGStation の既定の保存先ディレクトリをプール main に対応付ける。
	mapRecordedToMain = "recorded=main"
)

// importArgs は、kotatsu import epgstation の引数を返す。
func importArgs(args ...string) []string {
	return append([]string{"import", "epgstation"}, args...)
}

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
			cmd.SetArgs(importArgs(append([]string{"--dry-run", sourceFlag, db.URL}, tt.args...)...))
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

func TestImportEPGStationRequiresUser(t *testing.T) {
	t.Parallel()

	db := epgstationtest.New(t, epgstationtest.SQLite)
	cmd := cli.NewCommand(importOptions(t))
	cmd.SetArgs(importArgs(sourceFlag, db.URL))
	require.EqualError(t, cmd.ExecuteContext(t.Context()), "--user is required to import; run with --dry-run to only see the plan")
}

// TestImportEPGStation は、取り込みの結果を表にして出し、2 度目の取り込みでは前に取り込んだものを飛ばすことを確かめる。
func TestImportEPGStation(t *testing.T) {
	t.Parallel()

	first := `EPGStation import result

KIND                IMPORTED  ALREADY IMPORTED  SKIPPED
Rules               1         0                 0
Manual reserves     1         0                 2
Recordings          1         0                 1
Recorded histories  1         0                 0

Skipped
  KIND             ID  REASON
  Manual reserves  1   program not found: service 0, event 1
  Manual reserves  3   the reservation has already started
  Recordings       2   the program is still being recorded
`
	second := `EPGStation import result

KIND                IMPORTED  ALREADY IMPORTED  SKIPPED
Rules               0         1                 0
Manual reserves     0         1                 2
Recordings          0         1                 1
Recorded histories  0         1                 0

Skipped
  KIND             ID  REASON
  Manual reserves  1   program not found: service 0, event 1
  Manual reserves  3   the reservation has already started
  Recordings       2   the program is still being recorded
`
	tests := []struct {
		name     string
		services bool
		username string
		want     []string
		wantErr  string
	}{
		{name: "imports and then skips what was imported", services: true, username: adminUsername, want: []string{first, second}, wantErr: ""},
		{
			name: "channels are not synced yet", services: false, username: adminUsername, want: []string{},
			wantErr: "kotatsu has no services yet; start kotatsu and let it sync the channels from the tuner server before importing",
		},
		{name: "unknown user", services: true, username: "nobody", want: []string{}, wantErr: "user not found: nobody"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source := epgstationtest.New(t, epgstationtest.SQLite)
			epgstationtest.Exec(t, source.Writer, epgstationSeed()...)
			opts, pool := importDatabase(t, tt.services)
			run := func() (string, error) {
				var out bytes.Buffer
				cmd := cli.NewCommand(opts)
				cmd.SetOut(&out)
				cmd.SetArgs(importArgs(sourceFlag, source.URL, poolFlag, mapRecordedToMain, "--user", tt.username))
				err := cmd.ExecuteContext(t.Context())

				return out.String(), err
			}
			if tt.wantErr != "" {
				_, err := run()
				require.EqualError(t, err, tt.wantErr)
				var imported int
				require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM import_sources").Scan(&imported))
				assert.Zero(t, imported)

				return
			}
			for _, want := range tt.want {
				out, err := run()
				require.NoError(t, err)
				assert.Equal(t, want, out)
				// 2 度目の取り込みでも、取り込んだ記録は増えない。
				var imported int
				require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM import_sources").Scan(&imported))
				assert.Equal(t, 4, imported)
			}
		})
	}
}

// importDatabase は、マイグレーションした Kotatsu の DB と、ユーザー adminUsername を用意する。
// services なら、EPGStation のテストのデータが参照するサービスも用意する。
func importDatabase(t *testing.T, services bool) (*cli.Options, *pgxpool.Pool) {
	t.Helper()

	dbConfig := databasetest.New(t)
	opts := importOptions(t)
	env := testEnv()
	env["KOTATSU_DATABASE_URL"] = dbConfig.URL.Value()
	env["KOTATSU_STORAGE_POOLS"] = "main=" + t.TempDir() + ",archive=" + t.TempDir()
	env["KOTATSU_STORAGE_DATA_DIR"] = t.TempDir()
	defaults := cli.DefaultOptions(slog.New(slog.DiscardHandler))
	opts.OpenDatabase, opts.Migrate = defaults.OpenDatabase, defaults.Migrate
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
	_, err = auth.NewUsers(opts.Clock).Create(t.Context(), pool, &auth.NewUser{
		Username: adminUsername, DisplayName: adminUsername, Role: auth.RoleAdmin, PasswordHash: nil,
	})
	require.NoError(t, err)
	if services {
		_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
		require.NoError(t, err)
		_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
			VALUES (3273601024, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $1, $1)`, opts.Clock.Now())
		require.NoError(t, err)
	}

	return opts, pool
}

// importOptions は、保存先プールを 2 つ設定し、今を 2026-10-10 21:00 (JST) にした Options を返す。
func importOptions(t *testing.T) *cli.Options {
	t.Helper()

	env := testEnv()
	env["KOTATSU_STORAGE_POOLS"] = "main=" + t.TempDir() + ",archive=" + t.TempDir()
	env["KOTATSU_STORAGE_DATA_DIR"] = t.TempDir()
	opts := testOptions(slog.New(slog.DiscardHandler), cli.DefaultRoles(), func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	})
	opts.Clock = clocktest.NewFake(time.UnixMilli(1791633600000))

	return opts
}

func TestSourceWithPassword(t *testing.T) {
	t.Parallel()

	const (
		envName = "KOTATSU_IMPORT_SOURCE_PASSWORD"
		mariadb = "mysql://epgstation@mariadb:3306/epgstation"
	)
	// withUserinfo は、URL の userinfo にパスワードを持つ取り込み元を作る。
	withUserinfo := (&url.URL{Scheme: "mysql", User: url.UserPassword("epgstation", "old"), Host: "mariadb:3306", Path: "/epgstation"}).String()

	tests := []struct {
		name        string
		source      string
		env         map[string]string
		wantSame    bool
		wantDecoded string
		wantErr     bool
	}{
		{name: "環境変数がなければそのまま", source: mariadb, env: map[string]string{}, wantSame: true},
		{name: "空の環境変数は無視する", source: mariadb, env: map[string]string{envName: ""}, wantSame: true},
		{name: "記号を含むパスワードを入れる", source: mariadb + "?parseTime=true", env: map[string]string{envName: "p@ss/w:rd#?"}, wantDecoded: "p@ss/w:rd#?"},
		{name: "URL にもパスワードがあれば拒む", source: withUserinfo, env: map[string]string{envName: "new"}, wantErr: true},
		{name: "ユーザー名がなければ拒む", source: "mysql://mariadb:3306/epgstation", env: map[string]string{envName: "x"}, wantErr: true},
		{name: "ユーザー名が空なら拒む", source: "mysql://@mariadb:3306/epgstation", env: map[string]string{envName: "x"}, wantErr: true},
		{name: "URL として読めなければ拒む", source: "mysql://epgstation@mariadb:3306/epg\x7fstation", env: map[string]string{envName: "x"}, wantErr: true},
		{name: "SQLite では使わない", source: "sqlite:///data/database.db", env: map[string]string{envName: "x"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := cli.SourceWithPassword(tt.source, tt.env)
			if tt.wantErr {
				require.ErrorIs(t, err, cli.ErrSourcePassword)

				return
			}
			require.NoError(t, err)
			if tt.wantSame {
				assert.Equal(t, tt.source, got)

				return
			}
			// 記号はエスケープされ、URL として読み直すと元のパスワードに戻る。ほかの部分は変わらない。
			u, err := url.Parse(got)
			require.NoError(t, err)
			password, ok := u.User.Password()
			assert.True(t, ok)
			assert.Equal(t, tt.wantDecoded, password)
			assert.Equal(t, "epgstation", u.User.Username())
			assert.Equal(t, "mariadb:3306", u.Host)
			assert.Equal(t, "/epgstation", u.Path)
			assert.Equal(t, "parseTime=true", u.RawQuery)
		})
	}
}
