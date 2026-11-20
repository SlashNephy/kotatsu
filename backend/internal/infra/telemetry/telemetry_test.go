package telemetry_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/telemetry"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
)

func TestMetricsHandlerExposesRuntimeMetrics(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	registry := telemetry.NewRegistry()
	telemetry.NewMetricsHandler(registry, registry, false).
		ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	assert.Contains(t, rec.Body.String(), "go_goroutines ")
	assert.Contains(t, rec.Body.String(), "process_start_time_seconds ")
}

// TestMetricsHandlerPprof は、設定で有効にしたときだけ /debug/pprof/ を返すことを確かめる (#1959)。
func TestMetricsHandlerPprof(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		withPprof bool
		path      string
		wantCode  int
		wantBody  string
	}{
		{name: "無効なら一覧を返さない", withPprof: false, path: "/debug/pprof/", wantCode: http.StatusNotFound},
		{name: "無効ならプロファイルを返さない", withPprof: false, path: "/debug/pprof/goroutine?debug=1", wantCode: http.StatusNotFound},
		{name: "有効なら一覧を返す", withPprof: true, path: "/debug/pprof/", wantCode: http.StatusOK, wantBody: "goroutine"},
		{name: "有効なら名前の付いたプロファイルを返す", withPprof: true, path: "/debug/pprof/goroutine?debug=1", wantCode: http.StatusOK, wantBody: "goroutine profile:"},
		{name: "有効でも /metrics はそのまま返す", withPprof: true, path: "/metrics", wantCode: http.StatusOK, wantBody: "go_goroutines "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			registry := telemetry.NewRegistry()
			telemetry.NewMetricsHandler(registry, registry, tt.withPprof).
				ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil))

			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantBody)
		})
	}
}

// TestMetricsHandlerMergesGatherers は、/metrics が、インスタンスとクラスターのレジストリをまとめて返すことを確かめる (#962)。
func TestMetricsHandlerMergesGatherers(t *testing.T) {
	t.Parallel()

	instance, cluster := prometheus.NewRegistry(), prometheus.NewRegistry()
	instance.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{Name: "instance_value", Help: "instance"}))
	cluster.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{Name: "cluster_value", Help: "cluster"}))

	rec := httptest.NewRecorder()
	telemetry.NewMetricsHandler(prometheus.Gatherers{instance, cluster}, instance, false).
		ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "instance_value 0")
	assert.Contains(t, rec.Body.String(), "cluster_value 0")
}

func TestTracerProviderSendsTracesOnlyWhenConfigured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured bool
	}{
		{name: "configured", configured: true},
		{name: "not configured", configured: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var received atomic.Int32
			collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/v1/traces" {
					received.Add(1)
				}
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(collector.Close)

			cfg := &config.Telemetry{OTLPEndpoint: ""}
			if tt.configured {
				cfg.OTLPEndpoint = collector.URL
			}

			provider, shutdown, err := telemetry.NewTracerProvider(t.Context(), cfg, nil)
			require.NoError(t, err)

			_, span := provider.Tracer("test").Start(t.Context(), "operation")
			assert.Equal(t, tt.configured, span.IsRecording())
			span.End()

			// shutdown は溜まっているトレースを送り切る。
			require.NoError(t, shutdown(t.Context()))
			if tt.configured {
				assert.Positive(t, received.Load())
			} else {
				assert.Zero(t, received.Load())
			}
		})
	}
}
