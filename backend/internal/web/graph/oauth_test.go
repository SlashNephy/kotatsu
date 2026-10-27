package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

const approveMutation = `mutation($authorizationQuery: String!, $scopes: [String!]!) { approveOAuthAuthorization(query: $authorizationQuery, scopes: $scopes) }`

// TestApproveOAuthAuthorizationRejects は、同意に直近の認証が要り、クエリが読めなければ断ることを確かめる (#963)。
// 同意から認可コードの発行までは、internal/usecase/oauth のテストで確かめる。
func TestApproveOAuthAuthorizationRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		stale bool
		want  string
	}{
		{name: "直近の認証がない", query: "client_id=https%3A%2F%2Fexample.com%2Fclient.json", stale: true, want: codeReauthRequired},
		{name: "読めないクエリ", query: "client_id=%zz", stale: false, want: codeBadUserInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			if tt.stale {
				f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
			}
			assert.Equal(t, []string{tt.want}, f.query(t, approveMutation, map[string]any{"authorizationQuery": tt.query, "scopes": []string{"mcp:read"}}, nil))
		})
	}
}

// TestOAuthAuthorizationHidesFetchError は、クライアントのメタデータを取りに行けなかった理由を、応答に書かないことを確かめる (#1015)。
// 理由には、名前を引いたアドレスや DNS のエラーのような、サーバーから見た内側のネットワークが入る。
func TestOAuthAuthorizationHidesFetchError(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	admin := f.as(t, "admin", auth.RoleAdmin)
	require.Empty(t, admin.query(t, updateSettings, map[string]any{varInput: map[string]any{fieldMCPEnabled: true}}, nil))
	viewer := f.as(t, "guest", auth.RoleViewer)

	// ループバックのアドレスには、取りに行く前に断る。断った理由には、そのアドレスが入る。
	const query = `query($q: String!) { oauthAuthorization(query: $q) { errorRedirectUrl } }`
	variables := map[string]any{"q": "client_id=https%3A%2F%2F127.0.0.1%3A8443%2Fclient.json&redirect_uri=https%3A%2F%2Fa.example%2Fb"}

	assert.Equal(t, []string{codeBadUserInput}, viewer.query(t, query, variables, nil))
	assert.Equal(t, []string{"invalid authorization request: the client metadata could not be fetched"}, viewer.messages(t, query, variables))
}

const (
	oauthGrantsQuery    = `{ oauthGrants { id clientId clientName scopes lastUsedAt } }`
	deleteGrantMutation = `mutation($id: ID!) { deleteOAuthGrant(id: $id) }`
)

type oauthGrant struct {
	ID         string     `json:"id"`
	ClientID   string     `json:"clientId"`
	ClientName string     `json:"clientName"`
	Scopes     []string   `json:"scopes"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

func (f *fixture) listOAuthGrants(t *testing.T) []oauthGrant {
	t.Helper()

	var got struct {
		OauthGrants []oauthGrant `json:"oauthGrants"`
	}
	require.Empty(t, f.query(t, oauthGrantsQuery, nil, &got))

	return got.OauthGrants
}

// TestManageOAuthGrants は、ログイン中のユーザーが、連携中の MCP のクライアントを一覧し、取り消せることを確かめる (#965)。
func TestManageOAuthGrants(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	const clientID = "https://claude.ai/oauth/claude-code-client-metadata"
	_, err := f.pool.Exec(t.Context(), `WITH grant_row AS (
			INSERT INTO oauth_grants (id, user_id, client_id, client_name, scopes, resource, created_at)
			VALUES (gen_random_uuid(), $1, $2, 'Claude Code', '{mcp:read,mcp:write}', 'https://kotatsu.example/mcp', now()) RETURNING id
		)
		INSERT INTO oauth_tokens (token_hash, grant_id, kind, created_at, expires_at)
		SELECT 'token', id, 'refresh', now(), now() + interval '30 days' FROM grant_row`, f.userID, clientID)
	require.NoError(t, err)

	grants := f.listOAuthGrants(t)
	require.Len(t, grants, 1)
	assert.Equal(t, oauthGrant{ID: grants[0].ID, ClientID: clientID, ClientName: "Claude Code", Scopes: []string{"mcp:read", "mcp:write"}, LastUsedAt: nil}, grants[0])
	// ほかのユーザーには見えず、取り消せない。
	other := f.as(t, otherUser, auth.RoleMember)
	assert.Empty(t, other.listOAuthGrants(t))
	assert.Equal(t, []string{codeNotFound}, other.query(t, deleteGrantMutation, map[string]any{varID: grants[0].ID}, nil))

	// 取り消しは、被害を抑える操作なので、直近の認証がなくてもできる。
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	var deleted struct {
		DeleteOAuthGrant string `json:"deleteOAuthGrant"`
	}
	require.Empty(t, f.query(t, deleteGrantMutation, map[string]any{varID: grants[0].ID}, &deleted))
	assert.Equal(t, grants[0].ID, deleted.DeleteOAuthGrant)
	assert.Empty(t, f.listOAuthGrants(t))
	// 連携のトークンも、まとめて消える。
	var tokens int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM oauth_tokens").Scan(&tokens))
	assert.Zero(t, tokens)
	var events int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM audit_events WHERE action = 'oauth_grant.delete' AND target_id = $1", clientID).Scan(&events))
	assert.Equal(t, 1, events)
	assert.Equal(t, []string{codeBadUserInput}, f.query(t, deleteGrantMutation, map[string]any{varID: "invalid"}, nil))
}
