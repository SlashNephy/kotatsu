package oauth

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

// MetadataPath と ProtectedResourceMetadataPath は、認可サーバーのメタデータ (RFC 8414) と、
// 保護リソースのメタデータ (RFC 9728) のパス。issuer はパスを持たないので、RFC 8414 3.1 の既定の場所に置く。
const (
	MetadataPath                  = "/.well-known/oauth-authorization-server"
	ProtectedResourceMetadataPath = "/.well-known/oauth-protected-resource" + ResourcePath
	AuthorizePath                 = "/oauth/authorize"
	// ExchangePath は、トークンのエンドポイントのパス。
	ExchangePath = "/oauth/token"
	RevokePath   = "/oauth/revoke"
)

// origin は、リクエストが接続したオリジンを返す。リバースプロキシを経由したときは、信頼済みのプロキシが伝えた値にする。
func origin(r *http.Request) string {
	if client := httpserver.ClientFrom(r.Context()); client != nil {
		return client.Origin()
	}
	scheme := schemeHTTP
	if r.TLS != nil {
		scheme = schemeHTTPS
	}

	return scheme + "://" + r.Host
}

// serveEnabled は、MCP が有効なら true を返す。無効なら 404 を返して false を返す。
// 無効なときは、認可サーバーがないものとして振る舞う。
func (s *Server) serveEnabled(w http.ResponseWriter, r *http.Request, logger *slog.Logger) bool {
	err := s.requireEnabled(r.Context())
	if errors.Is(err, ErrDisabled) {
		http.NotFound(w, r)

		return false
	}
	if err != nil {
		logger.ErrorContext(r.Context(), "failed to load the MCP setting", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)

		return false
	}

	return true
}

// MetadataHandler は、認可サーバーのメタデータ (RFC 8414) を返すハンドラーを返す。
func (s *Server) MetadataHandler(logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.serveEnabled(w, r, logger) {
			return
		}
		issuer := origin(r)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		writeJSON(w, http.StatusOK, map[string]any{
			"issuer":                                         issuer,
			"authorization_endpoint":                         issuer + AuthorizePath,
			"token_endpoint":                                 issuer + ExchangePath,
			"revocation_endpoint":                            issuer + RevokePath,
			"response_types_supported":                       []string{responseTypeCode},
			"grant_types_supported":                          []string{grantTypeAuthorizationCode, grantTypeRefreshToken},
			"code_challenge_methods_supported":               []string{CodeChallengeMethodS256},
			"token_endpoint_auth_methods_supported":          []string{authMethodNone},
			"revocation_endpoint_auth_methods_supported":     []string{authMethodNone},
			"scopes_supported":                               supportedScopes(),
			"client_id_metadata_document_supported":          true,
			"authorization_response_iss_parameter_supported": true,
		})
	})
}

// ProtectedResourceMetadataHandler は、MCP サーバーの保護リソースのメタデータ (RFC 9728) を返すハンドラーを返す。
func (s *Server) ProtectedResourceMetadataHandler(logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.serveEnabled(w, r, logger) {
			return
		}
		issuer := origin(r)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		writeJSON(w, http.StatusOK, map[string]any{
			"resource":                 ResourceURL(issuer),
			"authorization_servers":    []string{issuer},
			"scopes_supported":         supportedScopes(),
			"bearer_methods_supported": []string{"header"},
		})
	})
}

// AuthorizeHandler は、認可のエンドポイント (GET /oauth/authorize) のハンドラーを返す。
// ここでは何も確かめず、クエリをそのまま付けて、同意の画面へ送る。ログインしていなければ、画面がログインさせてから同意を求める。
// クエリを確かめ、クライアントのメタデータを取るのは、ログインした後の GraphQL (Prepare) で行う。
func (s *Server) AuthorizeHandler(logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.serveEnabled(w, r, logger) {
			return
		}
		target := url.URL{Path: ConsentPath, RawQuery: r.URL.RawQuery}
		http.Redirect(w, r, target.String(), http.StatusFound)
	})
}
