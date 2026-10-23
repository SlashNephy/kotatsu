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

	"github.com/SlashNephy/kotatsu/internal/auth"
)

const (
	settingsPath = "/settings/security"
	authTime     = "auth_time"
)

// oidcReauthFixture は、IdP のユーザーと紐付けたユーザーが、ログインしてから時間が経ったセッションを持つ oidcFixture。
type oidcReauthFixture struct {
	*oidcFixture

	user      *auth.User
	token     string
	principal *auth.Principal
}

func newOIDCReauthFixture(t *testing.T) *oidcReauthFixture {
	t.Helper()

	f := newOIDCFixture(t, auth.OIDCAutoCreate{})
	user := f.linkedUser(t)
	token, session, err := auth.NewSessions(&auth.SessionsOptions{Clock: f.clock}).Create(t.Context(), f.db, user.ID, f.client)
	require.NoError(t, err)
	// 重要な操作には再認証が要るよう、ログインから時間を進める。
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Minute)

	return &oidcReauthFixture{
		oidcFixture: f,
		user:        user,
		token:       token,
		principal:   &auth.Principal{UserID: user.ID, Role: user.Role, SessionID: session.ID, AuthenticatedAt: session.CreatedAt},
	}
}

// reauthenticate は、再認証を始め、IdP で認証し直し、principal のリクエストとして戻ってきたものを完了させる。
func (f *oidcReauthFixture) reauthenticate(t *testing.T, principal *auth.Principal) (*auth.OIDCReauthentication, string, error) {
	t.Helper()

	login, err := f.oidc.BeginReauthentication(t.Context(), f.db, f.principal, settingsPath, f.client)
	require.NoError(t, err)
	redirect, err := f.idp.Authorize(login.AuthorizationURL)
	require.NoError(t, err)
	u, err := url.Parse(redirect)
	require.NoError(t, err)

	return f.oidc.CompleteAs(t.Context(), f.db, login.Token, &auth.OIDCCallback{
		State: u.Query().Get("state"), Code: u.Query().Get("code"), Error: "",
	}, f.client, principal)
}

func (f *oidcReauthFixture) authenticatedAt(t *testing.T) time.Time {
	t.Helper()

	var at time.Time
	require.NoError(t, f.db.QueryRow(t.Context(), "SELECT authenticated_at FROM sessions WHERE id = $1", f.principal.SessionID).Scan(&at))

	return at.UTC()
}

// TestOIDCReauthentication は、OIDC で紐付けたユーザーが、ログイン中のセッションのまま、IdP でもう一度認証して、
// セッションの認証の日時を更新できることを確かめる (設計書 9.5)。IdP が認証し直したと言えないときは、再認証として扱わない。
func TestOIDCReauthentication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// prepare は、IdP で認証し直す前に、IdP の応答を変える。
		prepare func(f *oidcReauthFixture)
		// principal は、戻ってきたリクエストのログイン中のユーザーを返す。nil なら、再認証を始めたセッションにする。
		principal func(t *testing.T, f *oidcReauthFixture) *auth.Principal
		wantErr   error
	}{
		{
			name:    "IdP で認証し直した",
			prepare: func(f *oidcReauthFixture) { f.idp.Claims = map[string]any{authTime: f.clock.Now().Unix()} },
		},
		{
			name: "IdP と時計が少しずれている",
			prepare: func(f *oidcReauthFixture) {
				f.idp.Claims = map[string]any{authTime: f.clock.Now().Add(-30 * time.Second).Unix()}
			},
		},
		{
			name:    "IdP が auth_time を返さない",
			prepare: func(f *oidcReauthFixture) { f.idp.Claims = map[string]any{} },
			wantErr: auth.ErrOIDCStaleAuthentication,
		},
		{
			name: "IdP が、前のログインの auth_time を返す",
			prepare: func(f *oidcReauthFixture) {
				f.idp.Claims = map[string]any{authTime: f.clock.Now().Add(-5 * time.Minute).Unix()}
			},
			wantErr: auth.ErrOIDCStaleAuthentication,
		},
		{
			name: "IdP で、ほかのユーザーとして認証した",
			prepare: func(f *oidcReauthFixture) {
				f.idp.Subject = "another-subject"
				f.idp.Claims = map[string]any{authTime: f.clock.Now().Unix()}
			},
			wantErr: auth.ErrOIDCReauthenticationMismatch,
		},
		{
			name:    "ログインしていないリクエストで戻ってきた",
			prepare: func(f *oidcReauthFixture) { f.idp.Claims = map[string]any{authTime: f.clock.Now().Unix()} },
			principal: func(*testing.T, *oidcReauthFixture) *auth.Principal {
				return nil
			},
			wantErr: auth.ErrOIDCReauthenticationMismatch,
		},
		{
			name:    "同じユーザーの別のセッションで戻ってきた",
			prepare: func(f *oidcReauthFixture) { f.idp.Claims = map[string]any{authTime: f.clock.Now().Unix()} },
			principal: func(t *testing.T, f *oidcReauthFixture) *auth.Principal {
				t.Helper()

				_, other, err := auth.NewSessions(&auth.SessionsOptions{Clock: f.clock}).Create(t.Context(), f.db, f.user.ID, f.client)
				require.NoError(t, err)

				return &auth.Principal{UserID: f.user.ID, Role: f.user.Role, SessionID: other.ID, AuthenticatedAt: other.CreatedAt}
			},
			wantErr: auth.ErrOIDCReauthenticationMismatch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newOIDCReauthFixture(t)
			before := f.authenticatedAt(t)
			tt.prepare(f)
			principal := f.principal
			if tt.principal != nil {
				principal = tt.principal(t, f)
			}

			result, returnTo, err := f.reauthenticate(t, principal)
			assert.Equal(t, settingsPath, returnTo)
			// IdP には、ログイン中でも、もう一度認証させる。
			assert.Equal(t, "login", f.idp.LastAuthorization.Get("prompt"))
			assert.Equal(t, "0", f.idp.LastAuthorization.Get("max_age"))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, before, f.authenticatedAt(t), "the session is not reauthenticated")
				assert.Zero(t, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.reauthenticated'"))

				return
			}
			require.NoError(t, err)
			assert.Equal(t, &auth.OIDCReauthentication{AuthenticatedAt: f.clock.Now(), ReturnTo: settingsPath}, result)
			assert.Equal(t, f.clock.Now().UTC(), f.authenticatedAt(t))
			assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM audit_events WHERE action = 'user.reauthenticated'
				AND summary->>'method' = 'oidc' AND summary->>'sessionId' = $1`, f.principal.SessionID.String()))
			// 再認証は、新しいセッションを作らない。
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM sessions"))
		})
	}
}

// TestOIDCReauthenticationIsNotALogin は、再認証として始めたものを、ログインとしては完了させず、
// ログインとして始めたものも、再認証としては扱わないことを確かめる。
func TestOIDCReauthenticationIsNotALogin(t *testing.T) {
	t.Parallel()

	f := newOIDCReauthFixture(t)
	f.idp.Claims = map[string]any{authTime: f.clock.Now().Unix()}
	login, err := f.oidc.BeginReauthentication(t.Context(), f.db, f.principal, "", f.client)
	require.NoError(t, err)
	redirect, err := f.idp.Authorize(login.AuthorizationURL)
	require.NoError(t, err)
	u, err := url.Parse(redirect)
	require.NoError(t, err)
	_, err = f.oidc.Complete(t.Context(), f.db, login.Token, &auth.OIDCCallback{State: u.Query().Get("state"), Code: u.Query().Get("code"), Error: ""}, f.client)
	require.ErrorIs(t, err, auth.ErrOIDCReauthenticationMismatch)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM sessions"), "no session is created")

	// ログインとして始めたものは、ログイン中のリクエストで戻ってきても、ログインとして完了させ、再認証にはしない。
	before := f.authenticatedAt(t)
	result, err := f.login(t, nil)
	require.NoError(t, err)
	assert.NotEqual(t, f.principal.SessionID, result.Session.ID)
	assert.Equal(t, before, f.authenticatedAt(t))
}

// TestBeginOIDCReauthenticationRejects は、IdP と紐付いていないユーザーと、ほかのサイトに戻させるパスを拒むことを確かめる。
func TestBeginOIDCReauthenticationRejects(t *testing.T) {
	t.Parallel()

	f := newOIDCReauthFixture(t)
	tests := []struct {
		name     string
		returnTo string
		wantErr  error
	}{
		{name: "ほかのオリジン", returnTo: "https://evil.example/", wantErr: auth.ErrInvalidReturnTo},
		{name: "スキームのない URL", returnTo: "//evil.example/", wantErr: auth.ErrInvalidReturnTo},
		{name: "バックスラッシュ", returnTo: "/\\evil.example/", wantErr: auth.ErrInvalidReturnTo},
		{name: "相対パス", returnTo: "settings", wantErr: auth.ErrInvalidReturnTo},
		{name: "改行", returnTo: "/settings\r\nX-Injected: 1", wantErr: auth.ErrInvalidReturnTo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := f.oidc.BeginReauthentication(t.Context(), f.db, f.principal, tt.returnTo, f.client)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}

	t.Run("紐付いていないユーザー", func(t *testing.T) {
		t.Parallel()

		user, err := f.users.Create(t.Context(), f.db, &auth.NewUser{Username: "yuzu", DisplayName: "yuzu", Role: auth.RoleMember, PasswordHash: nil})
		require.NoError(t, err)
		_, session, err := auth.NewSessions(&auth.SessionsOptions{Clock: f.clock}).Create(t.Context(), f.db, user.ID, f.client)
		require.NoError(t, err)
		principal := &auth.Principal{UserID: user.ID, Role: user.Role, SessionID: session.ID, AuthenticatedAt: session.CreatedAt}
		_, err = f.oidc.BeginReauthentication(t.Context(), f.db, principal, "", f.client)
		require.ErrorIs(t, err, auth.ErrOIDCNotLinked)
	})
}

// TestOIDCReauthenticationHandlers は、画面からの再認証の始まりから、IdP から戻って、始めたときのパスに戻るまでを、HTTP で確かめる。
func TestOIDCReauthenticationHandlers(t *testing.T) {
	t.Parallel()

	f := newOIDCReauthFixture(t)
	// Cookie の期限は時計から決まる。過去の時刻のままだと、Cookie の入れ物が期限切れとして捨てる。
	f.clock.Advance(time.Since(f.clock.Now()))
	client := func(*http.Request) *auth.ClientInfo { return f.client }
	mux := http.NewServeMux()
	mux.Handle("POST /auth/reauth/oidc", f.oidc.ReauthenticationHandler(f.db, client))
	mux.Handle("GET "+auth.OIDCCallbackPath, f.oidc.CallbackHandler(f.db, client))
	server := httptest.NewServer(auth.NewSessions(&auth.SessionsOptions{Clock: f.clock}).Middleware(f.db)(mux))
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	httpClient := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	// sessionToken は、リクエストに付けるログイン中のセッションのトークン。空なら付けない。
	sessionToken := f.token
	do := func(method, target, body string) (int, string, []byte) {
		req, err := http.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", jsonType)
		if sessionToken != "" {
			req.Header.Set("Cookie", auth.CookieName+"="+sessionToken)
		}
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		defer func() {
			_ = resp.Body.Close()
		}()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		return resp.StatusCode, resp.Header.Get("Location"), data
	}
	// begin は、再認証を始めて、IdP で認証し直し、テストのサーバーのコールバックの URL を返す。
	begin := func() string {
		status, _, body := do(http.MethodPost, server.URL+"/auth/reauth/oidc", `{"returnTo": "/settings/security?tab=passkeys"}`)
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

	// IdP が前のログインの auth_time を返したら、始めたときのパスに、理由を付けて戻す。
	f.idp.Claims = map[string]any{authTime: f.clock.Now().Add(-time.Hour).Unix()}
	status, location, _ := do(http.MethodGet, begin(), "")
	assert.Equal(t, http.StatusSeeOther, status)
	assert.Equal(t, "/settings/security?error=oidc_reauth_stale&tab=passkeys", location)

	// IdP で認証し直したら、セッションの認証の日時を更新し、始めたときのパスに戻す。
	f.idp.Claims = map[string]any{authTime: f.clock.Now().Unix()}
	status, location, _ = do(http.MethodGet, begin(), "")
	assert.Equal(t, http.StatusSeeOther, status)
	assert.Equal(t, "/settings/security?tab=passkeys", location)
	assert.WithinDuration(t, f.clock.Now(), f.authenticatedAt(t), time.Second)

	// ログインしていなければ、始められない。
	sessionToken = ""
	status, _, _ = do(http.MethodPost, server.URL+"/auth/reauth/oidc", `{}`)
	assert.Equal(t, http.StatusUnauthorized, status)
}
