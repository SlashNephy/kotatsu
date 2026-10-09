package telemetry_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/config"
	"github.com/SlashNephy/kotatsu/internal/telemetry"
)

func TestMetricsHandlerExposesRuntimeMetrics(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	telemetry.NewMetricsHandler(telemetry.NewRegistry()).
		ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	assert.Contains(t, rec.Body.String(), "go_goroutines ")
	assert.Contains(t, rec.Body.String(), "process_start_time_seconds ")
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

			provider, shutdown, err := telemetry.NewTracerProvider(t.Context(), cfg)
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
