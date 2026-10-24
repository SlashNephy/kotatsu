package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// TestRequireRecentAuthentication は、重要な操作の前に、直近 10 分以内の認証を求めることを確かめる。
func TestRequireRecentAuthentication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		elapsed time.Duration
		wantErr error
	}{
		{name: "just logged in", elapsed: 0, wantErr: nil},
		{name: "within ten minutes", elapsed: 10 * time.Minute, wantErr: nil},
		{name: "over ten minutes", elapsed: 10*time.Minute + time.Second, wantErr: auth.ErrReauthenticationRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newSessionFixture(t, newDB(t))
			token, _ := f.login(t)
			f.clock.Advance(tt.elapsed)
			// セッションを使っても、認証した日時は変わらない。
			principal, err := f.sessions.Authenticate(t.Context(), f.db, token)
			require.NoError(t, err)

			err = auth.RequireRecentAuthentication(auth.WithPrincipal(t.Context(), principal), f.clock)
			if tt.wantErr == nil {
				require.NoError(t, err)

				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}

	require.ErrorIs(t, auth.RequireRecentAuthentication(t.Context(), newSessionFixture(t, newDB(t)).clock), auth.ErrUnauthenticated)
}

// TestLogoutHandler は、ログアウトでセッションを失効させ、Cookie を消すことを確かめる。
func TestLogoutHandler(t *testing.T) {
	t.Parallel()

	f := newSessionFixture(t, newDB(t))
	token, _ := f.login(t)
	handler := f.sessions.Middleware(f.db)(f.sessions.LogoutHandler(f.db))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Cookie", auth.CookieName+"="+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, auth.CookieName, cookies[0].Name)
	assert.Equal(t, -1, cookies[0].MaxAge)
	_, err := f.sessions.Authenticate(t.Context(), f.db, token)
	require.ErrorIs(t, err, auth.ErrSessionInvalid)
}

// TestReauthenticationStatusHandler は、ログイン中のセッションの認証の日時と、重要な操作ができるかを返すことを確かめる。
// 入力を始める前に、IdP での再認証を求めるかを決めるのに使う (#916)。
func TestReauthenticationStatusHandler(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		principal *auth.Principal
		wantCode  int
		wantBody  string
	}{
		{
			name:      "10 分以内に認証した",
			principal: &auth.Principal{AuthenticatedAt: now.Add(-10 * time.Minute)},
			wantCode:  http.StatusOK,
			wantBody:  `{"authenticatedAt":"2026-10-23T11:50:00Z","recent":true}`,
		},
		{
			name:      "10 分より前に認証した",
			principal: &auth.Principal{AuthenticatedAt: now.Add(-10*time.Minute - time.Second)},
			wantCode:  http.StatusOK,
			wantBody:  `{"authenticatedAt":"2026-10-23T11:49:59Z","recent":false}`,
		},
		{
			name:      "ログインしていない",
			principal: nil,
			wantCode:  http.StatusUnauthorized,
			wantBody:  `{"error":{"code":"UNAUTHENTICATED","message":"log in first"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			if tt.principal != nil {
				ctx = auth.WithPrincipal(ctx, tt.principal)
			}
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/reauth", nil)
			rec := httptest.NewRecorder()
			auth.ReauthenticationStatusHandler(clocktest.NewFake(now)).ServeHTTP(rec, req)

			assert.Equal(t, tt.wantCode, rec.Code)
			assert.JSONEq(t, tt.wantBody, rec.Body.String())
		})
	}
}
