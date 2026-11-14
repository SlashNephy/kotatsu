package auth_test

import (
	"log/slog"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// issueStreamURL は、f のユーザーが発行した外部プレイヤー用の URL の記録を作り、その ID を返す (#1757)。
func issueStreamURL(t *testing.T, f *sessionFixture) uuid.UUID {
	t.Helper()

	urlID := id.New()
	err := auth.NewStreamURLs(f.db, f.clock, audit.NewRecorder(f.clock), nil).Create(t.Context(), audit.User(f.userID), &auth.NewStreamURL{
		ID: urlID, UserID: f.userID, Resource: "/api/streams/services/3273601024", TargetName: "こたつテスト1", ExpiresAt: f.clock.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	return urlID
}

func TestURLUsersWhileDatabaseIsUnreachable(t *testing.T) {
	t.Parallel()

	db := newDB(t)

	tests := []struct {
		name string
		// beforeOutage は、DB を止める前に、一度ロールを引いた後で行う操作。
		beforeOutage    func(t *testing.T, f *sessionFixture, users *auth.URLUsers, urlID uuid.UUID)
		wantUnavailable bool
	}{
		{
			name:         "looked up before the outage",
			beforeOutage: func(*testing.T, *sessionFixture, *auth.URLUsers, uuid.UUID) {},
		},
		{
			// 同じ tenban でユーザーを無効にしたら、覚えた結果を忘れる。
			name: "forgotten before the outage",
			beforeOutage: func(t *testing.T, f *sessionFixture, users *auth.URLUsers, _ uuid.UUID) {
				t.Helper()
				require.NoError(t, f.users.Disable(t.Context(), f.db, f.userID))
				users.Forget(f.userID)
			},
			wantUnavailable: true,
		},
		{
			// 同じ tenban で URL を取り消したら、覚えた結果を忘れる (#1757)。
			name: "revoked before the outage",
			beforeOutage: func(t *testing.T, f *sessionFixture, users *auth.URLUsers, urlID uuid.UUID) {
				t.Helper()
				require.NoError(t, auth.NewStreamURLs(f.db, f.clock, audit.NewRecorder(f.clock), users).Delete(t.Context(), audit.User(f.userID), urlID))
			},
			wantUnavailable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newSessionFixture(t, db)
			urlID := issueStreamURL(t, f)
			conn := databasetest.NewSwitch(t, db)
			users := auth.NewURLUsers(conn.Pool, f.clock, slog.New(slog.DiscardHandler))
			_, err := users.Role(t.Context(), urlID, f.userID)
			require.NoError(t, err)
			tt.beforeOutage(t, f, users, urlID)

			conn.Down()
			role, err := users.Role(t.Context(), urlID, f.userID)
			if tt.wantUnavailable {
				require.ErrorIs(t, err, auth.ErrDatabaseUnavailable)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, domain.RoleMember, role)
			assert.Equal(t, int64(1), users.FallbackUsed())
		})
	}
}

// TestURLUsersRejectsUnusableURLs は、ユーザーを無効にしたか、URL を取り消したか、記録と違うユーザーの URL を断ることを確かめる (#1757)。
func TestURLUsersRejectsUnusableURLs(t *testing.T) {
	t.Parallel()

	db := newDB(t)

	tests := []struct {
		name string
		// prepare は、URL の記録を作った後で行う操作。確かめる URL の記録の ID とユーザーを返す。
		prepare func(t *testing.T, f *sessionFixture, urlID uuid.UUID) (uuid.UUID, uuid.UUID)
	}{
		{
			name: "disabled user",
			prepare: func(t *testing.T, f *sessionFixture, urlID uuid.UUID) (uuid.UUID, uuid.UUID) {
				t.Helper()
				require.NoError(t, f.users.Disable(t.Context(), f.db, f.userID))

				return urlID, f.userID
			},
		},
		{
			name: "revoked URL",
			prepare: func(t *testing.T, f *sessionFixture, urlID uuid.UUID) (uuid.UUID, uuid.UUID) {
				t.Helper()
				require.NoError(t, auth.NewStreamURLs(f.db, f.clock, audit.NewRecorder(f.clock), nil).Delete(t.Context(), audit.User(f.userID), urlID))

				return urlID, f.userID
			},
		},
		{
			name: "another user",
			prepare: func(_ *testing.T, _ *sessionFixture, urlID uuid.UUID) (uuid.UUID, uuid.UUID) {
				return urlID, id.New()
			},
		},
		{
			name: "unknown URL",
			prepare: func(_ *testing.T, f *sessionFixture, _ uuid.UUID) (uuid.UUID, uuid.UUID) {
				return id.New(), f.userID
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newSessionFixture(t, db)
			urlID, userID := tt.prepare(t, f, issueStreamURL(t, f))
			_, err := auth.NewURLUsers(f.db, f.clock, slog.New(slog.DiscardHandler)).Role(t.Context(), urlID, userID)
			require.ErrorIs(t, err, auth.ErrUserUnusable)
		})
	}
}
