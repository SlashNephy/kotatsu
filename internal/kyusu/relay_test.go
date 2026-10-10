package kyusu_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/kyusu"
	"github.com/SlashNephy/kotatsu/internal/registry"
)

const (
	// hlsPattern は、ライブ視聴の HLS のパス。
	hlsPattern = "GET /streams/services/{id}/hls/{preset}/{file}"
	// kyusuRole は、kyusu のロールの名前。
	kyusuRole = "kyusu"
)

// fakeInstances は、決まった kyusu のインスタンスを返す。
type fakeInstances struct {
	instances []*registry.Instance
}

func (f *fakeInstances) ListAlive(_ context.Context, role string) ([]*registry.Instance, error) {
	if role != kyusuRole {
		return []*registry.Instance{}, nil
	}

	return f.instances, nil
}

var _ kyusu.Instances = new(fakeInstances)

// fakeKyusu は、トークンを確かめて、受け取ったパスを残し、パスに応じた応答を返す kyusu。
type fakeKyusu struct {
	mu    sync.Mutex
	paths []string
}

func (k *fakeKyusu) requests() []string {
	k.mu.Lock()
	defer k.mu.Unlock()

	return append([]string{}, k.paths...)
}

// newFakeKyusu は、fakeKyusu を立て、そのインスタンスを返す。
func newFakeKyusu(t *testing.T, token *kyusu.Token) (*fakeKyusu, *registry.Instance) {
	t.Helper()

	k := &fakeKyusu{mu: sync.Mutex{}, paths: []string{}}
	server := httptest.NewServer(token.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k.mu.Lock()
		k.paths = append(k.paths, r.URL.Path)
		k.mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/missing.ts") {
			http.Error(w, "the live session is not running", http.StatusNotFound)

			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, "#EXTM3U\n")
	})))
	t.Cleanup(server.Close)
	address, err := url.Parse(server.URL)
	require.NoError(t, err)

	return k, &registry.Instance{ID: uuid.New(), Role: kyusuRole, Address: address.Host}
}

func newHLSServer(t *testing.T, instances kyusu.Instances, token *kyusu.Token) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle(hlsPattern, kyusu.NewRelay(instances, token, slog.New(slog.DiscardHandler)))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

// relayed は、中継した応答。
type relayed struct {
	status       int
	contentType  string
	cacheControl string
	body         string
}

func fetch(t *testing.T, rawURL string) *relayed {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return &relayed{
		status: resp.StatusCode, contentType: resp.Header.Get("Content-Type"), cacheControl: resp.Header.Get("Cache-Control"), body: string(body),
	}
}

// TestRelayRelaysToKyusu は、ライブ視聴の HLS を、トークンを付けて kyusu に中継し、
// 同じサービスと画質は、インスタンスの並びによらず同じ kyusu に中継することを確かめる (設計書 7.3)。
func TestRelayRelaysToKyusu(t *testing.T) {
	t.Parallel()

	token, err := kyusu.NewToken(keyMaterial)
	require.NoError(t, err)
	first, firstInstance := newFakeKyusu(t, token)
	second, secondInstance := newFakeKyusu(t, token)
	forward := newHLSServer(t, &fakeInstances{instances: []*registry.Instance{firstInstance, secondInstance}}, token)
	backward := newHLSServer(t, &fakeInstances{instances: []*registry.Instance{secondInstance, firstInstance}}, token)

	got := fetch(t, forward.URL+"/streams/services/3273601024/hls/720p/playlist.m3u8")
	assert.Equal(t, &relayed{status: http.StatusOK, contentType: "application/vnd.apple.mpegurl", cacheControl: "no-store", body: "#EXTM3U\n"}, got)
	assert.Equal(t, http.StatusOK, fetch(t, backward.URL+"/streams/services/3273601024/hls/720p/segment00000.ts").status)

	// どちらの tenban からも、同じ kyusu に届く。
	assert.ElementsMatch(t, []string{
		"/internal/kyusu/live/3273601024/720p/playlist.m3u8", "/internal/kyusu/live/3273601024/720p/segment00000.ts",
	}, append(first.requests(), second.requests()...))
	assert.True(t, len(first.requests()) == 2 || len(second.requests()) == 2, "both requests reach the same kyusu")

	// サービスと画質の組が違えば、ほかの kyusu も使う。
	for _, preset := range kyusu.Presets() {
		for _, service := range []string{"3273601024", "3273601025", "3273601026"} {
			require.Equal(t, http.StatusOK, fetch(t, forward.URL+"/streams/services/"+service+"/hls/"+preset.Name+"/playlist.m3u8").status)
		}
	}
	assert.NotEmpty(t, first.requests())
	assert.NotEmpty(t, second.requests())
}

// TestRelayErrors は、kyusu がないときと、kyusu がエラーを返したときの状態を確かめる。
func TestRelayErrors(t *testing.T) {
	t.Parallel()

	token, err := kyusu.NewToken(keyMaterial)
	require.NoError(t, err)
	_, instance := newFakeKyusu(t, token)
	tests := []struct {
		name       string
		instances  []*registry.Instance
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name: "kyusu が動いていない", instances: []*registry.Instance{}, path: "/streams/services/3273601024/hls/720p/playlist.m3u8",
			wantStatus: http.StatusServiceUnavailable, wantBody: "server-side transcoding is not available; no kyusu instance is running",
		},
		{
			name: "kyusu のエラーをそのまま返す", instances: []*registry.Instance{instance}, path: "/streams/services/3273601024/hls/720p/missing.ts",
			wantStatus: http.StatusNotFound, wantBody: "the live session is not running",
		},
		{
			name: "届かない kyusu", path: "/streams/services/3273601024/hls/720p/playlist.m3u8",
			instances:  []*registry.Instance{{ID: uuid.New(), Role: kyusuRole, Address: "127.0.0.1:1"}},
			wantStatus: http.StatusBadGateway, wantBody: "failed to reach kyusu",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := newHLSServer(t, &fakeInstances{instances: tt.instances}, token)
			got := fetch(t, server.URL+tt.path)
			assert.Equal(t, tt.wantStatus, got.status)
			assert.Equal(t, tt.wantBody, strings.TrimSpace(got.body))
		})
	}
}

// TestRecordingRelay は、録画の仮想 HLS を、書き方をそろえた録画の ID で kyusu に中継し、録画の ID でないものは中継しないことを確かめる。
func TestRecordingRelay(t *testing.T) {
	t.Parallel()

	token, err := kyusu.NewToken(keyMaterial)
	require.NoError(t, err)
	recordingID := uuid.New().String()
	tests := []struct {
		name       string
		id         string
		wantStatus int
		wantPaths  []string
	}{
		{
			name: "録画の ID", id: recordingID, wantStatus: http.StatusOK,
			wantPaths: []string{"/internal/kyusu/recordings/" + recordingID + "/720p/playlist.m3u8"},
		},
		{
			name: "大文字の録画の ID", id: strings.ToUpper(recordingID), wantStatus: http.StatusOK,
			wantPaths: []string{"/internal/kyusu/recordings/" + recordingID + "/720p/playlist.m3u8"},
		},
		{name: "サービスの ID", id: "3273601024", wantStatus: http.StatusBadRequest, wantPaths: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend, instance := newFakeKyusu(t, token)
			mux := http.NewServeMux()
			mux.Handle("GET /streams/recordings/{id}/hls/{preset}/{file}",
				kyusu.NewRecordingRelay(&fakeInstances{instances: []*registry.Instance{instance}}, token, slog.New(slog.DiscardHandler)))
			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)

			assert.Equal(t, tt.wantStatus, fetch(t, server.URL+"/streams/recordings/"+tt.id+"/hls/720p/playlist.m3u8").status)
			assert.Equal(t, tt.wantPaths, backend.requests())
		})
	}
}
