package streamapi_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/servicelogo"
	"github.com/SlashNephy/kotatsu/internal/web/streamapi"
)

const logoPattern = "GET /media/services/{id}/logo"

// htmlLogo は、ロゴの代わりに HTML を返す、壊れたチューナーサーバー。
type htmlLogo struct{}

func (htmlLogo) ServiceLogo(context.Context, int64) ([]byte, error) {
	return []byte("<!DOCTYPE html><script>alert(1)</script>"), nil
}

// collectedLogos は、録画している TS から集めたロゴの代わり。サービス ID ごとに PNG を持つ。
type collectedLogos map[int64][]byte

func (c collectedLogos) ServiceLogo(_ context.Context, serviceID int64) ([]byte, error) {
	if data, ok := c[serviceID]; ok {
		return data, nil
	}

	return nil, servicelogo.ErrNoLogo
}

// logoServer は、tuner を上流にした tenban のロゴの中継を立てる。
func logoServer(t *testing.T, tuner streamapi.LogoFetcher) *httptest.Server {
	t.Helper()

	return logoServerWithCollected(t, tuner, nil)
}

// logoServerWithCollected は、tuner を上流にし、collected を集めたロゴにした、tenban のロゴの中継を立てる。
func logoServerWithCollected(t *testing.T, tuner, collected streamapi.LogoFetcher) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle(logoPattern, streamapi.NewLogoHandler(tuner, collected, slog.New(slog.DiscardHandler)))
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
		tuner      streamapi.LogoFetcher
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

// TestLogoHandlerCollected は、チューナーサーバーがロゴを返さないときに、録画している TS から集めたロゴを返すことを確かめる (#1308)。
// チューナーサーバーがロゴを返せば、そちらを先にする。
func TestLogoHandlerCollected(t *testing.T) {
	t.Parallel()

	client := fakeTunerClient(t)
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 48, 27))))
	// GR 26 のサービスは、チューナーサーバーが画像をまだ受信していない。
	notReceived := int64(3273701032)
	collected := collectedLogos{notReceived: buf.Bytes(), 3273601024: buf.Bytes()}
	tests := []struct {
		name       string
		tuner      streamapi.LogoFetcher
		id         string
		wantStatus int
		wantWidth  int
	}{
		{name: "tuner server has the logo", tuner: client, id: serviceID, wantStatus: http.StatusOK, wantWidth: 64},
		{name: "logo not received", tuner: client, id: "3273701032", wantStatus: http.StatusOK, wantWidth: 48},
		{name: "tuner server not configured", tuner: nil, id: "3273701032", wantStatus: http.StatusOK, wantWidth: 48},
		{name: "not collected", tuner: client, id: "1", wantStatus: http.StatusNotFound, wantWidth: 0},
		{name: "not collected without tuner server", tuner: nil, id: "1", wantStatus: http.StatusServiceUnavailable, wantWidth: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := logoServerWithCollected(t, tt.tuner, collected)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/media/services/"+tt.id+"/logo", nil)
			require.NoError(t, err)
			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			require.Equal(t, tt.wantStatus, resp.StatusCode, string(body))
			if tt.wantStatus != http.StatusOK {
				return
			}
			assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
			config, err := png.DecodeConfig(bytes.NewReader(body))
			require.NoError(t, err)
			assert.Equal(t, tt.wantWidth, config.Width)
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

// TestLogoHandlerKeepsCacheControl は、手前で Cache-Control を決めていれば、上書きしないことを確かめる (#1324)。
// IPTV の配信は、URL に秘密の値を含むので、ロゴも途中のキャッシュに残させない。
func TestLogoHandlerKeepsCacheControl(t *testing.T) {
	t.Parallel()

	handler := streamapi.NewLogoHandler(fakeTunerClient(t), nil, slog.New(slog.DiscardHandler))
	mux := http.NewServeMux()
	mux.Handle(logoPattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		handler.ServeHTTP(w, r)
	}))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/media/services/"+serviceID+"/logo", nil)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "private, no-store", resp.Header.Get("Cache-Control"))
}
