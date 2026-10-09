package graph_test

import (
	"bufio"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
)

const (
	contentTypeJSON = "application/json"
	viewerQuery     = `{"query":"{ viewer { id username displayName role } }"}`
	badUserInput    = `"code":"BAD_USER_INPUT"`
)

func testNow() time.Time {
	return time.Date(2026, 10, 10, 12, 10, 0, 0, time.UTC)
}

type fixture struct {
	pool     *pgxpool.Pool
	server   *httptest.Server
	clock    *clocktest.Fake
	sessions *auth.Sessions
	// token は、ログイン済みのセッションのトークン。
	token string
}

// as は、role のユーザーでログインしたセッションで、同じサーバーを使う fixture を返す。
func (f *fixture) as(t *testing.T, username string, role auth.Role) *fixture {
	t.Helper()

	user, err := auth.NewUsers(f.clock).Create(t.Context(), f.pool, &auth.NewUser{
		Username: username, DisplayName: username, Role: role, PasswordHash: nil,
	})
	require.NoError(t, err)
	token, _, err := f.sessions.Create(t.Context(), f.pool, user.ID, nil)
	require.NoError(t, err)

	return &fixture{pool: f.pool, server: f.server, clock: f.clock, sessions: f.sessions, token: token}
}

// newFixture は、ログイン済みのセッションを 1 つ持つ DB と、セッションのミドルウェアを通した /graphql を用意する。
func newFixture(t *testing.T, opts *graph.Options) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	clk := clocktest.NewFake(testNow())
	user, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{
		Username: testUsername, DisplayName: "こたつ", Role: auth.RoleMember, PasswordHash: nil,
	})
	require.NoError(t, err)
	sessions := auth.NewSessions(&auth.SessionsOptions{IdleTimeout: 0, AbsoluteTimeout: 0, IsSecure: nil, Clock: clk})
	token, _, err := sessions.Create(t.Context(), pool, user.ID, nil)
	require.NoError(t, err)

	if opts == nil {
		opts = &graph.Options{}
	}
	opts.DB, opts.Clock, opts.Logger = pool, clk, slog.New(slog.DiscardHandler)
	mux := http.NewServeMux()
	mux.Handle("/graphql", sessions.Middleware(pool)(graph.NewHandler(opts)))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return &fixture{pool: pool, server: server, clock: clk, sessions: sessions, token: token}
}

type request struct {
	method      string
	contentType string
	accept      string
	body        string
	loggedIn    bool
}

func (f *fixture) do(t *testing.T, req request) (int, string) {
	t.Helper()

	r, err := http.NewRequestWithContext(t.Context(), req.method, f.server.URL+"/graphql", strings.NewReader(req.body))
	require.NoError(t, err)
	if req.contentType != "" {
		r.Header.Set("Content-Type", req.contentType)
	}
	if req.accept != "" {
		r.Header.Set("Accept", req.accept)
	}
	if req.loggedIn {
		r.Header.Set("Cookie", auth.CookieName+"="+f.token)
	}
	resp, err := http.DefaultClient.Do(r)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(body)
}

func TestHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		opts       *graph.Options
		req        request
		wantStatus int
		// wantBody は、応答の本文に含まれるべき文字列。
		wantBody []string
	}{
		{
			name:       "logged-in user gets the viewer",
			req:        request{method: http.MethodPost, contentType: contentTypeJSON, body: viewerQuery, loggedIn: true},
			wantStatus: http.StatusOK,
			wantBody:   []string{`"username":"kotatsu"`, `"displayName":"こたつ"`, `"role":"MEMBER"`},
		},
		{
			name:       "unauthenticated request is rejected before GraphQL",
			req:        request{method: http.MethodPost, contentType: contentTypeJSON, body: viewerQuery, loggedIn: false},
			wantStatus: http.StatusUnauthorized,
			wantBody:   []string{`"code":"UNAUTHENTICATED"`},
		},
		{
			name:       "unauthenticated introspection is rejected",
			req:        request{method: http.MethodPost, contentType: contentTypeJSON, body: `{"query":"{ __schema { queryType { name } } }"}`, loggedIn: false},
			wantStatus: http.StatusUnauthorized,
			wantBody:   []string{`"code":"UNAUTHENTICATED"`},
		},
		{
			name:       "logged-in user can introspect",
			req:        request{method: http.MethodPost, contentType: contentTypeJSON, body: `{"query":"{ __schema { queryType { name } } }"}`, loggedIn: true},
			wantStatus: http.StatusOK,
			wantBody:   []string{`"queryType":{"name":"Query"}`},
		},
		{
			name:       "unknown field is a validation error, not an internal error",
			req:        request{method: http.MethodPost, contentType: contentTypeJSON, body: `{"query":"{ viewer { nope } }"}`, loggedIn: true},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   []string{`"code":"GRAPHQL_VALIDATION_FAILED"`, `Cannot query field \"nope\"`},
		},
		{
			name:       "syntax error is a parse error, not an internal error",
			req:        request{method: http.MethodPost, contentType: contentTypeJSON, body: `{"query":"{ viewer { "}`, loggedIn: true},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   []string{`"code":"GRAPHQL_PARSE_FAILED"`},
		},
		{
			name:       "GET query is rejected",
			req:        request{method: http.MethodGet, contentType: "", body: "", loggedIn: true},
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   []string{badUserInput},
		},
		{
			name:       "form post is rejected",
			req:        request{method: http.MethodPost, contentType: "application/x-www-form-urlencoded", body: "query={viewer{id}}", loggedIn: true},
			wantStatus: http.StatusUnsupportedMediaType,
			wantBody:   []string{badUserInput},
		},
		{
			name:       "text/plain post is rejected",
			req:        request{method: http.MethodPost, contentType: "text/plain", body: viewerQuery, loggedIn: true},
			wantStatus: http.StatusUnsupportedMediaType,
			wantBody:   []string{badUserInput},
		},
		{
			name:       "too deep query is rejected",
			opts:       &graph.Options{MaxDepth: 1},
			req:        request{method: http.MethodPost, contentType: contentTypeJSON, body: viewerQuery, loggedIn: true},
			wantStatus: http.StatusOK,
			wantBody:   []string{`"code":"DEPTH_LIMIT_EXCEEDED"`, `"data":null`},
		},
		{
			name: "fragments are counted in the depth",
			opts: &graph.Options{MaxDepth: 1},
			req: request{
				method: http.MethodPost, contentType: contentTypeJSON, loggedIn: true,
				body: `{"query":"{ ...v } fragment v on Query { viewer { id } }"}`,
			},
			wantStatus: http.StatusOK,
			wantBody:   []string{`"code":"DEPTH_LIMIT_EXCEEDED"`},
		},
		{
			name:       "too complex query is rejected",
			opts:       &graph.Options{MaxComplexity: 2},
			req:        request{method: http.MethodPost, contentType: contentTypeJSON, body: viewerQuery, loggedIn: true},
			wantStatus: http.StatusOK,
			wantBody:   []string{`"code":"COMPLEXITY_LIMIT_EXCEEDED"`, `"data":null`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, body := newFixture(t, tt.opts).do(t, tt.req)
			assert.Equal(t, tt.wantStatus, status, body)
			for _, want := range tt.wantBody {
				assert.Contains(t, body, want)
			}
		})
	}
}

// TestSubscriptionOverSSE は、subscription が SSE で届くことを確かめる。
func TestSubscriptionOverSSE(t *testing.T) {
	t.Parallel()

	f := newFixture(t, &graph.Options{HeartbeatInterval: 10 * time.Millisecond})
	r, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.server.URL+"/graphql", strings.NewReader(`{"query":"subscription { heartbeat }"}`))
	require.NoError(t, err)
	r.Header.Set("Content-Type", contentTypeJSON)
	r.Header.Set("Accept", "text/event-stream")
	r.Header.Set("Cookie", auth.CookieName+"="+f.token)
	resp, err := http.DefaultClient.Do(r)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")

	// 2 回分の時刻が、SSE の next イベントとして届く。
	scanner := bufio.NewScanner(resp.Body)
	events := 0
	for scanner.Scan() && events < 2 {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"heartbeat":"2026-10-10T12:10:00Z"`) {
			events++
		}
	}
	assert.Equal(t, 2, events)
}
