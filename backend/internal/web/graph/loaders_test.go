package graph_test

import (
	"log/slog"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

// TestLoaderFailsEveryKeyWhenTheQueryFails は、まとめて引くクエリが失敗したとき、まとめたすべてのキーにエラーを返すことを確かめる。
func TestLoaderFailsEveryKeyWhenTheQueryFails(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	// 閉じたプールでは、クエリが失敗する。
	pool.Close()

	users, err := graph.LoadAll(t.Context(), graph.NewLoaders(pool).Users, []uuid.UUID{uuid.New(), uuid.New(), uuid.New()})
	require.Error(t, err)
	assert.Len(t, users, 3)
}

// TestLoadServices は、loadServices が、見つからないサービスだけを飛ばし、クエリの失敗はエラーとして返すことを確かめる (#1474)。
func TestLoadServices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// closePool は、引く前にプールを閉じて、クエリを失敗させるかどうか。
		closePool bool
		wantErr   bool
	}{
		{name: "skips services that are not found"},
		{name: "returns the error when the query fails", closePool: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool, err := database.Open(t.Context(), databasetest.New(t))
			require.NoError(t, err)
			require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
			if tt.closePool {
				pool.Close()
			} else {
				t.Cleanup(pool.Close)
			}

			services, err := graph.LoadServices(t.Context(), pool, []int64{998, 999})
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, services)

				return
			}
			require.NoError(t, err)
			assert.Empty(t, services)
		})
	}
}
