package oauth

import (
	"net/url"
	"strings"
)

// schemeHTTPS は、https のスキーム。
const schemeHTTPS = "https"

// validRedirectURI は、クライアントのメタデータに載せてよいリダイレクト URI かどうかを返す。
// https、ループバックの http (RFC 8252 7.3)、スマートフォンのアプリが使う独自のスキーム (RFC 8252 7.1) を受け付ける。
// フラグメントを含むものは受け付けない (RFC 6749 3.1.2)。
func validRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || strings.Contains(raw, "#") || u.User != nil {
		return false
	}
	switch u.Scheme {
	case schemeHTTPS:
		return u.Hostname() != ""
	case "http":
		return isLoopback(u)
	case "":
		return false
	default:
		// 独自のスキームは、ドメインを逆にした名前 (com.example.app) にする決まりなので、`.` を含むものだけを受け付ける。
		// javascript: や data: のような、ブラウザが解釈するスキームを受け付けないためでもある。
		return strings.Contains(u.Scheme, ".")
	}
}

// MatchRedirectURI は、認可のリクエストの redirect_uri が、クライアントのメタデータの registered のどれかと合うかどうかを返す。
// 文字列として完全に一致するものだけを受け付ける。ループバックの http だけは、ポートを無視して比べる (RFC 8252 7.3)。
// ネイティブのアプリは、空いているポートをその都度選んで待ち受けるからである。Claude Code は、ポートのない
// http://localhost/callback を登録し、選んだポートを付けて要求してくる。
func MatchRedirectURI(registered []string, requested string) bool {
	req, err := url.Parse(requested)
	if err != nil || !validRedirectURI(requested) {
		return false
	}
	for _, candidate := range registered {
		if candidate == requested {
			return true
		}
		reg, err := url.Parse(candidate)
		if err != nil || !isLoopback(reg) || !isLoopback(req) {
			continue
		}
		if reg.Hostname() == req.Hostname() && reg.EscapedPath() == req.EscapedPath() && reg.RawQuery == req.RawQuery {
			return true
		}
	}

	return false
}

// isLoopback は、u がループバックのアドレスへの http の URI かどうかを返す。
func isLoopback(u *url.URL) bool {
	if u.Scheme != "http" {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
