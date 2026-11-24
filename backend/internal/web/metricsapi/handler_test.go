package metricsapi_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/infra/telemetry"
	"github.com/SlashNephy/kotatsu/internal/web/metricsapi"
)

const (
	metricsPath     = "/metrics"
	pprofIndexPath  = "/debug/pprof/"
	pprofSymbolPath = "/debug/pprof/symbol"
	allowGet        = "GET, HEAD"
	notFoundBody    = "404 page not found\n"
)

func TestHandlerExposesRuntimeMetrics(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	registry := telemetry.NewRegistry()
	metricsapi.NewHandler(slog.New(slog.DiscardHandler), registry, registry, false).
		ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, metricsPath, nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	assert.Contains(t, rec.Body.String(), "go_goroutines ")
	assert.Contains(t, rec.Body.String(), "process_start_time_seconds ")
}

// TestHandlerRoutes は、http.ServeMux で受けていたときと同じパス、メソッド、応答を返すことを確認する (#1959、#2008)。
func TestHandlerRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		withPprof    bool
		method       string
		path         string
		wantCode     int
		wantAllow    string
		wantLocation string
		wantBody     string
	}{
		{name: "HEAD でも /metrics を返す", method: http.MethodHead, path: metricsPath, wantCode: http.StatusOK},
		{name: "/metrics に POST すると 405", method: http.MethodPost, path: metricsPath, wantCode: http.StatusMethodNotAllowed, wantAllow: allowGet, wantBody: "Method Not Allowed\n"},
		{name: "末尾にスラッシュを付けた /metrics は 404", method: http.MethodGet, path: "/metrics/", wantCode: http.StatusNotFound, wantBody: notFoundBody},
		{name: "二重のスラッシュは整えた先へリダイレクトする", method: http.MethodGet, path: "//metrics", wantCode: http.StatusTemporaryRedirect, wantLocation: "/metrics"},
		{name: "知らないパスは素の 404", method: http.MethodPost, path: "/nope", wantCode: http.StatusNotFound, wantBody: notFoundBody},
		{name: "無効なら一覧を返さない", method: http.MethodGet, path: pprofIndexPath, wantCode: http.StatusNotFound, wantBody: notFoundBody},
		{name: "無効なら末尾のスラッシュがなくてもリダイレクトしない", method: http.MethodGet, path: "/debug/pprof", wantCode: http.StatusNotFound},
		{name: "無効ならプロファイルを返さない", method: http.MethodGet, path: "/debug/pprof/goroutine?debug=1", wantCode: http.StatusNotFound},
		{name: "無効なら symbol に POST しても 404", method: http.MethodPost, path: pprofSymbolPath, wantCode: http.StatusNotFound},
		{name: "有効なら一覧を返す", withPprof: true, method: http.MethodGet, path: pprofIndexPath, wantCode: http.StatusOK, wantBody: "goroutine"},
		{name: "有効なら HEAD でも一覧を返す", withPprof: true, method: http.MethodHead, path: pprofIndexPath, wantCode: http.StatusOK},
		{name: "有効なら末尾にスラッシュを付けた先へリダイレクトする", withPprof: true, method: http.MethodGet, path: "/debug/pprof", wantCode: http.StatusTemporaryRedirect, wantLocation: "/debug/pprof/"},
		{name: "有効なら名前の付いたプロファイルを返す", withPprof: true, method: http.MethodGet, path: "/debug/pprof/goroutine?debug=1", wantCode: http.StatusOK, wantBody: "goroutine profile:"},
		{name: "名前の付いたプロファイルに POST すると 405", withPprof: true, method: http.MethodPost, path: "/debug/pprof/heap", wantCode: http.StatusMethodNotAllowed, wantAllow: allowGet},
		{name: "cmdline に POST すると 405", withPprof: true, method: http.MethodPost, path: "/debug/pprof/cmdline", wantCode: http.StatusMethodNotAllowed, wantAllow: allowGet},
		{name: "symbol は POST でも返す", withPprof: true, method: http.MethodPost, path: pprofSymbolPath, wantCode: http.StatusOK, wantBody: "num_symbols:"},
		{name: "symbol に PUT すると 405", withPprof: true, method: http.MethodPut, path: pprofSymbolPath, wantCode: http.StatusMethodNotAllowed, wantAllow: "GET, HEAD, POST"},
		{name: "有効でも /metrics はそのまま返す", withPprof: true, method: http.MethodGet, path: metricsPath, wantCode: http.StatusOK, wantBody: "go_goroutines "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			registry := telemetry.NewRegistry()
			metricsapi.NewHandler(slog.New(slog.DiscardHandler), registry, registry, tt.withPprof).
				ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))

			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Equal(t, tt.wantAllow, rec.Header().Get("Allow"))
			assert.Equal(t, tt.wantLocation, rec.Header().Get("Location"))
			assert.Contains(t, rec.Body.String(), tt.wantBody)
		})
	}
}

// TestHandlerMergesGatherers は、/metrics が、インスタンスとクラスターのレジストリをまとめて返すことを確認する (#962)。
func TestHandlerMergesGatherers(t *testing.T) {
	t.Parallel()

	instance, cluster := prometheus.NewRegistry(), prometheus.NewRegistry()
	instance.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{Name: "instance_value", Help: "instance"}))
	cluster.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{Name: "cluster_value", Help: "cluster"}))

	rec := httptest.NewRecorder()
	metricsapi.NewHandler(slog.New(slog.DiscardHandler), prometheus.Gatherers{instance, cluster}, instance, false).
		ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, metricsPath, nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "instance_value 0")
	assert.Contains(t, rec.Body.String(), "cluster_value 0")
}
