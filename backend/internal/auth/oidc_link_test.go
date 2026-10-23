package auth_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
)

const (
	securityPath = "/settings/security"
	unlinkedName = "yuzu"
)

// oidcLinkFixture は、IdP のユーザーと紐付いていないユーザーが、直前に認証したセッションを持つ oidcFixture。
type oidcLinkFixture struct {
	*oidcFixture

	user      *auth.User
	token     string
	principal *auth.Principal
}

func newOIDCLinkFixture(t *testing.T) *oidcLinkFixture {
	t.Helper()

	f := newOIDCFixture(t, auth.OIDCAutoCreate{})
	user, err := f.users.Create(t.Context(), f.db, &auth.NewUser{Username: unlinkedName, DisplayName: unlinkedName, Role: auth.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	token, session, err := auth.NewSessions(&auth.SessionsOptions{Clock: f.clock}).Create(t.Context(), f.db, user.ID, f.client)
	require.NoError(t, err)

	return &oidcLinkFixture{
		oidcFixture: f,
		user:        user,
		token:       token,
		principal:   &auth.Principal{UserID: user.ID, Role: user.Role, SessionID: session.ID, AuthenticatedAt: session.CreatedAt},
	}
}

// begin は、紐付けを始め、IdP でログインして戻ってきたパラメーターを返す。
func (f *oidcLinkFixture) begin(t *testing.T) (string, *auth.OIDCCallback) {
	t.Helper()

	login, err := f.oidc.BeginLink(t.Context(), f.db, f.principal, securityPath, f.client)
	require.NoError(t, err)
	redirect, err := f.idp.Authorize(login.AuthorizationURL)
	require.NoError(t, err)
	u, err := url.Parse(redirect)
	require.NoError(t, err)

	return login.Token, &auth.OIDCCallback{State: u.Query().Get("state"), Code: u.Query().Get("code"), Error: ""}
}

// linkedSubjects は、ユーザーに紐付いた IdP のユーザーの subject を返す。
func (f *oidcLinkFixture) linkedSubjects(t *testing.T) []string {
	t.Helper()

	rows, err := f.db.Query(t.Context(), "SELECT subject FROM user_oidc_identities WHERE user_id = $1 ORDER BY subject", f.user.ID)
	require.NoError(t, err)
	subjects := []string{}
	for rows.Next() {
		var subject string
		require.NoError(t, rows.Scan(&subject))
		subjects = append(subjects, subject)
	}
	require.NoError(t, rows.Err())

	return subjects
}

// TestOIDCLinkFromSession は、ログイン中のユーザーが、IdP でログインして、自分を IdP のユーザーと紐付けられることを確かめる (設計書 9.4)。
// 紐付けを始めたセッションのまま戻ってこなければ、紐付けない。
func TestOIDCLinkFromSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// prepare は、紐付けを始めた後、戻ってくる前に、状態を変える。
		prepare func(t *testing.T, f *oidcLinkFixture)
		// principal は、戻ってきたリクエストのログイン中のユーザーを返す。nil なら、紐付けを始めたセッションにする。
		principal    func(t *testing.T, f *oidcLinkFixture) *auth.Principal
		wantErr      error
		wantSubjects []string
	}{
		{
			name:         "紐付ける",
			wantSubjects: []string{idpSubject},
		},
		{
			name: "IdP のユーザーが、ほかのユーザーと紐付いている",
			prepare: func(t *testing.T, f *oidcLinkFixture) {
				t.Helper()

				f.linkedUser(t)
			},
			wantErr:      auth.ErrOIDCIdentityLinked,
			wantSubjects: []string{},
		},
		{
			name: "戻ってくるまでに、ほかの経路で紐付けた",
			prepare: func(t *testing.T, f *oidcLinkFixture) {
				t.Helper()

				require.NoError(t, f.oidc.Link(t.Context(), f.db, audit.System("cli"), unlinkedName, "another-subject"))
			},
			wantErr:      auth.ErrOIDCAlreadyLinked,
			wantSubjects: []string{"another-subject"},
		},
		{
			name: "ログインしていないリクエストで戻ってきた",
			principal: func(*testing.T, *oidcLinkFixture) *auth.Principal {
				return nil
			},
			wantErr:      auth.ErrOIDCLinkMismatch,
			wantSubjects: []string{},
		},
		{
			name: "同じユーザーの別のセッションで戻ってきた",
			principal: func(t *testing.T, f *oidcLinkFixture) *auth.Principal {
				t.Helper()

				_, other, err := auth.NewSessions(&auth.SessionsOptions{Clock: f.clock}).Create(t.Context(), f.db, f.user.ID, f.client)
				require.NoError(t, err)

				return &auth.Principal{UserID: f.user.ID, Role: f.user.Role, SessionID: other.ID, AuthenticatedAt: other.CreatedAt}
			},
			wantErr:      auth.ErrOIDCLinkMismatch,
			wantSubjects: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newOIDCLinkFixture(t)
			token, callback := f.begin(t)
			if tt.prepare != nil {
				tt.prepare(t, f)
			}
			principal := f.principal
			if tt.principal != nil {
				principal = tt.principal(t, f)
			}

			reauth, returnTo, err := f.oidc.CompleteAs(t.Context(), f.db, token, callback, f.client, principal)
			assert.Nil(t, reauth, "linking is not a reauthentication")
			assert.Equal(t, securityPath, returnTo)
			assert.Equal(t, tt.wantSubjects, f.linkedSubjects(t))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Zero(t, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.oidc_linked' AND actor_type = 'user'"))

				return
			}
			require.NoError(t, err)
			assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM user_oidc_identities WHERE user_id = $1 AND last_used_at = $2`, f.user.ID, f.clock.Now()))
			assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM audit_events WHERE action = 'user.oidc_linked'
				AND actor_user_id = $1 AND target_id = $2 AND summary->>'subject' = $3`, f.user.ID, f.user.ID.String(), idpSubject))
			// 紐付けは、新しいセッションを作らない。
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM sessions"))
		})
	}
}

// TestOIDCLinkIsNotALogin は、紐付けとして始めたものを、ログインとしては完了させないことを確かめる。
func TestOIDCLinkIsNotALogin(t *testing.T) {
	t.Parallel()

	f := newOIDCLinkFixture(t)
	token, callback := f.begin(t)
	_, err := f.oidc.Complete(t.Context(), f.db, token, callback, f.client)
	require.ErrorIs(t, err, auth.ErrOIDCLinkMismatch)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM sessions"), "no session is created")
	assert.Empty(t, f.linkedSubjects(t))
}

// TestBeginOIDCLinkRejects は、直前に認証していないセッション、既に紐付いたユーザー、ほかのサイトに戻させるパスを拒むことを確かめる。
func TestBeginOIDCLinkRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prepare  func(t *testing.T, f *oidcLinkFixture)
		returnTo string
		wantErr  error
	}{
		{
			name:    "直前に認証していない",
			prepare: func(_ *testing.T, f *oidcLinkFixture) { f.clock.Advance(auth.RecentAuthenticationWindow + time.Minute) },
			wantErr: auth.ErrReauthenticationRequired,
		},
		{
			name: "既に紐付いている",
			prepare: func(t *testing.T, f *oidcLinkFixture) {
				t.Helper()

				require.NoError(t, f.oidc.Link(t.Context(), f.db, audit.System("cli"), unlinkedName, idpSubject))
			},
			wantErr: auth.ErrOIDCAlreadyLinked,
		},
		{name: "ほかのオリジンに戻させる", returnTo: "https://evil.example/", wantErr: auth.ErrInvalidReturnTo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newOIDCLinkFixture(t)
			if tt.prepare != nil {
				tt.prepare(t, f)
			}
			_, err := f.oidc.BeginLink(t.Context(), f.db, f.principal, tt.returnTo, f.client)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestOIDCLinkHandlers は、画面からの紐付けの始まりから、IdP から戻って、始めたときのパスに戻るまでを、HTTP で確かめる。
func TestOIDCLinkHandlers(t *testing.T) {
	t.Parallel()

	f := newOIDCLinkFixture(t)
	// Cookie の期限は時計から決まる。過去の時刻のままだと、Cookie の入れ物が期限切れとして捨てる。
	f.clock.Advance(time.Since(f.clock.Now()))
	// 時計を進めたので、直前に認証したセッションを作り直す。
	token, _, err := auth.NewSessions(&auth.SessionsOptions{Clock: f.clock}).Create(t.Context(), f.db, f.user.ID, f.client)
	require.NoError(t, err)
	f.token = token
	client := func(*http.Request) *auth.ClientInfo { return f.client }
	mux := http.NewServeMux()
	mux.Handle("GET /auth/oidc", f.oidc.StatusHandler(f.db))
	mux.Handle("POST /auth/oidc/link", f.oidc.LinkHandler(f.db, client))
	mux.Handle("GET "+auth.OIDCCallbackPath, f.oidc.CallbackHandler(f.db, client))
	server := httptest.NewServer(auth.NewSessions(&auth.SessionsOptions{Clock: f.clock}).Middleware(f.db)(mux))
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	httpClient := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	do := func(method, target, body string) (int, string, []byte) {
		req, err := http.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", jsonType)
		req.Header.Set("Cookie", auth.CookieName+"="+f.token)
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		defer func() {
			_ = resp.Body.Close()
		}()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		return resp.StatusCode, resp.Header.Get("Location"), data
	}
	linked := func() bool {
		status, _, body := do(http.MethodGet, server.URL+"/auth/oidc", "")
		require.Equal(t, http.StatusOK, status, string(body))
		var got struct {
			Linked bool `json:"linked"`
		}
		require.NoError(t, json.Unmarshal(body, &got))

		return got.Linked
	}
	// begin は、紐付けを始めて、IdP でログインし、テストのサーバーのコールバックの URL を返す。
	begin := func() string {
		status, _, body := do(http.MethodPost, server.URL+"/auth/oidc/link", `{"returnTo": "/settings/security?tab=security"}`)
		require.Equal(t, http.StatusOK, status, string(body))
		var begun struct {
			AuthorizationURL string `json:"authorizationUrl"`
		}
		require.NoError(t, json.Unmarshal(body, &begun))
		redirect, err := f.idp.Authorize(begun.AuthorizationURL)
		require.NoError(t, err)
		u := mustParseURL(t, redirect)

		return server.URL + u.Path + "?" + u.RawQuery
	}

	assert.False(t, linked())

	// IdP のユーザーが、ほかのユーザーと紐付いていたら、始めたときのパスに、理由を付けて戻す。
	f.idp.Subject = "taken-subject"
	other, err := f.users.Create(t.Context(), f.db, &auth.NewUser{Username: "other", DisplayName: "other", Role: auth.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	require.NoError(t, f.oidc.Link(t.Context(), f.db, audit.System("cli"), other.Username, "taken-subject"))
	status, location, _ := do(http.MethodGet, begin(), "")
	assert.Equal(t, http.StatusSeeOther, status)
	assert.Equal(t, "/settings/security?error=oidc_link_taken&tab=security", location)
	assert.False(t, linked())

	// 紐付けたら、始めたときのパスに、linked=oidc を付けて戻す。
	f.idp.Subject = idpSubject
	status, location, _ = do(http.MethodGet, begin(), "")
	assert.Equal(t, http.StatusSeeOther, status)
	assert.Equal(t, "/settings/security?linked=oidc&tab=security", location)
	assert.True(t, linked())

	// 紐付いたら、もう始められない。
	status, _, body := do(http.MethodPost, server.URL+"/auth/oidc/link", `{}`)
	assert.Equal(t, http.StatusConflict, status)
	assert.Contains(t, string(body), `"code":"OIDC_ALREADY_LINKED"`)

	// 直前に認証していなければ、再認証を求める。
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Minute)
	_, err = f.db.Exec(t.Context(), "DELETE FROM user_oidc_identities WHERE user_id = $1", f.user.ID)
	require.NoError(t, err)
	status, _, body = do(http.MethodPost, server.URL+"/auth/oidc/link", `{}`)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Contains(t, string(body), `"code":"REAUTHENTICATION_REQUIRED"`)
}
