package auth_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
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
	"github.com/SlashNephy/kotatsu/internal/oidctest"
)

const (
	idpClientID  = "kotatsu-web"
	idpClientKey = "kotatsu-oidc-client-key-material"
	idpSubject   = "idp-user-1"
	linkedName   = "kotatsu"
	groupsClaim  = "groups"
	nameClaim    = "preferred_username"
	adminsGroup  = "kotatsu-admins"
	membersGroup = "kotatsu-members"
	viewersGroup = "kotatsu-viewers"
)

type oidcFixture struct {
	db     *pgxpool.Pool
	clock  *clocktest.Fake
	idp    *oidctest.Provider
	oidc   *auth.OIDC
	users  *auth.Users
	client *auth.ClientInfo
}

// newOIDCFixture は、テスト用の IdP と、それを使う OIDC を用意する。autoCreate で、自動作成の設定を変える。
func newOIDCFixture(t *testing.T, autoCreate auth.OIDCAutoCreate) *oidcFixture {
	t.Helper()

	clk := clocktest.NewFake(testNow())
	idp, err := oidctest.New(idpClientID, idpClientKey, testOrigin+auth.OIDCCallbackPath)
	require.NoError(t, err)
	t.Cleanup(idp.Close)
	// ID トークンの発行と期限は、Kotatsu と同じ時計で決める。
	idp.Now = clk.Now
	idp.Subject = idpSubject

	return &oidcFixture{
		db:    newDB(t),
		clock: clk,
		idp:   idp,
		oidc: auth.NewOIDC(&auth.OIDCOptions{
			Settings: auth.OIDCSettings{
				Issuer: idp.Issuer(), ClientID: idpClientID, ClientSecret: idpClientKey, RedirectURL: "",
				Scopes: []string{"openid", "profile", groupsClaim}, DisplayName: "Test IdP", UsernameClaim: nameClaim,
				AutoCreate: autoCreate,
			},
			Clock: clk, Recorder: audit.NewRecorder(clk), Logger: slog.New(slog.DiscardHandler),
			Sessions: auth.NewSessions(&auth.SessionsOptions{Clock: clk}), HTTPClient: nil,
		}),
		users:  auth.NewUsers(clk),
		client: &auth.ClientInfo{UserAgent: browserUA, SourceIP: netip.MustParseAddr("192.168.1.10"), Host: testHost, Secure: true},
	}
}

// login は、ログインを始め、IdP でログインし、戻ってきたパラメーターを change で変えてから、ログインを完了させる。
func (f *oidcFixture) login(t *testing.T, change func(t *testing.T, f *oidcFixture, token *string, callback *auth.OIDCCallback)) (*auth.LoginResult, error) {
	t.Helper()

	login, err := f.oidc.Begin(t.Context(), f.db, f.client)
	require.NoError(t, err)
	redirect, err := f.idp.Authorize(login.AuthorizationURL)
	require.NoError(t, err)
	u, err := url.Parse(redirect)
	require.NoError(t, err)
	assert.Equal(t, testOrigin+auth.OIDCCallbackPath, u.Scheme+"://"+u.Host+u.Path)
	token := login.Token
	callback := &auth.OIDCCallback{State: u.Query().Get("state"), Code: u.Query().Get("code"), Error: ""}
	if change != nil {
		change(t, f, &token, callback)
	}

	return f.oidc.Complete(t.Context(), f.db, token, callback, f.client)
}

func (f *oidcFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()

	var n int
	require.NoError(t, f.db.QueryRow(t.Context(), query, args...).Scan(&n))

	return n
}

func (f *oidcFixture) linkedUser(t *testing.T) *auth.User {
	t.Helper()

	user, err := f.users.Create(t.Context(), f.db, &auth.NewUser{Username: linkedName, DisplayName: linkedName, Role: auth.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	require.NoError(t, f.oidc.Link(t.Context(), f.db, audit.System("cli"), linkedName, idpSubject))

	return user
}

// autoCreateGroups は、管理者、メンバー、閲覧者のグループを 1 つずつ決めた自動作成の設定。
func autoCreateGroups() auth.OIDCAutoCreate {
	return auth.OIDCAutoCreate{
		Enabled: true, GroupsClaim: groupsClaim,
		AdminGroups: []string{adminsGroup}, MemberGroups: []string{membersGroup}, ViewerGroups: []string{viewersGroup},
	}
}

// TestOIDCLogin は、事前に紐付けたユーザーでログインでき、紐付けていないユーザーは、自動作成の条件を満たすときだけ作ることを確かめる (設計書 9.4)。
func TestOIDCLogin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		autoCreate auth.OIDCAutoCreate
		prepare    func(t *testing.T, f *oidcFixture)
		claims     map[string]any
		wantErr    error
		wantRole   auth.Role
		// wantUsername は、ログインしたユーザーの名前。
		wantUsername string
		// wantDisplayName は、ログインしたユーザーの表示名。空なら確かめない。
		wantDisplayName string
	}{
		{
			name: "linked user", wantRole: auth.RoleMember, wantUsername: linkedName,
			prepare: func(t *testing.T, f *oidcFixture) { t.Helper(); f.linkedUser(t) },
		},
		{
			name: "linked user is disabled", wantErr: auth.ErrOIDCAccessDenied,
			prepare: func(t *testing.T, f *oidcFixture) {
				t.Helper()
				require.NoError(t, f.users.Disable(t.Context(), f.db, f.linkedUser(t).ID))
			},
		},
		{name: "not linked and auto_create is disabled", wantErr: auth.ErrOIDCAccessDenied},
		{
			name: "auto_create picks the strongest role", autoCreate: autoCreateGroups(),
			claims:   map[string]any{nameClaim: mikanName, nameText: mikanLabel, groupsClaim: []string{viewersGroup, adminsGroup}},
			wantRole: auth.RoleAdmin, wantUsername: mikanName, wantDisplayName: mikanLabel,
		},
		{
			name: "auto_create accepts a single group string", autoCreate: autoCreateGroups(),
			claims:   map[string]any{nameClaim: nekoName, groupsClaim: membersGroup},
			wantRole: auth.RoleMember, wantUsername: nekoName,
		},
		{
			name: "auto_create rejects users outside the groups", autoCreate: autoCreateGroups(),
			claims: map[string]any{nameClaim: "guest", groupsClaim: []string{"everyone"}}, wantErr: auth.ErrOIDCAccessDenied,
		},
		{
			name: "auto_create needs the username claim", autoCreate: autoCreateGroups(),
			claims: map[string]any{groupsClaim: []string{membersGroup}}, wantErr: auth.ErrOIDCAccessDenied,
		},
		{
			name: "auto_create does not take over an existing username", autoCreate: autoCreateGroups(),
			claims:  map[string]any{nameClaim: linkedName, groupsClaim: []string{adminsGroup}},
			wantErr: auth.ErrOIDCUsernameTaken,
			prepare: func(t *testing.T, f *oidcFixture) {
				t.Helper()
				_, err := f.users.Create(t.Context(), f.db, &auth.NewUser{Username: "Kotatsu", DisplayName: linkedName, Role: auth.RoleViewer, PasswordHash: nil})
				require.NoError(t, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newOIDCFixture(t, tt.autoCreate)
			if tt.prepare != nil {
				tt.prepare(t, f)
			}
			if tt.claims != nil {
				f.idp.Claims = tt.claims
			}
			users := f.count(t, "SELECT count(*) FROM users")

			result, err := f.login(t, nil)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Zero(t, f.count(t, "SELECT count(*) FROM sessions"))
				assert.Equal(t, users, f.count(t, "SELECT count(*) FROM users"), "no user must be created")
				assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.login_failed' AND summary->>'method' = 'oidc' AND summary->>'subject' = $1", idpSubject))

				return
			}
			require.NoError(t, err)
			var (
				username    string
				displayName string
				role        string
				hasHash     bool
			)
			require.NoError(t, f.db.QueryRow(t.Context(), "SELECT username, display_name, role, password_hash IS NOT NULL FROM users WHERE id = $1", result.Session.UserID).
				Scan(&username, &displayName, &role, &hasHash))
			assert.Equal(t, tt.wantUsername, username)
			if tt.wantDisplayName != "" {
				assert.Equal(t, tt.wantDisplayName, displayName)
			}
			assert.Equal(t, string(tt.wantRole), role)
			assert.False(t, hasHash, "an OIDC user has no password")
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM user_oidc_identities WHERE user_id = $1 AND issuer = $2 AND subject = $3 AND last_used_at = $4",
				result.Session.UserID, f.idp.Issuer(), idpSubject, testNow()))
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.login' AND summary->>'method' = 'oidc'"))
			assert.Zero(t, f.count(t, "SELECT count(*) FROM oidc_logins"), "the login state is consumed")

			// 2 回目からは、紐付けたユーザーでログインし、新しくは作らない。ロールも変えない。
			f.idp.Claims[groupsClaim] = []string{viewersGroup}
			again, err := f.login(t, nil)
			require.NoError(t, err)
			assert.Equal(t, result.Session.UserID, again.Session.UserID)
			require.NoError(t, f.db.QueryRow(t.Context(), "SELECT role FROM users WHERE id = $1", again.Session.UserID).Scan(&role))
			assert.Equal(t, string(tt.wantRole), role)
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM user_oidc_identities WHERE user_id = $1 AND last_used_at IS NOT NULL", again.Session.UserID))
		})
	}
}

// TestOIDCPicture は、ログインのたびに、ID トークンの picture を、アイコンの URL として書き直すことを確かめる (#778)。
// http か https の絶対 URL でなければ、ないものとして扱う。
func TestOIDCPicture(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		picture any
		want    *string
	}{
		{name: "https", picture: "https://idp.example/avatar.png", want: new("https://idp.example/avatar.png")},
		{name: "http", picture: "http://idp.example/avatar.png", want: new("http://idp.example/avatar.png")},
		{name: "ない", picture: nil, want: nil},
		{name: "javascript", picture: "javascript:alert(1)", want: nil},
		{name: "相対", picture: "/avatar.png", want: nil},
		{name: "文字列でない", picture: 1, want: nil},
		{name: "長すぎる", picture: "https://idp.example/" + strings.Repeat("a", 2048), want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newOIDCFixture(t, auth.OIDCAutoCreate{})
			f.linkedUser(t)
			// 前のログインの picture は、残さずに書き直す。
			f.idp.Claims = map[string]any{"picture": "https://idp.example/old.png"}
			_, err := f.login(t, nil)
			require.NoError(t, err)
			f.idp.Claims = map[string]any{}
			if tt.picture != nil {
				f.idp.Claims["picture"] = tt.picture
			}
			result, err := f.login(t, nil)
			require.NoError(t, err)
			var got *string
			require.NoError(t, f.db.QueryRow(t.Context(), "SELECT picture FROM user_oidc_identities WHERE user_id = $1", result.Session.UserID).Scan(&got))
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestOIDCRejectsTamperedLogin は、state、nonce、audience、PKCE が合わないログインや、使用済みの状態を拒むことを確かめる。
func TestOIDCRejectsTamperedLogin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(t *testing.T, f *oidcFixture)
		change  func(t *testing.T, f *oidcFixture, token *string, callback *auth.OIDCCallback)
		wantErr error
	}{
		{name: "state mismatch", wantErr: auth.ErrOIDCStateMismatch, change: func(_ *testing.T, _ *oidcFixture, _ *string, c *auth.OIDCCallback) { c.State = "forged" }},
		{name: "unknown login", wantErr: auth.ErrOIDCLoginExpired, change: func(_ *testing.T, _ *oidcFixture, token *string, _ *auth.OIDCCallback) { *token = "unknown" }},
		{name: "provider denied", wantErr: auth.ErrOIDCAccessDenied, change: func(_ *testing.T, _ *oidcFixture, _ *string, c *auth.OIDCCallback) { c.Error = "access_denied" }},
		{name: "unknown code", wantErr: auth.ErrOIDCTokenInvalid, change: func(_ *testing.T, _ *oidcFixture, _ *string, c *auth.OIDCCallback) { c.Code = "forged" }},
		{
			name: "nonce mismatch", wantErr: auth.ErrOIDCTokenInvalid,
			prepare: func(_ *testing.T, f *oidcFixture) { f.idp.WrongNonce = true },
		},
		{
			name: "token for another client", wantErr: auth.ErrOIDCTokenInvalid,
			prepare: func(_ *testing.T, f *oidcFixture) { f.idp.WrongAudience = true },
		},
		{
			name: "code verifier mismatch", wantErr: auth.ErrOIDCTokenInvalid,
			// IdP から戻る前に、保存した code_verifier を書き換える。PKCE がなければ、認可コードを盗んだ人が引き換えられる。
			change: func(t *testing.T, f *oidcFixture, _ *string, _ *auth.OIDCCallback) {
				t.Helper()
				_, err := f.db.Exec(t.Context(), "UPDATE oidc_logins SET code_verifier = 'forged-verifier-forged-verifier-forged-verifier'")
				require.NoError(t, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newOIDCFixture(t, auth.OIDCAutoCreate{})
			f.linkedUser(t)
			if tt.prepare != nil {
				tt.prepare(t, f)
			}
			_, err := f.login(t, tt.change)
			require.ErrorIs(t, err, tt.wantErr)
			assert.Zero(t, f.count(t, "SELECT count(*) FROM sessions"))
		})
	}
}

// TestOIDCAuthorizationRequest は、認可のリクエストに、PKCE (S256)、state、nonce、スコープを付けることを確かめる。
func TestOIDCAuthorizationRequest(t *testing.T) {
	t.Parallel()

	f := newOIDCFixture(t, auth.OIDCAutoCreate{})
	f.linkedUser(t)
	_, err := f.login(t, nil)
	require.NoError(t, err)
	got := f.idp.LastAuthorization
	assert.Equal(t, "S256", got.Get("code_challenge_method"))
	assert.NotEmpty(t, got.Get("code_challenge"))
	assert.NotEmpty(t, got.Get("state"))
	assert.NotEmpty(t, got.Get("nonce"))
	assert.Equal(t, "openid profile groups", got.Get("scope"))
	assert.Equal(t, testOrigin+auth.OIDCCallbackPath, got.Get("redirect_uri"))
}

// TestOIDCLoginExpires は、IdP から戻るのが遅すぎたログインを拒むことを確かめる。
func TestOIDCLoginExpires(t *testing.T) {
	t.Parallel()

	f := newOIDCFixture(t, auth.OIDCAutoCreate{})
	f.linkedUser(t)
	_, err := f.login(t, func(_ *testing.T, f *oidcFixture, _ *string, _ *auth.OIDCCallback) {
		f.clock.Advance(auth.OIDCLoginTTL)
	})
	require.ErrorIs(t, err, auth.ErrOIDCLoginExpired)
}

// TestOIDCLoginPurgesExpiredLogins は、IdP から戻らなかったログインの状態を、次にログインを始めるときに消すことを確かめる。
func TestOIDCLoginPurgesExpiredLogins(t *testing.T) {
	t.Parallel()

	f := newOIDCFixture(t, auth.OIDCAutoCreate{})
	_, err := f.oidc.Begin(t.Context(), f.db, f.client)
	require.NoError(t, err)
	f.clock.Advance(auth.OIDCLoginTTL)
	_, err = f.oidc.Begin(t.Context(), f.db, f.client)
	require.NoError(t, err)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM oidc_logins"))
}

// TestOIDCLink は、紐付けの重なりを拒むことを確かめる。
func TestOIDCLink(t *testing.T) {
	t.Parallel()

	f := newOIDCFixture(t, auth.OIDCAutoCreate{})
	f.linkedUser(t)
	_, err := f.users.Create(t.Context(), f.db, &auth.NewUser{Username: otherName, DisplayName: otherName, Role: auth.RoleMember, PasswordHash: nil})
	require.NoError(t, err)

	tests := []struct {
		name     string
		username string
		subject  string
		wantErr  error
	}{
		{name: "subject linked to another user", username: otherName, subject: idpSubject, wantErr: auth.ErrOIDCIdentityLinked},
		{name: "unknown user", username: "nobody", subject: "idp-user-2", wantErr: auth.ErrUserNotFound},
		{name: "empty subject", username: otherName, subject: "", wantErr: auth.ErrInvalidUser},
	}
	for _, tt := range tests {
		require.ErrorIs(t, f.oidc.Link(t.Context(), f.db, audit.System("cli"), tt.username, tt.subject), tt.wantErr, tt.name)
	}
}

// TestOIDCUnavailable は、設定がないときや、IdP に届かないときのエラーを確かめる。
func TestOIDCUnavailable(t *testing.T) {
	t.Parallel()

	f := newOIDCFixture(t, auth.OIDCAutoCreate{})
	disabled := auth.NewOIDC(&auth.OIDCOptions{Clock: f.clock, Recorder: audit.NewRecorder(f.clock), Logger: slog.New(slog.DiscardHandler)})
	_, err := disabled.Begin(t.Context(), f.db, f.client)
	require.ErrorIs(t, err, auth.ErrOIDCDisabled)

	f.idp.Close()
	_, err = f.oidc.Begin(t.Context(), f.db, f.client)
	require.ErrorIs(t, err, auth.ErrOIDCUnavailable)
}

// TestOIDCHandlers は、ログインの開始から、IdP を経て、セッションの Cookie を受け取るまでを HTTP で確かめる。
func TestOIDCHandlers(t *testing.T) {
	t.Parallel()

	f := newOIDCFixture(t, auth.OIDCAutoCreate{})
	f.linkedUser(t)
	// Cookie の期限は時計から決まる。過去の時刻のままだと、Cookie の入れ物が期限切れとして捨てる。
	f.clock.Advance(time.Since(f.clock.Now()))
	client := func(*http.Request) *auth.ClientInfo { return f.client }
	mux := http.NewServeMux()
	mux.Handle("GET /auth/oidc", f.oidc.StatusHandler(f.db))
	mux.Handle("POST /auth/oidc", f.oidc.BeginHandler(f.db, client))
	mux.Handle("GET "+auth.OIDCCallbackPath, f.oidc.CallbackHandler(f.db, client))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	// リダイレクトは、テストで 1 つずつたどる。
	httpClient := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	type response struct {
		status   int
		location string
		body     []byte
	}
	do := func(method, target string) *response {
		req, err := http.NewRequestWithContext(t.Context(), method, target, strings.NewReader("{}"))
		require.NoError(t, err)
		req.Header.Set("Content-Type", jsonType)
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		defer func() {
			_ = resp.Body.Close()
		}()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		return &response{status: resp.StatusCode, location: resp.Header.Get("Location"), body: body}
	}
	cookie := func(name string) string {
		for _, c := range jar.Cookies(mustParseURL(t, server.URL+"/auth/")) {
			if c.Name == name {
				return c.Value
			}
		}

		return ""
	}
	// IdP は、画面の代わりに、テストのサーバーのコールバックへ戻す。
	callback := func(redirect string) string {
		u := mustParseURL(t, redirect)

		return server.URL + u.Path + "?" + u.RawQuery
	}

	var status struct {
		Enabled     bool   `json:"enabled"`
		DisplayName string `json:"displayName"`
	}
	require.NoError(t, json.Unmarshal(do(http.MethodGet, server.URL+"/auth/oidc").body, &status))
	assert.True(t, status.Enabled)
	assert.Equal(t, "Test IdP", status.DisplayName)

	// state を書き換えたリダイレクトは、ログインさせずに、ログインの画面へ戻す。
	var begin struct {
		AuthorizationURL string `json:"authorizationUrl"`
	}
	require.NoError(t, json.Unmarshal(do(http.MethodPost, server.URL+"/auth/oidc").body, &begin))
	redirect, err := f.idp.Authorize(begin.AuthorizationURL)
	require.NoError(t, err)
	forged := mustParseURL(t, callback(redirect))
	query := forged.Query()
	query.Set("state", "forged")
	forged.RawQuery = query.Encode()
	resp := do(http.MethodGet, forged.String())
	assert.Equal(t, http.StatusSeeOther, resp.status)
	assert.Equal(t, "/login?error=oidc_invalid", resp.location)
	assert.Empty(t, cookie(auth.CookieName))
	assert.Empty(t, cookie(auth.OIDCCookieName), "the login state cookie must be cleared")

	// 正しく戻ってくれば、セッションの Cookie を受け取り、ホームへ進む。
	require.NoError(t, json.Unmarshal(do(http.MethodPost, server.URL+"/auth/oidc").body, &begin))
	assert.NotEmpty(t, cookie(auth.OIDCCookieName))
	redirect, err = f.idp.Authorize(begin.AuthorizationURL)
	require.NoError(t, err)
	resp = do(http.MethodGet, callback(redirect))
	assert.Equal(t, http.StatusSeeOther, resp.status)
	assert.Equal(t, "/", resp.location)
	assert.NotEmpty(t, cookie(auth.CookieName), "the session cookie must be set")
	assert.Empty(t, cookie(auth.OIDCCookieName))

	// 同じリダイレクトを繰り返しても、ログインさせない。
	resp = do(http.MethodGet, callback(redirect))
	assert.Equal(t, "/login?error=oidc_expired", resp.location)
}

// TestOIDCHandlersUnavailable は、OIDC を設定していないときや、IdP に届かないときの応答を確かめる。
func TestOIDCHandlersUnavailable(t *testing.T) {
	t.Parallel()

	f := newOIDCFixture(t, auth.OIDCAutoCreate{})
	client := func(*http.Request) *auth.ClientInfo { return f.client }
	disabled := auth.NewOIDC(&auth.OIDCOptions{Clock: f.clock, Recorder: audit.NewRecorder(f.clock), Logger: slog.New(slog.DiscardHandler)})
	f.idp.Close()

	tests := []struct {
		name       string
		handler    http.Handler
		method     string
		wantStatus int
		want       string
	}{
		{name: "status when disabled", handler: disabled.StatusHandler(f.db), method: http.MethodGet, wantStatus: http.StatusOK, want: `{"enabled":false,"displayName":"","linked":false}`},
		{name: "begin when disabled", handler: disabled.BeginHandler(f.db, client), method: http.MethodPost, wantStatus: http.StatusNotFound, want: `"code":"OIDC_DISABLED"`},
		{name: "begin when the provider is down", handler: f.oidc.BeginHandler(f.db, client), method: http.MethodPost, wantStatus: http.StatusServiceUnavailable, want: `"code":"OIDC_UNAVAILABLE"`},
	}
	for _, tt := range tests {
		req := httptest.NewRequestWithContext(t.Context(), tt.method, "/auth/oidc", strings.NewReader("{}"))
		req.Header.Set("Content-Type", jsonType)
		rec := httptest.NewRecorder()
		tt.handler.ServeHTTP(rec, req)
		assert.Equal(t, tt.wantStatus, rec.Code, tt.name)
		assert.Contains(t, rec.Body.String(), tt.want, tt.name)
	}
}
