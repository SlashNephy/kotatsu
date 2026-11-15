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
	"github.com/SlashNephy/kotatsu/internal/usecase/oauth"
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

// deviceAuth は、acceptedDevice だけを、連携を取り消されるまで受け付ける。アクセストークンの期限は、expired で切らせる。
type deviceAuth struct {
	revoked atomic.Bool
	expired atomic.Bool
	grantID uuid.UUID
}

// acceptedDevice は、受け付ける端末のアクセストークン。
const acceptedDevice = "kotatsu_at_valid"

func (a *deviceAuth) AuthenticateDevice(_ context.Context, token, _ string) (*auth.Principal, *oauth.Grant, error) {
	if token != acceptedDevice || a.revoked.Load() || a.expired.Load() {
		return nil, nil, auth.ErrUnauthenticated
	}
	principal := &auth.Principal{UserID: uuid.New(), Role: domain.RoleViewer, SessionID: uuid.Nil(), AuthenticatedAt: time.Time{}, OAuthClientID: "tv"}

	return principal, &oauth.Grant{ID: a.grantID, ClientID: "tv", ClientName: "テレビ", Scopes: []string{oauth.ScopeEPGStation}}, nil
}

func (a *deviceAuth) DeviceGrantRole(_ context.Context, grantID uuid.UUID) (domain.Role, error) {
	if grantID != a.grantID || a.revoked.Load() {
		return "", auth.ErrUnauthenticated
	}

	return domain.RoleViewer, nil
}

// TestCredentials は、API トークンと端末のトークンを、それぞれの印で見分けて確かめ、
// 端末のトークンが違っても API トークンとしては確かめず、401 を返すことを確かめる (#1527)。
func TestCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		header     string
		wantStatus int
		wantDevice bool
	}{
		{name: "API トークン", header: "Bearer " + accepted, wantStatus: http.StatusNoContent},
		{name: "端末のトークン", header: "Bearer " + acceptedDevice, wantStatus: http.StatusNoContent, wantDevice: true},
		{name: "知らない API トークン", header: "Bearer " + refused, wantStatus: http.StatusUnauthorized},
		// kotatsu_at_ も kotatsu_ で始まるが、API トークンとしては確かめない。
		{name: "知らない端末のトークンは、ほかの認証へ回さない", header: "Bearer kotatsu_at_unknown", wantStatus: http.StatusUnauthorized},
		{name: "トークンがなければ、後ろに任せる", header: "", wantStatus: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tokens := &acceptAll{}
			devices := &deviceAuth{grantID: uuid.New()}
			var gotDevice bool
			handler := epgstationapi.NewCredentials(tokens, devices).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotDevice = oauth.DeviceGrantFrom(r.Context()) != nil
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/epgstation/api/version", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantDevice, gotDevice)
			assert.False(t, tokens.sawDeviceToken.Load(), "a device token was passed to the API token authenticator")
		})
	}
}

// refused は、acceptAll が受け付けない API トークン。
const refused = "kotatsu_unknown"

// acceptAll は、refused のほかの API トークンを受け付け、端末のトークンを渡されたら覚える。
type acceptAll struct {
	sawDeviceToken atomic.Bool
}

func (a *acceptAll) Authenticate(_ context.Context, token string) (*auth.Principal, error) {
	if oauth.IsAccessToken(token) {
		a.sawDeviceToken.Store(true)
	}
	if token == refused {
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

// TestStopWhenRevoked は、ライブの TS を流している間に、管理者が API を無効にしたか、API トークンか端末の連携を取り消したら、
// 流しているストリームも止め、使えるうちは止めないことを確かめる (#1769、#1527)。
// 端末のアクセストークンの期限が来ても、連携が使えるうちは止めない。
func TestStopWhenRevoked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    string
		revoke   func(gate *apiGate, tokens *tokenAuth, devices *deviceAuth)
		wantStop bool
	}{
		{name: "管理者が無効にした", token: accepted, revoke: func(gate *apiGate, _ *tokenAuth, _ *deviceAuth) { gate.disabled.Store(true) }, wantStop: true},
		{name: "トークンを取り消した", token: accepted, revoke: func(_ *apiGate, tokens *tokenAuth, _ *deviceAuth) { tokens.revoked.Store(true) }, wantStop: true},
		{name: "使えるうちは流し続ける", token: accepted, revoke: func(*apiGate, *tokenAuth, *deviceAuth) {}, wantStop: false},
		{name: "端末の連携を取り消した", token: acceptedDevice, revoke: func(_ *apiGate, _ *tokenAuth, devices *deviceAuth) { devices.revoked.Store(true) }, wantStop: true},
		{name: "端末のアクセストークンの期限が来ても流し続ける", token: acceptedDevice, revoke: func(_ *apiGate, _ *tokenAuth, devices *deviceAuth) { devices.expired.Store(true) }, wantStop: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gate, tokens, devices := new(apiGate), new(tokenAuth), &deviceAuth{grantID: uuid.New()}
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
			credentials := epgstationapi.NewCredentials(tokens, devices)
			server := httptest.NewServer(credentials.Middleware(epgstationapi.StopWhenRevokedEvery(gate, credentials, flow, 10*time.Millisecond)))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer "+tt.token)
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
			tt.revoke(gate, tokens, devices)
			select {
			case <-stopped:
				assert.True(t, tt.wantStop, "the stream stopped while it was still allowed")
			case <-time.After(500 * time.Millisecond):
				assert.False(t, tt.wantStop, "the stream kept flowing after it was revoked")
			}
		})
	}
}
