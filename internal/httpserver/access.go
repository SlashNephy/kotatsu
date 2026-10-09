package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/config"
)

// Client は、リクエストの送信元。信頼済みのプロキシを経由したときは、プロキシが伝えた値にする。
type Client struct {
	// IP は送信元の IP アドレス。
	IP netip.Addr
	// Secure は、送信元が HTTPS でつないでいるかどうか。
	Secure bool
	// Host は、送信元が接続したホスト名 (Origin の検査に使う)。
	Host string
}

// Origin は、送信元が接続したオリジン (scheme://host) を返す。
func (c *Client) Origin() string {
	scheme := "http"
	if c.Secure {
		scheme = "https"
	}

	return scheme + "://" + c.Host
}

type clientKey struct{}

// ClientFrom は、AccessPolicy.Middleware が context に入れた送信元を返す。入っていなければ nil を返す。
func ClientFrom(ctx context.Context) *Client {
	client, _ := ctx.Value(clientKey{}).(*Client)

	return client
}

// forwardingHeaders は、リバースプロキシが送信元を伝えるヘッダーを返す。信頼済みのプロキシからでなければ受け付けない。
func forwardingHeaders() []string {
	return []string{"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "X-Real-IP", "Forwarded"}
}

// AccessPolicy は、接続を受け付ける送信元を決める (設計書 9.5)。
type AccessPolicy struct {
	allowed []netip.Prefix
	trusted []netip.Prefix
	logger  *slog.Logger
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

	return &AccessPolicy{allowed: allowed, trusted: trusted, logger: logger}, nil
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

		client := &Client{IP: peer, Secure: r.TLS != nil, Host: r.Host}
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

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientKey{}, client)))
	})
}

func (p *AccessPolicy) isTrusted(addr netip.Addr) bool {
	return contains(p.trusted, addr)
}

// applyForwarded は、信頼済みのプロキシが伝えた送信元を client に反映する。
// X-Forwarded-For は、右から順に信頼済みのプロキシを飛ばし、最初の信頼していないアドレスを送信元にする。
// 左側は送信元が自由に書けるので、信頼済みのプロキシが足した右側だけを信じる。
func (p *AccessPolicy) applyForwarded(client *Client, header http.Header) error {
	hops := []netip.Addr{}
	for _, value := range header.Values("X-Forwarded-For") {
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

	// 複数のプロキシを経由すると、プロキシごとの値がカンマで並ぶ。最初の値が、送信元に最も近いプロキシの見たものである。
	if proto := firstValue(header.Get("X-Forwarded-Proto")); proto != "" {
		client.Secure = strings.EqualFold(proto, "https")
	}
	if host := firstValue(header.Get("X-Forwarded-Host")); host != "" {
		client.Host = host
	}

	return nil
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
// 同じでなければ拒否するミドルウェアを返す (設計書 9.5)。/auth/* の CSRF を防ぐ。
// AccessPolicy.Middleware の後ろに置く。
func RequireSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)

			return
		}

		client := ClientFrom(r.Context())
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

	return strings.EqualFold(ua.Scheme, ub.Scheme) && strings.EqualFold(ua.Host, ub.Host)
}
