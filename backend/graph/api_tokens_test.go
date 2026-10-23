package graph_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/auth"
)

const (
	listIssuedQuery = `{ apiTokens { id name lastUsedAt } }`
	issueMutation   = `mutation($name: String!) { createApiToken(name: $name) { token apiToken { id name } } }`
	revokeMutation  = `mutation($id: ID!) { deleteApiToken(id: $id) }`
	clientName      = "macOS の kiririn"
)

type apiToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

func (f *fixture) listAPITokens(t *testing.T) []apiToken {
	t.Helper()

	var got struct {
		APITokens []apiToken `json:"apiTokens"`
	}
	require.Empty(t, f.query(t, listIssuedQuery, nil, &got))

	return got.APITokens
}

// TestManageAPITokens は、ログイン中のユーザーが、API トークンを発行し、一覧し、取り消せることを確かめる (#431)。
func TestManageAPITokens(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	assert.Empty(t, f.listAPITokens(t))

	var created struct {
		CreateAPIToken struct {
			Token    string   `json:"token"`
			APIToken apiToken `json:"apiToken"`
		} `json:"createApiToken"`
	}
	require.Empty(t, f.query(t, issueMutation, map[string]any{varName: clientName}, &created))
	assert.True(t, strings.HasPrefix(created.CreateAPIToken.Token, "kotatsu_"))
	assert.Equal(t, clientName, created.CreateAPIToken.APIToken.Name)
	assert.Equal(t, []apiToken{{ID: created.CreateAPIToken.APIToken.ID, Name: clientName, LastUsedAt: nil}}, f.listAPITokens(t))
	// 一覧にはトークンそのものを出さないので、ほかのユーザーからも自分のトークンは見えない。
	assert.Empty(t, f.as(t, otherUser, auth.RoleMember).listAPITokens(t))

	// ほかのユーザーのトークンは消せない。
	assert.Equal(t, []string{codeNotFound}, f.as(t, "another", auth.RoleMember).query(t, revokeMutation, map[string]any{varID: created.CreateAPIToken.APIToken.ID}, nil))

	var deleted struct {
		DeleteAPIToken string `json:"deleteApiToken"`
	}
	require.Empty(t, f.query(t, revokeMutation, map[string]any{varID: created.CreateAPIToken.APIToken.ID}, &deleted))
	assert.Equal(t, created.CreateAPIToken.APIToken.ID, deleted.DeleteAPIToken)
	assert.Empty(t, f.listAPITokens(t))
}

// TestCreateAPITokenRejects は、発行に直近の認証と、正しい名前が要ることを確かめる。
func TestCreateAPITokenRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		variables map[string]any
		stale     bool
		want      string
	}{
		{name: "空の名前", variables: map[string]any{varName: "  "}, stale: false, want: codeBadUserInput},
		{name: "長すぎる名前", variables: map[string]any{varName: strings.Repeat("あ", 65)}, stale: false, want: codeBadUserInput},
		{name: "直近の認証がない", variables: map[string]any{varName: clientName}, stale: true, want: codeReauthRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			if tt.stale {
				f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
			}
			assert.Equal(t, []string{tt.want}, f.query(t, issueMutation, tt.variables, nil))
			assert.Empty(t, f.listAPITokens(t))
		})
	}

	f := newFixture(t, nil)
	assert.Equal(t, []string{codeBadUserInput}, f.query(t, revokeMutation, map[string]any{varID: graph.GlobalID("Passkey", "x")}, nil))
}
