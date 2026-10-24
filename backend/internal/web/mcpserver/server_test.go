package mcpserver_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/oauth"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
	"github.com/SlashNephy/kotatsu/internal/web/mcpserver"
)

const (
	readAccess  = "kotatsu_at_read"
	writeAccess = "kotatsu_at_write"
	otherAccess = "kotatsu_at_other"
	// legacyVersion は、Stateless のままでもつながるかを確かめる、古い版。
	legacyVersion = "2025-11-25"
)

// fakeAuthenticator は、決まったトークンだけを受け付ける。otherAccess は、ほかの MCP サーバー宛てのトークンとして扱う。
type fakeAuthenticator struct {
	resource string
	enabled  bool
	userID   uuid.UUID
}

var _ mcpserver.Authenticator = new(fakeAuthenticator)

func (a *fakeAuthenticator) Enabled(context.Context) (bool, error) { return a.enabled, nil }

func (a *fakeAuthenticator) Authenticate(_ context.Context, token, resource string) (*auth.Principal, *oauth.Grant, error) {
	scopes := map[string][]string{readAccess: {oauth.ScopeRead}, writeAccess: {oauth.ScopeWrite}}[token]
	switch {
	case token == otherAccess || resource != a.resource:
		return nil, nil, oauth.ErrWrongAudience
	case scopes == nil:
		return nil, nil, auth.ErrUnauthenticated
	}
	principal := &auth.Principal{UserID: a.userID, Role: auth.RoleMember, OAuthClientID: "https://claude.ai/oauth/claude-code-client-metadata"}

	return principal, &oauth.Grant{ID: uuid.New(), ClientID: principal.OAuthClientID, ClientName: "Claude Code", Scopes: scopes}, nil
}

// fakeGraphQL は、受け取った操作を覚え、決まった応答を返す GraphQL のハンドラー。
type fakeGraphQL struct {
	mu        sync.Mutex
	queries   []string
	variables []map[string]any
	principal *auth.Principal
	response  string
}

func (g *fakeGraphQL) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	g.mu.Lock()
	g.queries = append(g.queries, req.Query)
	g.variables = append(g.variables, req.Variables)
	g.principal = auth.PrincipalFrom(r.Context())
	response := g.response
	g.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, response)
}

type fixture struct {
	server  *httptest.Server
	auth    *fakeAuthenticator
	graphql *fakeGraphQL
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	f := &fixture{auth: &fakeAuthenticator{enabled: true, userID: uuid.New()}, graphql: &fakeGraphQL{response: `{"data":{"ok":true}}`}}
	handler, err := mcpserver.NewHandler(f.auth, f.graphql, graph.Schema(), slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	f.server = httptest.NewServer(handler)
	t.Cleanup(f.server.Close)
	f.auth.resource = f.server.URL + "/mcp"

	return f
}

// bearer は、Authorization を付けて送る http.RoundTripper。
type bearer struct {
	token string
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)

	return http.DefaultTransport.RoundTrip(r)
}

// connect は、公式の SDK のクライアントで、token を持ってつなぐ。version が空なら、最新の版でつなぐ。
func (f *fixture) connect(t *testing.T, token, version string) *mcp.ClientSession {
	t.Helper()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: f.server.URL + "/mcp", HTTPClient: &http.Client{Transport: bearer{token: token}}}
	session, err := client.Connect(t.Context(), transport, &mcp.ClientSessionOptions{ProtocolVersion: version})
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	return session
}

// TestListTools は、ツールを決まった順に返し、読むツールと消すツールに印を付けることを確かめる。
func TestListTools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
	}{
		{name: "2026-07-28", version: ""},
		// Stateless のままでも、古い版のクライアントがつながる。
		{name: legacyVersion, version: legacyVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			result, err := f.connect(t, readAccess, tt.version).ListTools(t.Context(), nil)
			require.NoError(t, err)
			names := make([]string, 0, len(result.Tools))
			var search *mcp.Tool
			for _, tool := range result.Tools {
				if tool.Name == "search_programs" {
					search = tool
				}
				names = append(names, tool.Name)
				switch {
				case strings.HasPrefix(tool.Name, "delete_"):
					require.NotNil(t, tool.Annotations.DestructiveHint)
					assert.True(t, *tool.Annotations.DestructiveHint, tool.Name)
				case strings.HasPrefix(tool.Name, "list_"), strings.HasPrefix(tool.Name, "get_"), strings.HasPrefix(tool.Name, "search_"):
					assert.True(t, tool.Annotations.ReadOnlyHint, tool.Name)
				default:
					assert.False(t, tool.Annotations.ReadOnlyHint, tool.Name)
				}
			}
			// キャッシュしやすいよう、決まった順 (名前の順) に返す。
			assert.Equal(t, []string{
				"create_reservation", "create_rule", "delete_reservation", "delete_rule", "get_reservation", "get_rule", "list_genres",
				"list_reservations", "list_rules", "list_services", "search_programs", "search_recordings", "update_reservation", "update_rule",
			}, names)
			require.NotNil(t, search)
			schema, err := json.Marshal(search.InputSchema)
			require.NoError(t, err)
			assert.Contains(t, string(schema), `"required":["conditions"]`)
			assert.Contains(t, string(schema), `"keywordMatch"`)
		})
	}
}

// TestServerIcons は、serverInfo で、そのまま描ける Kotatsu のアイコンを data URI で返すことを確かめる。
func TestServerIcons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
	}{
		{name: "2026-07-28", version: ""},
		{name: legacyVersion, version: legacyVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			result := f.connect(t, readAccess, tt.version).InitializeResult()
			require.NotNil(t, result)
			require.NotNil(t, result.ServerInfo)
			types := make([]string, 0, len(result.ServerInfo.Icons))
			for _, icon := range result.ServerInfo.Icons {
				prefix := "data:" + icon.MIMEType + ";base64,"
				require.True(t, strings.HasPrefix(icon.Source, prefix), icon.MIMEType)
				data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(icon.Source, prefix))
				require.NoError(t, err)
				// 中身が、名乗った MIME タイプの画像であることを確かめる。http.DetectContentType は SVG を見分けない。
				if icon.MIMEType == "image/svg+xml" {
					assert.True(t, strings.HasPrefix(string(data), "<svg "))
				} else {
					assert.Equal(t, icon.MIMEType, http.DetectContentType(data))
				}
				types = append(types, icon.MIMEType)
			}
			assert.Equal(t, []string{"image/svg+xml", "image/png"}, types)
		})
	}
}

// TestCallTool は、ツールを、トークンのユーザーとして GraphQL で実行し、data を返すことを確かめる。
func TestCallTool(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	result, err := f.connect(t, writeAccess, "").CallTool(t.Context(), &mcp.CallToolParams{
		Name: "create_reservation", Arguments: map[string]any{"input": map[string]any{"program": "UHJvZ3JhbTox"}},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, result.Content)
	assert.JSONEq(t, `{"ok":true}`, string(mustMarshal(t, result.StructuredContent)))
	require.Len(t, f.graphql.queries, 1)
	assert.Contains(t, f.graphql.queries[0], "createProgramReservation")
	assert.Equal(t, map[string]any{"input": map[string]any{"program": "UHJvZ3JhbTox"}}, f.graphql.variables[0])
	require.NotNil(t, f.graphql.principal)
	assert.Equal(t, f.auth.userID, f.graphql.principal.UserID)
	// 監査ログに残すため、経由したクライアントを持たせる。
	assert.Equal(t, "https://claude.ai/oauth/claude-code-client-metadata", f.graphql.principal.OAuthClientID)

	// ジャンルの一覧は、GraphQL を使わずに答える。
	result, err = f.connect(t, readAccess, "").CallTool(t.Context(), &mcp.CallToolParams{Name: "list_genres", Arguments: map[string]any{}})
	require.NoError(t, err)
	assert.Contains(t, string(mustMarshal(t, result.StructuredContent)), "アニメ・特撮")
}

// TestCallToolErrors は、GraphQL のエラーと、大きすぎる応答を、ツールのエラーとして返すことを確かめる。
func TestCallToolErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response string
		want     string
	}{
		{name: "GraphQL error", response: `{"errors":[{"message":"viewers cannot change rules and reservations","extensions":{"code":"FORBIDDEN"}}],"data":null}`, want: "FORBIDDEN: viewers cannot change rules and reservations"},
		{name: "too large", response: `{"data":{"text":"` + strings.Repeat("a", 70_000) + `"}}`, want: "the result is too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			f.graphql.response = tt.response
			result, err := f.connect(t, writeAccess, "").CallTool(t.Context(), &mcp.CallToolParams{Name: "list_rules", Arguments: map[string]any{}})
			require.NoError(t, err)
			assert.True(t, result.IsError)
			require.Len(t, result.Content, 1)
			text, ok := result.Content[0].(*mcp.TextContent)
			require.True(t, ok)
			assert.Contains(t, text.Text, tt.want)
		})
	}
}

// post は、/mcp に JSON-RPC の本文を送り、応答の状態コードとヘッダーを返す。
func (f *fixture) post(t *testing.T, token, body string, header map[string]string) (int, http.Header) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.server.URL+"/mcp", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", legacyVersion)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range header {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	return resp.StatusCode, resp.Header
}

// TestAuthorization は、トークンがなければ 401、ほかの MCP サーバー宛てなら 401 (invalid_token)、
// スコープが足りなければ 403 (insufficient_scope) を返すことを確かめる。Mcp-Name ヘッダーを偽っても、すり抜けられない。
func TestAuthorization(t *testing.T) {
	t.Parallel()

	const createReservation = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_reservation","arguments":{"input":{"program":"x"}}}}`
	tests := []struct {
		name          string
		access        string
		header        map[string]string
		wantStatus    int
		wantChallenge string
	}{
		{name: "no token", access: "", wantStatus: http.StatusUnauthorized, wantChallenge: `scope="mcp:read mcp:write"`},
		{name: "unknown token", access: "kotatsu_at_" + "unknown", wantStatus: http.StatusUnauthorized, wantChallenge: `error="invalid_token"`},
		{name: "other audience", access: otherAccess, wantStatus: http.StatusUnauthorized, wantChallenge: `error="invalid_token"`},
		{name: "read scope", access: readAccess, wantStatus: http.StatusForbidden, wantChallenge: `error="insufficient_scope", scope="mcp:write"`},
		{
			name: "forged Mcp-Name", access: readAccess, header: map[string]string{"Mcp-Name": "list_services", "Mcp-Method": "tools/call"},
			wantStatus: http.StatusForbidden, wantChallenge: `error="insufficient_scope"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			status, header := f.post(t, tt.access, createReservation, tt.header)
			assert.Equal(t, tt.wantStatus, status)
			challenge := header.Get("WWW-Authenticate")
			assert.Contains(t, challenge, tt.wantChallenge)
			assert.Contains(t, challenge, `resource_metadata="`+f.server.URL+`/.well-known/oauth-protected-resource/mcp"`)
			assert.Empty(t, f.graphql.queries)
		})
	}
}

// TestDisabled は、管理者が MCP を有効にしていなければ、/mcp がないものとして 404 を返すことを確かめる。
func TestDisabled(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.auth.enabled = false
	status, _ := f.post(t, writeAccess, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	assert.Equal(t, http.StatusNotFound, status)
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()

	encoded, err := json.Marshal(v)
	require.NoError(t, err)

	return encoded
}
