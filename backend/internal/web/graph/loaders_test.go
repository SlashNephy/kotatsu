package graph_test

import (
	"log/slog"
	"testing"
	"uuid"

	"github.com/jackc/pgx/v5"
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

	// missing は、どのサービスの ID でもない値。
	const missing = 999

	tests := []struct {
		name string
		// closePool は、引く前にプールを閉じて、クエリを失敗させるかどうか。
		closePool bool
		wantErr   bool
	}{
		{name: "keeps found services in order and skips missing ones"},
		{name: "returns the error when the query fails", closePool: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			f.syncEPG(t)
			rows, err := f.pool.Query(t.Context(), `SELECT id, name FROM services ORDER BY id LIMIT 2`)
			require.NoError(t, err)
			type service struct {
				ID   int64
				Name string
			}
			services, err := pgx.CollectRows(rows, pgx.RowToStructByPos[service])
			require.NoError(t, err)
			require.Len(t, services, 2)
			first, second := services[0], services[1]
			if tt.closePool {
				f.pool.Close()
			}

			got, err := graph.LoadServices(t.Context(), f.pool, []int64{second.ID, missing, first.ID})
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)

				return
			}
			require.NoError(t, err)
			names := make([]string, 0, len(got))
			for _, s := range got {
				names = append(names, s.Name)
			}
			assert.Equal(t, []string{second.Name, first.Name}, names)
		})
	}
}
