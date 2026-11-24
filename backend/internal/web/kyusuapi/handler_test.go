package kyusuapi_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
	"github.com/SlashNephy/kotatsu/internal/web/kyusuapi"
)

const (
	keyMaterial  = "kotatsu-kyusu-test-application-key-0123456789"
	notFoundBody = "404 page not found\n"
	allowGet     = "GET, HEAD"
	livePlaylist = "/internal/kyusu/live/abc/720p/1/playlist.m3u8"
	invalidID    = "invalid ID\n"
)

// TestHandlerRoutes は、http.ServeMux で受けていたときと同じく、トークンをルートより先に確認し、
// 1 つの区切りにだけ当たる名前の部分でルートを選ぶことを確認する (#2016)。
// ID とプリセットの読めないリクエストは、配信を始める前に断るので、ルートに当たったかを、その応答で見分ける。
func TestHandlerRoutes(t *testing.T) {
	t.Parallel()

	token, err := kyusu.NewToken(keyMaterial)
	require.NoError(t, err)
	clk := clocktest.NewFake(time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC))
	logger := slog.New(slog.DiscardHandler)
	live, err := kyusu.NewLive(&kyusu.LiveOptions{WorkDir: t.TempDir(), Clock: clk, Logger: logger})
	require.NoError(t, err)
	vod, err := kyusu.NewVOD(&kyusu.VODOptions{WorkDir: t.TempDir(), Clock: clk, Logger: logger})
	require.NoError(t, err)
	liveHandler := kyusuapi.LiveHandler(logger, live, token)
	vodHandler := kyusuapi.VODHandler(logger, vod, token)

	tests := []struct {
		name         string
		handler      http.Handler
		method       string
		path         string
		noToken      bool
		wantCode     int
		wantAllow    string
		wantLocation string
		wantBody     string
	}{
		{name: "トークンがなければ、当たらないパスでも 401", handler: liveHandler, method: http.MethodGet, path: "/internal/kyusu/live/nope", noToken: true, wantCode: http.StatusUnauthorized, wantBody: "Unauthorized\n"},
		{name: "ライブのルートに当たる", handler: liveHandler, method: http.MethodGet, path: livePlaylist, wantCode: http.StatusBadRequest, wantBody: "invalid service ID\n"},
		{name: "ライブのルートは HEAD にも答える", handler: liveHandler, method: http.MethodHead, path: livePlaylist, wantCode: http.StatusBadRequest},
		{name: "崩れたパスは整えた先へリダイレクトする", handler: liveHandler, method: http.MethodGet, path: "/internal/kyusu/live//720p/1/playlist.m3u8", wantCode: http.StatusTemporaryRedirect, wantLocation: "/internal/kyusu/live/720p/1/playlist.m3u8"},
		{name: "ライブのルートに POST すると 405", handler: liveHandler, method: http.MethodPost, path: livePlaylist, wantCode: http.StatusMethodNotAllowed, wantAllow: allowGet},
		{name: "エスケープした ID を戻して読む", handler: liveHandler, method: http.MethodGet, path: "/internal/kyusu/live/%31/nopreset/1/playlist.m3u8", wantCode: http.StatusNotFound, wantBody: "unknown preset\n"},
		{name: "ライブの区切りが多いと当たらない", handler: liveHandler, method: http.MethodGet, path: "/internal/kyusu/live/1/720p/1/a/b", wantCode: http.StatusNotFound, wantBody: notFoundBody},
		{name: "ライブのファイル名が空だと当たらない", handler: liveHandler, method: http.MethodGet, path: "/internal/kyusu/live/1/720p/1/", wantCode: http.StatusNotFound, wantBody: notFoundBody},
		{name: "ライブのハンドラーは録画を受けない", handler: liveHandler, method: http.MethodGet, path: "/internal/kyusu/recordings/abc/720p/1/playlist.m3u8", wantCode: http.StatusNotFound, wantBody: notFoundBody},
		{name: "録画のルートに当たる", handler: vodHandler, method: http.MethodGet, path: "/internal/kyusu/recordings/bad/720p/1/playlist.m3u8", wantCode: http.StatusBadRequest, wantBody: invalidID},
		{name: "タイムシフトのルートに当たる", handler: vodHandler, method: http.MethodGet, path: "/internal/kyusu/timeshift/bad/720p/1/playlist.m3u8", wantCode: http.StatusBadRequest, wantBody: invalidID},
		{name: "ライブラリのルートに当たる", handler: vodHandler, method: http.MethodGet, path: "/internal/kyusu/library/bad/720p/1/playlist.m3u8", wantCode: http.StatusBadRequest, wantBody: invalidID},
		{name: "録画のルートに POST すると 405", handler: vodHandler, method: http.MethodPost, path: "/internal/kyusu/recordings/bad/720p/1/playlist.m3u8", wantCode: http.StatusMethodNotAllowed, wantAllow: allowGet},
		{name: "録画の区切りが多いと当たらない", handler: vodHandler, method: http.MethodGet, path: "/internal/kyusu/recordings/bad/720p/1/a/b", wantCode: http.StatusNotFound, wantBody: notFoundBody},
		{name: "VOD のハンドラーはライブを受けない", handler: vodHandler, method: http.MethodGet, path: livePlaylist, wantCode: http.StatusNotFound, wantBody: notFoundBody},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil)
			if !tt.noToken {
				token.Authorize(req)
			}
			rec := httptest.NewRecorder()
			tt.handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Equal(t, tt.wantAllow, rec.Header().Get("Allow"))
			assert.Equal(t, tt.wantLocation, rec.Header().Get("Location"))
			assert.Contains(t, rec.Body.String(), tt.wantBody)
		})
	}
}
