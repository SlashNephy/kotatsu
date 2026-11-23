package database_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/db"
	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
)

// TestSchemaSumMatchesSchema は、schema.sql を変えたのに、スキーマの版 (schema.sum) を上げ忘れていないかを確かめる (#2040)。
// 落ちたら、mise run //backend:db:sum を実行する。
func TestSchemaSumMatchesSchema(t *testing.T) {
	t.Parallel()

	_, digest := db.SchemaSum()
	// Windows で取り出すと、改行が CRLF になる。mise run //backend:db:sum は LF のファイルから計算する。
	sum := sha256.Sum256([]byte(strings.ReplaceAll(db.Schema, "\r\n", "\n")))
	assert.Equal(t, hex.EncodeToString(sum[:]), digest, "schema.sql has changed; run mise run //backend:db:sum")
}

func TestMigrateAppliesToEmptyDatabase(t *testing.T) {
	t.Parallel()

	pool := open(t, databasetest.New(t))

	require.ErrorIs(t, database.CheckSchema(t.Context(), pool), database.ErrSchemaOutdated, "an empty database must be rejected")

	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	require.NoError(t, database.CheckSchema(t.Context(), pool))
	// 空の DB には書き換えるデータがないので、1 回だけのスクリプトは流さずに、流したものとして記録する。
	assert.Equal(t, scriptVersions(t), appliedScripts(t, pool))
	// 失敗の理由ごとに 0 件の行を作る。
	assert.Equal(t, len(domain.RecordingFailureReasonValues()), countRows(t, pool, "recording_failure_counts"))

	// 2 回目は、スキーマを変えない。
	var logs bytes.Buffer
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.NewTextHandler(&logs, nil)), pool))
	assert.NotContains(t, logs.String(), "schema change applied")
	require.NoError(t, database.CheckSchema(t.Context(), pool))
}

// TestMigrateFromGoose は、goose のマイグレーションで上げていた DB を、新しい仕組みへ移せることを確かめる (#2040)。
func TestMigrateFromGoose(t *testing.T) {
	t.Parallel()

	t.Run("keeps the goose version for the old binaries", func(t *testing.T) {
		t.Parallel()

		pool := open(t, databasetest.New(t))
		loadGooseFixture(t, pool)

		require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
		require.NoError(t, database.CheckSchema(t.Context(), pool))
		// ローリング更新の間に古いバイナリが読むので、goose の版は残す。
		var gooseVersion int64
		require.NoError(t, pool.QueryRow(t.Context(), "SELECT max(version_id) FROM goose_db_version").Scan(&gooseVersion))
		assert.Equal(t, database.GooseFloorVersion, gooseVersion)
	})

	t.Run("runs the one-time scripts newer than the goose version", func(t *testing.T) {
		t.Parallel()

		pool := open(t, databasetest.New(t))
		loadGooseFixture(t, pool)
		recordingID := insertRecordingWithDrops(t, pool, 3)

		require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
		require.NoError(t, database.CheckSchema(t.Context(), pool))
		// 判定に数えるドロップの列を足し、今までの録画を埋めるスクリプトを流した。
		var judgedDrops int64
		require.NoError(t, pool.QueryRow(t.Context(), "SELECT judged_drops FROM recordings WHERE id = $1", recordingID).Scan(&judgedDrops))
		assert.Equal(t, int64(3), judgedDrops)
		assert.Equal(t, scriptVersions(t), appliedScripts(t, pool))
	})

	t.Run("does not run the scripts that goose has run", func(t *testing.T) {
		t.Parallel()

		pool := open(t, databasetest.New(t))
		loadGooseFixture(t, pool)
		// goose の版より古いスクリプトは、goose のマイグレーションで流したものとみなす。流せば失敗する。
		fsys := withScript(t, fmt.Sprintf("scripts/%d_ran_by_goose.after.sql", database.GooseFloorVersion), "SELECT 1 / 0;")

		require.NoError(t, database.MigrateWithScripts(t.Context(), slog.New(slog.DiscardHandler), pool, fsys))
		assert.Contains(t, appliedScripts(t, pool), database.GooseFloorVersion)
	})

	t.Run("rejects a database older than v2026.11.5", func(t *testing.T) {
		t.Parallel()

		pool := open(t, databasetest.New(t))
		loadGooseFixture(t, pool)
		// v2026.11.5 の最後のマイグレーションを当てる前の DB にする。
		_, err := pool.Exec(t.Context(), "DELETE FROM goose_db_version WHERE version_id = $1", database.GooseFloorVersion)
		require.NoError(t, err)

		require.ErrorIs(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool), database.ErrDatabaseTooOld)
	})
}

// TestMigrateRefusesDestructiveChange は、表や列を消す差を当てずに止めることを確かめる (#2040)。
// 古いバイナリで kotatsu migrate を実行したときに、新しいバイナリが足した列を消さない。
func TestMigrateRefusesDestructiveChange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		alter string
	}{
		{name: "column", alter: "ALTER TABLE users ADD COLUMN added_by_newer_binary text"},
		{name: "table", alter: "CREATE TABLE added_by_newer_binary (id bigint PRIMARY KEY)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := open(t, databasetest.New(t))
			require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
			_, err := pool.Exec(t.Context(), tt.alter)
			require.NoError(t, err)

			require.ErrorIs(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool), database.ErrDestructiveChange)
			var exists bool
			require.NoError(t, pool.QueryRow(t.Context(), `SELECT EXISTS (
				SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND (table_name = 'added_by_newer_binary' OR column_name = 'added_by_newer_binary')
			)`).Scan(&exists))
			assert.True(t, exists)
		})
	}
}

// TestForgetScriptsAfter は、流していないことにしたスクリプトを、次の Migrate で流し直すことを確かめる (#2040)。
// バックアップを戻した後に、書き出したときの版より新しいスクリプトを、戻した行に流すのに使う。
func TestForgetScriptsAfter(t *testing.T) {
	t.Parallel()

	pool := open(t, databasetest.New(t))
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	recordingID := insertRecordingWithDrops(t, pool, 5)

	require.NoError(t, database.ForgetScriptsAfter(t.Context(), pool, database.GooseFloorVersion))
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	var judgedDrops int64
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT judged_drops FROM recordings WHERE id = $1", recordingID).Scan(&judgedDrops))
	assert.Equal(t, int64(5), judgedDrops)
	assert.Equal(t, scriptVersions(t), appliedScripts(t, pool))
}

// TestMigrateRunsBeforeScripts は、差を当てる前に流すスクリプトで、schema.sql から外した列を消してから差を当て、
// 流したことを記録して 2 度は流さないことを確かめる (#2040)。goose から移る DB でも、記録する表を先に作って記録する。
func TestMigrateRunsBeforeScripts(t *testing.T) {
	t.Parallel()

	const version = 29991231000000
	fsys := withScript(t, fmt.Sprintf("scripts/%d_drop_users_legacy.before.sql", version), "ALTER TABLE users DROP COLUMN legacy;")
	tests := []struct {
		name    string
		prepare func(t *testing.T, pool *pgxpool.Pool)
	}{
		{
			name: "migrated database",
			prepare: func(t *testing.T, pool *pgxpool.Pool) {
				t.Helper()
				require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
			},
		},
		{
			name: "goose database without the table of the scripts",
			prepare: func(t *testing.T, pool *pgxpool.Pool) {
				t.Helper()
				loadGooseFixture(t, pool)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := open(t, databasetest.New(t))
			tt.prepare(t, pool)
			// schema.sql にない列。消すスクリプトがなければ、消す差として止まる。
			_, err := pool.Exec(t.Context(), "ALTER TABLE users ADD COLUMN legacy text")
			require.NoError(t, err)
			require.ErrorIs(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool), database.ErrDestructiveChange)

			require.NoError(t, database.MigrateWithScripts(t.Context(), slog.New(slog.DiscardHandler), pool, fsys))
			require.NoError(t, database.CheckSchema(t.Context(), pool))
			assert.Contains(t, appliedScripts(t, pool), int64(version))
			// 2 度目は流さない (流せば、消した列がなくて失敗する)。
			require.NoError(t, database.MigrateWithScripts(t.Context(), slog.New(slog.DiscardHandler), pool, fsys))
		})
	}
}

// TestMigrateConcurrently は、kotatsu migrate を同時に実行しても、どれも失敗しないことを確かめる。
// Helm のフックの Job と手で実行した migrate、2 つの helm upgrade などで重なりうる。
func TestMigrateConcurrently(t *testing.T) {
	t.Parallel()

	cfg := databasetest.New(t)
	const runs = 4
	errs := make(chan error, runs)
	for range runs {
		pool := open(t, cfg)
		go func() {
			errs <- database.WithMigrationLock(t.Context(), slog.New(slog.DiscardHandler), pool, func(ctx context.Context) error {
				return database.Migrate(ctx, slog.New(slog.DiscardHandler), pool)
			})
		}()
	}
	for range runs {
		require.NoError(t, <-errs)
	}

	pool := open(t, cfg)
	require.NoError(t, database.CheckSchema(t.Context(), pool))
	assert.Equal(t, scriptVersions(t), appliedScripts(t, pool))
}

func TestCheckSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setup は、スキーマを当てたあとの DB を書き換える。
		setup   func(t *testing.T, pool *pgxpool.Pool)
		wantErr error
	}{
		{
			name:  "up to date",
			setup: func(*testing.T, *pgxpool.Pool) {},
		},
		{
			// ローリング更新で、新しいバイナリが migrate を実行したあとも古いバイナリは動き続ける。
			name: "database is one version ahead",
			setup: func(t *testing.T, pool *pgxpool.Pool) {
				t.Helper()
				setSchemaVersion(t, pool, database.SchemaVersion()+1)
			},
		},
		{
			name: "database is ahead and incompatible with this binary",
			setup: func(t *testing.T, pool *pgxpool.Pool) {
				t.Helper()
				setSchemaVersion(t, pool, database.SchemaVersion()+1)
				raiseMinBinaryVersion(t, pool, database.SchemaVersion()+1)
			},
			wantErr: database.ErrBinaryOutdated,
		},
		{
			name: "database is one version behind",
			setup: func(t *testing.T, pool *pgxpool.Pool) {
				t.Helper()
				_, err := pool.Exec(t.Context(), "UPDATE schema_compatibility SET schema_version = $1", database.SchemaVersion()-1)
				require.NoError(t, err)
			},
			wantErr: database.ErrSchemaOutdated,
		},
		{
			name: "schema version is not recorded",
			setup: func(t *testing.T, pool *pgxpool.Pool) {
				t.Helper()
				_, err := pool.Exec(t.Context(), "DELETE FROM schema_compatibility")
				require.NoError(t, err)
			},
			wantErr: database.ErrSchemaOutdated,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := open(t, databasetest.New(t))
			require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
			tt.setup(t, pool)

			err := database.CheckSchema(t.Context(), pool)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
		})
	}
}

func TestMigrateDoesNotLowerMinBinaryVersion(t *testing.T) {
	t.Parallel()

	pool := open(t, databasetest.New(t))
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	// 新しいバイナリが下限を上げたあとに、古いバイナリで migrate を実行した場合を再現する。
	raised := database.CompatibleSince + 1
	raiseMinBinaryVersion(t, pool, raised)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	got, err := sqlc.New(pool).GetSchemaCompatibility(t.Context())
	require.NoError(t, err)
	assert.Equal(t, raised, got.MinBinaryVersion)
}

// TestMigrateFillsRecordingsNormalized は、kotatsu migrate が、正規化した番組名と概要の列がない録画の列を埋め、
// 埋まっている録画は書き換えないことを確かめる (#1044)。
func TestMigrateFillsRecordingsNormalized(t *testing.T) {
	t.Parallel()

	pool := open(t, databasetest.New(t))
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	now := time.Now()
	_, err := pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES (3273601024, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $1, $1)`, now)
	require.NoError(t, err)
	// 1 回に埋める数より多い録画も、すべて埋める。
	_, err = pool.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
		SELECT gen_random_uuid(), 3273601024, 'ＡＢＣ' || i, '', $1, $1, 'completed', $1, $1, $1 FROM generate_series(1, 501) AS i`, now)
	require.NoError(t, err)

	tests := []struct {
		name                      string
		description               string
		nameNormalized            *string
		descriptionNormalized     *string
		wantNameNormalized        string
		wantDescriptionNormalized string
	}{
		{
			name: "ＴＯＫＹＯ　ＭＸ🈟", description: "第１話", nameNormalized: nil, descriptionNormalized: nil,
			wantNameNormalized: "TOKYO MX[新]", wantDescriptionNormalized: "第1話",
		},
		{
			name: "概要だけない", description: "ｺﾀﾂ", nameNormalized: new("概要だけない"), descriptionNormalized: nil,
			wantNameNormalized: "概要だけない", wantDescriptionNormalized: "コタツ",
		},
		{
			name: "埋まっている", description: "", nameNormalized: new("書き換えない"), descriptionNormalized: new("書き換えない"),
			wantNameNormalized: "書き換えない", wantDescriptionNormalized: "書き換えない",
		},
	}
	ids := make([]uuid.UUID, len(tests))
	for i, tt := range tests {
		ids[i] = id.New()
		_, err := pool.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, name_normalized, description_normalized,
			start_at, end_at, state, recorded_from, created_at, updated_at)
			VALUES ($1, 3273601024, $2, $3, $4, $5, $6, $6, 'completed', $6, $6, $6)`,
			ids[i], tt.name, tt.description, tt.nameNormalized, tt.descriptionNormalized, now)
		require.NoError(t, err)
	}

	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	var remaining int
	require.NoError(t, pool.QueryRow(t.Context(),
		"SELECT count(*) FROM recordings WHERE name_normalized IS NULL OR description_normalized IS NULL").Scan(&remaining))
	assert.Zero(t, remaining)
	var bulk string
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT name_normalized FROM recordings WHERE name = 'ＡＢＣ501'").Scan(&bulk))
	assert.Equal(t, "ABC501", bulk)
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotName, gotDescription string
			require.NoError(t, pool.QueryRow(t.Context(),
				"SELECT name_normalized, description_normalized FROM recordings WHERE id = $1", ids[i]).Scan(&gotName, &gotDescription))
			assert.Equal(t, tt.wantNameNormalized, gotName)
			assert.Equal(t, tt.wantDescriptionNormalized, gotDescription)
		})
	}
}

// TestMigrateFillsCapturesNormalized は、kotatsu migrate が、正規化した字幕の文の列がないキャプチャの列を埋め、
// 字幕が出ていなかったキャプチャには空を書き、埋まっているキャプチャは書き換えないことを確かめる (#2084)。
func TestMigrateFillsCapturesNormalized(t *testing.T) {
	t.Parallel()

	pool := open(t, databasetest.New(t))
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	now := time.Now()
	userID := id.New()
	_, err := pool.Exec(t.Context(), "INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES ($1, 'alice', 'Alice', 'member', $2, $2)", userID, now)
	require.NoError(t, err)
	// 1 回に埋める数より多いキャプチャも、すべて埋める。
	_, err = pool.Exec(t.Context(), `INSERT INTO captures (id, user_id, service_name, service_name_normalized, program_name_normalized, caption, broadcast_at, taken_at, size, created_at)
		SELECT gen_random_uuid(), $1, 'こたつテスト1', 'こたつテスト1', '', 'ＡＢＣ' || i, $2, $2, 1, $2 FROM generate_series(1, 501) AS i`, userID, now)
	require.NoError(t, err)

	tests := []struct {
		name              string
		caption           *string
		captionNormalized *string
		want              string
	}{
		{name: "字幕の文を正規化する", caption: new("ＫＯＴＡＴＳＵ山のふもとから"), captionNormalized: nil, want: "KOTATSU山のふもとから"},
		{name: "字幕が出ていなければ空にする", caption: nil, captionNormalized: nil, want: ""},
		{name: "埋まっていれば書き換えない", caption: new("けさの様子"), captionNormalized: new("埋めてある字幕"), want: "埋めてある字幕"},
	}
	ids := make([]uuid.UUID, len(tests))
	for i, tt := range tests {
		ids[i] = id.New()
		_, err := pool.Exec(t.Context(), `INSERT INTO captures (id, user_id, service_name, service_name_normalized, program_name_normalized,
			caption, caption_normalized, broadcast_at, taken_at, size, created_at)
			VALUES ($1, $2, 'こたつテスト1', 'こたつテスト1', '', $3, $4, $5, $5, 1, $5)`,
			ids[i], userID, tt.caption, tt.captionNormalized, now)
		require.NoError(t, err)
	}

	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	var remaining int
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM captures WHERE caption_normalized IS NULL").Scan(&remaining))
	assert.Zero(t, remaining)
	var bulk string
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT caption_normalized FROM captures WHERE caption = 'ＡＢＣ501'").Scan(&bulk))
	assert.Equal(t, "ABC501", bulk)
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got string
			require.NoError(t, pool.QueryRow(t.Context(), "SELECT caption_normalized FROM captures WHERE id = $1", ids[i]).Scan(&got))
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestMigrateFillsRecordingHistorySeries は、kotatsu migrate が、シリーズ名の列がない録画履歴の列を、番組名から読んで埋め、
// 埋まっている履歴は書き換えないことを確かめる (#1499)。
func TestMigrateFillsRecordingHistorySeries(t *testing.T) {
	t.Parallel()

	// filled は、埋まっている履歴のシリーズ名。番組名から読める値と違うものにして、書き換えないことを確かめる。
	const filled = "書き換えない"

	pool := open(t, databasetest.New(t))
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	now := time.Now()
	_, err := pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES (3273601024, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $1, $1)`, now)
	require.NoError(t, err)
	// 1 回に埋める数より多い履歴も、すべて埋める。
	_, err = pool.Exec(t.Context(), `INSERT INTO recording_history (id, service_id, title_key, name, start_at, end_at, created_at)
		SELECT gen_random_uuid(), 3273601024, '', 'ＡＢＣ探偵 ＃' || i, $1, $1, $1 FROM generate_series(1, 501) AS i`, now)
	require.NoError(t, err)

	tests := []struct {
		name   string
		series *string
		want   string
	}{
		{name: "[新]こたつ探偵 第２期 ＃１「消えた蜜柑」", series: nil, want: "こたつ探偵第2期"},
		// 話数だけの番組名は、シリーズ名を読めないので空で埋め、次の migrate で読み直さない。
		{name: "＃１", series: nil, want: ""},
		{name: "埋まっている #1", series: new(filled), want: filled},
	}
	ids := make([]uuid.UUID, len(tests))
	for i, tt := range tests {
		ids[i] = id.New()
		_, err := pool.Exec(t.Context(), `INSERT INTO recording_history (id, service_id, title_key, name, series, start_at, end_at, created_at)
			VALUES ($1, 3273601024, '', $2, $3, $4, $4, $4)`, ids[i], tt.name, tt.series, now)
		require.NoError(t, err)
	}

	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	var remaining int
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM recording_history WHERE series IS NULL").Scan(&remaining))
	assert.Zero(t, remaining)
	var bulk string
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT series FROM recording_history WHERE name = 'ＡＢＣ探偵 ＃501'").Scan(&bulk))
	assert.Equal(t, "abc探偵", bulk)
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got string
			require.NoError(t, pool.QueryRow(t.Context(), "SELECT series FROM recording_history WHERE id = $1", ids[i]).Scan(&got))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOpenScansTimeInUTCAndRoundTripsID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		pooler bool
	}{
		{name: "direct", pooler: false},
		{name: "pooler", pooler: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := databasetest.New(t)
			cfg.Pooler = tt.pooler
			pool := open(t, cfg)

			_, err := pool.Exec(t.Context(), "CREATE TABLE items (id uuid PRIMARY KEY, at timestamptz NOT NULL)")
			require.NoError(t, err)

			jst := time.FixedZone("JST", 9*60*60)
			at := time.Date(2026, 10, 9, 21, 0, 0, 123456000, jst)
			ids := []uuid.UUID{id.New(), id.New(), id.New()}
			for _, itemID := range slices.Backward(ids) {
				_, err := pool.Exec(t.Context(), "INSERT INTO items (id, at) VALUES ($1, $2)", itemID, at)
				require.NoError(t, err)
			}

			// 2 回目以降もプリペアドステートメントの扱いで失敗しないことを確かめるため、同じクエリを繰り返す。
			for range 2 {
				rows, err := pool.Query(t.Context(), "SELECT id, at FROM items ORDER BY id")
				require.NoError(t, err)
				type item struct {
					ID uuid.UUID
					At time.Time
				}
				got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[item])
				require.NoError(t, err)

				require.Len(t, got, len(ids))
				for i, g := range got {
					// UUIDv7 は生成した順に並ぶので、DB の並び順も生成した順になる。
					assert.Equal(t, ids[i], g.ID)
					assert.Equal(t, time.UTC, g.At.Location())
					assert.True(t, at.Equal(g.At), "want %s, got %s", at, g.At)
				}
			}
		})
	}
}

func TestOpenFailsWithoutLeakingPassword(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// 待ち受けのないポートに接続させる。
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword("user", "hunter2"),
		Host:     "127.0.0.1:1",
		Path:     "/kotatsu",
		RawQuery: "connect_timeout=1",
	}
	_, err := database.Open(ctx, &config.Database{URL: config.Secret(u.String()), Pooler: false})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "hunter2")
}

func open(t *testing.T, cfg *config.Database) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}

// setSchemaVersion は、このバイナリが知らない新しいスキーマの版が当たった状態を作る。
func setSchemaVersion(t *testing.T, pool *pgxpool.Pool, version int64) {
	t.Helper()

	_, err := pool.Exec(t.Context(), "UPDATE schema_compatibility SET schema_version = $1", version)
	require.NoError(t, err)
}

func raiseMinBinaryVersion(t *testing.T, pool *pgxpool.Pool, version int64) {
	t.Helper()

	require.NoError(t, sqlc.New(pool).RecordSchemaVersion(t.Context(), sqlc.RecordSchemaVersionParams{
		MinBinaryVersion: version,
		UpdatedAt:        time.Now(),
	}))
}

// loadGooseFixture は、v2026.11.5 の kotatsu migrate (goose のマイグレーション) で上げた DB を作る (testdata)。
func loadGooseFixture(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ddl, err := os.ReadFile("testdata/goose_v2026.11.5.sql")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), string(ddl))
	require.NoError(t, err)
}

// withScript は、埋め込んだ 1 回だけのスクリプトに、name のスクリプトを足した FS を返す。
func withScript(t *testing.T, name, body string) fstest.MapFS {
	t.Helper()

	fsys := fstest.MapFS{name: &fstest.MapFile{Data: []byte(body)}}
	names, err := fs.Glob(db.Scripts, "scripts/*.sql")
	require.NoError(t, err)
	for _, n := range names {
		data, err := fs.ReadFile(db.Scripts, n)
		require.NoError(t, err)
		fsys[n] = &fstest.MapFile{Data: data}
	}

	return fsys
}

// scriptVersions は、埋め込んだ 1 回だけのスクリプトの版を古い順に返す。
func scriptVersions(t *testing.T) []int64 {
	t.Helper()

	names, err := fs.Glob(db.Scripts, "scripts/*.sql")
	require.NoError(t, err)
	versions := make([]int64, 0, len(names))
	for _, name := range names {
		prefix, _, _ := strings.Cut(strings.TrimPrefix(name, "scripts/"), "_")
		version, err := strconv.ParseInt(prefix, 10, 64)
		require.NoError(t, err)
		versions = append(versions, version)
	}
	slices.Sort(versions)

	return versions
}

func appliedScripts(t *testing.T, pool *pgxpool.Pool) []int64 {
	t.Helper()

	versions, err := sqlc.New(pool).ListAppliedSchemaScripts(t.Context())
	require.NoError(t, err)

	return versions
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()

	var n int
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&n))

	return n
}

// insertRecordingWithDrops は、セグメントのない、drops 件のドロップのある、エラーありで終えた録画を入れる。
// 判定に数えるドロップの列 (judged_drops) を足す前の DB にも入れられるよう、その列は書かない。
func insertRecordingWithDrops(t *testing.T, pool *pgxpool.Pool, drops int64) uuid.UUID {
	t.Helper()

	now := time.Now()
	_, err := pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10) ON CONFLICT DO NOTHING")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES (3273601024, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $1, $1) ON CONFLICT DO NOTHING`, now)
	require.NoError(t, err)
	recordingID := id.New()
	_, err = pool.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at, drops)
		VALUES ($1, 3273601024, 'こたつの時間', '', $2, $2, 'completed_with_errors', $2, $2, $2, $3)`, recordingID, now, drops)
	require.NoError(t, err)

	return recordingID
}

// TestOpenLimitsIdleTransactionsAndStatements は、接続に、トランザクションの中で何もしない時間と、1 つの問い合わせの時間の
// 上限を設けることを確かめる (#527)。固まったトランザクションが、neko のリースを取れないまま残らないようにする。
// トランザクションプーリングを経由するときは、接続の開始のパラメーターを渡せないので、設けない。
func TestOpenLimitsIdleTransactionsAndStatements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		pooler        bool
		wantIdle      string
		wantStatement string
	}{
		{name: "direct", pooler: false, wantIdle: "1min", wantStatement: "5min"},
		{name: "pooler", pooler: true, wantIdle: "0", wantStatement: "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := databasetest.New(t)
			cfg.Pooler = tt.pooler
			pool, err := database.Open(t.Context(), cfg)
			require.NoError(t, err)
			t.Cleanup(pool.Close)

			var idle, statement string
			require.NoError(t, pool.QueryRow(t.Context(), "SELECT current_setting('idle_in_transaction_session_timeout'), current_setting('statement_timeout')").Scan(&idle, &statement))
			assert.Equal(t, tt.wantIdle, idle)
			assert.Equal(t, tt.wantStatement, statement)
		})
	}
}

// TestMigrateRereadsSeriesWithDashVariants は、波ダッシュとダッシュの書き方を揃える前に書いたシリーズ名のうち、揃える文字を含むものを、
// kotatsu migrate が読み直すことを確かめる (#2039)。録画履歴は番組名から読み直し、番組は次の全量の突き合わせで読み直すよう空に戻す。
func TestMigrateRereadsSeriesWithDashVariants(t *testing.T) {
	t.Parallel()

	// 揃える前のバージョン (v2026.11.5) の goose のマイグレーションで上げた DB から、新しい仕組みに移るときに、
	// 1 回だけのスクリプトで読み直す (#2040)。
	pool := open(t, databasetest.New(t))
	loadGooseFixture(t, pool)

	now := time.Now()
	_, err := pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES (3273601024, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $1, $1)`, now)
	require.NoError(t, err)

	histories := []struct {
		name, series, want string
	}{
		{name: "灯台の地図〜霧笛の夜〜　＃３", series: "灯台の地図〜霧笛の夜〜", want: "灯台の地図~霧笛の夜~"},
		{name: "灯台の地図－霧笛の夜−　＃３", series: "灯台の地図-霧笛の夜−", want: "灯台の地図-霧笛の夜-"},
		{name: "灯台の地図　ー霧笛の夜ー　＃３", series: "灯台の地図ー霧笛の夜ー", want: "灯台の地図-霧笛の夜-"},
		// 揃える文字を含まないシリーズ名は、書き換えない。
		{name: "灯台の地図　＃３", series: "揃える文字を含まない名前", want: "揃える文字を含まない名前"},
	}
	ids := make([]uuid.UUID, len(histories))
	for i, h := range histories {
		ids[i] = id.New()
		_, err := pool.Exec(t.Context(), `INSERT INTO recording_history (id, service_id, title_key, name, series, start_at, end_at, created_at)
			VALUES ($1, 3273601024, '', $2, $3, $4, $4, $4)`, ids[i], h.name, h.series, now)
		require.NoError(t, err)
	}
	programs := []struct {
		eventID   int64
		series    string
		wantReset bool
	}{
		{eventID: 1, series: "灯台の地図〜霧笛の夜〜", wantReset: true},
		{eventID: 2, series: "灯台の地図—霧笛の夜—", wantReset: true},
		{eventID: 3, series: "灯台の地図", wantReset: false},
	}
	for _, p := range programs {
		_, err := pool.Exec(t.Context(), `INSERT INTO programs (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
			name, description, name_normalized, description_normalized, extended, genres, related_items, series, episode_number, created_at, updated_at)
			VALUES (3273601024, $1, $1, $2, $2, false, true, 'テスト', '', 'テスト', '', '{}', '[]', '[]', $3, 3, $2, $2)`,
			p.eventID, now, p.series)
		require.NoError(t, err)
	}

	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	for i, h := range histories {
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()

			var got string
			require.NoError(t, pool.QueryRow(t.Context(), "SELECT series FROM recording_history WHERE id = $1", ids[i]).Scan(&got))
			assert.Equal(t, h.want, got)
		})
	}
	for _, p := range programs {
		t.Run(p.series, func(t *testing.T) {
			t.Parallel()

			var series *string
			var number *int32
			require.NoError(t, pool.QueryRow(t.Context(), "SELECT series, episode_number FROM programs WHERE event_id = $1", p.eventID).
				Scan(&series, &number))
			if p.wantReset {
				assert.Nil(t, series)
				assert.Nil(t, number)
			} else {
				assert.Equal(t, new(p.series), series)
				assert.Equal(t, new(int32(3)), number)
			}
		})
	}
}
