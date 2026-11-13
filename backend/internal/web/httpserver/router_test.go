package httpserver_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

// recordingHandler は、ログの記録の属性を覚える。
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
