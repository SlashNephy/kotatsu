package epgstationapi_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/web/epgstationapi"
)

// apiGate は、管理者が EPGStation 互換の API を有効にしているかを、テストの途中で変えられる。
type apiGate struct {
	disabled atomic.Bool
	err      error
}

func (g *apiGate) EPGStationAPIEnabled(context.Context) (bool, error) {
	return !g.disabled.Load(), g.err
}

// tokenAuth は、accepted だけを、取り消されるまで受け付ける。
type tokenAuth struct {
	revoked atomic.Bool
}

// accepted は、受け付ける API トークン。
const accepted = "kotatsu_valid"

func (a *tokenAuth) Authenticate(_ context.Context, token string) (*auth.Principal, error) {
	if token != accepted || a.revoked.Load() {
		return nil, auth.ErrUnauthenticated
	}

	return &auth.Principal{UserID: uuid.New(), Role: domain.RoleMember, SessionID: uuid.Nil(), AuthenticatedAt: time.Time{}, OAuthClientID: ""}, nil
}

// TestRequireEnabled は、管理者が EPGStation 互換の API を有効にしている間だけ答え、無効なら 404 を返すことを確かめる (#1769)。
func TestRequireEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		disabled   bool
		err        error
		wantStatus int
	}{
		{name: "有効", wantStatus: http.StatusNoContent},
		{name: "無効なら、API があることを知らせない", disabled: true, wantStatus: http.StatusNotFound},
		{name: "DB に届かない", err: fmt.Errorf("lookup: %w", settings.ErrDatabaseUnavailable), wantStatus: http.StatusServiceUnavailable},
		{name: "そのほかの誤り", err: errors.New("broken"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gate := &apiGate{err: tt.err}
			gate.disabled.Store(tt.disabled)
			handler := epgstationapi.RequireEnabled(gate, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/epgstation/api/version", nil))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestStopWhenRevoked は、ライブの TS を流している間に、管理者が API を無効にしたか、トークンを取り消したら、
// 流しているストリームも止め、使えるうちは止めないことを確かめる (#1769)。
func TestStopWhenRevoked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		revoke   func(gate *apiGate, tokens *tokenAuth)
		wantStop bool
	}{
		{name: "管理者が無効にした", revoke: func(gate *apiGate, _ *tokenAuth) { gate.disabled.Store(true) }, wantStop: true},
		{name: "トークンを取り消した", revoke: func(_ *apiGate, tokens *tokenAuth) { tokens.revoked.Store(true) }, wantStop: true},
		{name: "使えるうちは流し続ける", revoke: func(*apiGate, *tokenAuth) {}, wantStop: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gate, tokens := new(apiGate), new(tokenAuth)
			streaming, stopped := make(chan struct{}), make(chan struct{})
			flow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(stopped)
				flusher, _ := w.(http.Flusher)
				once := false
				for {
					select {
					case <-r.Context().Done():
						return
					case <-time.After(5 * time.Millisecond):
					}
					if _, err := io.WriteString(w, "ts"); err != nil {
						return
					}
					flusher.Flush()
					if !once {
						once = true
						close(streaming)
					}
				}
			})
			server := httptest.NewServer(epgstationapi.StopWhenRevokedEvery(gate, tokens, flow, 10*time.Millisecond))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer "+accepted)
			client := server.Client()
			// 流し続けるので、要求して読み捨てるのは別の goroutine にする。
			go func() {
				res, err := client.Do(req)
				if err != nil {
					return
				}
				defer res.Body.Close()
				_, _ = io.Copy(io.Discard, res.Body)
			}()

			<-streaming
			tt.revoke(gate, tokens)
			select {
			case <-stopped:
				assert.True(t, tt.wantStop, "the stream stopped while it was still allowed")
			case <-time.After(500 * time.Millisecond):
				assert.False(t, tt.wantStop, "the stream kept flowing after it was revoked")
			}
		})
	}
}
