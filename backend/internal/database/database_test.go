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

	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
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
