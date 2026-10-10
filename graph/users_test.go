package graph_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/netip"
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
	sessionsQuery   = `{ sessions { id current userAgent sourceIp } }`
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
	assert.Equal(t, []string{tooWeak}, admin.query(t, resetQuery, map[string]any{varID: created.CreateUser.ID, varValue: weakValue}, nil))
	require.Empty(t, admin.query(t, resetQuery, map[string]any{varID: created.CreateUser.ID, varValue: strongPhrase + "!"}, nil))

	// パスワードを省くと、パスワードを持たないユーザーにする。
	var passwordless struct {
		CreateUser userResult `json:"createUser"`
	}
	require.Empty(t, admin.query(t, createUserQuery, map[string]any{
		varInput: map[string]any{fieldUser: "neko", fieldDisplay: "ねこ", fieldRole: roleViewer},
	}, &passwordless))
	var hasHash bool
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT password_hash IS NOT NULL FROM users WHERE username = 'neko'").Scan(&hasHash))
	assert.False(t, hasHash)

	// 自分自身は無効にできず、最後の管理者を外せない。
	self := graph.GlobalID("User", admin.userID.String())
	assert.Equal(t, []string{codeBadUserInput}, admin.query(t, disableQuery, map[string]any{varID: self}, nil))
	assert.Equal(t, []string{"you cannot disable yourself"}, admin.messages(t, disableQuery, map[string]any{varID: self}), "the message has no location prefix")
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
	assert.Equal(t, []string{testUsername, adminName, mikanUser, "neko"}, usernames)

	var audited int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_events
		WHERE action IN ('user.created', 'user.updated', 'user.disabled', 'user.enabled', 'user.password_set') AND actor_user_id = $1 AND user_agent = $2`,
		admin.userID, testUserAgent).Scan(&audited))
	assert.Equal(t, 6, audited)

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

	// 同じユーザーで、送信元の分かるセッションを、もう 1 つ作る。
	_, other, err := f.sessions.Create(t.Context(), f.pool, f.userID, &auth.ClientInfo{
		UserAgent: "kotatsu-other/1.0", SourceIP: netip.MustParseAddr("192.168.1.20"), Host: "", Secure: false,
	})
	require.NoError(t, err)
	var listed struct {
		Sessions []struct {
			ID        string  `json:"id"`
			Current   bool    `json:"current"`
			UserAgent string  `json:"userAgent"`
			SourceIP  *string `json:"sourceIp"`
		} `json:"sessions"`
	}
	require.Empty(t, f.query(t, sessionsQuery, nil, &listed))
	require.Len(t, listed.Sessions, 2)
	sourceIPs := map[bool]*string{}
	for _, session := range listed.Sessions {
		sourceIPs[session.Current] = session.SourceIP
	}
	require.Len(t, sourceIPs, 2, "exactly one session is current")
	assert.Nil(t, sourceIPs[true], "the fixture session has no source")
	require.NotNil(t, sourceIPs[false])
	assert.Equal(t, "192.168.1.20", *sourceIPs[false])

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

// messages は、GraphQL を叩き、エラーのメッセージを返す。
func (f *fixture) messages(t *testing.T, query string, variables any) []string {
	t.Helper()

	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.server.URL+"/graphql", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentTypeJSON)
	req.Header.Set("Cookie", auth.CookieName+"="+f.token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()
	var decoded gqlResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
	messages := make([]string, 0, len(decoded.Errors))
	for _, e := range decoded.Errors {
		messages = append(messages, e.Message)
	}

	return messages
}
