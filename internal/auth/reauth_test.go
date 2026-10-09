package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
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
