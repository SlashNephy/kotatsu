package databroadcastapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"syscall"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/publicnet"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/databroadcastapi"
)

// allowPort は、port への接続だけを、グローバルなアドレスとみなして通す Control を返す。ほかは publicnet.Control で確かめる。
// httptest のサーバーは 127.0.0.1 で待ち受けるので、テストの上流を「インターネットのサーバー」に見立てるのに使う。
func allowPort(port string) databroadcastapi.Control {
	return func(network, address string, conn syscall.RawConn) error {
		if _, p, err := net.SplitHostPort(address); err == nil && p == port {
			return nil
		}

		return publicnet.Control(network, address, conn)
	}
}

func portOf(t *testing.T, server *httptest.Server) string {
	t.Helper()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)

	return u.Port()
}

func enabledIs(enabled bool) func(context.Context) (bool, error) {
	return func(context.Context) (bool, error) { return enabled, nil }
}

// post は、ログイン中のユーザーとして、handler に JSON を送り、応答を返す。principal が nil なら、ログインしていないものとする。
func post(t *testing.T, handler http.Handler, principal *auth.Principal, body any) *httptest.ResponseRecorder {
	t.Helper()

	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, databroadcastapi.RequestPath, bytes.NewReader(encoded))
	if principal != nil {
		req = req.WithContext(auth.WithPrincipal(req.Context(), principal))
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec
}

// vote は、上流が受け取った POST。
type vote struct {
	method      string
	contentType string
	body        string
}

func viewer() *auth.Principal {
	return &auth.Principal{
		UserID: uuid.New(), Role: auth.RoleViewer, SessionID: uuid.New(), AuthenticatedAt: clock.Real{}.Now(), OAuthClientID: "",
	}
}

// TestRequestHandler は、文書のリクエストを中継し、グローバルでない宛先 (リダイレクトの先と、名前を引いた先を含む) を断ることを確かめる。
func TestRequestHandler(t *testing.T) {
	t.Parallel()

	// internal は、LAN の中のサーバーに見立てる。中継で届いてはならない。
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "secret")
	}))
	t.Cleanup(internal.Close)
	internalURL, err := url.Parse(internal.URL)
	require.NoError(t, err)

	// upstream は、インターネットのサーバーに見立てる。votes には、upstream が受け取った POST を、クエリの case ごとに入れる。
	var votes sync.Map
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/document.bml":
			w.Header().Set("Content-Type", "text/X-arib-bml; charset=EUC-JP")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = io.WriteString(w, "<bml/>")
		case "/vote":
			body, _ := io.ReadAll(r.Body)
			votes.Store(r.URL.Query().Get("case"), vote{method: r.Method, contentType: r.Header.Get("Content-Type"), body: string(body)})
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, "accepted")
		case "/missing":
			http.NotFound(w, r)
		case "/redirect-internal":
			http.Redirect(w, r, internal.URL+"/", http.StatusFound)
		case "/redirect-localhost":
			http.Redirect(w, r, "http://localhost:"+internalURL.Port()+"/", http.StatusFound)
		case "/redirect-file":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		case "/large":
			_, _ = w.Write(make([]byte, databroadcastapi.MaxResponseBytes+1))
		default:
			http.Error(w, "unexpected path", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(upstream.Close)

	relay := databroadcastapi.NewWith(enabledIs(true), allowPort(portOf(t, upstream)), "80")

	tests := []struct {
		name       string
		request    databroadcastapi.Request
		wantStatus int
		want       *databroadcastapi.Response
		// wantVote は、upstream が受け取るはずの POST。POST を送らないなら nil。
		wantVote *vote
	}{
		{
			name:       "GET を中継する",
			request:    databroadcastapi.Request{Method: http.MethodGet, URL: upstream.URL + "/document.bml", Body: nil},
			wantStatus: http.StatusOK,
			want: &databroadcastapi.Response{
				Status: http.StatusOK, ContentType: "text/X-arib-bml; charset=EUC-JP", CacheControl: "no-store", Body: []byte("<bml/>"),
			},
			wantVote: nil,
		},
		{
			name:       "POST の本文をフォームとして送る",
			request:    databroadcastapi.Request{Method: http.MethodPost, URL: upstream.URL + "/vote?case=form", Body: []byte("Denbun=%a4%a2")},
			wantStatus: http.StatusOK,
			want: &databroadcastapi.Response{
				Status: http.StatusOK, ContentType: "text/plain", CacheControl: "",
				Body: []byte("accepted"),
			},
			wantVote: &vote{method: http.MethodPost, contentType: "application/x-www-form-urlencoded", body: "Denbun=%a4%a2"},
		},
		{
			name:       "上流の状態コードを、中継の成否と分けて返す",
			request:    databroadcastapi.Request{Method: http.MethodGet, URL: upstream.URL + "/missing", Body: nil},
			wantStatus: http.StatusOK,
			want: &databroadcastapi.Response{
				Status: http.StatusNotFound, ContentType: "text/plain; charset=utf-8", CacheControl: "",
				Body: []byte("404 page not found\n"),
			},
			wantVote: nil,
		},
		{
			name:       "ループバックのアドレスへは接続しない",
			request:    databroadcastapi.Request{Method: http.MethodGet, URL: internal.URL + "/", Body: nil},
			wantStatus: http.StatusForbidden,
			want:       nil,
			wantVote:   nil,
		},
		{
			name:       "名前を引くとループバックになる宛先へは接続しない",
			request:    databroadcastapi.Request{Method: http.MethodGet, URL: "http://localhost:" + internalURL.Port() + "/", Body: nil},
			wantStatus: http.StatusForbidden,
			want:       nil,
			wantVote:   nil,
		},
		{
			name:       "ループバックのアドレスへのリダイレクトは追わない",
			request:    databroadcastapi.Request{Method: http.MethodGet, URL: upstream.URL + "/redirect-internal", Body: nil},
			wantStatus: http.StatusForbidden,
			want:       nil,
			wantVote:   nil,
		},
		{
			name:       "名前を引くとループバックになる宛先へのリダイレクトは追わない",
			request:    databroadcastapi.Request{Method: http.MethodPost, URL: upstream.URL + "/redirect-localhost", Body: []byte("Denbun=1")},
			wantStatus: http.StatusForbidden,
			want:       nil,
			wantVote:   nil,
		},
		{
			name:       "http と https でない宛先へのリダイレクトは追わない",
			request:    databroadcastapi.Request{Method: http.MethodGet, URL: upstream.URL + "/redirect-file", Body: nil},
			wantStatus: http.StatusBadGateway,
			want:       nil,
			wantVote:   nil,
		},
		{
			name:       "大きすぎる応答は返さない",
			request:    databroadcastapi.Request{Method: http.MethodGet, URL: upstream.URL + "/large", Body: nil},
			wantStatus: http.StatusBadGateway,
			want:       nil,
			wantVote:   nil,
		},
		{
			name:       "http と https でない URL は受け付けない",
			request:    databroadcastapi.Request{Method: http.MethodGet, URL: "file:///etc/passwd", Body: nil},
			wantStatus: http.StatusBadRequest,
			want:       nil,
			wantVote:   nil,
		},
		{
			name:       "GET と POST でないメソッドは受け付けない",
			request:    databroadcastapi.Request{Method: http.MethodDelete, URL: upstream.URL + "/vote", Body: nil},
			wantStatus: http.StatusBadRequest,
			want:       nil,
			wantVote:   nil,
		},
		{
			name:       "本文のある GET は受け付けない",
			request:    databroadcastapi.Request{Method: http.MethodGet, URL: upstream.URL + "/vote", Body: []byte("Denbun=1")},
			wantStatus: http.StatusBadRequest,
			want:       nil,
			wantVote:   nil,
		},
		{
			name:       "大きすぎる本文は受け付けない",
			request:    databroadcastapi.Request{Method: http.MethodPost, URL: upstream.URL + "/vote", Body: make([]byte, databroadcastapi.MaxBodyBytes+1)},
			wantStatus: http.StatusRequestEntityTooLarge,
			want:       nil,
			wantVote:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := post(t, relay.RequestHandler(), viewer(), tt.request)
			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			if tt.want == nil {
				return
			}
			assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			var got databroadcastapi.Response
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			assert.Equal(t, tt.want, &got)
			if tt.wantVote != nil {
				got, ok := votes.Load("form")
				require.True(t, ok, "the upstream must receive the POST")
				assert.Equal(t, *tt.wantVote, got)
			}
		})
	}
}

// TestRequestHandlerAuthorization は、ログインしていないときと、管理者が中継を有効にしていないときに、中継しないことを確かめる。
func TestRequestHandlerAuthorization(t *testing.T) {
	t.Parallel()

	requested := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requested <- struct{}{}
	}))
	t.Cleanup(upstream.Close)

	tests := []struct {
		name       string
		enabled    bool
		principal  *auth.Principal
		wantStatus int
	}{
		{name: "無効なら、ないものとして答える", enabled: false, principal: viewer(), wantStatus: http.StatusNotFound},
		{name: "ログインしていなければ断る", enabled: true, principal: nil, wantStatus: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			relay := databroadcastapi.NewWith(enabledIs(tt.enabled), allowPort(portOf(t, upstream)), "80")
			rec := post(t, relay.RequestHandler(), tt.principal, databroadcastapi.Request{Method: http.MethodGet, URL: upstream.URL + "/", Body: nil})
			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Empty(t, requested, "the upstream must not be requested")
		})
	}
}

// TestConnectivityHandler は、接続を確かめる宛先も、グローバルでないアドレスなら接続しないことを確かめる。
func TestConnectivityHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		destination string
		wantStatus  int
		wantSuccess bool
	}{
		{name: "ループバックのアドレスへは接続しない", destination: "127.0.0.1", wantStatus: http.StatusOK, wantSuccess: false},
		{name: "IPv6 のループバックのアドレスへは接続しない", destination: "::1", wantStatus: http.StatusOK, wantSuccess: false},
		{name: "名前を引くとループバックになる宛先へは接続しない", destination: "localhost", wantStatus: http.StatusOK, wantSuccess: false},
		{name: "ポートの付いた宛先は受け付けない", destination: "example.com:22", wantStatus: http.StatusBadRequest, wantSuccess: false},
		{name: "空の宛先は受け付けない", destination: "", wantStatus: http.StatusBadRequest, wantSuccess: false},
		{name: "パスの付いた宛先は受け付けない", destination: "example.com/path", wantStatus: http.StatusBadRequest, wantSuccess: false},
	}
	relay := databroadcastapi.NewWith(enabledIs(true), publicnet.Control, "80")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := post(t, relay.ConnectivityHandler(), viewer(), databroadcastapi.ConnectivityRequest{Destination: tt.destination, TimeoutMillis: 1000})
			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			if tt.wantStatus != http.StatusOK {
				return
			}
			var got databroadcastapi.ConnectivityResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			assert.Equal(t, &databroadcastapi.ConnectivityResponse{Success: tt.wantSuccess, IPAddress: nil, ResponseTimeMillis: nil}, &got)
		})
	}
}

// TestConnectivityHandlerSuccess は、接続できた宛先の、接続したアドレスと、かかった時間を返すことを確かめる。
func TestConnectivityHandlerSuccess(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(upstream.Close)
	port := portOf(t, upstream)
	relay := databroadcastapi.NewWith(enabledIs(true), allowPort(port), port)

	rec := post(t, relay.ConnectivityHandler(), viewer(), databroadcastapi.ConnectivityRequest{Destination: "127.0.0.1", TimeoutMillis: 0})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got databroadcastapi.ConnectivityResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Success)
	assert.Equal(t, new("127.0.0.1"), got.IPAddress)
	assert.NotNil(t, got.ResponseTimeMillis)
}
