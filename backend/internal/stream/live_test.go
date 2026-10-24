package stream_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/stream"
	"github.com/SlashNephy/kotatsu/internal/ts"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	// serviceID は、faketuner の「こたつテスト1」のサービス ID。testdata/ts/test.ts を流す。
	serviceID = "3273601024"
	// livePattern は、ライブ視聴のパス。
	livePattern = "GET /streams/services/{id}"
)

func loadTestTS(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ts", "test.ts"))
	require.NoError(t, err)

	return data
}

// liveServer は、faketuner を上流にした tenban のライブ視聴を立てる。tuner が false なら、チューナーサーバーを設定しない。
func liveServer(t *testing.T, tuner bool) (*httptest.Server, *faketuner.Server) {
	t.Helper()

	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	fake := faketuner.New(faketuner.DefaultOptions(clk.Now(), loadTestTS(t)))
	upstream := httptest.NewServer(fake)
	t.Cleanup(upstream.Close)
	var opener stream.ServiceOpener
	if tuner {
		client, err := mirakurun.New(&mirakurun.Options{
			URL: upstream.URL, Username: "", Password: "", UserAgent: "", HTTPClient: nil, RequestTimeout: 0, DetectInterval: 0, Clock: clk,
		})
		require.NoError(t, err)
		opener = client
	}
	mux := http.NewServeMux()
	mux.Handle(livePattern, stream.NewLiveHandler(opener, slog.New(slog.DiscardHandler)))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server, fake
}

// usersOf は、チューナーを使っている利用者の数を返す。
func usersOf(fake *faketuner.Server) int {
	users := 0
	for _, tuner := range fake.Tuners() {
		users += len(tuner.Users)
	}

	return users
}

// TestLiveHandlerRelaysAndReleases は、スクランブルを解除したストリームを TS のまま中継し、
// クライアントが切断すると、上流の接続も閉じてチューナーを解放することを確かめる (設計書 7.2)。
func TestLiveHandlerRelaysAndReleases(t *testing.T) {
	t.Parallel()

	server, fake := liveServer(t, true)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/streams/services/"+serviceID, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "video/mp2t", resp.Header.Get("Content-Type"))
	packets := make([]byte, 100*ts.PacketSize)
	_, err = io.ReadFull(resp.Body, packets)
	require.NoError(t, err)
	for i := 0; i < len(packets); i += ts.PacketSize {
		require.Equal(t, byte(0x47), packets[i], "packet %d must start with the sync byte", i/ts.PacketSize)
	}
	assert.Equal(t, 1, usersOf(fake), "the stream holds a tuner")

	cancel()
	assert.Eventually(t, func() bool { return usersOf(fake) == 0 }, 5*time.Second, 10*time.Millisecond, "the tuner is released")
}

// TestLiveHandlerHead は、HEAD ではヘッダーだけを返し、チューナーを確保しないことを確かめる。
func TestLiveHandlerHead(t *testing.T) {
	t.Parallel()

	server, fake := liveServer(t, true)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodHead, server.URL+"/streams/services/"+serviceID, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "video/mp2t", resp.Header.Get("Content-Type"))
	assert.Never(t, func() bool { return usersOf(fake) > 0 }, 200*time.Millisecond, 10*time.Millisecond, "no tuner is acquired")
}

// TestLiveHandlerErrors は、ストリームを開けない理由に応じた状態を返すことを確かめる。
func TestLiveHandlerErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tuner      bool
		serviceID  string
		wantStatus int
		wantBody   string
	}{
		{name: "サービスがない", tuner: true, serviceID: "3273601099", wantStatus: http.StatusNotFound, wantBody: "service not found"},
		{name: "サービス ID が数でない", tuner: true, serviceID: "nhk", wantStatus: http.StatusBadRequest, wantBody: "invalid service ID"},
		{
			name: "チューナーサーバーを設定していない", tuner: false, serviceID: serviceID,
			wantStatus: http.StatusServiceUnavailable, wantBody: "tuner server is not configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, fake := liveServer(t, tt.tuner)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/streams/services/"+tt.serviceID, nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			assert.Equal(t, tt.wantBody, strings.TrimSpace(string(body)))
			assert.Zero(t, usersOf(fake))
		})
	}
}
