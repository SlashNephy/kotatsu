// Package oauth は、MCP のクライアントにトークンを発行する、OAuth の認可サーバーの部品 (#429、#963)。
// クライアントの登録は Client ID Metadata Documents (CIMD) だけで受け付ける。client_id はクライアントのメタデータの URL で、
// Kotatsu がその URL を取りに行き、リダイレクト URI などを確かめる。
package oauth

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

const (
	// maxClientIDLength は、client_id の長さの上限。
	maxClientIDLength = 2048
	// authMethodNone は、クライアントの秘密を持たない公開クライアントの、トークンのエンドポイントでの認証の方法。
	// Kotatsu が受け付けるのは、これだけ。
	authMethodNone = "none"
	// grantTypeAuthorizationCode と responseTypeCode は、Kotatsu が受け付ける認可の方法。
	grantTypeAuthorizationCode = "authorization_code"
	responseTypeCode           = "code"
)

var (
	// ErrInvalidClientID は、client_id が CIMD の URL の決まりに合わないことを表す。
	ErrInvalidClientID = errors.New("invalid client_id")
	// ErrInvalidClientMetadata は、クライアントのメタデータが正しくないか、Kotatsu で使えないことを表す。
	ErrInvalidClientMetadata = errors.New("invalid client metadata")
)

// ParseClientID は、client_id が CIMD の URL の決まりに合うかを確かめて、URL を返す。
// https で、パスを持ち、`.` と `..` のセグメント、フラグメント、ユーザー情報を含まないものだけを受け付ける。
func ParseClientID(raw string) (*url.URL, error) {
	if raw == "" || len(raw) > maxClientIDLength {
		return nil, fmt.Errorf("%w: must be 1 to %d bytes", ErrInvalidClientID, maxClientIDLength)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidClientID, err)
	}
	switch {
	case u.Scheme != schemeHTTPS:
		return nil, fmt.Errorf("%w: the scheme must be https", ErrInvalidClientID)
	case u.Host == "" || u.Hostname() == "":
		return nil, fmt.Errorf("%w: the host is missing", ErrInvalidClientID)
	case u.User != nil:
		return nil, fmt.Errorf("%w: must not contain user information", ErrInvalidClientID)
	case u.Fragment != "" || strings.Contains(raw, "#"):
		return nil, fmt.Errorf("%w: must not contain a fragment", ErrInvalidClientID)
	case u.EscapedPath() == "" || u.EscapedPath() == "/":
		return nil, fmt.Errorf("%w: the path is missing", ErrInvalidClientID)
	}
	for segment := range strings.SplitSeq(u.EscapedPath(), "/") {
		if segment == "." || segment == ".." {
			return nil, fmt.Errorf("%w: must not contain dot segments", ErrInvalidClientID)
		}
	}

	return u, nil
}

// ClientMetadata は、CIMD で取ってきたクライアントのメタデータのうち、Kotatsu が使う項目。
type ClientMetadata struct {
	ClientID   string `json:"client_id"`
	ClientName string `json:"client_name"`
	ClientURI  string `json:"client_uri"`
	LogoURI    string `json:"logo_uri"`
	// RedirectURIs は、認可の後に戻してよい URI。認可のリクエストの redirect_uri は、この中のどれかでなければならない。
	RedirectURIs  []string `json:"redirect_uris"`
	GrantTypes    []string `json:"grant_types"`
	ResponseTypes []string `json:"response_types"`
	// TokenEndpointAuthMethod は、クライアントが好む、トークンのエンドポイントでの認証の方法。
	TokenEndpointAuthMethod string `json:"token_endpoint_auth_method"`
	// TokenEndpointAuthMethods は、クライアントが使える認証の方法。ChatGPT は、好む方法 (private_key_jwt) とは別に、
	// 使える方法として none も載せ、認可サーバーが受け付ける方法の中から選ぶ。
	TokenEndpointAuthMethods []string `json:"token_endpoint_auth_methods_supported"`
	// ClientSecret は、文書に秘密が載っていないことを確かめるためだけに読む。CIMD は誰でも読めるので、秘密を載せてはならない。
	ClientSecret *string `json:"client_secret"`
}

// validate は、clientID の URL から取ってきたメタデータを、Kotatsu で使えるかどうか確かめる。
func (m *ClientMetadata) validate(clientID string) error {
	switch {
	case m.ClientID != clientID:
		// 正規化せず、文字列として比べる (draft-ietf-oauth-client-id-metadata-document 4.1)。
		return fmt.Errorf("%w: client_id does not match the document URL", ErrInvalidClientMetadata)
	case strings.TrimSpace(m.ClientName) == "":
		return fmt.Errorf("%w: client_name is missing", ErrInvalidClientMetadata)
	case m.ClientSecret != nil:
		return fmt.Errorf("%w: must not contain client_secret", ErrInvalidClientMetadata)
	case len(m.RedirectURIs) == 0:
		return fmt.Errorf("%w: redirect_uris is missing", ErrInvalidClientMetadata)
	case !m.canUseNone():
		return fmt.Errorf("%w: the client must support the token endpoint auth method none", ErrInvalidClientMetadata)
	case len(m.GrantTypes) > 0 && !slices.Contains(m.GrantTypes, grantTypeAuthorizationCode):
		return fmt.Errorf("%w: grant_types must contain authorization_code", ErrInvalidClientMetadata)
	case len(m.ResponseTypes) > 0 && !slices.Contains(m.ResponseTypes, responseTypeCode):
		return fmt.Errorf("%w: response_types must contain code", ErrInvalidClientMetadata)
	}
	for _, uri := range m.RedirectURIs {
		if !validRedirectURI(uri) {
			return fmt.Errorf("%w: unsupported redirect URI %q", ErrInvalidClientMetadata, uri)
		}
	}

	return nil
}

// canUseNone は、クライアントが、秘密を持たない公開クライアントとして、トークンのエンドポイントを使えるかどうかを返す。
// どちらの項目もなければ、秘密を持てない CIMD のクライアントなので、公開クライアントとみなす。
func (m *ClientMetadata) canUseNone() bool {
	if m.TokenEndpointAuthMethod == "" && len(m.TokenEndpointAuthMethods) == 0 {
		return true
	}

	return m.TokenEndpointAuthMethod == authMethodNone || slices.Contains(m.TokenEndpointAuthMethods, authMethodNone)
}
