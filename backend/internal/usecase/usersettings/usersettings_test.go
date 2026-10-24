package usersettings_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/usersettings"
)

// TestTouch は、ユーザーごとに設定の版数を上げ、変えるたびに変更を通知することを確かめる。
func TestTouch(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	users := make([]uuid.UUID, 0, 2)
	for _, name := range []string{"alice", "bob"} {
		userID := id.New()
		require.NoError(t, sqlc.New(pool).InsertUser(t.Context(), sqlc.InsertUserParams{
			ID: userID, Username: name, DisplayName: name, Role: "member", PasswordHash: nil, CreatedAt: clk.Now(),
		}))
		users = append(users, userID)
	}
	alice, bob := users[0], users[1]
	touch := func(userID uuid.UUID) {
		require.NoError(t, pgx.BeginFunc(t.Context(), pool, func(tx pgx.Tx) error {
			return usersettings.Touch(t.Context(), tx, clk, userID)
		}))
	}
	versions := func() (int64, int64, int64) {
		a, err := usersettings.Version(t.Context(), pool, alice)
		require.NoError(t, err)
		b, err := usersettings.Version(t.Context(), pool, bob)
		require.NoError(t, err)
		var channel int64
		require.NoError(t, pool.QueryRow(t.Context(),
			"SELECT coalesce((SELECT version FROM change_versions WHERE channel = $1), 0)", usersettings.NotifyChannel).Scan(&channel))

		return a, b, channel
	}

	a, b, channel := versions()
	assert.Equal(t, [3]int64{0, 0, 0}, [3]int64{a, b, channel}, "never changed")
	touch(alice)
	touch(alice)
	touch(bob)
	a, b, channel = versions()
	assert.Equal(t, [3]int64{2, 1, 3}, [3]int64{a, b, channel})
}
