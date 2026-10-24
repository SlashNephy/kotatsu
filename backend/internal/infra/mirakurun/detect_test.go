package mirakurun_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
)

// stubServer は、種類を途中で切り替えられるチューナーサーバーのスタブ。
// /api/version、/events、/api/onair だけを、各サーバーと同じように返す。
type stubServer struct {
	kind atomic.Value
}

func newStubServer(t *testing.T, kind mirakurun.ServerKind) (*stubServer, *httptest.Server) {
	t.Helper()

	stub := new(stubServer)
	stub.kind.Store(kind)
	server := httptest.NewServer(stub)
	t.Cleanup(server.Close)

	return stub, server
}

func (s *stubServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	kind, _ := s.kind.Load().(mirakurun.ServerKind)
	switch {
	case r.URL.Path == pathVersion && kind == mirakurun.ServerMahiron:
		_, _ = w.Write([]byte(`{"current":"5.2.2","latest":"","server":"mahiron"}`))
	case r.URL.Path == pathVersion:
		_, _ = w.Write([]byte(`{"current":"4.0.0","latest":"4.0.0"}`))
	case r.URL.Path == pathSSEEvents && kind == mirakurun.ServerMirakc:
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: tuner.status-changed\ndata: {\"tunerIndex\":0}\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	case r.URL.Path == "/api/onair" && kind == mirakurun.ServerMirakc:
		// 延長で終了時刻が未定になった番組と、次の番組がない状態。形は mirakc の OpenAPI の WebOnairProgram に従う。
		_, _ = w.Write([]byte(`[{"serviceId":3273601024,"current":{"id":327360102401405,"eventId":1405,"serviceId":1024,` +
			`"networkId":32736,"startAt":1791702600000,"duration":1,"isFree":true,"name":"中継"},"next":null}]`))
	case r.URL.Path == pathSSEEvents && kind == "spa":
		// SPA のフォールバックで、知らないパスにも HTML を返すサーバー。
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html>"))
	case r.URL.Path == "/api/events/stream":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte("[\n{\"resource\":\"tuner\",\"type\":\"update\",\"data\":{},\"time\":0}\n,\n"))
	default:
		http.NotFound(w, r)
	}
}

func stubMirakurunVersion() *mirakurun.Version {
	return &mirakurun.Version{Current: "4.0.0", Latest: "4.0.0", Server: ""}
}

func TestDetect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind mirakurun.ServerKind
		want *mirakurun.ServerInfo
	}{
		{
			kind: mirakurun.ServerMirakurun,
			want: &mirakurun.ServerInfo{
				Kind:         mirakurun.ServerMirakurun,
				Version:      stubMirakurunVersion(),
				EventsStream: true,
			},
		},
		{
			kind: mirakurun.ServerMirakc,
			want: &mirakurun.ServerInfo{
				Kind:      mirakurun.ServerMirakc,
				Version:   stubMirakurunVersion(),
				SSEEvents: true,
				OnAir:     true,
			},
		},
		{
			kind: mirakurun.ServerMahiron,
			want: &mirakurun.ServerInfo{
				Kind:         mirakurun.ServerMahiron,
				Version:      &mirakurun.Version{Current: "5.2.2", Latest: "", Server: "mahiron"},
				EventsStream: true,
			},
		},
		{
			kind: "spa",
			want: &mirakurun.ServerInfo{
				Kind:         mirakurun.ServerMirakurun,
				Version:      stubMirakurunVersion(),
				EventsStream: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			t.Parallel()

			_, server := newStubServer(t, tt.kind)
			client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
			require.NoError(t, err)

			got, err := client.Detect(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestServerRedetects(t *testing.T) {
	t.Parallel()

	stub, server := newStubServer(t, mirakurun.ServerMirakurun)
	clk := clocktest.NewFake(testNow())
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL, DetectInterval: 10 * time.Minute, Clock: clk})
	require.NoError(t, err)

	info, err := client.Server(t.Context())
	require.NoError(t, err)
	assert.Equal(t, mirakurun.ServerMirakurun, info.Kind)

	// サーバーが mirakc に入れ替わっても、間隔が過ぎるまでは前の結果を使う。
	stub.kind.Store(mirakurun.ServerMirakc)
	clk.Advance(9 * time.Minute)
	info, err = client.Server(t.Context())
	require.NoError(t, err)
	assert.Equal(t, mirakurun.ServerMirakurun, info.Kind)

	// 間隔が過ぎたら判定し直す。
	clk.Advance(time.Minute)
	info, err = client.Server(t.Context())
	require.NoError(t, err)
	assert.Equal(t, mirakurun.ServerMirakc, info.Kind)

	// Invalidate したら、間隔を待たずに判定し直す。
	stub.kind.Store(mirakurun.ServerMahiron)
	client.Invalidate()
	info, err = client.Server(t.Context())
	require.NoError(t, err)
	assert.Equal(t, mirakurun.ServerMahiron, info.Kind)
}

func TestOpenEventsByServerKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind         mirakurun.ServerKind
		wantResource string
		wantType     string
	}{
		{kind: mirakurun.ServerMirakurun, wantResource: resourceTuner, wantType: typeUpdate},
		{kind: mirakurun.ServerMahiron, wantResource: resourceTuner, wantType: typeUpdate},
		{kind: mirakurun.ServerMirakc, wantResource: resourceTuner, wantType: typeStatusChanged},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			t.Parallel()

			_, server := newStubServer(t, tt.kind)
			client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
			require.NoError(t, err)

			stream, err := client.OpenEvents(t.Context(), nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = stream.Close() })

			event, err := stream.Next()
			require.NoError(t, err)
			assert.Equal(t, tt.wantResource, event.Resource)
			assert.Equal(t, tt.wantType, event.Type)
		})
	}
}

func TestOpenEventsInvalidatesOnFailure(t *testing.T) {
	t.Parallel()

	stub, server := newStubServer(t, mirakurun.ServerMirakc)
	clk := clocktest.NewFake(testNow())
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL, Clock: clk})
	require.NoError(t, err)

	info, err := client.Server(t.Context())
	require.NoError(t, err)
	require.Equal(t, mirakurun.ServerMirakc, info.Kind)

	// mirakc から Mirakurun に入れ替わると /events が 404 になり、開くのに失敗する。
	stub.kind.Store(mirakurun.ServerMirakurun)
	_, err = client.OpenEvents(t.Context(), nil)
	require.True(t, mirakurun.IsStatus(err, http.StatusNotFound))

	// 失敗したら判定の結果を捨てるので、時計を進めなくても次は Mirakurun として開ける。
	stream, err := client.OpenEvents(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stream.Close() })
	event, err := stream.Next()
	require.NoError(t, err)
	assert.Equal(t, typeUpdate, event.Type)
}
