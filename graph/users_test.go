package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/auth"
)

const (
	usersQuery      = `{ users { id username displayName role disabled } }`
	createUserQuery = `mutation($input: CreateUserInput!) { createUser(input: $input) { id username displayName role disabled } }`
	updateUserQuery = `mutation($id: ID!, $input: UpdateUserInput!) { updateUser(id: $id, input: $input) { id displayName role } }`
	disableQuery    = `mutation($id: ID!) { disableUser(id: $id) { id disabled } }`
	enableQuery     = `mutation($id: ID!) { enableUser(id: $id) { id disabled } }`
	resetQuery      = `mutation($id: ID!, $value: String!) { resetUserPassword(id: $id, password: $value) { id } }`
	changeQuery     = `mutation($value: String!) { changePassword(password: $value) }`
	sessionsQuery   = `{ sessions { id current userAgent } }`
	revokeQuery     = `mutation($id: ID!) { revokeSession(id: $id) }`

	adminName    = "admin"
	weakValue    = "kotatsukotatsu"
	roleMember   = "MEMBER"
	roleViewer   = "VIEWER"
	fieldSecret  = "password"
	fieldDisplay = "displayName"
	mikanUser    = "mikan"
	fieldRole    = "role"
	fieldUser    = "username"
	mikanLabel   = "みかん"
	strongPhrase = "neko-to-mikan-no-fuyu-2026"
	tooWeak      = "BAD_USER_INPUT:too_weak"
	varValue     = "value"
)

type userResult struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Disabled    bool   `json:"disabled"`
}

// TestUserManagementNeedsAnAdministrator は、管理者でないユーザーには、ユーザーの管理をさせないことを確かめる (設計書 9.7)。
func TestUserManagementNeedsAnAdministrator(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	target := graph.GlobalID("User", f.userID.String())
	tests := []struct {
		name      string
		query     string
		variables map[string]any
	}{
		{name: "list", query: usersQuery},
		{name: "create", query: createUserQuery, variables: map[string]any{varInput: map[string]any{fieldUser: mikanUser, fieldDisplay: mikanLabel, fieldRole: roleMember}}},
		{name: "update", query: updateUserQuery, variables: map[string]any{varID: target, varInput: map[string]any{fieldRole: "ADMIN"}}},
		{name: "disable", query: disableQuery, variables: map[string]any{varID: target}},
		{name: "enable", query: enableQuery, variables: map[string]any{varID: target}},
		{name: "reset password", query: resetQuery, variables: map[string]any{varID: target, varValue: strongPhrase}},
	}
	for _, role := range []auth.Role{auth.RoleMember, auth.RoleViewer} {
		actor := f.as(t, "as-"+string(role), role)
		for _, tt := range tests {
			assert.Equal(t, []string{codeForbidden}, actor.query(t, tt.query, tt.variables, nil), "%s by %s", tt.name, role)
		}
	}
	var role string
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT role FROM users WHERE id = $1", f.userID).Scan(&role))
	assert.Equal(t, "member", role, "the user must not be promoted")
}

// TestManageUsers は、管理者が、ユーザーを作り、変え、無効にし、有効に戻し、パスワードを設定し直せることを確かめる。
func TestManageUsers(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	admin := f.as(t, adminName, auth.RoleAdmin)

	// パスワードが規定に当たれば、理由を返す。
	assert.Equal(t, []string{tooWeak}, admin.query(t, createUserQuery, map[string]any{
		varInput: map[string]any{fieldUser: mikanUser, fieldDisplay: mikanLabel, fieldRole: roleMember, fieldSecret: weakValue},
	}, nil))

	var created struct {
		CreateUser userResult `json:"createUser"`
	}
	require.Empty(t, admin.query(t, createUserQuery, map[string]any{
		varInput: map[string]any{fieldUser: mikanUser, fieldDisplay: mikanLabel, fieldRole: roleViewer, fieldSecret: strongPhrase},
	}, &created))
	assert.Equal(t, userResult{ID: created.CreateUser.ID, Username: mikanUser, DisplayName: mikanLabel, Role: roleViewer, Disabled: false}, created.CreateUser)
	assert.Equal(t, []string{codeBadUserInput}, admin.query(t, createUserQuery, map[string]any{
		varInput: map[string]any{fieldUser: "MIKAN", fieldDisplay: mikanLabel, fieldRole: roleViewer},
	}, nil), "the username is taken")

	var updated struct {
		UpdateUser userResult `json:"updateUser"`
	}
	require.Empty(t, admin.query(t, updateUserQuery, map[string]any{varID: created.CreateUser.ID, varInput: map[string]any{fieldRole: roleMember}}, &updated))
	assert.Equal(t, roleMember, updated.UpdateUser.Role)
	assert.Equal(t, mikanLabel, updated.UpdateUser.DisplayName, "omitted fields are kept")

	var disabled struct {
		DisableUser userResult `json:"disableUser"`
	}
	require.Empty(t, admin.query(t, disableQuery, map[string]any{varID: created.CreateUser.ID}, &disabled))
	assert.True(t, disabled.DisableUser.Disabled)
	var enabled struct {
		EnableUser userResult `json:"enableUser"`
	}
	require.Empty(t, admin.query(t, enableQuery, map[string]any{varID: created.CreateUser.ID}, &enabled))
	assert.False(t, enabled.EnableUser.Disabled)
	require.Empty(t, admin.query(t, resetQuery, map[string]any{varID: created.CreateUser.ID, varValue: strongPhrase + "!"}, nil))

	// 自分自身は無効にできず、最後の管理者を外せない。
	self := graph.GlobalID("User", admin.userID.String())
	assert.Equal(t, []string{codeBadUserInput}, admin.query(t, disableQuery, map[string]any{varID: self}, nil))
	assert.Equal(t, []string{codeBadUserInput}, admin.query(t, updateUserQuery, map[string]any{varID: self, varInput: map[string]any{fieldRole: roleMember}}, nil))
	assert.Equal(t, []string{codeNotFound}, admin.query(t, disableQuery, map[string]any{varID: graph.GlobalID("User", "0199c1c0-0000-7000-8000-000000000000")}, nil))

	var listed struct {
		Users []userResult `json:"users"`
	}
	require.Empty(t, admin.query(t, usersQuery, nil, &listed))
	usernames := make([]string, 0, len(listed.Users))
	for _, user := range listed.Users {
		usernames = append(usernames, user.Username)
	}
	assert.Equal(t, []string{testUsername, adminName, mikanUser}, usernames)

	var audited int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_events
		WHERE action IN ('user.created', 'user.updated', 'user.disabled', 'user.enabled', 'user.password_set') AND actor_user_id = $1 AND user_agent = $2`,
		admin.userID, testUserAgent).Scan(&audited))
	assert.Equal(t, 5, audited)

	// ユーザーの管理は重要な操作なので、直前の再認証を求める。
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	assert.Equal(t, []string{codeReauthRequired}, admin.query(t, disableQuery, map[string]any{varID: created.CreateUser.ID}, nil))
}

// TestChangePasswordAndSessions は、ログイン中のユーザーが、パスワードを変え、自分のセッションを失効させられることを確かめる。
func TestChangePasswordAndSessions(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	assert.Equal(t, []string{tooWeak}, f.query(t, changeQuery, map[string]any{varValue: weakValue}, nil))
	var changed struct {
		ChangePassword bool `json:"changePassword"`
	}
	require.Empty(t, f.query(t, changeQuery, map[string]any{varValue: strongPhrase}, &changed))
	assert.True(t, changed.ChangePassword)
	var hasPassword bool
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT password_hash IS NOT NULL FROM users WHERE id = $1", f.userID).Scan(&hasPassword))
	assert.True(t, hasPassword)

	// 同じユーザーで、もう 1 つセッションを作る。
	_, other, err := f.sessions.Create(t.Context(), f.pool, f.userID, nil)
	require.NoError(t, err)
	var listed struct {
		Sessions []struct {
			ID      string `json:"id"`
			Current bool   `json:"current"`
		} `json:"sessions"`
	}
	require.Empty(t, f.query(t, sessionsQuery, nil, &listed))
	require.Len(t, listed.Sessions, 2)
	currents := 0
	for _, session := range listed.Sessions {
		if session.Current {
			currents++
		}
	}
	assert.Equal(t, 1, currents)

	otherID := graph.GlobalID("Session", other.ID.String())
	var revoked struct {
		RevokeSession string `json:"revokeSession"`
	}
	require.Empty(t, f.query(t, revokeQuery, map[string]any{varID: otherID}, &revoked))
	assert.Equal(t, otherID, revoked.RevokeSession)
	assert.Equal(t, []string{codeNotFound}, f.query(t, revokeQuery, map[string]any{varID: otherID}, nil))
	// ほかのユーザーのセッションは、失効させられない。
	mikan := f.as(t, mikanUser, auth.RoleMember)
	require.Empty(t, f.query(t, sessionsQuery, nil, &listed))
	assert.Equal(t, []string{codeNotFound}, mikan.query(t, revokeQuery, map[string]any{varID: listed.Sessions[0].ID}, nil))

	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	assert.Equal(t, []string{codeReauthRequired}, f.query(t, changeQuery, map[string]any{varValue: strongPhrase + "!"}, nil))
}
