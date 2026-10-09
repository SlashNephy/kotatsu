package auth_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
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
