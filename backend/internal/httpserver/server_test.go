package httpserver_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/httpserver"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
)

type fakePinger struct {
	err error
}

var _ httpserver.Pinger = new(fakePinger)

func (p *fakePinger) Ping(context.Context) error {
	return p.err
}

func TestHealthEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		pingErr    error
		wantStatus int
	}{
		{name: "healthz", path: "/healthz", wantStatus: http.StatusOK},
		{name: "healthz ignores the database", path: "/healthz", pingErr: errors.New("down"), wantStatus: http.StatusOK},
		{name: "readyz", path: "/readyz", wantStatus: http.StatusOK},
		{name: "readyz without the database", path: "/readyz", pingErr: errors.New("down"), wantStatus: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mux := http.NewServeMux()
			httpserver.RegisterHealth(mux, slog.New(slog.DiscardHandler), &fakePinger{err: tt.pingErr})

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil))
			assert.Equal(t, tt.wantStatus, rec.Code)
			// 認証なしで公開するので、成否のほかは返さない。
			assert.Equal(t, http.StatusText(tt.wantStatus)+"\n", rec.Body.String())
		})
	}
}

func TestReadyzWithRealDatabase(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)

	mux := http.NewServeMux()
	httpserver.RegisterHealth(mux, slog.New(slog.DiscardHandler), pool)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	pool.Close()
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestServeWaitsForInFlightRequests(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		_, _ = io.WriteString(w, "done")
	})

	ln, err := httpserver.Listen(t.Context(), "127.0.0.1:0")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	served := make(chan error, 1)
	go func() {
		served <- httpserver.Serve(ctx, slog.New(slog.DiscardHandler), ln, handler)
	}()

	type response struct {
		body string
		err  error
	}
	responded := make(chan response, 1)
	go func() {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String()+"/", nil)
		if err != nil {
			responded <- response{err: err}

			return
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			responded <- response{err: err}

			return
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		responded <- response{body: string(body), err: err}
	}()

	<-entered
	// 終了のシグナルを受けた状態にする。
	cancel()

	select {
	case err := <-served:
		t.Fatalf("server stopped before the in-flight request finished: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	res := <-responded
	require.NoError(t, res.err)
	assert.Equal(t, "done", res.body)
	require.NoError(t, <-served)
}

// TestServeStopsStreamsOnShutdown は、停止を始めたら、StopOnShutdown で包んだ終わらないストリームを打ち切り、
// ShutdownTimeout まで待たずに止まることを確かめる。
func TestServeStopsStreamsOnShutdown(t *testing.T) {
	t.Parallel()

	streaming := make(chan struct{})
	handler := httpserver.StopOnShutdown(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		ticker := time.NewTicker(10 * time.Millisecond)
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
	}))

	ln, err := httpserver.Listen(t.Context(), "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	served := make(chan error, 1)
	go func() {
		served <- httpserver.Serve(ctx, slog.New(slog.DiscardHandler), ln, handler)
	}()

	go func() {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String()+"/", nil)
		if err != nil {
			return
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)
	}()

	<-streaming
	cancel()
	select {
	case err := <-served:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the server kept waiting for the stream")
	}
}

func TestListenFailsWhenAddressIsInUse(t *testing.T) {
	t.Parallel()

	ln, err := httpserver.Listen(t.Context(), "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	_, err = httpserver.Listen(t.Context(), ln.Addr().String())
	require.ErrorContains(t, err, "failed to listen on "+ln.Addr().String())
}
