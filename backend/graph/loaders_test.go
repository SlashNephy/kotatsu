package graph_test

import (
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
)

// TestLoaderFailsEveryKeyWhenTheQueryFails は、まとめて引くクエリが失敗したとき、まとめたすべてのキーにエラーを返すことを確かめる。
func TestLoaderFailsEveryKeyWhenTheQueryFails(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	// 閉じたプールでは、クエリが失敗する。
	pool.Close()

	users, err := graph.NewLoaders(pool).Users.LoadAll(t.Context(), []uuid.UUID{uuid.New(), uuid.New(), uuid.New()})
	require.Error(t, err)
	assert.Len(t, users, 3)
}
