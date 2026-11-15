package oauth

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/SlashNephy/kotatsu/internal/lib/clientinfo"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
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
	if client := clientinfo.From(r.Context()); client != nil {
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

// serverGates は、管理者が有効にしている、認可サーバーの使い道。
type serverGates struct {
	// mcp は、MCP のクライアントに認可コードでトークンを発行するかどうか。
	mcp bool
	// device は、キーボードの無い端末にデバイス認可でトークンを発行するかどうか。EPGStation 互換の API の設定に従う。
	device bool
}

// allows は、kind の連携のトークンを、入れ替えたり受け付けたりしてよいかを返す。
func (g *serverGates) allows(kind string) bool {
	if kind == grantKindDevice {
		return g.device
	}

	return g.mcp
}

// serveAnyEnabled は、MCP か EPGStation 互換の API のどちらかが有効なら、有効なものを返して true を返す。
// どちらも無効なら 404 を返して false を返す。どちらも無効なときは、認可サーバーがないものとして振る舞う。
func (s *Server) serveAnyEnabled(w http.ResponseWriter, r *http.Request, logger *slog.Logger) (*serverGates, bool) {
	system, err := settings.Load(r.Context(), s.db)
	if err != nil {
		logger.ErrorContext(r.Context(), "failed to load the OAuth settings", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)

		return nil, false
	}
	gates := &serverGates{mcp: system.MCPEnabled, device: system.EPGStationAPIEnabled}
	if !gates.mcp && !gates.device {
		http.NotFound(w, r)

		return nil, false
	}

	return gates, true
}

// MetadataHandler は、認可サーバーのメタデータ (RFC 8414) を返すハンドラーを返す。
// MCP の認可 (認可コード) と、端末のデバイス認可のうち、管理者が有効にしているものだけを載せる。
func (s *Server) MetadataHandler(logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gates, ok := s.serveAnyEnabled(w, r, logger)
		if !ok {
			return
		}
		issuer := origin(r)
		metadata := map[string]any{
			"issuer":                                     issuer,
			"token_endpoint":                             issuer + ExchangePath,
			"revocation_endpoint":                        issuer + RevokePath,
			"response_types_supported":                   []string{},
			"token_endpoint_auth_methods_supported":      []string{authMethodNone},
			"revocation_endpoint_auth_methods_supported": []string{authMethodNone},
		}
		grantTypes := []string{}
		scopes := []string{}
		if gates.mcp {
			metadata["authorization_endpoint"] = issuer + AuthorizePath
			metadata["response_types_supported"] = []string{responseTypeCode}
			metadata["code_challenge_methods_supported"] = []string{CodeChallengeMethodS256}
			metadata["client_id_metadata_document_supported"] = true
			metadata["authorization_response_iss_parameter_supported"] = true
			grantTypes = append(grantTypes, grantTypeAuthorizationCode)
			scopes = append(scopes, supportedScopes()...)
		}
		if gates.device {
			metadata["device_authorization_endpoint"] = issuer + DeviceAuthorizationPath
			grantTypes = append(grantTypes, grantTypeDeviceCode)
			scopes = append(scopes, ScopeEPGStation)
		}
		metadata["grant_types_supported"] = append(grantTypes, grantTypeRefreshToken)
		metadata["scopes_supported"] = scopes
		w.Header().Set("Access-Control-Allow-Origin", "*")
		writeJSON(w, http.StatusOK, metadata)
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
