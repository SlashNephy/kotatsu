package auth_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
)

func TestCreateUser(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	users := auth.NewUsers(clocktest.NewFake(testNow()))
	_, err := users.Create(t.Context(), db, &auth.NewUser{Username: "Alice", DisplayName: "ありす", Role: auth.RoleAdmin, PasswordHash: nil})
	require.NoError(t, err)

	tests := []struct {
		name    string
		user    *auth.NewUser
		wantErr error
	}{
		{name: "member", user: &auth.NewUser{Username: "bob", DisplayName: "ぼぶ", Role: auth.RoleMember}},
		{name: "same username in different case", user: &auth.NewUser{Username: "alice", DisplayName: "別人", Role: auth.RoleViewer}, wantErr: auth.ErrUsernameTaken},
		{name: "unknown role", user: &auth.NewUser{Username: "carol", DisplayName: "きゃろる", Role: "owner"}, wantErr: auth.ErrInvalidRole},
		{name: "empty username", user: &auth.NewUser{Username: "", DisplayName: "名無し", Role: auth.RoleViewer}, wantErr: auth.ErrInvalidUser},
		{name: "surrounding spaces", user: &auth.NewUser{Username: " dave", DisplayName: "でいぶ", Role: auth.RoleViewer}, wantErr: auth.ErrInvalidUser},
		{name: "control character in username", user: &auth.NewUser{Username: "fr\nank", DisplayName: "ふらんく", Role: auth.RoleViewer}, wantErr: auth.ErrInvalidUser},
		{name: "whitespace-only display name", user: &auth.NewUser{Username: "grace", DisplayName: "\u3000 ", Role: auth.RoleViewer}, wantErr: auth.ErrInvalidUser},
		{name: "nil", user: nil, wantErr: auth.ErrInvalidUser},
		{name: "too long display name", user: &auth.NewUser{Username: "eve", DisplayName: strings.Repeat("長", 65), Role: auth.RoleViewer}, wantErr: auth.ErrInvalidUser},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			created, err := users.Create(t.Context(), db, tt.user)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)

			stored, err := sqlc.New(db).GetUser(t.Context(), created.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.user.Username, stored.Username)
			assert.Equal(t, string(tt.user.Role), stored.Role)
			assert.Nil(t, stored.DisabledAt)
		})
	}
}

func TestDisableUser(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	users := auth.NewUsers(clocktest.NewFake(testNow()))
	user, err := users.Create(t.Context(), db, &auth.NewUser{Username: "henry", DisplayName: "へんりー", Role: auth.RoleMember})
	require.NoError(t, err)

	tests := []struct {
		name    string
		userID  uuid.UUID
		wantErr error
	}{
		{name: "active user", userID: user.ID},
		{name: "already disabled", userID: user.ID, wantErr: auth.ErrUserNotFound},
		{name: "unknown user", userID: id.New(), wantErr: auth.ErrUserNotFound},
	}
	for _, tt := range tests {
		// 前のケースで無効にした結果を次のケースで使うので、順に動かす。
		err := users.Disable(t.Context(), db, tt.userID)
		if tt.wantErr != nil {
			require.ErrorIs(t, err, tt.wantErr, tt.name)

			continue
		}
		require.NoError(t, err, tt.name)
	}

	stored, err := sqlc.New(db).GetUser(t.Context(), user.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.DisabledAt)
	assert.True(t, testNow().Equal(*stored.DisabledAt))
}
