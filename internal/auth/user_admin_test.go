package auth_test

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/password"
)

type userAdminFixture struct {
	db    *pgxpool.Pool
	clock *clocktest.Fake
	admin *auth.UserAdmin
	users *auth.Users
	// root は、最初の管理者。
	root *auth.User
}

func newUserAdminFixture(t *testing.T) *userAdminFixture {
	t.Helper()

	clk := clocktest.NewFake(testNow())
	// -race の CI では、パスワードの強度の判定が遅くなる。
	policy, err := password.NewPolicy(&password.PolicyOptions{HIBP: nil, StrengthTimeout: 10 * time.Minute, Logger: slog.New(slog.DiscardHandler)})
	require.NoError(t, err)
	db := newDB(t)
	users := auth.NewUsers(clk)
	root, err := users.Create(t.Context(), db, &auth.NewUser{Username: rootName, DisplayName: rootName, Role: auth.RoleAdmin, PasswordHash: nil})
	require.NoError(t, err)

	return &userAdminFixture{db: db, clock: clk, admin: auth.NewUserAdmin(clk, audit.NewRecorder(clk), policy), users: users, root: root}
}

func (f *userAdminFixture) actor() *audit.Actor {
	return audit.User(f.root.ID)
}

func (f *userAdminFixture) create(t *testing.T, username string, role auth.Role) *auth.User {
	t.Helper()

	user, err := f.users.Create(t.Context(), f.db, &auth.NewUser{Username: username, DisplayName: username, Role: role, PasswordHash: nil})
	require.NoError(t, err)

	return user
}

func (f *userAdminFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()

	var n int
	require.NoError(t, f.db.QueryRow(t.Context(), query, args...).Scan(&n))

	return n
}

// TestUserAdminCreate は、管理者がユーザーを作れ、パスワードは規定を満たすときだけ設定することを確かめる。
func TestUserAdminCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		account auth.NewAccount
		wantErr error
		// wantRejected なら、パスワードの規定に当たったことを表す *password.RejectedError を返す。
		wantRejected bool
		wantHash     bool
	}{
		{name: "with a password", account: auth.NewAccount{Username: mikanName, DisplayName: mikanLabel, Role: auth.RoleMember, Password: adminPhrase}, wantHash: true},
		{name: "without a password", account: auth.NewAccount{Username: nekoName, DisplayName: "ねこ", Role: auth.RoleViewer, Password: ""}},
		{name: "weak password", account: auth.NewAccount{Username: mikanName, DisplayName: mikanLabel, Role: auth.RoleMember, Password: weakPhrase}, wantRejected: true},
		{name: "taken username", account: auth.NewAccount{Username: "ROOT", DisplayName: rootName, Role: auth.RoleMember, Password: ""}, wantErr: auth.ErrUsernameTaken},
		{name: "unknown role", account: auth.NewAccount{Username: mikanName, DisplayName: mikanLabel, Role: "owner", Password: ""}, wantErr: auth.ErrInvalidRole},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newUserAdminFixture(t)
			created, err := f.admin.Create(t.Context(), f.db, f.actor(), &tt.account)
			if tt.wantErr != nil || tt.wantRejected {
				if tt.wantRejected {
					_, ok := errors.AsType[*password.RejectedError](err)
					require.True(t, ok, "got %v", err)
				} else {
					require.ErrorIs(t, err, tt.wantErr)
				}
				assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM users"))

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.account.Role, created.Role)
			assert.Equal(t, boolToInt(tt.wantHash), f.count(t, "SELECT count(*) FROM users WHERE id = $1 AND password_hash IS NOT NULL", created.ID))
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.created' AND target_id = $1 AND actor_user_id = $2",
				created.ID.String(), f.root.ID))

			listed, err := f.admin.List(t.Context(), f.db)
			require.NoError(t, err)
			require.Len(t, listed, 2)
			assert.Equal(t, created.ID, listed[1].ID)
		})
	}
}

// TestUserAdminKeepsAnAdministrator は、有効な管理者が 1 人もいなくなる操作を拒むことを確かめる。
func TestUserAdminKeepsAnAdministrator(t *testing.T) {
	t.Parallel()

	member := auth.RoleMember
	tests := []struct {
		name string
		// prepare は、ほかのユーザーを用意し、操作の対象を返す。
		prepare func(t *testing.T, f *userAdminFixture) uuid.UUID
		op      func(t *testing.T, f *userAdminFixture, target uuid.UUID) error
		wantErr error
	}{
		{
			name: "demote the only administrator", wantErr: auth.ErrLastAdmin,
			prepare: func(_ *testing.T, f *userAdminFixture) uuid.UUID { return f.root.ID },
			op: func(t *testing.T, f *userAdminFixture, target uuid.UUID) error {
				t.Helper()
				_, err := f.admin.Update(t.Context(), f.db, f.actor(), target, &auth.UserChanges{DisplayName: nil, Role: &member})

				return err
			},
		},
		{
			name: "disable the only administrator", wantErr: auth.ErrLastAdmin,
			prepare: func(_ *testing.T, f *userAdminFixture) uuid.UUID { return f.root.ID },
			op: func(t *testing.T, f *userAdminFixture, target uuid.UUID) error {
				t.Helper()
				// 自分自身は無効にできないので、ほかのユーザーが操作したことにする。
				return f.admin.Disable(t.Context(), f.db, f.actor(), uuid.New(), target)
			},
		},
		{
			name: "a disabled administrator does not count", wantErr: auth.ErrLastAdmin,
			prepare: func(t *testing.T, f *userAdminFixture) uuid.UUID {
				t.Helper()
				require.NoError(t, f.users.Disable(t.Context(), f.db, f.create(t, "old-admin", auth.RoleAdmin).ID))

				return f.root.ID
			},
			op: func(t *testing.T, f *userAdminFixture, target uuid.UUID) error {
				t.Helper()
				_, err := f.admin.Update(t.Context(), f.db, f.actor(), target, &auth.UserChanges{DisplayName: nil, Role: &member})

				return err
			},
		},
		{
			name: "demote one of two administrators",
			prepare: func(t *testing.T, f *userAdminFixture) uuid.UUID {
				t.Helper()

				return f.create(t, "second-admin", auth.RoleAdmin).ID
			},
			op: func(t *testing.T, f *userAdminFixture, target uuid.UUID) error {
				t.Helper()
				_, err := f.admin.Update(t.Context(), f.db, f.actor(), target, &auth.UserChanges{DisplayName: nil, Role: &member})

				return err
			},
		},
		{
			name: "disable one of two administrators",
			prepare: func(t *testing.T, f *userAdminFixture) uuid.UUID {
				t.Helper()

				return f.create(t, "second-admin", auth.RoleAdmin).ID
			},
			op: func(t *testing.T, f *userAdminFixture, target uuid.UUID) error {
				t.Helper()

				return f.admin.Disable(t.Context(), f.db, f.actor(), f.root.ID, target)
			},
		},
		{
			name: "disable yourself", wantErr: auth.ErrCannotDisableSelf,
			prepare: func(t *testing.T, f *userAdminFixture) uuid.UUID {
				t.Helper()
				f.create(t, "second-admin", auth.RoleAdmin)

				return f.root.ID
			},
			op: func(t *testing.T, f *userAdminFixture, target uuid.UUID) error {
				t.Helper()

				return f.admin.Disable(t.Context(), f.db, f.actor(), f.root.ID, target)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newUserAdminFixture(t)
			target := tt.prepare(t, f)
			err := tt.op(t, f, target)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Zero(t, f.count(t, "SELECT count(*) FROM audit_events WHERE action IN ('user.updated', 'user.disabled')"))

				return
			}
			require.NoError(t, err)
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM users WHERE role = 'admin' AND disabled_at IS NULL"))
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action IN ('user.updated', 'user.disabled') AND target_id = $1", target.String()))
		})
	}
}

// TestUserAdminUpdateAndEnable は、表示名とロールを変え、無効にしたユーザーを有効に戻せることを確かめる。
func TestUserAdminUpdateAndEnable(t *testing.T) {
	t.Parallel()

	f := newUserAdminFixture(t)
	user := f.create(t, mikanName, auth.RoleViewer)
	name, role := mikanLabel, auth.RoleMember
	updated, err := f.admin.Update(t.Context(), f.db, f.actor(), user.ID, &auth.UserChanges{DisplayName: &name, Role: &role})
	require.NoError(t, err)
	assert.Equal(t, name, updated.DisplayName)
	assert.Equal(t, role, updated.Role)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.updated' AND summary->'role'->>'from' = 'viewer' AND summary->'role'->>'to' = 'member'"))

	// 何も変わらなければ、記録しない。
	_, err = f.admin.Update(t.Context(), f.db, f.actor(), user.ID, &auth.UserChanges{DisplayName: &name, Role: &role})
	require.NoError(t, err)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.updated'"))
	_, err = f.admin.Update(t.Context(), f.db, f.actor(), user.ID, nil)
	require.ErrorIs(t, err, auth.ErrInvalidUser)
	_, err = f.admin.Create(t.Context(), f.db, f.actor(), nil)
	require.ErrorIs(t, err, auth.ErrInvalidUser)

	blank := " "
	_, err = f.admin.Update(t.Context(), f.db, f.actor(), user.ID, &auth.UserChanges{DisplayName: &blank, Role: nil})
	require.ErrorIs(t, err, auth.ErrInvalidUser)
	_, err = f.admin.Update(t.Context(), f.db, f.actor(), uuid.New(), &auth.UserChanges{DisplayName: &name, Role: nil})
	require.ErrorIs(t, err, auth.ErrUserNotFound)

	require.NoError(t, f.admin.Disable(t.Context(), f.db, f.actor(), f.root.ID, user.ID))
	// 既に無効なら、何もしない。
	require.NoError(t, f.admin.Disable(t.Context(), f.db, f.actor(), f.root.ID, user.ID))
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.disabled'"))
	require.NoError(t, f.admin.Enable(t.Context(), f.db, f.actor(), user.ID))
	require.NoError(t, f.admin.Enable(t.Context(), f.db, f.actor(), user.ID))
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.enabled'"))
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM users WHERE id = $1 AND disabled_at IS NULL", user.ID))

	err = f.admin.ResetPassword(t.Context(), f.db, f.actor(), user.ID, "kotatsukotatsu")
	rejected, ok := errors.AsType[*password.RejectedError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, password.ReasonTooWeak, rejected.Reason)
	require.NoError(t, f.admin.ResetPassword(t.Context(), f.db, f.actor(), user.ID, adminPhrase))
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM users WHERE id = $1 AND password_hash IS NOT NULL", user.ID))
}

// TestUserAdminKeepsAnAdministratorConcurrently は、2 人の管理者を同時に外しても、1 人は管理者に残ることを確かめる。
func TestUserAdminKeepsAnAdministratorConcurrently(t *testing.T) {
	t.Parallel()

	f := newUserAdminFixture(t)
	second := f.create(t, "second-admin", auth.RoleAdmin)
	member := auth.RoleMember
	errs := make(chan error, 2)
	for _, target := range []uuid.UUID{f.root.ID, second.ID} {
		go func() {
			_, err := f.admin.Update(t.Context(), f.db, f.actor(), target, &auth.UserChanges{DisplayName: nil, Role: &member})
			errs <- err
		}()
	}
	results := []error{<-errs, <-errs}
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM users WHERE role = 'admin' AND disabled_at IS NULL"))
	succeeded := 0
	for _, err := range results {
		if err == nil {
			succeeded++
		} else {
			require.ErrorIs(t, err, auth.ErrLastAdmin)
		}
	}
	assert.Equal(t, 1, succeeded)
}
