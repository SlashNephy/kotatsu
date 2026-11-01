package spa_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/web/spa"
)

const noStore = "no-store"

const index = `<meta property="csp-nonce" nonce="__KOTATSU_CSP_NONCE__"><script nonce="__KOTATSU_CSP_NONCE__" src="/assets/app.js"></script>`

func builtApp() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte(index)},
		"assets/app.js":        {Data: []byte("console.log('kotatsu')")},
		"favicon.ico":          {Data: []byte("icon")},
		"manifest.webmanifest": {Data: []byte(`{"name":"Kotatsu"}`)},
	}
}

func TestHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		app    fstest.MapFS
		method string
		path   string
		// wantStatus と wantBody は応答の状態と本文。wantBody が空なら本文は確かめない。
		wantStatus int
		wantBody   string
		// wantIndex は、index.html を返すかどうか。
		wantIndex bool
		// wantCSP は、CSP を付けるかどうか。CSP の nonce は、index.html にだけ付ける。
		wantCSP     bool
		wantCache   string
		wantContent string
	}{
		{name: "root returns the index", app: builtApp(), method: http.MethodGet, path: "/", wantStatus: http.StatusOK, wantIndex: true, wantCSP: true, wantCache: noStore},
		{name: "index.html returns the index", app: builtApp(), method: http.MethodGet, path: "/index.html", wantStatus: http.StatusOK, wantIndex: true, wantCSP: true, wantCache: noStore},
		{name: "client route returns the index", app: builtApp(), method: http.MethodGet, path: "/recordings/123", wantStatus: http.StatusOK, wantIndex: true, wantCSP: true, wantCache: noStore},
		{
			name: "hashed asset is cached for long", app: builtApp(), method: http.MethodGet, path: "/assets/app.js",
			wantStatus: http.StatusOK, wantBody: "console.log('kotatsu')", wantCache: "public, max-age=31536000, immutable",
		},
		{name: "other file is revalidated", app: builtApp(), method: http.MethodGet, path: "/favicon.ico", wantStatus: http.StatusOK, wantBody: "icon", wantCache: "no-cache"},
		{
			name: "web app manifest has its media type", app: builtApp(), method: http.MethodGet, path: "/manifest.webmanifest",
			wantStatus: http.StatusOK, wantCache: "no-cache", wantContent: "application/manifest+json",
		},
		{name: "missing file is not the index", app: builtApp(), method: http.MethodGet, path: "/assets/missing.js", wantStatus: http.StatusNotFound},
		{name: "path traversal stays in the app", app: builtApp(), method: http.MethodGet, path: "/../../etc/passwd.conf", wantStatus: http.StatusNotFound},
		{name: "POST is not allowed", app: builtApp(), method: http.MethodPost, path: "/", wantStatus: http.StatusMethodNotAllowed},
		{name: "HEAD has no body", app: builtApp(), method: http.MethodHead, path: "/", wantStatus: http.StatusOK, wantCSP: true, wantCache: noStore},
		{name: "unbuilt frontend is unavailable", app: fstest.MapFS{}, method: http.MethodGet, path: "/", wantStatus: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, err := spa.NewHandler(tt.app, slog.New(slog.DiscardHandler))
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))
			body, err := io.ReadAll(rec.Result().Body)
			require.NoError(t, err)

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantBody != "" {
				assert.Equal(t, tt.wantBody, string(body))
			}
			if tt.wantCache != "" {
				assert.Equal(t, tt.wantCache, rec.Header().Get("Cache-Control"))
			}
			if tt.wantContent != "" {
				assert.Equal(t, tt.wantContent, rec.Header().Get("Content-Type"))
			}
			assert.Equal(t, tt.wantCSP, rec.Header().Get("Content-Security-Policy") != "")
			if !tt.wantIndex {
				return
			}
			// nonce は、本文の占位文字列のすべてと、CSP の script-src と style-src に同じ値で入る。
			nonce := regexp.MustCompile(`'nonce-([^']+)'`).FindStringSubmatch(rec.Header().Get("Content-Security-Policy"))
			require.Len(t, nonce, 2)
			assert.NotContains(t, string(body), "__KOTATSU_CSP_NONCE__")
			assert.Equal(t, 2, strings.Count(string(body), `nonce="`+nonce[1]+`"`))
			assert.Equal(t, spa.ContentSecurityPolicy(nonce[1]), rec.Header().Get("Content-Security-Policy"))
		})
	}
}

// TestNonceChangesPerResponse は、index.html を返すたびに、別の nonce を使うことを確かめる。
func TestNonceChangesPerResponse(t *testing.T) {
	t.Parallel()

	handler, err := spa.NewHandler(builtApp(), slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	nonces := map[string]bool{}
	for range 3 {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		nonces[rec.Header().Get("Content-Security-Policy")] = true
	}
	assert.Len(t, nonces, 3)
}

// TestContentSecurityPolicy は、明示的に許すのが WebAssembly の実行と NX-Jikkyo への接続だけであることを確かめる (設計書 10.4)。
func TestContentSecurityPolicy(t *testing.T) {
	t.Parallel()

	directives := map[string]string{}
	for directive := range strings.SplitSeq(spa.ContentSecurityPolicy("n"), "; ") {
		name, value, _ := strings.Cut(directive, " ")
		directives[name] = value
	}
	assert.Equal(t, "'self' 'nonce-n' 'wasm-unsafe-eval'", directives["script-src"])
	assert.Equal(t, "'self' blob: wss://nx-jikkyo.tsukumijima.net", directives["connect-src"])
	assert.Equal(t, "'none'", directives["object-src"])
	assert.Equal(t, "'none'", directives["frame-ancestors"])
	for name, value := range directives {
		assert.NotContains(t, value, "unsafe-inline", name)
		assert.NotContains(t, value, "'unsafe-eval'", name)
	}
}
