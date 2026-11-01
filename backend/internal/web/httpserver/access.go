package httpserver

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/lib/clientinfo"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
)

// defaultPort は、scheme の既定のポートを返す。Origin では省かれるので、比べる前に取り除く。
func defaultPort(scheme string) string {
	switch strings.ToLower(scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

// forwardingHeaders は、リバースプロキシが送信元を伝えるヘッダーを返す。信頼済みのプロキシからでなければ受け付けない。
func forwardingHeaders() []string {
	return []string{"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "X-Real-IP", "Forwarded"}
}

// AccessPolicy は、接続を受け付ける送信元を決める (設計書 9.5)。
type AccessPolicy struct {
	allowed []netip.Prefix
	trusted []netip.Prefix
	// clientIPHeader は、信頼済みのプロキシから届いたリクエストで、送信元を読むヘッダー。
	clientIPHeader string
	// allowedHosts は、受け付ける Host のホスト名 (小文字)。空なら、どの Host も受け付ける。
	allowedHosts []string
	logger       *slog.Logger
}

// NewAccessPolicy は、設定から AccessPolicy を作る。
func NewAccessPolicy(cfg *config.Server, logger *slog.Logger) (*AccessPolicy, error) {
	allowed, err := config.ParseNetworks(cfg.AllowedNetworks)
	if err != nil {
		return nil, fmt.Errorf("invalid server.allowed_networks: %w", err)
	}
	trusted, err := config.ParseNetworks(cfg.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("invalid server.trusted_proxies: %w", err)
	}

	hosts := make([]string, 0, len(cfg.AllowedHosts))
	for _, host := range cfg.AllowedHosts {
		hosts = append(hosts, strings.ToLower(strings.TrimSuffix(host, ".")))
	}

	return &AccessPolicy{allowed: allowed, trusted: trusted, clientIPHeader: cfg.ClientIPHeader, allowedHosts: hosts, logger: logger}, nil
}

// Middleware は、送信元を判定して context に入れ、許可していない送信元からのリクエストを拒否するミドルウェアを返す。
//
// 送信元は、信頼済みのプロキシから届いたときだけ X-Forwarded-For などから求める。
// 信頼していない送信元からこれらのヘッダー付きで届いたら、設定方法を案内するエラーを返す。
// Ingress を経由すると、送信元が Ingress のプライベートアドレスに見え、接続元の制限をすり抜けてしまうからである。
func (p *AccessPolicy) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer, err := remoteAddr(r)
		if err != nil {
			http.Error(w, "cannot determine the client address", http.StatusBadRequest)

			return
		}

		client := &clientinfo.Client{IP: peer, Secure: r.TLS != nil, Host: r.Host}
		if p.isTrusted(peer) {
			if err := p.applyForwarded(client, r.Header); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)

				return
			}
		} else if header := forwardedHeader(r.Header); header != "" {
			p.logger.WarnContext(r.Context(), "rejected a forwarded request from an untrusted address",
				slog.String("remote_addr", peer.String()), slog.String("header", header))
			http.Error(w, fmt.Sprintf(
				"%s was sent from %s, which is not a trusted proxy. "+
					"If this address is your reverse proxy, add it to server.trusted_proxies (KOTATSU_SERVER_TRUSTED_PROXIES).",
				header, peer), http.StatusBadRequest)

			return
		}

		if !contains(p.allowed, client.IP) {
			p.logger.WarnContext(r.Context(), "rejected a request from a disallowed address", slog.String("client", client.IP.String()))
			http.Error(w, fmt.Sprintf(
				"access from %s is not allowed. To allow it, add the network to server.allowed_networks (KOTATSU_SERVER_ALLOWED_NETWORKS).",
				client.IP), http.StatusForbidden)

			return
		}

		if !p.hostAllowed(client.Host) {
			p.logger.WarnContext(r.Context(), "rejected a request for a disallowed host", slog.String("host", client.Host))
			http.Error(w, fmt.Sprintf(
				"the host %s is not allowed. To allow it, add it to server.allowed_hosts (KOTATSU_SERVER_ALLOWED_HOSTS).",
				client.Host), http.StatusForbidden)

			return
		}

		next.ServeHTTP(w, r.WithContext(clientinfo.With(r.Context(), client)))
	})
}

// hostAllowed は、送信元が接続したホスト (Host か、信頼済みのプロキシが伝えた X-Forwarded-Host) を受け付けるかを返す (#546)。
// DNS リバインディングでは、Host が攻撃者のホスト名になるので、許したホスト名の一覧で拒む。
// IP アドレスと localhost は、外部のサイトのホスト名から向け直されたものではないので、いつも受け付ける。
// ヘルスチェックのように、IP アドレスで直接つなぐ要求も通すためでもある。
func (p *AccessPolicy) hostAllowed(hostport string) bool {
	if len(p.allowedHosts) == 0 {
		return true
	}
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	if _, err := netip.ParseAddr(host); err == nil || host == "localhost" {
		return true
	}

	return slices.Contains(p.allowedHosts, host)
}

func (p *AccessPolicy) isTrusted(addr netip.Addr) bool {
	return contains(p.trusted, addr)
}

// applyForwarded は、信頼済みのプロキシが伝えた送信元を client に反映する。
// X-Forwarded-For は、右から順に信頼済みのプロキシを飛ばし、最初の信頼していないアドレスを送信元にする。
// 左側は送信元が自由に書けるので、信頼済みのプロキシが足した右側だけを信じる。
// X-Forwarded-For が無ければ X-Real-IP を使う。どちらも無ければ、送信元が分からないので拒否する。
// プロキシ自身のアドレスで判定すると、プロキシのプライベートアドレスで接続元の制限をすり抜けてしまうからである。
// server.client_ip_header が X-Real-IP なら、X-Forwarded-For は読まず、X-Real-IP だけを使う。
func (p *AccessPolicy) applyForwarded(client *clientinfo.Client, header http.Header) error {
	if p.clientIPHeader == config.ClientIPHeaderXRealIP {
		if err := applyRealIP(client, header); err != nil {
			return err
		}
		applyForwardedOrigin(client, header)

		return nil
	}

	values := header.Values("X-Forwarded-For")
	if len(values) == 0 {
		if realIP := header.Get("X-Real-IP"); realIP != "" {
			values = []string{realIP}
		}
	}
	if len(values) == 0 {
		return fmt.Errorf("the request from the trusted proxy %s has neither X-Forwarded-For nor X-Real-IP; "+
			"configure the proxy to send X-Forwarded-For", client.IP)
	}

	hops := []netip.Addr{}
	for _, value := range values {
		for hop := range strings.SplitSeq(value, ",") {
			addr, err := parseHop(strings.TrimSpace(hop))
			if err != nil {
				return fmt.Errorf("invalid X-Forwarded-For: %w", err)
			}
			hops = append(hops, addr)
		}
	}
	for _, hop := range slices.Backward(hops) {
		client.IP = hop
		if !p.isTrusted(hop) {
			break
		}
	}

	applyForwardedOrigin(client, header)

	return nil
}

// applyRealIP は、信頼済みのプロキシが X-Real-IP に書いた送信元を client に反映する。
func applyRealIP(client *clientinfo.Client, header http.Header) error {
	realIP := header.Get("X-Real-IP")
	if realIP == "" {
		return fmt.Errorf("the request from the trusted proxy %s has no X-Real-IP; "+
			"configure the proxy to send X-Real-IP, or set server.client_ip_header to X-Forwarded-For", client.IP)
	}
	addr, err := parseHop(strings.TrimSpace(realIP))
	if err != nil {
		return fmt.Errorf("invalid X-Real-IP: %w", err)
	}
	client.IP = addr

	return nil
}

// applyForwardedOrigin は、信頼済みのプロキシが伝えた scheme とホストを client に反映する。
func applyForwardedOrigin(client *clientinfo.Client, header http.Header) {
	// 複数のプロキシを経由すると、プロキシごとの値がカンマで並ぶ。最初の値が、送信元に最も近いプロキシの見たものである。
	if proto := firstValue(header.Get("X-Forwarded-Proto")); proto != "" {
		client.Secure = strings.EqualFold(proto, "https")
	}
	if host := firstValue(header.Get("X-Forwarded-Host")); host != "" {
		client.Host = host
	}
}

// parseHop は、X-Forwarded-For の要素を IP アドレスにする。ポートや角括弧が付いていても読む。
func parseHop(hop string) (netip.Addr, error) {
	if addrPort, err := netip.ParseAddrPort(hop); err == nil {
		return addrPort.Addr().Unmap(), nil
	}
	addr, err := netip.ParseAddr(strings.Trim(hop, "[]"))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%q is not an IP address", hop)
	}

	return addr.Unmap(), nil
}

func firstValue(value string) string {
	first, _, _ := strings.Cut(value, ",")

	return strings.TrimSpace(first)
}

func forwardedHeader(header http.Header) string {
	for _, name := range forwardingHeaders() {
		if header.Get(name) != "" {
			return name
		}
	}

	return ""
}

func remoteAddr(r *http.Request) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid remote address %q: %w", r.RemoteAddr, err)
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid remote address %q: %w", r.RemoteAddr, err)
	}

	return addr.Unmap(), nil
}

func contains(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}

	return false
}

// RequireSameOrigin は、状態を変えるリクエスト (GET、HEAD、OPTIONS 以外) で、Origin が送信元の接続したオリジンと
// 同じでなければ拒否するミドルウェアを返す (設計書 9.5)。/auth/* と POST /media/captures の CSRF を防ぐ。
// AccessPolicy.Middleware の後ろに置く。
func RequireSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)

			return
		}

		client := clientinfo.From(r.Context())
		origin := r.Header.Get("Origin")
		if client == nil || origin == "" || !sameOrigin(origin, client.Origin()) {
			http.Error(w, "cross-origin request is not allowed", http.StatusForbidden)

			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin は、2 つのオリジンが、scheme とホスト (ポートを含む) で一致するかを返す。
func sameOrigin(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil {
		return false
	}
	ub, err := url.Parse(b)
	if err != nil {
		return false
	}

	return strings.EqualFold(ua.Scheme, ub.Scheme) && strings.EqualFold(hostWithoutDefaultPort(ua), hostWithoutDefaultPort(ub))
}

// hostWithoutDefaultPort は、ホストから scheme の既定のポートを取り除く (https://example.com:443 → example.com)。
func hostWithoutDefaultPort(u *url.URL) string {
	if port := u.Port(); port != "" && port == defaultPort(u.Scheme) {
		return u.Hostname()
	}

	return u.Host
}
