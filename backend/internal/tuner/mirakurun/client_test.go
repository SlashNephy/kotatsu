package mirakurun_test

import (
	"bytes"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

// testNow は、テストで使う現在時刻。
func testNow() time.Time {
	return time.Date(2026, 10, 9, 21, 10, 0, 0, time.UTC)
}

const (
	// grServiceID は、偽チューナーの GR 27 のサービス (networkId 0x7FE0、serviceId 0x0400) の ID。
	grServiceID = int64(32736_01024)

	resourceProgram   = "program"
	resourceTuner     = "tuner"
	typeUpdate        = "update"
	typeStatusChanged = "status-changed"

	pathVersion = "/api/version"
	// pathSSEEvents は、mirakc のイベントストリーム (SSE)。
	pathSSEEvents = "/events"
)

// recorder は、偽チューナーへのリクエストを記録する。
type recorder struct {
	handler http.Handler

	mu       sync.Mutex
	requests []*http.Request
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.requests = append(r.requests, req.Clone(req.Context()))
	r.mu.Unlock()
	r.handler.ServeHTTP(w, req)
}

func (r *recorder) last(path string) *http.Request {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, req := range slices.Backward(r.requests) {
		if req.URL.Path == path {
			return req
		}
	}

	return nil
}

func newFakeTuner(t *testing.T) (*faketuner.Server, *recorder, *mirakurun.Client) {
	t.Helper()

	fake := faketuner.New(faketuner.DefaultOptions(testNow(), nil))
	rec := &recorder{handler: fake}
	server := httptest.NewServer(rec)
	t.Cleanup(server.Close)

	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)

	return fake, rec, client
}

func TestClientGetters(t *testing.T) {
	t.Parallel()

	_, _, client := newFakeTuner(t)
	ctx := t.Context()

	t.Run("version", func(t *testing.T) {
		t.Parallel()

		version, err := client.Version(ctx)
		require.NoError(t, err)
		assert.Equal(t, "3.9.0-faketuner", version.Current)
		assert.Empty(t, version.Server)
	})

	t.Run("channels", func(t *testing.T) {
		t.Parallel()

		channels, err := client.Channels(ctx)
		require.NoError(t, err)
		require.Len(t, channels, 4)
		assert.Equal(t, "GR", channels[0].Type)
		assert.Equal(t, "27", channels[0].Channel)
		assert.Len(t, channels[0].Services, 2)
		assert.Equal(t, "BS", channels[3].Type)
	})

	t.Run("services", func(t *testing.T) {
		t.Parallel()

		services, err := client.Services(ctx)
		require.NoError(t, err)
		require.Len(t, services, 5)
		assert.Equal(t, grServiceID, services[0].ID)
		assert.Equal(t, &mirakurun.ChannelRef{Type: "GR", Channel: "27"}, services[0].Channel)
	})

	t.Run("tuners", func(t *testing.T) {
		t.Parallel()

		tuners, err := client.Tuners(ctx)
		require.NoError(t, err)
		require.Len(t, tuners, 3)
		assert.Equal(t, []string{"BS", "CS"}, tuners[2].Types)
	})
}

func TestClientPrograms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		query     *mirakurun.ProgramsQuery
		wantQuery url.Values
		wantCount int
	}{
		{
			name:      "all",
			query:     nil,
			wantQuery: url.Values{},
			wantCount: 5 * 14,
		},
		{
			name:      "service",
			query:     &mirakurun.ProgramsQuery{NetworkID: 0x7FE0, ServiceID: 0x0400},
			wantQuery: url.Values{"networkId": {"32736"}, "serviceId": {"1024"}},
			wantCount: 14,
		},
		{
			name:      "event",
			query:     &mirakurun.ProgramsQuery{NetworkID: 0x7FE0, ServiceID: 0x0400, EventID: 3},
			wantQuery: url.Values{"networkId": {"32736"}, "serviceId": {"1024"}, "eventId": {"3"}},
			wantCount: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, rec, client := newFakeTuner(t)
			programs, err := client.Programs(t.Context(), tt.query)
			require.NoError(t, err)
			assert.Len(t, programs, tt.wantCount)
			assert.Equal(t, tt.wantQuery, rec.last("/api/programs").URL.Query())
			for _, p := range programs {
				assert.Equal(t, faketuner.ProgramID(p.NetworkID, p.ServiceID, p.EventID), p.ID)
			}
		})
	}
}

func TestClientOpenServiceStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		opts         *mirakurun.StreamOptions
		wantDecode   string
		wantPriority string
	}{
		{name: "decode", opts: &mirakurun.StreamOptions{Decode: true, Priority: 2}, wantDecode: "1", wantPriority: "2"},
		{name: "raw", opts: &mirakurun.StreamOptions{Decode: false, Priority: -1}, wantDecode: "", wantPriority: "-1"},
		{name: "default", opts: nil, wantDecode: "", wantPriority: "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake, rec, client := newFakeTuner(t)
			stream, err := client.OpenServiceStream(t.Context(), grServiceID, tt.opts)
			require.NoError(t, err)

			packet := make([]byte, 188)
			_, err = io.ReadFull(stream, packet)
			require.NoError(t, err)
			assert.Equal(t, byte(0x47), packet[0], "TS packets start with the sync byte")

			req := rec.last("/api/services/3273601024/stream")
			require.NotNil(t, req)
			assert.Equal(t, tt.wantDecode, req.URL.Query().Get("decode"))
			assert.Equal(t, tt.wantPriority, req.Header.Get(mirakurun.PriorityHeader))
			assert.Equal(t, "kotatsu", req.UserAgent())

			users := fake.Tuners()[0].Users
			require.Len(t, users, 1)
			assert.Equal(t, tt.wantPriority, strconv.Itoa(users[0].Priority))

			require.NoError(t, stream.Close())
			// 閉じたら、チューナーが解放される。
			assert.Eventually(t, func() bool { return len(fake.Tuners()[0].Users) == 0 }, 5*time.Second, 10*time.Millisecond)
		})
	}
}

func TestClientOpenServiceStreamErrors(t *testing.T) {
	t.Parallel()

	// 地上波のチューナーを 1 本だけにして、チューナーが足りない状況を作る。
	opts := faketuner.DefaultOptions(testNow(), nil)
	opts.Tuners = []*faketuner.TunerConfig{{Name: "GR0", Types: []string{"GR"}}}
	server := httptest.NewServer(faketuner.New(opts))
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)

	occupied, err := client.OpenServiceStream(t.Context(), grServiceID, &mirakurun.StreamOptions{Decode: true, Priority: 1})
	require.NoError(t, err)
	t.Cleanup(func() { _ = occupied.Close() })

	tests := []struct {
		name       string
		serviceID  int64
		wantStatus int
	}{
		{name: "unknown service", serviceID: 1, wantStatus: http.StatusNotFound},
		// GR 26 のサービス。唯一のチューナーを、優先度の高い利用者が GR 27 で使っている。
		{name: "no available tuner", serviceID: faketuner.ServiceItemID(0x7FE1, 0x0408), wantStatus: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := client.OpenServiceStream(t.Context(), tt.serviceID, &mirakurun.StreamOptions{Decode: true, Priority: 0})
			var apiErr *mirakurun.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tt.wantStatus, apiErr.StatusCode)
			assert.NotEmpty(t, apiErr.Reason)
			assert.True(t, mirakurun.IsStatus(err, tt.wantStatus))
		})
	}
}

func TestClientServiceLogo(t *testing.T) {
	t.Parallel()

	_, rec, client := newFakeTuner(t)

	tests := []struct {
		name       string
		serviceID  int64
		wantStatus int
	}{
		{name: "logo", serviceID: grServiceID, wantStatus: http.StatusOK},
		// GR 26 のサービスは、ロゴの番号はあるが、画像をまだ受信していない。
		{name: "logo not received", serviceID: faketuner.ServiceItemID(0x7FE1, 0x0408), wantStatus: http.StatusServiceUnavailable},
		{name: "unknown service", serviceID: 1, wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logo, err := client.ServiceLogo(t.Context(), tt.serviceID)
			if tt.wantStatus != http.StatusOK {
				assert.True(t, mirakurun.IsStatus(err, tt.wantStatus), "error: %v", err)
				assert.Nil(t, logo)

				return
			}
			require.NoError(t, err)
			config, err := png.DecodeConfig(bytes.NewReader(logo))
			require.NoError(t, err)
			assert.Equal(t, 64, config.Width)
			assert.Equal(t, 36, config.Height)
			req := rec.last("/api/services/3273601024/logo")
			require.NotNil(t, req)
			assert.Equal(t, "kotatsu", req.UserAgent())
		})
	}
}

func TestClientBasicAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		url      func(base string) string
		username string
		password string
	}{
		{
			name:     "options",
			url:      func(base string) string { return base },
			username: "user",
			password: "secret",
		},
		{
			name:     "URL",
			url:      func(base string) string { return "http://user:secret@" + base[len("http://"):] },
			username: "",
			password: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if u, p, ok := r.BasicAuth(); !ok || u != "user" || p != "secret" {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"code":401,"reason":"unauthorized"}`))

					return
				}
				_, _ = w.Write([]byte(`{"current":"3.9.0","latest":"3.9.0"}`))
			}))
			t.Cleanup(server.Close)

			client, err := mirakurun.New(&mirakurun.Options{URL: tt.url(server.URL), Username: tt.username, Password: config.Secret(tt.password)})
			require.NoError(t, err)
			version, err := client.Version(t.Context())
			require.NoError(t, err)
			assert.Equal(t, "3.9.0", version.Current)
		})
	}
}

func TestClientAPIErrorHidesCredentials(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	t.Cleanup(server.Close)

	client, err := mirakurun.New(&mirakurun.Options{URL: "http://user:secret@" + server.URL[len("http://"):]})
	require.NoError(t, err)
	_, err = client.Channels(t.Context())
	require.Error(t, err)
	assert.True(t, mirakurun.IsStatus(err, http.StatusInternalServerError))
	assert.Contains(t, err.Error(), "boom")
	assert.NotContains(t, err.Error(), "secret")
}
