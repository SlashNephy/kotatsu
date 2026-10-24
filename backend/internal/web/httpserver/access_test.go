package httpserver_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

const (
	trustedProxy = "10.0.0.2"
	// publicAddr は、プライベートでないアドレス (文書用の TEST-NET-3)。
	publicAddr = "203.0.113.5"
	lanAddr    = "192.168.1.5"
	hostName   = "kotatsu.local:8080"

	headerXFF   = "X-Forwarded-For"
	headerXRI   = "X-Real-IP"
	headerXFH   = "X-Forwarded-Host"
	headerXFP   = "X-Forwarded-Proto"
	schemeHTTPS = "https"
	// proxiedHost は、リバースプロキシが受けたホスト名。
	proxiedHost = "tv.example.com"
	// publicRejected は、インターネットの送信元を拒否したときの応答。
	publicRejected = "access from 203.0.113.5 is not allowed"
	headerOrigin   = "Origin"
	// allowedHost は、server.allowed_hosts に載せるホスト名。reboundHost は、DNS リバインディングで攻撃者が使うホスト名。
	allowedHost = "kotatsu.local"
	// listenAddr は、server.listen。
	listenAddr  = ":8080"
	reboundHost = "attacker.example"
)

// clientEcho は、判定した送信元を応答のヘッダーに書く。
func clientEcho() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := httpserver.ClientFrom(r.Context())
		w.Header().Set("X-Client-IP", client.IP.String())
		w.Header().Set("X-Client-Origin", client.Origin())
	})
}

func TestAccessPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		wantStatus int
		wantIP     string
		wantOrigin string
		// wantBody は、拒否したときの応答に含まれるべき案内。
		wantBody string
		// clientIPHeader は、server.client_ip_header。空なら既定値を使う。
		clientIPHeader string
	}{
		{name: "private address", remoteAddr: lanAddr, wantStatus: http.StatusOK, wantIP: lanAddr, wantOrigin: "http://" + hostName},
		{name: "loopback", remoteAddr: "127.0.0.1", wantStatus: http.StatusOK, wantIP: "127.0.0.1", wantOrigin: "http://" + hostName},
		{name: "tailscale", remoteAddr: "100.101.102.103", wantStatus: http.StatusOK, wantIP: "100.101.102.103", wantOrigin: "http://" + hostName},
		{name: "IPv6 unique local", remoteAddr: "[fd00::1]", wantStatus: http.StatusOK, wantIP: "fd00::1", wantOrigin: "http://" + hostName},
		{
			name: "public address is rejected", remoteAddr: publicAddr, wantStatus: http.StatusForbidden,
			wantBody: "access from 203.0.113.5 is not allowed. To allow it, add the network to server.allowed_networks (KOTATSU_SERVER_ALLOWED_NETWORKS).",
		},
		{
			name: "forwarded header from an untrusted address is rejected with guidance", remoteAddr: lanAddr,
			headers:    map[string]string{headerXFF: publicAddr},
			wantStatus: http.StatusBadRequest,
			wantBody:   "X-Forwarded-For was sent from 192.168.1.5, which is not a trusted proxy. If this address is your reverse proxy, add it to server.trusted_proxies (KOTATSU_SERVER_TRUSTED_PROXIES).",
		},
		{
			name: "Forwarded header from an untrusted address is rejected too", remoteAddr: lanAddr,
			headers: map[string]string{"Forwarded": "for=203.0.113.5"}, wantStatus: http.StatusBadRequest, wantBody: "Forwarded was sent from 192.168.1.5",
		},
		{
			name: "client behind a trusted proxy", remoteAddr: trustedProxy,
			headers:    map[string]string{headerXFF: lanAddr, headerXFP: schemeHTTPS, headerXFH: proxiedHost},
			wantStatus: http.StatusOK, wantIP: lanAddr, wantOrigin: "https://" + proxiedHost,
		},
		{
			name: "public client behind a trusted proxy is rejected", remoteAddr: trustedProxy,
			headers:    map[string]string{headerXFF: publicAddr},
			wantStatus: http.StatusForbidden, wantBody: publicRejected,
		},
		{
			// 左側は送信元が自由に書けるので、信頼済みのプロキシが足した右側から送信元を決める。
			name: "spoofed private address on the left is ignored", remoteAddr: trustedProxy,
			headers:    map[string]string{headerXFF: "10.0.0.9, " + publicAddr},
			wantStatus: http.StatusForbidden, wantBody: publicRejected,
		},
		{
			name: "chain of trusted proxies", remoteAddr: trustedProxy,
			headers:    map[string]string{headerXFF: lanAddr + ", " + trustedProxy},
			wantStatus: http.StatusOK, wantIP: lanAddr, wantOrigin: "http://" + hostName,
		},
		{
			name: "X-Real-IP behind a trusted proxy", remoteAddr: trustedProxy,
			headers:    map[string]string{"X-Real-IP": publicAddr},
			wantStatus: http.StatusForbidden, wantBody: publicRejected,
		},
		{
			// プロキシ自身のプライベートアドレスで判定すると、制限をすり抜けてしまう。
			name: "trusted proxy without forwarded client address is rejected", remoteAddr: trustedProxy,
			wantStatus: http.StatusBadRequest, wantBody: "the request from the trusted proxy 10.0.0.2 has neither X-Forwarded-For nor X-Real-IP",
		},
		{
			name: "X-Forwarded-For wins over X-Real-IP by default", remoteAddr: trustedProxy,
			headers:    map[string]string{headerXFF: lanAddr, headerXRI: publicAddr},
			wantStatus: http.StatusOK, wantIP: lanAddr, wantOrigin: "http://" + hostName,
		},
		{
			name: "X-Real-IP mode ignores X-Forwarded-For written by the client", remoteAddr: trustedProxy,
			clientIPHeader: headerXRI, headers: map[string]string{headerXFF: lanAddr, headerXRI: publicAddr},
			wantStatus: http.StatusForbidden, wantBody: publicRejected,
		},
		{
			name: "X-Real-IP mode", remoteAddr: trustedProxy,
			clientIPHeader: headerXRI, headers: map[string]string{headerXFF: publicAddr, headerXRI: lanAddr},
			wantStatus: http.StatusOK, wantIP: lanAddr, wantOrigin: "http://" + hostName,
		},
		{
			name: "X-Real-IP mode applies the forwarded origin", remoteAddr: trustedProxy,
			clientIPHeader: headerXRI, headers: map[string]string{headerXRI: lanAddr, headerXFP: schemeHTTPS, headerXFH: proxiedHost},
			wantStatus: http.StatusOK, wantIP: lanAddr, wantOrigin: "https://" + proxiedHost,
		},
		{
			name: "X-Real-IP mode without X-Real-IP is rejected", remoteAddr: trustedProxy,
			clientIPHeader: headerXRI, headers: map[string]string{headerXFF: lanAddr},
			wantStatus: http.StatusBadRequest, wantBody: "the request from the trusted proxy 10.0.0.2 has no X-Real-IP",
		},
		{
			name: "invalid X-Real-IP", remoteAddr: trustedProxy,
			clientIPHeader: headerXRI, headers: map[string]string{headerXRI: "unknown"},
			wantStatus: http.StatusBadRequest, wantBody: `invalid X-Real-IP: "unknown" is not an IP address`,
		},
		{
			name: "invalid X-Forwarded-For", remoteAddr: trustedProxy,
			headers: map[string]string{headerXFF: "unknown"}, wantStatus: http.StatusBadRequest, wantBody: `invalid X-Forwarded-For: "unknown" is not an IP address`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clientIPHeader := tt.clientIPHeader
			if clientIPHeader == "" {
				clientIPHeader = config.Default().Server.ClientIPHeader
			}
			policy, err := httpserver.NewAccessPolicy(&config.Server{
				Listen: listenAddr, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{trustedProxy},
				ClientIPHeader: clientIPHeader,
			}, slog.New(slog.DiscardHandler))
			require.NoError(t, err)
			handler := policy.Middleware(clientEcho())

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+hostName+"/", nil)
			req.RemoteAddr = tt.remoteAddr + ":51234"
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			if tt.wantStatus != http.StatusOK {
				assert.Contains(t, rec.Body.String(), tt.wantBody)

				return
			}
			assert.Equal(t, tt.wantIP, rec.Header().Get("X-Client-IP"))
			assert.Equal(t, tt.wantOrigin, rec.Header().Get("X-Client-Origin"))
		})
	}
}

// TestAccessPolicyAllowedHosts は、server.allowed_hosts を設定したとき、一覧にない Host のリクエストを拒むことを確かめる (#546)。
// DNS リバインディングでは、Host が攻撃者のホスト名になる。
func TestAccessPolicyAllowedHosts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		allowedHosts []string
		host         string
		remoteAddr   string
		headers      map[string]string
		wantStatus   int
	}{
		{name: "no list allows any host", allowedHosts: []string{}, host: reboundHost, remoteAddr: lanAddr, wantStatus: http.StatusOK},
		{name: "listed host with a port", allowedHosts: []string{allowedHost}, host: hostName, remoteAddr: lanAddr, wantStatus: http.StatusOK},
		{name: "host names are compared case-insensitively", allowedHosts: []string{"Kotatsu.Local"}, host: "KOTATSU.local", remoteAddr: lanAddr, wantStatus: http.StatusOK},
		{name: "rebound host is rejected", allowedHosts: []string{allowedHost}, host: reboundHost + listenAddr, remoteAddr: lanAddr, wantStatus: http.StatusForbidden},
		{name: "IPv4 literal is always allowed", allowedHosts: []string{allowedHost}, host: "192.168.1.2" + listenAddr, remoteAddr: lanAddr, wantStatus: http.StatusOK},
		{name: "IPv6 literal is always allowed", allowedHosts: []string{allowedHost}, host: "[fd00::2]" + listenAddr, remoteAddr: lanAddr, wantStatus: http.StatusOK},
		{name: "localhost is always allowed", allowedHosts: []string{allowedHost}, host: "localhost" + listenAddr, remoteAddr: lanAddr, wantStatus: http.StatusOK},
		{
			name: "forwarded host from a trusted proxy is checked", allowedHosts: []string{proxiedHost}, host: trustedProxy + listenAddr, remoteAddr: trustedProxy,
			headers: map[string]string{headerXFF: lanAddr, headerXFH: proxiedHost}, wantStatus: http.StatusOK,
		},
		{
			name: "rebound forwarded host is rejected", allowedHosts: []string{proxiedHost}, host: proxiedHost, remoteAddr: trustedProxy,
			headers: map[string]string{headerXFF: lanAddr, headerXFH: reboundHost}, wantStatus: http.StatusForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			policy, err := httpserver.NewAccessPolicy(&config.Server{
				Listen: listenAddr, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{trustedProxy},
				ClientIPHeader: config.ClientIPHeaderXForwardedFor, AllowedHosts: tt.allowedHosts,
			}, slog.New(slog.DiscardHandler))
			require.NoError(t, err)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+tt.host+"/", nil)
			req.RemoteAddr = tt.remoteAddr + ":51234"
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}
			rec := httptest.NewRecorder()
			policy.Middleware(clientEcho()).ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			if tt.wantStatus == http.StatusForbidden {
				assert.Contains(t, rec.Body.String(), "To allow it, add it to server.allowed_hosts (KOTATSU_SERVER_ALLOWED_HOSTS).")
			}
		})
	}
}

func TestRequireSameOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		remoteAddr string
		headers    map[string]string
		wantStatus int
	}{
		{name: "GET does not need an Origin", method: http.MethodGet, remoteAddr: lanAddr, wantStatus: http.StatusOK},
		{name: "same origin", method: http.MethodPost, remoteAddr: lanAddr, headers: map[string]string{headerOrigin: "http://" + hostName}, wantStatus: http.StatusOK},
		{name: "different origin", method: http.MethodPost, remoteAddr: lanAddr, headers: map[string]string{headerOrigin: "http://evil.example.com"}, wantStatus: http.StatusForbidden},
		{name: "different port", method: http.MethodPost, remoteAddr: lanAddr, headers: map[string]string{headerOrigin: "http://kotatsu.local:9090"}, wantStatus: http.StatusForbidden},
		{name: "missing Origin", method: http.MethodPost, remoteAddr: lanAddr, wantStatus: http.StatusForbidden},
		{
			name: "same origin over HTTPS behind a trusted proxy", method: http.MethodPost, remoteAddr: trustedProxy,
			headers:    map[string]string{headerXFF: lanAddr, headerXFP: schemeHTTPS, headerXFH: proxiedHost, headerOrigin: "https://" + proxiedHost},
			wantStatus: http.StatusOK,
		},
		{
			name: "default port in the forwarded host", method: http.MethodPost, remoteAddr: trustedProxy,
			headers:    map[string]string{headerXFF: lanAddr, headerXFP: schemeHTTPS, headerXFH: proxiedHost + ":443", headerOrigin: "https://" + proxiedHost},
			wantStatus: http.StatusOK,
		},
		{
			name: "plain HTTP origin is not the HTTPS origin", method: http.MethodPost, remoteAddr: trustedProxy,
			headers:    map[string]string{headerXFF: lanAddr, headerXFP: schemeHTTPS, headerXFH: proxiedHost, headerOrigin: "http://" + proxiedHost},
			wantStatus: http.StatusForbidden,
		},
	}
	policy, err := httpserver.NewAccessPolicy(&config.Server{
		Listen: listenAddr, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{trustedProxy},
	}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	handler := policy.Middleware(httpserver.RequireSameOrigin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), tt.method, "http://"+hostName+"/auth/logout", nil)
			req.RemoteAddr = tt.remoteAddr + ":51234"
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
		})
	}
}
