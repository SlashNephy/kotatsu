package mirakurun_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
)

// stallingStream は、chunks 回だけ interval ごとに 188 バイトを送り、その後は接続を保ったまま何も送らない。
func stallingStream(t *testing.T, chunks int, interval time.Duration) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/MP2T")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		// 応答の頭は、すぐに返す。
		flusher.Flush()
		packet := make([]byte, 188)
		packet[0] = 0x47
		for range chunks {
			_, _ = w.Write(packet)
			flusher.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(interval):
			}
		}
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	return server
}

func TestClientOpenServiceStreamIdle(t *testing.T) {
	t.Parallel()

	const idle = 200 * time.Millisecond
	tests := []struct {
		name     string
		chunks   int
		interval time.Duration
		// wantBytes は、止まるまでに読めるバイト数。
		wantBytes int
	}{
		{name: "stalls right after the response", chunks: 0, interval: 0, wantBytes: 0},
		// 上限より短い間隔で送り続ける間は閉じない。送り終えてから上限が過ぎたら閉じる。
		{name: "stalls after sending for longer than the timeout", chunks: 5, interval: idle / 2, wantBytes: 5 * 188},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := stallingStream(t, tt.chunks, tt.interval)
			client, err := mirakurun.New(&mirakurun.Options{URL: server.URL, StreamIdleTimeout: idle})
			require.NoError(t, err)
			// 見張りは、ストリームを開いたときに始まるので、その前から測る。
			started := time.Now()
			stream, err := client.OpenServiceStream(t.Context(), 3273601024, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = stream.Close() })

			data, err := io.ReadAll(stream)

			require.ErrorIs(t, err, mirakurun.ErrStreamStalled)
			assert.Len(t, data, tt.wantBytes)
			// 最後にデータが届いてから、上限の分だけ待って閉じる。最後のデータは、(chunks - 1) 回の間隔の後に届く。
			lastData := time.Duration(max(tt.chunks-1, 0)) * tt.interval
			assert.GreaterOrEqual(t, time.Since(started), lastData+idle)
		})
	}
}

func TestClientOpenServiceStreamEnds(t *testing.T) {
	t.Parallel()

	// チューナーサーバーがストリームを閉じたときは、止まったとはみなさない。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 188))
	}))
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL, StreamIdleTimeout: 200 * time.Millisecond})
	require.NoError(t, err)
	stream, err := client.OpenServiceStream(t.Context(), 3273601024, nil)
	require.NoError(t, err)

	data, err := io.ReadAll(stream)

	require.NoError(t, err)
	assert.Len(t, data, 188)
	// 閉じるのが上限より遅れても、読み終えたストリームを、止まったことにしない。
	time.Sleep(300 * time.Millisecond)
	n, err := stream.Read(make([]byte, 188))
	assert.Zero(t, n)
	require.ErrorIs(t, err, io.EOF)
	require.NoError(t, stream.Close())
}

func TestClientOpenServiceStreamOpenTimeout(t *testing.T) {
	t.Parallel()

	const openTimeout = 200 * time.Millisecond
	tests := []struct {
		name string
		// delay は、チューナーサーバーが応答の頭を返すまでの時間。
		delay   time.Duration
		wantErr error
	}{
		{name: "responds in time", delay: 0, wantErr: nil},
		// チューナーの空きを待つなどして、応答しないまま上限が過ぎた。
		{name: "does not respond", delay: time.Hour, wantErr: mirakurun.ErrStreamOpenTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(tt.delay):
				}
				flusher, ok := w.(http.Flusher)
				if !ok {
					return
				}
				w.WriteHeader(http.StatusOK)
				flusher.Flush()
				// 応答した後は、上限より長くデータを送り続ける。開く時間の上限は、ストリームを止めない。
				packet := make([]byte, 188)
				for range 6 {
					_, _ = w.Write(packet)
					flusher.Flush()
					time.Sleep(openTimeout / 2)
				}
			}))
			t.Cleanup(server.Close)
			client, err := mirakurun.New(&mirakurun.Options{URL: server.URL, StreamOpenTimeout: openTimeout})
			require.NoError(t, err)

			started := time.Now()
			stream, err := client.OpenServiceStream(t.Context(), 3273601024, nil)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Less(t, time.Since(started), 5*time.Second)

				return
			}
			require.NoError(t, err)
			data, err := io.ReadAll(stream)
			require.NoError(t, err)
			assert.Len(t, data, 6*188)
			require.NoError(t, stream.Close())
		})
	}
}

// quietEventsServer は、イベントストリームを開いた後、quiet の間何も送らずに、イベントを 1 つ送るチューナーサーバー。
func quietEventsServer(t *testing.T, kind mirakurun.ServerKind, quiet time.Duration) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		wait := func() bool {
			select {
			case <-r.Context().Done():
				return false
			case <-time.After(quiet):
				return true
			}
		}
		switch {
		case r.URL.Path == pathVersion:
			_, _ = w.Write([]byte(`{"current":"4.0.0","latest":"4.0.0"}`))
		case r.URL.Path == pathSSEEvents && kind == mirakurun.ServerMirakc:
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher.Flush()
			if wait() {
				_, _ = w.Write([]byte("event: tuner.status-changed\ndata: {\"tunerIndex\":0}\n\n"))
				flusher.Flush()
			}
			<-r.Context().Done()
		case r.URL.Path == "/api/events/stream" && kind != mirakurun.ServerMirakc:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte("[\n"))
			flusher.Flush()
			if wait() {
				_, _ = w.Write([]byte("{\"resource\":\"tuner\",\"type\":\"update\",\"data\":{},\"time\":0}\n,\n"))
				flusher.Flush()
			}
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func TestClientOpenEventsQuiet(t *testing.T) {
	t.Parallel()

	// イベントストリームは、変化がなければ何も送らないので、録画のストリームの上限より長く黙っても閉じない (#954)。
	const idle = 200 * time.Millisecond
	tests := []struct {
		kind     mirakurun.ServerKind
		wantType string
	}{
		{kind: mirakurun.ServerMirakurun, wantType: typeUpdate},
		{kind: mirakurun.ServerMirakc, wantType: typeStatusChanged},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			t.Parallel()

			server := quietEventsServer(t, tt.kind, 3*idle)
			client, err := mirakurun.New(&mirakurun.Options{URL: server.URL, StreamIdleTimeout: idle})
			require.NoError(t, err)
			stream, err := client.OpenEvents(t.Context(), nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = stream.Close() })

			event, err := stream.Next()

			require.NoError(t, err)
			assert.Equal(t, tt.wantType, event.Type)
		})
	}
}
