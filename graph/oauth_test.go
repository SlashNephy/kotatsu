package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/auth"
)

const approveMutation = `mutation($authorizationQuery: String!, $scopes: [String!]!) { approveOAuthAuthorization(query: $authorizationQuery, scopes: $scopes) }`

// TestApproveOAuthAuthorizationRejects は、同意に直近の認証が要り、クエリが読めなければ断ることを確かめる (#963)。
// 同意から認可コードの発行までは、internal/oauth のテストで確かめる。
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
