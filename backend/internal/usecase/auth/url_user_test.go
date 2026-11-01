package auth_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

func TestURLUsersWhileDatabaseIsUnreachable(t *testing.T) {
	t.Parallel()

	db := newDB(t)

	tests := []struct {
		name string
		// beforeOutage は、DB を止める前に、一度ロールを引いた後で行う操作。
		beforeOutage    func(t *testing.T, f *sessionFixture, users *auth.URLUsers)
		wantUnavailable bool
	}{
		{
			name:         "looked up before the outage",
			beforeOutage: func(*testing.T, *sessionFixture, *auth.URLUsers) {},
		},
		{
			// 同じ tenban でユーザーを無効にしたら、覚えた結果を忘れる。
			name: "forgotten before the outage",
			beforeOutage: func(t *testing.T, f *sessionFixture, users *auth.URLUsers) {
				t.Helper()
				require.NoError(t, f.users.Disable(t.Context(), f.db, f.userID))
				users.Forget(f.userID)
			},
			wantUnavailable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newSessionFixture(t, db)
			conn := databasetest.NewSwitch(t, db)
			users := auth.NewURLUsers(conn.Pool, f.clock, slog.New(slog.DiscardHandler))
			_, err := users.Role(t.Context(), f.userID)
			require.NoError(t, err)
			tt.beforeOutage(t, f, users)

			conn.Down()
			role, err := users.Role(t.Context(), f.userID)
			if tt.wantUnavailable {
				require.ErrorIs(t, err, auth.ErrDatabaseUnavailable)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, auth.RoleMember, role)
			assert.Equal(t, int64(1), users.FallbackUsed())
		})
	}

	t.Run("disabled user", func(t *testing.T) {
		t.Parallel()

		f := newSessionFixture(t, db)
		require.NoError(t, f.users.Disable(t.Context(), f.db, f.userID))
		_, err := auth.NewURLUsers(f.db, f.clock, slog.New(slog.DiscardHandler)).Role(t.Context(), f.userID)
		require.ErrorIs(t, err, auth.ErrUserUnusable)
	})
}
