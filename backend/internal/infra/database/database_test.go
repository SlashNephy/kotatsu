package database_test

import (
	"context"
	"log/slog"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
)

func TestCompatibleSinceIsEmbeddedMigration(t *testing.T) {
	t.Parallel()

	// 下限を上げ忘れた、あるいは存在しないバージョンを書いた、という誤りを検出する。
	assert.Contains(t, database.MigrationVersions(), database.CompatibleSince)
	assert.LessOrEqual(t, database.CompatibleSince, database.LatestVersion())
}

func TestMigrateAppliesToEmptyDatabase(t *testing.T) {
	t.Parallel()

	pool := open(t, databasetest.New(t))

	require.ErrorIs(t, database.CheckSchema(t.Context(), pool), database.ErrSchemaOutdated, "an empty database must be rejected")

	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	require.NoError(t, database.CheckSchema(t.Context(), pool))

	// 2 回目は何も適用しない。
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	require.NoError(t, database.CheckSchema(t.Context(), pool))

	var applied int
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM goose_db_version WHERE version_id > 0").Scan(&applied))
	assert.Equal(t, len(database.MigrationVersions()), applied)
}

// TestMigrateConcurrently は、kotatsu migrate を同時に実行しても、どれも失敗せず、マイグレーションを一度ずつ適用することを確かめる。
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
	var applied int
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM goose_db_version WHERE version_id > 0").Scan(&applied))
	assert.Equal(t, len(database.MigrationVersions()), applied)
}

func TestCheckSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setup は、マイグレーションを適用したあとの DB を書き換える。
		setup   func(t *testing.T, pool *pgxpool.Pool)
		wantErr error
	}{
		{
			name:  "up to date",
			setup: func(*testing.T, *pgxpool.Pool) {},
		},
		{
			// ローリング更新で、新しいバイナリが migrate を実行したあとも古いバイナリは動き続ける。
			name: "database is one migration ahead",
			setup: func(t *testing.T, pool *pgxpool.Pool) {
				t.Helper()
				applyFakeMigration(t, pool, database.LatestVersion()+1)
			},
		},
		{
			name: "database is ahead and incompatible with this binary",
			setup: func(t *testing.T, pool *pgxpool.Pool) {
				t.Helper()
				applyFakeMigration(t, pool, database.LatestVersion()+1)
				raiseMinBinaryVersion(t, pool, database.LatestVersion()+1)
			},
			wantErr: database.ErrBinaryOutdated,
		},
		{
			name: "migration was rolled back",
			setup: func(t *testing.T, pool *pgxpool.Pool) {
				t.Helper()
				_, err := pool.Exec(t.Context(), "DELETE FROM goose_db_version WHERE version_id = $1", database.LatestVersion())
				require.NoError(t, err)
			},
			wantErr: database.ErrSchemaOutdated,
		},
		{
			name: "compatible binary version is not recorded",
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

	got, err := sqlc.New(pool).GetMinBinaryVersion(t.Context())
	require.NoError(t, err)
	assert.Equal(t, raised, got)
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

// applyFakeMigration は、このバイナリが知らない新しいマイグレーションが適用された状態を作る。
func applyFakeMigration(t *testing.T, pool *pgxpool.Pool, version int64) {
	t.Helper()

	_, err := pool.Exec(t.Context(), "INSERT INTO goose_db_version (version_id, is_applied) VALUES ($1, true)", version)
	require.NoError(t, err)
}

func raiseMinBinaryVersion(t *testing.T, pool *pgxpool.Pool, version int64) {
	t.Helper()

	require.NoError(t, sqlc.New(pool).RaiseMinBinaryVersion(t.Context(), sqlc.RaiseMinBinaryVersionParams{
		MinBinaryVersion: version,
		UpdatedAt:        time.Now(),
	}))
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
