package stream_test

import (
	"bytes"
	"context"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/stream"
	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const logoPattern = "GET /media/services/{id}/logo"

// htmlLogo は、ロゴの代わりに HTML を返す、壊れたチューナーサーバー。
type htmlLogo struct{}

func (htmlLogo) ServiceLogo(context.Context, int64) ([]byte, error) {
	return []byte("<!DOCTYPE html><script>alert(1)</script>"), nil
}

// logoServer は、tuner を上流にした tenban のロゴの中継を立てる。
func logoServer(t *testing.T, tuner stream.LogoFetcher) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle(logoPattern, stream.NewLogoHandler(tuner, slog.New(slog.DiscardHandler)))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func fakeTunerClient(t *testing.T) *mirakurun.Client {
	t.Helper()

	upstream := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC), nil)))
	t.Cleanup(upstream.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: upstream.URL})
	require.NoError(t, err)

	return client
}

func TestLogoHandler(t *testing.T) {
	t.Parallel()

	client := fakeTunerClient(t)
	tests := []struct {
		name       string
		tuner      stream.LogoFetcher
		id         string
		wantStatus int
	}{
		{name: "logo", tuner: client, id: serviceID, wantStatus: http.StatusOK},
		// GR 26 のサービスは、ロゴの番号はあるが、チューナーサーバーが画像をまだ受信していない。
		{name: "logo not received", tuner: client, id: "3273701032", wantStatus: http.StatusNotFound},
		{name: "unknown service", tuner: client, id: "1", wantStatus: http.StatusNotFound},
		{name: "invalid service ID", tuner: client, id: "abc", wantStatus: http.StatusBadRequest},
		{name: "tuner server not configured", tuner: nil, id: serviceID, wantStatus: http.StatusServiceUnavailable},
		{name: "not an image", tuner: htmlLogo{}, id: serviceID, wantStatus: http.StatusBadGateway},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := logoServer(t, tt.tuner)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/media/services/"+tt.id+"/logo", nil)
			require.NoError(t, err)
			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tt.wantStatus, resp.StatusCode, string(body))
			if tt.wantStatus != http.StatusOK {
				assert.NotEqual(t, "image/png", resp.Header.Get("Content-Type"))

				return
			}
			assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
			assert.Equal(t, "private, max-age=86400", resp.Header.Get("Cache-Control"))
			assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
			assert.NotEmpty(t, resp.Header.Get("ETag"))
			config, err := png.DecodeConfig(bytes.NewReader(body))
			require.NoError(t, err)
			assert.Equal(t, 64, config.Width)
		})
	}
}

// TestLogoHandlerNotModified は、ブラウザが持っているロゴと同じなら、本文を送らずに 304 を返すことを確かめる。
func TestLogoHandlerNotModified(t *testing.T) {
	t.Parallel()

	server := logoServer(t, fakeTunerClient(t))
	url := server.URL + "/media/services/" + serviceID + "/logo"
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	first, err := server.Client().Do(req)
	require.NoError(t, err)
	_ = first.Body.Close()
	etag := first.Header.Get("ETag")
	require.NotEmpty(t, etag)

	req, err = http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("If-None-Match", etag)
	second, err := server.Client().Do(req)
	require.NoError(t, err)
	defer second.Body.Close()

	assert.Equal(t, http.StatusNotModified, second.StatusCode)
}
