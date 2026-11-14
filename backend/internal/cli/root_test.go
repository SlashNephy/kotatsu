package cli_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/oauth"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// appHTML は、画面 (SPA) の応答の 1 行目。
	appHTML = "<!doctype html>"
	// notFoundBody は、http.NotFound の応答の本文。
	notFoundBody = "404 page not found"
	// guidePath は、画面のパスの例。
	guidePath = "/guide"
)

// TestTenbanRoutesRoot は、tenban の一番外側の echo のルーターが (#1606)、http.ServeMux の fallback をなくしても、
// mux のときと同じく、崩れたパスを整えた先へリダイレクトし、/api と /internal の下の知らないパスに画面を返さず、
// ほかのパスに画面を返し、OAuth の認可サーバーと MCP サーバーの端点に答えることを確かめる。
func TestTenbanRoutesRoot(t *testing.T) {
	t.Parallel()

	base, _ := startTenban(t)
	tests := []struct {
		name         string
		method       string
		path         string
		wantStatus   int
		wantLocation string
		wantBody     string
	}{
		{name: "app", method: http.MethodGet, path: "/", wantStatus: http.StatusOK, wantLocation: "", wantBody: appHTML},
		{name: "app route", method: http.MethodGet, path: guidePath, wantStatus: http.StatusOK, wantLocation: "", wantBody: appHTML},
		{name: "app with POST", method: http.MethodPost, path: guidePath, wantStatus: http.StatusMethodNotAllowed, wantLocation: "", wantBody: "method not allowed"},
		{name: "double slash before app", method: http.MethodGet, path: "/" + guidePath, wantStatus: http.StatusTemporaryRedirect, wantLocation: guidePath, wantBody: `<a href="/guide">Temporary Redirect</a>.`},
		{name: "dot segments with query", method: http.MethodGet, path: "/a/../guide?x=1", wantStatus: http.StatusTemporaryRedirect, wantLocation: "/guide?x=1", wantBody: `<a href="/guide?x=1">Temporary Redirect</a>.`},
		{name: "dot segments with POST", method: http.MethodPost, path: "/a/../internal/graphql", wantStatus: http.StatusTemporaryRedirect, wantLocation: "/internal/graphql", wantBody: ""},
		{name: "api without slash", method: http.MethodGet, path: "/api", wantStatus: http.StatusTemporaryRedirect, wantLocation: "/api/", wantBody: `<a href="/api/">Temporary Redirect</a>.`},
		{name: "internal without slash", method: http.MethodGet, path: "/internal", wantStatus: http.StatusTemporaryRedirect, wantLocation: "/internal/", wantBody: `<a href="/internal/">Temporary Redirect</a>.`},
		{name: "unknown api", method: http.MethodGet, path: "/api/nope", wantStatus: http.StatusNotFound, wantLocation: "", wantBody: notFoundBody},
		{name: "unknown internal", method: http.MethodPost, path: "/internal/nope", wantStatus: http.StatusNotFound, wantLocation: "", wantBody: notFoundBody},
		{name: "POST to calendar", method: http.MethodPost, path: "/api/calendar/x/reservations.ics", wantStatus: http.StatusNotFound, wantLocation: "", wantBody: notFoundBody},
		{name: "OAuth metadata", method: http.MethodGet, path: oauth.MetadataPath, wantStatus: http.StatusNotFound, wantLocation: "", wantBody: notFoundBody},
		{name: "OAuth metadata HEAD", method: http.MethodHead, path: oauth.MetadataPath, wantStatus: http.StatusNotFound, wantLocation: "", wantBody: ""},
		{name: "OAuth metadata with POST", method: http.MethodPost, path: oauth.MetadataPath, wantStatus: http.StatusMethodNotAllowed, wantLocation: "", wantBody: "method not allowed"},
		{name: "protected resource metadata", method: http.MethodGet, path: oauth.ProtectedResourceMetadataPath, wantStatus: http.StatusNotFound, wantLocation: "", wantBody: notFoundBody},
		{name: "authorize", method: http.MethodGet, path: oauth.AuthorizePath, wantStatus: http.StatusNotFound, wantLocation: "", wantBody: notFoundBody},
		{name: "token", method: http.MethodPost, path: oauth.ExchangePath, wantStatus: http.StatusNotFound, wantLocation: "", wantBody: notFoundBody},
		{name: "token with GET", method: http.MethodGet, path: oauth.ExchangePath, wantStatus: http.StatusOK, wantLocation: "", wantBody: appHTML},
		{name: "revoke", method: http.MethodPost, path: oauth.RevokePath, wantStatus: http.StatusNotFound, wantLocation: "", wantBody: notFoundBody},
		{name: "MCP without token", method: http.MethodPost, path: oauth.ResourcePath, wantStatus: http.StatusNotFound, wantLocation: "", wantBody: notFoundBody},
		{name: "MCP with GET", method: http.MethodGet, path: oauth.ResourcePath, wantStatus: http.StatusNotFound, wantLocation: "", wantBody: notFoundBody},
		{name: "under MCP", method: http.MethodGet, path: oauth.ResourcePath + "/x", wantStatus: http.StatusOK, wantLocation: "", wantBody: appHTML},
	}
	client := httpclienttest.New(t)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), tt.method, base+tt.path, http.NoBody)
			require.NoError(t, err)
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer func() {
				_ = resp.Body.Close()
			}()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			first, _, _ := strings.Cut(strings.TrimSpace(string(body)), "\n")
			assert.Equal(t, tt.wantStatus, resp.StatusCode, tt.name)
			assert.Equal(t, tt.wantLocation, resp.Header.Get("Location"), tt.name)
			assert.Equal(t, tt.wantBody, first, tt.name)
		})
	}
}

// TestTenbanServesOAuth は、MCP を有効にしたとき、echo のルーターで受けた認可サーバーと MCP サーバーの端点が答えることを確かめる (#1606)。
func TestTenbanServesOAuth(t *testing.T) {
	t.Parallel()

	base, pool := startTenban(t)
	clk := new(clock.Real)
	_, err := settings.NewStore(clk, audit.NewRecorder(clk)).Update(t.Context(), pool, audit.System("test"), &settings.Changes{MCPEnabled: new(true)})
	require.NoError(t, err)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantType   string
	}{
		{name: "metadata", method: http.MethodGet, path: oauth.MetadataPath, wantStatus: http.StatusOK, wantType: jsonContentType},
		{name: "metadata HEAD", method: http.MethodHead, path: oauth.MetadataPath, wantStatus: http.StatusOK, wantType: jsonContentType},
		{name: "protected resource metadata", method: http.MethodGet, path: oauth.ProtectedResourceMetadataPath, wantStatus: http.StatusOK, wantType: jsonContentType},
		{name: "token without grant", method: http.MethodPost, path: oauth.ExchangePath, wantStatus: http.StatusBadRequest, wantType: jsonContentType},
		{name: "MCP without token", method: http.MethodPost, path: oauth.ResourcePath, wantStatus: http.StatusUnauthorized, wantType: "text/plain"},
	}
	client := httpclienttest.New(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), tt.method, base+tt.path, http.NoBody)
			require.NoError(t, err)
			resp, err := client.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			mediaType, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";")
			assert.Equal(t, tt.wantType, mediaType)
		})
	}
}
