package oauth_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/oauth"
	"github.com/SlashNephy/kotatsu/internal/settings"
)

const (
	testOrigin   = "https://kotatsu.example"
	testResource = testOrigin + "/mcp"
	testVerifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	testState    = "af0ifjsldkj"
	// testCallback は、Claude Code が、その都度選んだポートを付けて要求してくるリダイレクト URI。
	testCallback = "http://localhost:53682/callback"

	paramGrantType     = "grant_type"
	errorInvalidGrant  = "invalid_grant"
	errorInvalidTarget = "invalid_target"
)

func testNow() time.Time {
	return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
}

// fixture は、MCP を有効にした DB と、認可サーバーと、メンバーと視聴者のユーザー。
type fixture struct {
	db       *pgxpool.Pool
	clock    *clocktest.Fake
	server   *oauth.Server
	clientID string
	member   *auth.Principal
	viewer   *auth.Principal
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	clk := clocktest.NewFake(testNow())
	users := auth.NewUsers(clk)
	member, err := users.Create(t.Context(), pool, &auth.NewUser{Username: "member", DisplayName: "member", Role: auth.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	viewer, err := users.Create(t.Context(), pool, &auth.NewUser{Username: "viewer", DisplayName: "viewer", Role: auth.RoleViewer, PasswordHash: nil})
	require.NoError(t, err)
	setMCPEnabled(t, pool, true)

	srv, _ := metadataServer(t, "", claudeCodeDocument)
	server := oauth.NewServer(pool, clk, oauth.NewFetcherWithTransport(clk, srv.Client().Transport), audit.NewRecorder(clk))

	return &fixture{
		db: pool, clock: clk, server: server, clientID: srv.URL + metadataPath,
		member: &auth.Principal{UserID: member.ID, Role: auth.RoleMember},
		viewer: &auth.Principal{UserID: viewer.ID, Role: auth.RoleViewer},
	}
}

func setMCPEnabled(t *testing.T, db *pgxpool.Pool, enabled bool) {
	t.Helper()

	clk := clocktest.NewFake(testNow())
	_, err := settings.NewStore(clk, audit.NewRecorder(clk)).Update(t.Context(), db, audit.System("test"), &settings.Changes{MCPEnabled: &enabled})
	require.NoError(t, err)
}

func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// authorizeQuery は、Claude Code が送ってくるような、認可のエンドポイントのクエリを返す。
func (f *fixture) authorizeQuery() url.Values {
	return url.Values{
		"response_type":         {paramCode},
		paramClientID:           {f.clientID},
		paramRedirectURI:        {testCallback},
		"state":                 {testState},
		"code_challenge":        {challenge(testVerifier)},
		"code_challenge_method": {"S256"},
		paramResource:           {testResource},
		"scope":                 {"mcp:read mcp:write offline_access"},
	}
}

// approve は、principal として同意し、戻す URL から認可コードを取り出す。
func (f *fixture) approve(t *testing.T, principal *auth.Principal, scopes []string) string {
	t.Helper()

	redirect, err := f.server.Approve(t.Context(), audit.User(principal.UserID), principal, testOrigin, f.authorizeQuery(), scopes)
	require.NoError(t, err)
	u, err := url.Parse(redirect)
	require.NoError(t, err)
	assert.Equal(t, "localhost:53682", u.Host)
	assert.Equal(t, testState, u.Query().Get("state"))
	assert.Equal(t, testOrigin, u.Query().Get("iss"))

	return u.Query().Get(paramCode)
}

// post は、フォームを handler に送り、状態コードと JSON の本文を返す。
func post(t *testing.T, handler http.Handler, path string, form url.Values) (int, map[string]any) {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, testOrigin+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	body := map[string]any{}
	if rec.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	}

	return rec.Code, body
}

func (f *fixture) exchange(t *testing.T, code string) (int, map[string]any) {
	t.Helper()

	return post(t, f.server.TokenHandler(slog.New(slog.DiscardHandler)), oauth.ExchangePath, url.Values{
		paramGrantType: {grantAuthorizationCode}, paramCode: {code}, paramRedirectURI: {testCallback},
		paramClientID: {f.clientID}, "code_verifier": {testVerifier}, paramResource: {testResource},
	})
}

func (f *fixture) refresh(t *testing.T, refreshToken string) (int, map[string]any) {
	t.Helper()

	return post(t, f.server.TokenHandler(slog.New(slog.DiscardHandler)), oauth.ExchangePath, url.Values{
		paramGrantType: {grantRefreshToken}, grantRefreshToken: {refreshToken}, paramClientID: {f.clientID}, paramResource: {testResource},
	})
}

// TestAuthorizationCodeFlow は、同意から、トークンの発行、入れ替え、使い回しを見つけて取り消すまでを確かめる。
func TestAuthorizationCodeFlow(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	req, err := f.server.Prepare(t.Context(), testOrigin, f.authorizeQuery())
	require.NoError(t, err)
	assert.Equal(t, claudeCodeName, req.Client.ClientName)
	// offline_access のような知らないスコープは無視する。
	assert.Equal(t, []string{oauth.ScopeRead, oauth.ScopeWrite}, req.Scopes)

	code := f.approve(t, f.member, []string{oauth.ScopeWrite, oauth.ScopeRead})
	status, body := f.exchange(t, code)
	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, "Bearer", body["token_type"])
	assert.InDelta(t, 3600, body["expires_in"], 0)
	assert.Equal(t, "mcp:read mcp:write", body["scope"])
	access, _ := body["access_token"].(string)
	refresh, _ := body[grantRefreshToken].(string)
	assert.True(t, strings.HasPrefix(access, "kotatsu_at_"))
	assert.True(t, strings.HasPrefix(refresh, "kotatsu_rt_"))

	principal, grant, err := f.server.Authenticate(t.Context(), access, testResource)
	require.NoError(t, err)
	assert.Equal(t, f.member.UserID, principal.UserID)
	assert.Equal(t, auth.RoleMember, principal.Role)
	assert.Equal(t, f.clientID, grant.ClientID)
	assert.True(t, oauth.HasScope(grant.Scopes, oauth.ScopeRead))
	// ほかのオリジンの MCP サーバーには使えない。末尾の / と大文字と小文字は区別しない。
	_, _, err = f.server.Authenticate(t.Context(), access, "https://other.example/mcp")
	require.ErrorIs(t, err, oauth.ErrWrongAudience)
	_, _, err = f.server.Authenticate(t.Context(), access, "HTTPS://KOTATSU.EXAMPLE/mcp/")
	require.NoError(t, err)

	// 認可コードは一度しか使えない。
	status, body = f.exchange(t, code)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, errorInvalidGrant, body["error"])

	// リフレッシュトークンを入れ替える。
	status, body = f.refresh(t, refresh)
	require.Equal(t, http.StatusOK, status, body)
	newAccess, _ := body["access_token"].(string)
	newRefresh, _ := body[grantRefreshToken].(string)
	assert.NotEqual(t, refresh, newRefresh)
	_, _, err = f.server.Authenticate(t.Context(), newAccess, testResource)
	require.NoError(t, err)

	// 使い済みのリフレッシュトークンがもう一度来たら、連携ごと取り消す。
	status, body = f.refresh(t, refresh)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, errorInvalidGrant, body["error"])
	_, _, err = f.server.Authenticate(t.Context(), newAccess, testResource)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
	status, _ = f.refresh(t, newRefresh)
	assert.Equal(t, http.StatusBadRequest, status)
}

// TestAccessTokenExpires は、アクセストークンが 1 時間で使えなくなり、ユーザーを無効にしたら使えなくなることを確かめる。
func TestAccessTokenExpires(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	_, body := f.exchange(t, f.approve(t, f.member, []string{oauth.ScopeRead}))
	access, _ := body["access_token"].(string)
	refresh, _ := body[grantRefreshToken].(string)

	f.clock.Advance(59 * time.Minute)
	_, _, err := f.server.Authenticate(t.Context(), access, testResource)
	require.NoError(t, err)
	f.clock.Advance(time.Minute)
	_, _, err = f.server.Authenticate(t.Context(), access, testResource)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)

	// 同意した後にロールを下げられたら、書くスコープは使えない。
	_, body = f.exchange(t, f.approve(t, f.member, []string{oauth.ScopeRead, oauth.ScopeWrite}))
	writer, _ := body["access_token"].(string)
	writerRefresh, _ := body["refresh_token"].(string)
	_, err = f.db.Exec(t.Context(), "UPDATE users SET role = 'viewer' WHERE id = $1", f.member.UserID)
	require.NoError(t, err)
	_, grant, err := f.server.Authenticate(t.Context(), writer, testResource)
	require.NoError(t, err)
	assert.Equal(t, []string{oauth.ScopeRead}, grant.Scopes)
	status, body := f.refresh(t, writerRefresh)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, oauth.ScopeRead, body["scope"])

	_, err = f.db.Exec(t.Context(), "UPDATE users SET disabled_at = now() WHERE id = $1", f.member.UserID)
	require.NoError(t, err)
	status, body = f.refresh(t, refresh)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, errorInvalidGrant, body["error"])
}

// TestApproveScopes は、同意で選べるスコープを、求められたもので、ロールに許すものに限ることを確かめる。
func TestApproveScopes(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	tests := []struct {
		name      string
		principal *auth.Principal
		scopes    []string
		wantErr   bool
	}{
		{name: "member writes", principal: f.member, scopes: []string{oauth.ScopeRead, oauth.ScopeWrite}, wantErr: false},
		{name: "viewer reads", principal: f.viewer, scopes: []string{oauth.ScopeRead}, wantErr: false},
		{name: "viewer cannot write", principal: f.viewer, scopes: []string{oauth.ScopeWrite}, wantErr: true},
		{name: "unknown scope", principal: f.member, scopes: []string{"openid"}, wantErr: true},
		{name: "nothing chosen", principal: f.member, scopes: []string{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := f.server.Approve(t.Context(), audit.User(tt.principal.UserID), tt.principal, testOrigin, f.authorizeQuery(), tt.scopes)
			if tt.wantErr {
				require.ErrorIs(t, err, oauth.ErrInvalidScopeChoice)

				return
			}
			require.NoError(t, err)
			// 同意したことは、監査ログに残す。成功するのは、ユーザーごとに 1 つだけ。
			var events int
			require.NoError(t, f.db.QueryRow(t.Context(),
				"SELECT count(*) FROM audit_events WHERE action = 'oauth_grant.create' AND actor_user_id = $1", tt.principal.UserID).Scan(&events))
			assert.Equal(t, 1, events)
		})
	}
}

// TestPrepareErrors は、クライアントへ戻せない誤りは画面に出し、戻せる誤りはエラーを付けて戻すことを確かめる。
func TestPrepareErrors(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	tests := []struct {
		name string
		edit func(query url.Values)
		// wantCode は、クライアントへ戻すエラー。空なら、戻せないエラー (ErrInvalidAuthorizationRequest)。
		wantCode string
	}{
		{name: "http client_id", edit: func(q url.Values) { q.Set(paramClientID, "http://example.com/client.json") }, wantCode: ""},
		{name: "unregistered redirect_uri", edit: func(q url.Values) { q.Set(paramRedirectURI, "https://evil.example/cb") }, wantCode: ""},
		{name: "no redirect_uri", edit: func(q url.Values) { q.Del(paramRedirectURI) }, wantCode: ""},
		{name: "duplicated parameter", edit: func(q url.Values) { q.Add(paramRedirectURI, "https://evil.example/cb") }, wantCode: ""},
		{name: "implicit grant", edit: func(q url.Values) { q.Set("response_type", "token") }, wantCode: "unsupported_response_type"},
		{name: "no PKCE", edit: func(q url.Values) { q.Del("code_challenge") }, wantCode: "invalid_request"},
		{name: "plain PKCE", edit: func(q url.Values) { q.Set("code_challenge_method", "plain") }, wantCode: "invalid_request"},
		{name: "no resource", edit: func(q url.Values) { q.Del(paramResource) }, wantCode: errorInvalidTarget},
		{name: "other resource", edit: func(q url.Values) { q.Set(paramResource, "https://other.example/mcp") }, wantCode: errorInvalidTarget},
		{name: "no supported scope", edit: func(q url.Values) { q.Set("scope", "openid profile") }, wantCode: "invalid_scope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			query := f.authorizeQuery()
			tt.edit(query)
			_, err := f.server.Prepare(t.Context(), testOrigin, query)
			if tt.wantCode == "" {
				require.ErrorIs(t, err, oauth.ErrInvalidAuthorizationRequest)

				return
			}
			var authErr *oauth.AuthorizationError
			require.ErrorAs(t, err, &authErr)
			assert.Equal(t, tt.wantCode, authErr.Code)
			u, err := url.Parse(authErr.RedirectURL)
			require.NoError(t, err)
			assert.Equal(t, "localhost:53682", u.Host)
			assert.Equal(t, tt.wantCode, u.Query().Get("error"))
			assert.Equal(t, testState, u.Query().Get("state"))
			assert.Equal(t, testOrigin, u.Query().Get("iss"))
		})
	}
}

// TestTokenErrors は、トークンのエンドポイントが、正しくない引き換えを断り、認可コードを使えなくすることを確かめる。
func TestTokenErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		edit      func(form url.Values)
		wantError string
	}{
		{name: "wrong code_verifier", edit: func(form url.Values) { form.Set("code_verifier", strings.Repeat("a", 43)) }, wantError: errorInvalidGrant},
		{name: "wrong client_id", edit: func(form url.Values) { form.Set(paramClientID, "https://evil.example/client.json") }, wantError: errorInvalidGrant},
		{name: "wrong redirect_uri", edit: func(form url.Values) { form.Set(paramRedirectURI, "http://localhost:1/callback") }, wantError: errorInvalidGrant},
		{name: "wrong resource", edit: func(form url.Values) { form.Set(paramResource, "https://other.example/mcp") }, wantError: errorInvalidTarget},
		{name: "unsupported grant type", edit: func(form url.Values) { form.Set(paramGrantType, "password") }, wantError: "unsupported_grant_type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			code := f.approve(t, f.member, []string{oauth.ScopeRead})
			form := url.Values{
				paramGrantType: {grantAuthorizationCode}, paramCode: {code}, paramRedirectURI: {testCallback},
				paramClientID: {f.clientID}, "code_verifier": {testVerifier}, paramResource: {testResource},
			}
			tt.edit(form)
			status, body := post(t, f.server.TokenHandler(slog.New(slog.DiscardHandler)), oauth.ExchangePath, form)
			assert.Equal(t, http.StatusBadRequest, status)
			assert.Equal(t, tt.wantError, body["error"])
			if tt.wantError == "unsupported_grant_type" {
				return
			}
			// 引き換えに失敗した認可コードは、正しく送り直しても使えない。
			status, body = f.exchange(t, code)
			assert.Equal(t, http.StatusBadRequest, status)
			assert.Equal(t, errorInvalidGrant, body["error"])
		})
	}
}

// TestRevoke は、アクセストークンの失効はそのトークンだけを、リフレッシュトークンの失効は連携ごと取り消すことを確かめる。
func TestRevoke(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	revoke := func(token, clientID string) int {
		status, _ := post(t, f.server.RevokeHandler(slog.New(slog.DiscardHandler)), oauth.RevokePath, url.Values{
			"token": {token}, paramClientID: {clientID},
		})

		return status
	}
	_, body := f.exchange(t, f.approve(t, f.member, []string{oauth.ScopeRead}))
	access, _ := body["access_token"].(string)
	refresh, _ := body[grantRefreshToken].(string)

	// ほかのクライアントのトークンは消さないが、成功を返す。
	assert.Equal(t, http.StatusOK, revoke(access, "https://evil.example/client.json"))
	_, _, err := f.server.Authenticate(t.Context(), access, testResource)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, revoke(access, f.clientID))
	_, _, err = f.server.Authenticate(t.Context(), access, testResource)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
	status, body := f.refresh(t, refresh)
	require.Equal(t, http.StatusOK, status)
	newRefresh, _ := body[grantRefreshToken].(string)
	newAccess, _ := body["access_token"].(string)

	assert.Equal(t, http.StatusOK, revoke(newRefresh, f.clientID))
	_, _, err = f.server.Authenticate(t.Context(), newAccess, testResource)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
	// 知らないトークンでも成功を返す。
	assert.Equal(t, http.StatusOK, revoke("kotatsu_rt_unknown", f.clientID))
}

// TestDisabled は、管理者が MCP を有効にしていなければ、認可サーバーがないものとして振る舞うことを確かめる。
func TestDisabled(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	_, body := f.exchange(t, f.approve(t, f.member, []string{oauth.ScopeRead}))
	access, _ := body["access_token"].(string)
	logger := slog.New(slog.DiscardHandler)
	get := func(handler http.Handler, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, testOrigin+path, http.NoBody))

		return rec
	}

	rec := get(f.server.MetadataHandler(logger), oauth.MetadataPath)
	require.Equal(t, http.StatusOK, rec.Code)
	var metadata map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &metadata))
	assert.Equal(t, testOrigin, metadata["issuer"])
	assert.Equal(t, testOrigin+"/oauth/token", metadata["token_endpoint"])
	assert.Equal(t, true, metadata["client_id_metadata_document_supported"])
	assert.Equal(t, true, metadata["authorization_response_iss_parameter_supported"])
	assert.Equal(t, []any{authNone}, metadata["token_endpoint_auth_methods_supported"])
	rec = get(f.server.ProtectedResourceMetadataHandler(logger), oauth.ProtectedResourceMetadataPath)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"resource":"https://kotatsu.example/mcp","authorization_servers":["https://kotatsu.example"],`+
		`"scopes_supported":["mcp:read","mcp:write"],"bearer_methods_supported":["header"]}`, rec.Body.String())
	rec = get(f.server.AuthorizeHandler(logger), oauth.AuthorizePath+"?client_id=a&state=b")
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/authorize?client_id=a&state=b", rec.Header().Get("Location"))

	setMCPEnabled(t, f.db, false)
	for _, tt := range []struct {
		handler http.Handler
		path    string
	}{
		{handler: f.server.MetadataHandler(logger), path: oauth.MetadataPath},
		{handler: f.server.ProtectedResourceMetadataHandler(logger), path: oauth.ProtectedResourceMetadataPath},
		{handler: f.server.AuthorizeHandler(logger), path: oauth.AuthorizePath},
	} {
		assert.Equal(t, http.StatusNotFound, get(tt.handler, tt.path).Code, tt.path)
	}
	_, _, err := f.server.Authenticate(t.Context(), access, testResource)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
	_, err = f.server.Prepare(t.Context(), testOrigin, f.authorizeQuery())
	require.ErrorIs(t, err, oauth.ErrDisabled)
}

// TestPrune は、期限を過ぎた認可コードとトークンと、トークンがなくなった連携を消すことを確かめる。
func TestPrune(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.approve(t, f.member, []string{oauth.ScopeRead})
	_, body := f.exchange(t, f.approve(t, f.member, []string{oauth.ScopeRead}))
	refresh, _ := body[grantRefreshToken].(string)
	count := func(table string) int {
		var n int
		require.NoError(t, f.db.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&n))

		return n
	}

	// 1 日後には、認可コードとアクセストークンが期限を過ぎている。リフレッシュトークンと連携は残る。
	f.clock.Advance(24 * time.Hour)
	deleted, err := oauth.Prune(t.Context(), f.db, f.clock)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)
	assert.Zero(t, count("oauth_authorization_codes"))
	assert.Equal(t, 1, count("oauth_tokens"))
	assert.Equal(t, 1, count("oauth_grants"))
	status, _ := f.refresh(t, refresh)
	require.Equal(t, http.StatusOK, status)

	// 30 日使わなければ、リフレッシュトークンが期限を過ぎ、連携も消える。
	f.clock.Advance(30 * 24 * time.Hour)
	_, err = oauth.Prune(t.Context(), f.db, f.clock)
	require.NoError(t, err)
	assert.Zero(t, count("oauth_tokens"))
	assert.Zero(t, count("oauth_grants"))
}
