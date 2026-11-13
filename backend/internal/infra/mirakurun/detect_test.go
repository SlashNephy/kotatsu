package mirakurun_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
)

// stubServer は、種類を途中で切り替えられるチューナーサーバーのスタブ。
// /api/version、/events、/api/onair だけを、各サーバーと同じように返す。
type stubServer struct {
	kind atomic.Value
}

// stubVersion は、スタブが Mirakurun と mirakc として名乗る版。
const stubVersion = "4.0.0"

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
	case r.URL.Path == pathVersion && kind == mirakurun.ServerKindMahiron:
		_, _ = w.Write([]byte(`{"current":"5.2.2","latest":"","server":"mahiron"}`))
	case r.URL.Path == pathVersion:
		_, _ = w.Write([]byte(`{"current":"4.0.0","latest":"4.0.0"}`))
	case r.URL.Path == pathSSEEvents && kind == mirakurun.ServerKindMirakc:
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: tuner.status-changed\ndata: {\"tunerIndex\":0}\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	case r.URL.Path == "/api/onair" && kind == mirakurun.ServerKindMirakc:
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
	return &mirakurun.Version{Current: stubVersion, Latest: stubVersion, Server: ""}
}

func TestDetect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind mirakurun.ServerKind
		want *mirakurun.ServerInfo
	}{
		{
			kind: mirakurun.ServerKindMirakurun,
			want: &mirakurun.ServerInfo{
				Kind:         mirakurun.ServerKindMirakurun,
				Version:      stubMirakurunVersion(),
				EventsStream: true,
			},
		},
		{
			kind: mirakurun.ServerKindMirakc,
			want: &mirakurun.ServerInfo{
				Kind:      mirakurun.ServerKindMirakc,
				Version:   stubMirakurunVersion(),
				SSEEvents: true,
				OnAir:     true,
			},
		},
		{
			kind: mirakurun.ServerKindMahiron,
			want: &mirakurun.ServerInfo{
				Kind:         mirakurun.ServerKindMahiron,
				Version:      &mirakurun.Version{Current: "5.2.2", Latest: "", Server: "mahiron"},
				EventsStream: true,
			},
		},
		{
			kind: "spa",
			want: &mirakurun.ServerInfo{
				Kind:         mirakurun.ServerKindMirakurun,
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

	stub, server := newStubServer(t, mirakurun.ServerKindMirakurun)
	clk := clocktest.NewFake(testNow())
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL, DetectInterval: 10 * time.Minute, Clock: clk})
	require.NoError(t, err)

	info, err := client.Server(t.Context())
	require.NoError(t, err)
	assert.Equal(t, mirakurun.ServerKindMirakurun, info.Kind)

	// サーバーが mirakc に入れ替わっても、間隔が過ぎるまでは前の結果を使う。
	stub.kind.Store(mirakurun.ServerKindMirakc)
	clk.Advance(9 * time.Minute)
	info, err = client.Server(t.Context())
	require.NoError(t, err)
	assert.Equal(t, mirakurun.ServerKindMirakurun, info.Kind)

	// 間隔が過ぎたら判定し直す。
	clk.Advance(time.Minute)
	info, err = client.Server(t.Context())
	require.NoError(t, err)
	assert.Equal(t, mirakurun.ServerKindMirakc, info.Kind)

	// Invalidate したら、間隔を待たずに判定し直す。
	stub.kind.Store(mirakurun.ServerKindMahiron)
	client.Invalidate()
	info, err = client.Server(t.Context())
	require.NoError(t, err)
	assert.Equal(t, mirakurun.ServerKindMahiron, info.Kind)
}

func TestOpenEventsByServerKind(t *testing.T) {
	t.Parallel()

	// Mirakurun と Mahiron の tuner.update も、mirakc の tuner.status-changed も、チューナーが変わったことと読む。
	tests := []struct {
		kind mirakurun.ServerKind
	}{
		{kind: mirakurun.ServerKindMirakurun},
		{kind: mirakurun.ServerKindMahiron},
		{kind: mirakurun.ServerKindMirakc},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			t.Parallel()

			_, server := newStubServer(t, tt.kind)
			client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
			require.NoError(t, err)

			stream, err := client.OpenEvents(t.Context())
			require.NoError(t, err)
			t.Cleanup(func() { _ = stream.Close() })

			event, err := stream.Next()
			require.NoError(t, err)
			assert.Equal(t, tunerserver.EventTypeTunerChanged, event.Type)
		})
	}
}

func TestOpenEventsInvalidatesOnFailure(t *testing.T) {
	t.Parallel()

	stub, server := newStubServer(t, mirakurun.ServerKindMirakc)
	clk := clocktest.NewFake(testNow())
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL, Clock: clk})
	require.NoError(t, err)

	info, err := client.Server(t.Context())
	require.NoError(t, err)
	require.Equal(t, mirakurun.ServerKindMirakc, info.Kind)

	// mirakc から Mirakurun に入れ替わると /events が 404 になり、開くのに失敗する。
	stub.kind.Store(mirakurun.ServerKindMirakurun)
	_, err = client.OpenEvents(t.Context())
	var apiErr *mirakurun.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusNotFound, apiErr.StatusCode)

	// 失敗したら判定の結果を捨てるので、時計を進めなくても次は Mirakurun として開ける。
	stream, err := client.OpenEvents(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = stream.Close() })
	event, err := stream.Next()
	require.NoError(t, err)
	assert.Equal(t, tunerserver.EventTypeTunerChanged, event.Type)
}

// TestIdentify は、判定した種類と名乗った版を、種類に依らない型で返すことを確かめる (#1538)。
func TestIdentify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind mirakurun.ServerKind
		want *tunerserver.Identity
	}{
		{kind: mirakurun.ServerKindMirakurun, want: &tunerserver.Identity{Kind: tunerserver.KindMirakurun, Version: stubVersion}},
		{kind: mirakurun.ServerKindMirakc, want: &tunerserver.Identity{Kind: tunerserver.KindMirakc, Version: stubVersion}},
		{kind: mirakurun.ServerKindMahiron, want: &tunerserver.Identity{Kind: tunerserver.KindMahiron, Version: "5.2.2"}},
	}
	for _, tt := range tests {
		t.Run(tt.kind.String(), func(t *testing.T) {
			t.Parallel()

			_, server := newStubServer(t, tt.kind)
			client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
			require.NoError(t, err)

			got, err := client.Identify(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
