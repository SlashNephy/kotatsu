package telemetry_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/SlashNephy/kotatsu/internal/infra/telemetry"
)

// TestRedactingExporter は、otelhttp が作る本物の span を通して、視聴者を識別できる属性と再生の ID が送られないことを確かめる (#1364)。
func TestRedactingExporter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		wantPath string
	}{
		{name: "recording stream", path: "/internal/streams/recordings/42/hls/1080p/0/seg-00001.ts", wantPath: "/internal/streams/:id/:id/:id/:id/:id/:id"},
		{name: "uuid", path: "/internal/media/recordings/01926d1e-7c3a-7b1a-9c11-2f3b4c5d6e7f/thumbnail.jpg", wantPath: "/internal/media/:id/:id/:id"},
		{name: "username without digits", path: "/internal/media/anime-following/annict/alice/avatar", wantPath: "/internal/media/:id/:id/:id/:id"},
		{name: "no id", path: "/internal/graphql", wantPath: "/internal/graphql"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			memory := tracetest.NewInMemoryExporter()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(telemetry.NewRedactingExporter(memory)))
			handler := otelhttp.NewHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), "test", otelhttp.WithTracerProvider(provider))

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil)
			req.RemoteAddr = "192.0.2.10:54321"
			req.Header.Set("X-Forwarded-For", "198.51.100.7")
			req.Header.Set("User-Agent", "viewer-device/1.0")
			handler.ServeHTTP(httptest.NewRecorder(), req)

			spans := memory.GetSpans()
			require.Len(t, spans, 1)
			attributes := map[attribute.Key]string{}
			for _, kv := range spans[0].Attributes {
				attributes[kv.Key] = kv.Value.String()
				assert.NotContains(t, kv.Value.String(), "192.0.2.10", kv.Key)
				assert.NotContains(t, kv.Value.String(), "198.51.100.7", kv.Key)
				assert.NotContains(t, kv.Value.String(), "viewer-device", kv.Key)
			}
			assert.Equal(t, tt.wantPath, attributes["url.path"])
			assert.Equal(t, "GET", attributes["http.request.method"], "調査に使う属性は残す")
			assert.NotContains(t, attributes, attribute.Key("client.address"))
			assert.NotContains(t, attributes, attribute.Key("network.peer.address"))
			assert.NotContains(t, attributes, attribute.Key("user_agent.original"))
		})
	}
}
