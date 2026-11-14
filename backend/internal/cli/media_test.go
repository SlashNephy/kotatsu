package cli_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// TestTenbanServesMedia は、tenban が、画像と実況の過去ログ (/internal/media/*) を echo のルーターで受け (#1606)、
// mux のときと同じく、セッションの Cookie で認可し、状態を変える POST と WebSocket の接続で Origin を確かめることを確かめる。
func TestTenbanServesMedia(t *testing.T) {
	t.Parallel()

	base, pool := startTenban(t)
	clk := new(clock.Real)
	user, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{
		Username: "media", DisplayName: "Media", Role: domain.RoleViewer, PasswordHash: nil,
	})
	require.NoError(t, err)
	session, _, err := auth.NewSessions(nil).Create(t.Context(), pool, user.ID, nil)
	require.NoError(t, err)
	const logo = "/internal/media/services/3273601024/logo"
	const missing = "/internal/media/recordings/0199d000-0000-7000-8000-000000000999/"

	tests := []struct {
		name       string
		method     string
		path       string
		session    string
		wantStatus int
		wantAllow  string
	}{
		// セッションで認可するので、ログインしていなければ 401 になる。
		{name: "logo without session", method: http.MethodGet, path: logo, session: "", wantStatus: http.StatusUnauthorized, wantAllow: ""},
		// この tenban にはチューナーサーバーがないので、ロゴは取れない。:id は、包んだハンドラーが r.PathValue("id") で読む。
		{name: "logo", method: http.MethodGet, path: logo, session: session, wantStatus: http.StatusServiceUnavailable, wantAllow: ""},
		{name: "logo HEAD", method: http.MethodHead, path: logo, session: session, wantStatus: http.StatusServiceUnavailable, wantAllow: ""},
		{name: "logo with invalid ID", method: http.MethodGet, path: "/internal/media/services/abc/logo", session: session, wantStatus: http.StatusBadRequest, wantAllow: ""},
		{name: "thumbnail of missing recording", method: http.MethodGet, path: missing + "thumbnail.jpg", session: session, wantStatus: http.StatusNotFound, wantAllow: ""},
		{name: "jikkyo of missing recording", method: http.MethodGet, path: missing + "jikkyo.json", session: session, wantStatus: http.StatusNotFound, wantAllow: ""},
		{name: "capture without session", method: http.MethodGet, path: "/internal/media/captures/0199d000-0000-7000-8000-000000000999", session: "", wantStatus: http.StatusUnauthorized, wantAllow: ""},
		// 状態を変える POST と WebSocket の接続は、Origin がなければ受け付けない。
		{name: "capture upload without origin", method: http.MethodPost, path: "/internal/media/captures", session: session, wantStatus: http.StatusForbidden, wantAllow: ""},
		{name: "data broadcast without origin", method: http.MethodPost, path: "/internal/media/data-broadcast/requests", session: session, wantStatus: http.StatusForbidden, wantAllow: ""},
		{name: "live jikkyo without origin", method: http.MethodGet, path: "/internal/media/jikkyo/jk1/ws", session: session, wantStatus: http.StatusForbidden, wantAllow: ""},
		// メソッドが違えば、mux のときと同じく 405 にする。
		{name: "DELETE to captures", method: http.MethodDelete, path: "/internal/media/captures", session: session, wantStatus: http.StatusMethodNotAllowed, wantAllow: "POST"},
		{name: "POST to logo", method: http.MethodPost, path: logo, session: session, wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET, HEAD"},
		{name: "unknown path", method: http.MethodGet, path: "/internal/media/nope", session: session, wantStatus: http.StatusNotFound, wantAllow: ""},
	}
	client := httpclienttest.New(t)
	// 実況の過去ログのファイル名は、録画の画像の :file より先に当たる。どちらも、ない録画には 404 を返すので、
	// 画像のハンドラーだけが先に付ける Cache-Control で、どちらが答えたかを見分ける。
	t.Run("jikkyo file name wins over image file", func(t *testing.T) {
		t.Parallel()

		for file, want := range map[string]string{"thumbnail.jpg": "no-store", "jikkyo.json": ""} {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+missing+file, http.NoBody)
			require.NoError(t, err)
			req.Header.Set("Cookie", auth.CookieName+"="+session)
			resp, err := client.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusNotFound, resp.StatusCode, file)
			assert.Equal(t, want, resp.Header.Get("Cache-Control"), file)
		}
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, allow, body := doAuthRequest(t, client, tt.method, base+tt.path, tt.session, "")
			assert.Equal(t, tt.wantStatus, status, body)
			assert.Equal(t, tt.wantAllow, allow)
		})
	}
}
