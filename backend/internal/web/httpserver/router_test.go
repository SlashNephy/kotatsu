package httpserver_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

// recordingHandler は、ログの記録の属性を記録する。
type recordingHandler struct {
	mu    sync.Mutex
	attrs []map[string]string
	with  []slog.Attr
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]string{}
	for _, a := range h.with {
		attrs[a.Key] = a.Value.String()
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()

		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.attrs = append(h.attrs, attrs)

	return nil
}

func (h *recordingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &recordingRoot{root: h, with: append(append([]slog.Attr{}, h.with...), attrs...)}
}

func (h *recordingHandler) WithGroup(string) slog.Handler { return h }

// recordingRoot は、WithAttrs で足した属性を持ち、記録は元の recordingHandler に書く。
type recordingRoot struct {
	root *recordingHandler
	with []slog.Attr
}

func (h *recordingRoot) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingRoot) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(h.with...)

	return h.root.Handle(ctx, r)
}

func (h *recordingRoot) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &recordingRoot{root: h.root, with: append(append([]slog.Attr{}, h.with...), attrs...)}
}

func (h *recordingRoot) WithGroup(string) slog.Handler { return h }

func TestRouter(t *testing.T) {
	t.Parallel()

	logs := &recordingHandler{mu: sync.Mutex{}, attrs: []map[string]string{}, with: nil}
	fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	router := httpserver.NewRouter(slog.New(logs), fallback)
	router.GET("/moved/:name", func(c *echo.Context) error {
		c.Logger().Info("served", slog.String("name", c.Param("name")))

		return c.NoContent(http.StatusNoContent)
	}, httpserver.Component("moved"))

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "moved route", method: http.MethodGet, path: "/moved/a", wantStatus: http.StatusNoContent},
		// echo のルートに当たらなければ、移していない mux (fallback) に渡す。
		{name: "fallback", method: http.MethodGet, path: "/other", wantStatus: http.StatusTeapot},
		{name: "fallback root", method: http.MethodPost, path: "/", wantStatus: http.StatusTeapot},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))
		assert.Equal(t, tt.wantStatus, rec.Code, tt.name)
	}

	// 移したルートのハンドラーは、部品の名前の付いたリクエストのロガーに書く。
	require.Len(t, logs.attrs, 1)
	assert.Equal(t, map[string]string{"component": "moved", "name": "a"}, logs.attrs[0])
}

func TestHandle(t *testing.T) {
	t.Parallel()

	const both = "/both"
	ok := func(status int) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
	}
	router := httpserver.NewRouter(slog.New(slog.DiscardHandler), ok(http.StatusTeapot))
	httpserver.Handle(router, both, map[string]http.Handler{http.MethodGet: ok(http.StatusOK), http.MethodPost: ok(http.StatusCreated)})
	httpserver.Handle(router, "/post", map[string]http.Handler{http.MethodPost: ok(http.StatusCreated)})

	// 移す前の http.ServeMux と同じ応答を返すことを確認する。
	mux := http.NewServeMux()
	mux.Handle("GET /both", ok(http.StatusOK))
	mux.Handle("POST /both", ok(http.StatusCreated))
	mux.Handle("POST /post", ok(http.StatusCreated))

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantAllow  string
	}{
		{name: "GET", method: http.MethodGet, path: both, wantStatus: http.StatusOK, wantAllow: ""},
		{name: "HEAD answers like GET", method: http.MethodHead, path: both, wantStatus: http.StatusOK, wantAllow: ""},
		{name: "POST to both", method: http.MethodPost, path: both, wantStatus: http.StatusCreated, wantAllow: ""},
		{name: "other method", method: http.MethodDelete, path: both, wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET, HEAD, POST"},
		{name: "GET to POST only", method: http.MethodGet, path: "/post", wantStatus: http.StatusMethodNotAllowed, wantAllow: "POST"},
		{name: "OPTIONS to POST only", method: http.MethodOptions, path: "/post", wantStatus: http.StatusMethodNotAllowed, wantAllow: "POST"},
	}
	for _, tt := range tests {
		for name, handler := range map[string]http.Handler{echoName: router, muxName: mux} {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))
			assert.Equal(t, tt.wantStatus, rec.Code, "%s: %s", name, tt.name)
			assert.Equal(t, tt.wantAllow, rec.Header().Get("Allow"), "%s: %s", name, tt.name)
		}
	}
}

// TestHandleWebSocket は、Handle で登録した net/http のハンドラーが、echo の応答を通して WebSocket の接続を受け入れられる
// (Hijack できる) ことを確認する。実況のコメントの中継 (/internal/media/jikkyo/:channel/ws) が使う。
func TestHandleWebSocket(t *testing.T) {
	t.Parallel()

	router := httpserver.NewRouter(slog.New(slog.DiscardHandler), http.NotFoundHandler())
	httpserver.Handle(router, "/ws/:channel", map[string]http.Handler{
		http.MethodGet: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer func() { _ = conn.CloseNow() }()
			_ = conn.Write(r.Context(), websocket.MessageText, []byte(r.PathValue("channel")))
		}),
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	conn, resp, err := websocket.Dial(t.Context(), "ws"+server.URL[len("http"):]+"/ws/jk1", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	// 接続できたときの応答の本文は、websocket.Dial が空にしている。
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	defer func() { _ = conn.CloseNow() }()
	_, message, err := conn.Read(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "jk1", string(message))
}

// TestCanonicalPath は、CanonicalPath を掛けた echo が、http.ServeMux と同じく、崩れたパスと配下を受けるパターンの
// 末尾のスラッシュのないパスを、同じ先へリダイレクトすることを確認する。
func TestCanonicalPath(t *testing.T) {
	t.Parallel()

	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	router := httpserver.NewRouter(slog.New(slog.DiscardHandler), ok)
	router.Pre(httpserver.CanonicalPath("/api/"))
	mux := http.NewServeMux()
	mux.Handle("/api/", ok)
	mux.Handle("/", ok)

	tests := []struct {
		name         string
		method       string
		target       string
		wantStatus   int
		wantLocation string
	}{
		{name: "clean", method: http.MethodGet, target: guidePath, wantStatus: http.StatusOK, wantLocation: ""},
		{name: "trailing slash is kept", method: http.MethodGet, target: "/guide/", wantStatus: http.StatusOK, wantLocation: ""},
		{name: "double slash", method: http.MethodGet, target: "//guide//x", wantStatus: http.StatusTemporaryRedirect, wantLocation: "/guide/x"},
		{name: "dot segments with query", method: http.MethodPost, target: "/a/./b/../guide?x=1", wantStatus: http.StatusTemporaryRedirect, wantLocation: "/a/guide?x=1"},
		{name: "escaped slash is kept", method: http.MethodGet, target: "/a%2Fb//c", wantStatus: http.StatusTemporaryRedirect, wantLocation: "/a%2Fb/c"},
		{name: "subtree without slash", method: http.MethodGet, target: "/api?x=1", wantStatus: http.StatusTemporaryRedirect, wantLocation: "/api/?x=1"},
		{name: "subtree with double slash", method: http.MethodGet, target: "//api", wantStatus: http.StatusTemporaryRedirect, wantLocation: "/api/"},
		// ブラウザは「/\」を「//」と読むので、\ はエスケープしたまま返し、ほかのホストへのリダイレクトにしない。
		{name: "leading backslash", method: http.MethodGet, target: `/\evil.example/`, wantStatus: http.StatusOK, wantLocation: ""},
		{name: "double slash before backslash", method: http.MethodGet, target: `//\evil.example/`, wantStatus: http.StatusTemporaryRedirect, wantLocation: "/%5Cevil.example/"},
	}
	for _, tt := range tests {
		for name, handler := range map[string]http.Handler{echoName: router, muxName: mux} {
			req := httptest.NewRequestWithContext(t.Context(), tt.method, "http://example.com"+tt.target, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.wantStatus, rec.Code, "%s: %s", name, tt.name)
			assert.Equal(t, tt.wantLocation, rec.Header().Get("Location"), "%s: %s", name, tt.name)
		}
	}
}

// echoName は、http.ServeMux と並べて確認するときの、echo のルーターの名前。
const echoName = "echo"

// muxName は、並べて確認する http.ServeMux の名前。
const muxName = "mux"

// gotHeader は、ハンドラーが読んだ値を返すヘッダー。
const gotHeader = "X-Got"

// fallbackBody は、どのルートにも当たらないときの本文。
const fallbackBody = "fallback\n"

// TestRouterPathParams は、名前を付けたパスの部分を、echo が http.ServeMux と同じく読むことを確認する (#2016)。
// echo は、末尾の名前の部分をスラッシュをまたいで当て、エスケープしたパスで当てた値を戻さないからである。
func TestRouterPathParams(t *testing.T) {
	t.Parallel()

	notFound := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, strings.TrimSuffix(fallbackBody, "\n"), http.StatusNotFound)
	})
	params := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(gotHeader, r.PathValue("a")+"|"+r.PathValue("b"))
	})
	router := httpserver.NewRouter(slog.New(slog.DiscardHandler), notFound)
	httpserver.Handle(router, "/p/:a/:b", map[string]http.Handler{http.MethodGet: params})
	router.GET("/rest/*", echo.WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(gotHeader, r.URL.Path)
	})))
	mux := http.NewServeMux()
	mux.Handle("GET /p/{a}/{b}", params)
	mux.Handle("/rest/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(gotHeader, r.URL.Path)
	}))
	mux.Handle("/", notFound)

	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
		wantGot    string
	}{
		{name: "当たる", method: http.MethodGet, target: "/p/1/2", wantStatus: http.StatusOK, wantGot: "1|2"},
		{name: "区切りが多いと当たらない", method: http.MethodGet, target: "/p/1/2/3", wantStatus: http.StatusNotFound, wantGot: ""},
		{name: "区切りが多いとメソッドが違っても当たらない", method: http.MethodPost, target: "/p/1/2/3", wantStatus: http.StatusNotFound, wantGot: ""},
		{name: "空の区切りには当たらない", method: http.MethodGet, target: "/p/1/", wantStatus: http.StatusNotFound, wantGot: ""},
		{name: "エスケープした値を戻す", method: http.MethodGet, target: "/p/%31/a%20b", wantStatus: http.StatusOK, wantGot: "1|a b"},
		{name: "エスケープしたパーセントは 1 度だけ戻す", method: http.MethodGet, target: "/p/a%25b/c", wantStatus: http.StatusOK, wantGot: "a%b|c"},
		{name: "名前のない残りはスラッシュを含んでよい", method: http.MethodGet, target: "/rest/a/b/c", wantStatus: http.StatusOK, wantGot: "/rest/a/b/c"},
	}
	for _, tt := range tests {
		for name, handler := range map[string]http.Handler{echoName: router, muxName: mux} {
			req := httptest.NewRequestWithContext(t.Context(), tt.method, "http://example.com"+tt.target, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.wantStatus, rec.Code, "%s: %s", name, tt.name)
			assert.Equal(t, tt.wantGot, rec.Header().Get(gotHeader), "%s: %s", name, tt.name)
		}
	}
}

// TestRouterContextParam は、echo のハンドラーが c.Param で読む値も、戻した値になることを確認する (#2016)。
// カレンダーと IPTV の端点は、秘密の値を c.Param で読む。
func TestRouterContextParam(t *testing.T) {
	t.Parallel()

	router := httpserver.NewRouter(slog.New(slog.DiscardHandler), http.NotFoundHandler())
	router.GET("/c/:token", func(c *echo.Context) error {
		c.Response().Header().Set(gotHeader, c.Param("token"))

		return c.NoContent(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/c/a%2Db%7E", nil))
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "a-b~", rec.Header().Get(gotHeader))
}

// TestRouterRejectsPathLikeParams は、戻すと「/」を含むか、「.」か「..」になる名前の部分を、どのルートにも当たらなかったものとして
// 扱うことを確認する (#2016)。ハンドラーが値をパスに組み直しても、ほかのパスを指させないためである。http.ServeMux はこれらを値として渡す。
func TestRouterRejectsPathLikeParams(t *testing.T) {
	t.Parallel()

	notFound := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, strings.TrimSuffix(fallbackBody, "\n"), http.StatusNotFound)
	})
	router := httpserver.NewRouter(slog.New(slog.DiscardHandler), notFound)
	httpserver.Handle(router, "/p/:a/:b", map[string]http.Handler{http.MethodGet: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(gotHeader, r.PathValue("a")+"|"+r.PathValue("b"))
	})})

	for _, target := range []string{"/p/1%2F2/3", "/p/1/..%2F..%2Fmetrics", "/p/%2E%2E/3", "/p/%2E/3", "/p/1/%2e%2e", "/p/./3", "/p/1/.."} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com"+target, nil))
		assert.Equal(t, http.StatusNotFound, rec.Code, target)
		assert.Equal(t, fallbackBody, rec.Body.String(), target)
		assert.Empty(t, rec.Header().Get(gotHeader), target)
	}
}
