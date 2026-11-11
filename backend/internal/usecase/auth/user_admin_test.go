package auth_test

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/password"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// snowPhrase は、mikan の名前を含まない、十分強いパスワード。
const snowPhrase = "yuki-ga-furu-yoru-2026"

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
	policy := newTestPolicy(t)
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
		{name: "with a password", account: auth.NewAccount{Username: mikanName, DisplayName: mikanLabel, Role: auth.RoleMember, Password: snowPhrase}, wantHash: true},
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
	require.NoError(t, f.admin.ResetPassword(t.Context(), f.db, f.actor(), user.ID, snowPhrase))
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

// TestUserAdminDisableRevokesSessions は、無効にしたユーザーのセッションを失効させ、有効に戻しても使えないことを確かめる。
func TestUserAdminDisableRevokesSessions(t *testing.T) {
	t.Parallel()

	f := newUserAdminFixture(t)
	member := f.create(t, "member", auth.RoleMember)
	sessions := auth.NewSessions(&auth.SessionsOptions{Clock: f.clock})
	token, _, err := sessions.Create(t.Context(), f.db, member.ID, &auth.ClientInfo{})
	require.NoError(t, err)

	require.NoError(t, f.admin.Disable(t.Context(), f.db, f.actor(), f.root.ID, member.ID))
	require.NoError(t, f.admin.Enable(t.Context(), f.db, f.actor(), member.ID))

	_, err = sessions.Authenticate(t.Context(), f.db, token)
	require.ErrorIs(t, err, auth.ErrSessionInvalid)
}

// recordingEraser は、呼ばれた様子を記録する、テストの UserDataEraser。
type recordingEraser struct {
	db *pgxpool.Pool
	// prepareErr は、PrepareUserErasure が返すエラー。
	prepareErr error
	// finished は、コミットの後の関数が呼ばれたかどうか。
	finished bool
	// usersAtFinish は、コミットの後の関数を呼んだときに残っていた、消すユーザーの行の数。
	usersAtFinish int
}

var _ auth.UserDataEraser = new(recordingEraser)

func (e *recordingEraser) PrepareUserErasure(_ context.Context, _ pgx.Tx, userID uuid.UUID) (func(context.Context), error) {
	if e.prepareErr != nil {
		return nil, e.prepareErr
	}

	return func(ctx context.Context) {
		e.finished = true
		_ = e.db.QueryRow(ctx, "SELECT count(*) FROM users WHERE id = $1", userID).Scan(&e.usersAtFinish)
	}, nil
}

// TestUserAdminDeleteRemovesPersonalData は、ユーザーを消すと、個人のデータが消え、監査ログと、ほかのユーザーのデータは残ることを確かめる (#1382)。
func TestUserAdminDeleteRemovesPersonalData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		delete func(t *testing.T, f *userAdminFixture, target *auth.User, eraser auth.UserDataEraser) error
		// actor は、記録されるはずの操作者。
		actor func(f *userAdminFixture, target *auth.User) uuid.UUID
	}{
		{
			name: "by an administrator",
			delete: func(t *testing.T, f *userAdminFixture, target *auth.User, eraser auth.UserDataEraser) error {
				t.Helper()

				return f.admin.Delete(t.Context(), f.db, f.actor(), f.root.ID, target.ID, eraser)
			},
			actor: func(f *userAdminFixture, _ *auth.User) uuid.UUID { return f.root.ID },
		},
		{
			name: "by the user",
			delete: func(t *testing.T, f *userAdminFixture, target *auth.User, eraser auth.UserDataEraser) error {
				t.Helper()

				return f.admin.DeleteSelf(t.Context(), f.db, audit.User(target.ID), target.ID, eraser)
			},
			actor: func(_ *userAdminFixture, target *auth.User) uuid.UUID { return target.ID },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newUserAdminFixture(t)
			target := f.create(t, "leaving", auth.RoleViewer)
			other := f.create(t, "staying", auth.RoleViewer)
			now := f.clock.Now()
			for _, userID := range []uuid.UUID{target.ID, other.ID} {
				_, err := f.db.Exec(t.Context(), `INSERT INTO user_settings (user_id, updated_at) VALUES ($1, $2)`, userID, now)
				require.NoError(t, err)
			}
			_, err := f.db.Exec(t.Context(), `INSERT INTO notification_events (id, kind, occurred_at, payload, audience) VALUES ($1, 'recording_failed', $2, '{}', $3)`,
				uuid.New(), now, []uuid.UUID{target.ID, other.ID})
			require.NoError(t, err)
			eraser := &recordingEraser{db: f.db, prepareErr: nil, finished: false, usersAtFinish: -1}

			require.NoError(t, tt.delete(t, f, target, eraser))

			assert.Equal(t, 0, f.count(t, "SELECT count(*) FROM users WHERE id = $1", target.ID))
			assert.Equal(t, 0, f.count(t, "SELECT count(*) FROM user_settings WHERE user_id = $1", target.ID))
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM user_settings WHERE user_id = $1", other.ID))
			assert.Equal(t, 0, f.count(t, "SELECT count(*) FROM notification_events WHERE $1 = ANY(audience)", target.ID))
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM notification_events WHERE $1 = ANY(audience)", other.ID))
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.deleted' AND target_id = $1 AND actor_user_id = $2",
				target.ID.String(), tt.actor(f, target)))
			assert.True(t, eraser.finished)
			assert.Equal(t, 0, eraser.usersAtFinish, "the cleanup runs after the commit")
		})
	}
}

// TestUserAdminDeleteRefuses は、消せないユーザーを断り、何も消さないことを確かめる (#1382)。
func TestUserAdminDeleteRefuses(t *testing.T) {
	t.Parallel()

	eraserFailure := errors.New("the eraser failed")
	tests := []struct {
		name string
		op   func(t *testing.T, f *userAdminFixture, eraser auth.UserDataEraser) (uuid.UUID, error)
		fail error
		want error
	}{
		{
			name: "yourself from the user management",
			op: func(t *testing.T, f *userAdminFixture, eraser auth.UserDataEraser) (uuid.UUID, error) {
				t.Helper()

				return f.root.ID, f.admin.Delete(t.Context(), f.db, f.actor(), f.root.ID, f.root.ID, eraser)
			},
			fail: nil,
			want: auth.ErrCannotDeleteSelf,
		},
		{
			name: "the last enabled administrator by themselves",
			op: func(t *testing.T, f *userAdminFixture, eraser auth.UserDataEraser) (uuid.UUID, error) {
				t.Helper()

				return f.root.ID, f.admin.DeleteSelf(t.Context(), f.db, f.actor(), f.root.ID, eraser)
			},
			fail: nil,
			want: auth.ErrLastAdmin,
		},
		{
			name: "the last enabled administrator when the other is disabled",
			op: func(t *testing.T, f *userAdminFixture, eraser auth.UserDataEraser) (uuid.UUID, error) {
				t.Helper()
				disabled := f.create(t, "disabled-admin", auth.RoleAdmin)
				require.NoError(t, f.admin.Disable(t.Context(), f.db, f.actor(), f.root.ID, disabled.ID))

				return f.root.ID, f.admin.DeleteSelf(t.Context(), f.db, f.actor(), f.root.ID, eraser)
			},
			fail: nil,
			want: auth.ErrLastAdmin,
		},
		{
			name: "when an eraser fails",
			op: func(t *testing.T, f *userAdminFixture, eraser auth.UserDataEraser) (uuid.UUID, error) {
				t.Helper()
				target := f.create(t, "kept-on-failure", auth.RoleViewer)

				return target.ID, f.admin.Delete(t.Context(), f.db, f.actor(), f.root.ID, target.ID, eraser)
			},
			fail: eraserFailure,
			want: eraserFailure,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newUserAdminFixture(t)
			eraser := &recordingEraser{db: f.db, prepareErr: tt.fail, finished: false, usersAtFinish: -1}

			target, err := tt.op(t, f, eraser)

			require.ErrorIs(t, err, tt.want)
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM users WHERE id = $1", target))
			assert.Equal(t, 0, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.deleted'"))
			assert.False(t, eraser.finished)
		})
	}
}

// TestUserAdminDeleteAllowsDisabledAdministrator は、有効な管理者が残るなら、無効にした管理者を消せることを確かめる (#1382)。
func TestUserAdminDeleteAllowsDisabledAdministrator(t *testing.T) {
	t.Parallel()

	f := newUserAdminFixture(t)
	disabled := f.create(t, "disabled-admin", auth.RoleAdmin)
	require.NoError(t, f.admin.Disable(t.Context(), f.db, f.actor(), f.root.ID, disabled.ID))

	require.NoError(t, f.admin.Delete(t.Context(), f.db, f.actor(), f.root.ID, disabled.ID))

	assert.Equal(t, 0, f.count(t, "SELECT count(*) FROM users WHERE id = $1", disabled.ID))
}

// TestUserAdminDeleteUnknownUser は、いないユーザーを消そうとすると、ErrUserNotFound を返すことを確かめる (#1382)。
func TestUserAdminDeleteUnknownUser(t *testing.T) {
	t.Parallel()

	f := newUserAdminFixture(t)

	require.ErrorIs(t, f.admin.Delete(t.Context(), f.db, f.actor(), f.root.ID, uuid.New()), auth.ErrUserNotFound)
}
