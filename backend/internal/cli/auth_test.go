package cli_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// authSetupPath は、初回セットアップが要るかを返し、最初の管理者を作る端点。
const authSetupPath = "/internal/auth/setup"

// TestTenbanServesAuth は、tenban が、ログインやログアウトなどの手続き (/internal/auth/*) を echo のルーターで受け (#1606)、
// mux のときと同じく、セッションの Cookie を読み、状態を変える POST で Origin を確認し、メソッドの違う要求に 405 を返すことを確認する。
func TestTenbanServesAuth(t *testing.T) {
	t.Parallel()

	base, pool := startTenban(t)
	clk := new(clock.Real)
	user, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{
		Username: adminUsername, DisplayName: "Auth", Role: domain.RoleViewer, PasswordHash: nil,
	})
	require.NoError(t, err)
	session, _, err := auth.NewSessions(nil).Create(t.Context(), pool, user.ID, nil)
	require.NoError(t, err)
	loggedOut, _, err := auth.NewSessions(nil).Create(t.Context(), pool, user.ID, nil)
	require.NoError(t, err)

	tests := []struct {
		name       string
		method     string
		path       string
		session    string
		origin     string
		wantStatus int
		wantAllow  string
	}{
		{name: "setup status", method: http.MethodGet, path: authSetupPath, session: "", origin: "", wantStatus: http.StatusOK, wantAllow: ""},
		// http.ServeMux の GET のパターンは HEAD にも答えたので、echo に移しても答える。
		{name: "setup status HEAD", method: http.MethodHead, path: authSetupPath, session: "", origin: "", wantStatus: http.StatusOK, wantAllow: ""},
		// セッションの Cookie を読むので、ログインしていなければ 401、していれば 200 になる。
		{name: "reauth status without session", method: http.MethodGet, path: "/internal/auth/reauth", session: "", origin: "", wantStatus: http.StatusUnauthorized, wantAllow: ""},
		{name: "reauth status with session", method: http.MethodGet, path: "/internal/auth/reauth", session: session, origin: "", wantStatus: http.StatusOK, wantAllow: ""},
		// 状態を変える POST は、同じオリジンからだけ受け付ける。
		{name: "logout without origin", method: http.MethodPost, path: "/internal/auth/logout", session: loggedOut, origin: "", wantStatus: http.StatusForbidden, wantAllow: ""},
		{name: "login from another origin", method: http.MethodPost, path: "/internal/auth/login", session: "", origin: "https://evil.example", wantStatus: http.StatusForbidden, wantAllow: ""},
		// メソッドが違えば、mux のときと同じく 405 にし、受け付けるメソッドを Allow で示す。
		{name: "GET to login", method: http.MethodGet, path: "/internal/auth/login", session: "", origin: "", wantStatus: http.StatusMethodNotAllowed, wantAllow: "POST"},
		{name: "DELETE to setup", method: http.MethodDelete, path: authSetupPath, session: "", origin: "", wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET, HEAD, POST"},
		// Annict と AniList から戻ってくる認可は、記録先ごとの /internal/auth/<記録先>/callback で受ける (#1906)。
		// 知らない state なので連携せず、設定の画面へ戻す。
		{name: "Annict callback", method: http.MethodGet, path: "/internal/auth/annict/callback?state=forged&code=abc", session: session, origin: "", wantStatus: http.StatusSeeOther, wantAllow: ""},
		{name: "AniList callback", method: http.MethodGet, path: "/internal/auth/anilist/callback?state=forged&code=abc", session: session, origin: "", wantStatus: http.StatusSeeOther, wantAllow: ""},
		// リリースしていない v2026.11.5 の途中の形 (/internal/auth/anime-records/<記録先>/callback) は、受け付けない。
		{name: "callback under anime-records", method: http.MethodGet, path: "/internal/auth/anime-records/annict/callback?state=forged&code=abc", session: session, origin: "", wantStatus: http.StatusNotFound, wantAllow: ""},
		// 知らないパスは、mux に渡り、画面を返さずに 404 になる。
		{name: "unknown path", method: http.MethodGet, path: "/internal/auth/nope", session: session, origin: "", wantStatus: http.StatusNotFound, wantAllow: ""},
	}
	client := httpclienttest.New(t)
	// 記録先から戻ってきた認可の 303 を、たどらずに確かめる。
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, allow, body := doAuthRequest(t, client, tt.method, base+tt.path, tt.session, tt.origin)
			assert.Equal(t, tt.wantStatus, status, body)
			assert.Equal(t, tt.wantAllow, allow)
		})
	}

	// 同じオリジンからのログアウトは、セッションを失効させる。失効したセッションでは、ログインしていないことになる。
	t.Run("logout", func(t *testing.T) {
		t.Parallel()

		status, _, body := doAuthRequest(t, client, http.MethodPost, base+"/internal/auth/logout", loggedOut, base)
		assert.Equal(t, http.StatusNoContent, status, body)
		status, _, body = doAuthRequest(t, client, http.MethodGet, base+"/internal/auth/reauth", loggedOut, "")
		assert.Equal(t, http.StatusUnauthorized, status, body)
	})
}

// doAuthRequest は、セッションの Cookie と Origin を付けて送り、応答の状態コードと Allow と本文を返す。
func doAuthRequest(t *testing.T, client *http.Client, method, url, session, origin string) (int, string, string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, url, http.NoBody)
	require.NoError(t, err)
	if session != "" {
		req.Header.Set("Cookie", auth.CookieName+"="+session)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, resp.Header.Get("Allow"), string(b)
}
