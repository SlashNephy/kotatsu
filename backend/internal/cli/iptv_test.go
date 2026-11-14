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
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

// TestTenbanServesIPTV は、tenban が、echo のルーターで IPTV の M3U と XMLTV を返すことを確かめる (#1606)。
// http.ServeMux の GET のパターンは HEAD にも答えたので、HEAD にも答える。
func TestTenbanServesIPTV(t *testing.T) {
	t.Parallel()

	base, pool := startTenban(t)
	clk := new(clock.Real)
	user, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{
		Username: "iptv", DisplayName: "IPTV", Role: domain.RoleMember, PasswordHash: nil,
	})
	require.NoError(t, err)
	token, _, err := auth.NewIPTVFeeds(pool, clk, audit.NewRecorder(clk)).Create(t.Context(), audit.User(user.ID), user.ID)
	require.NoError(t, err)
	// IPTV のクライアント用の URL は、管理者が有効にしている間だけ受け付ける (#1755)。
	_, err = settings.NewStore(clk, audit.NewRecorder(clk)).Update(t.Context(), pool, audit.System("test"), &settings.Changes{IPTVFeedsEnabled: new(true)})
	require.NoError(t, err)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantType   string
		wantBody   string
	}{
		{name: "playlist", method: http.MethodGet, path: "/api/iptv/" + token + "/channels.m3u", wantStatus: http.StatusOK, wantType: "audio/x-mpegurl; charset=utf-8", wantBody: "#EXTM3U"},
		{name: "playlist HEAD", method: http.MethodHead, path: "/api/iptv/" + token + "/channels.m3u", wantStatus: http.StatusOK, wantType: "audio/x-mpegurl; charset=utf-8", wantBody: ""},
		{name: "guide", method: http.MethodGet, path: "/api/iptv/" + token + "/guide.xml", wantStatus: http.StatusOK, wantType: "application/xml; charset=utf-8", wantBody: "<tv"},
		// 秘密の値が違えば、見つからないことと区別させない。
		{name: "wrong token", method: http.MethodGet, path: "/api/iptv/kotatsu_iptv_wrong/channels.m3u", wantStatus: http.StatusNotFound, wantType: textPlain, wantBody: ""},
		{name: "wrong token stream", method: http.MethodGet, path: "/api/iptv/kotatsu_iptv_wrong/services/3273601024/stream.m2ts", wantStatus: http.StatusNotFound, wantType: textPlain, wantBody: ""},
		// 配信とロゴは、包んだ http.Handler が r.PathValue("id") でサービスを読む。読めなければ 400 (invalid service ID) になる。
		// この tenban にはチューナーサーバーがないので、どちらも 503 になる。ID が数でなければ 400 になる。
		{name: "stream", method: http.MethodGet, path: "/api/iptv/" + token + "/services/3273601024/stream.m2ts", wantStatus: http.StatusServiceUnavailable, wantType: textPlain, wantBody: ""},
		{name: "logo", method: http.MethodGet, path: "/api/iptv/" + token + "/services/3273601024/logo", wantStatus: http.StatusServiceUnavailable, wantType: textPlain, wantBody: ""},
		{name: "invalid service ID", method: http.MethodGet, path: "/api/iptv/" + token + "/services/abc/stream.m2ts", wantStatus: http.StatusBadRequest, wantType: textPlain, wantBody: "invalid service ID"},
		{name: "unknown file", method: http.MethodGet, path: "/api/iptv/" + token + "/unknown", wantStatus: http.StatusNotFound, wantType: textPlain, wantBody: ""},
	}
	client := httpclienttest.New(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), tt.method, base+tt.path, nil)
			require.NoError(t, err)
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer func() {
				_ = resp.Body.Close()
			}()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, resp.StatusCode, string(body))
			assert.Equal(t, tt.wantType, resp.Header.Get("Content-Type"))
			assert.Contains(t, string(body), tt.wantBody)
			if tt.wantStatus == http.StatusOK {
				assert.Equal(t, "private, no-store", resp.Header.Get("Cache-Control"))
			}
		})
	}
}
