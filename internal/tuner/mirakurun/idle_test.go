package mirakurun_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
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
