package httpserver_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

// TestStopWhenRevoked は、配信している間に allowed が false を返したら、流しているストリームを止め、
// true を返し続ける間は止めないことを確かめる (#1755)。
func TestStopWhenRevoked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// revoke は、流し始めた後に、使えなくするかどうか。
		revoke   bool
		wantStop bool
	}{
		{name: "使えなくしたら止める", revoke: true, wantStop: true},
		{name: "使えるうちは流し続ける", revoke: false, wantStop: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var allowed atomic.Bool
			allowed.Store(true)
			streaming := make(chan struct{})
			stopped := make(chan struct{})
			stream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(stopped)
				flusher, _ := w.(http.Flusher)
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
				once := false
				for {
					select {
					case <-r.Context().Done():
						return
					case <-ticker.C:
					}
					if _, err := io.WriteString(w, "chunk"); err != nil {
						return
					}
					flusher.Flush()
					if !once {
						once = true
						close(streaming)
					}
				}
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				httpserver.StopWhenRevoked(w, r, stream, 10*time.Millisecond, func(context.Context) bool {
					return allowed.Load()
				})
			}))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			require.NoError(t, err)
			// 流し続けるので、要求して読み捨てるのは別の goroutine にする。
			client := httpclienttest.New(t)
			go func() {
				res, err := client.Do(req)
				if err != nil {
					return
				}
				defer res.Body.Close()
				_, _ = io.Copy(io.Discard, res.Body)
			}()

			<-streaming
			if tt.revoke {
				allowed.Store(false)
			}
			select {
			case <-stopped:
				assert.True(t, tt.wantStop, "the stream stopped while it was still allowed")
			case <-time.After(500 * time.Millisecond):
				assert.False(t, tt.wantStop, "the stream kept flowing after it was revoked")
			}
		})
	}
}
