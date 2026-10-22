package graph

import (
	"context"
	"errors"
	"net/url"
	"slices"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/httpserver"
	"github.com/SlashNephy/kotatsu/internal/oauth"
)

// errOAuthUnavailable は、認可サーバーを扱う設定がないことを表す。
var errOAuthUnavailable = errors.New("the OAuth authorization server is not configured")

// oauthQuery は、認可のエンドポイントに届いたクエリを読み、リクエストが接続したオリジンと一緒に返す。
func oauthQuery(ctx context.Context, raw string) (url.Values, string, error) {
	query, err := url.ParseQuery(raw)
	if err != nil {
		return nil, "", NewError(CodeBadUserInput, "invalid authorization query")
	}
	client := httpserver.ClientFrom(ctx)
	if client == nil {
		return nil, "", errors.New("the request origin is unknown")
	}

	return query, client.Origin(), nil
}

// oauthAuthorizationOf は、確かめた認可のリクエストを GraphQL の形にする。
func oauthAuthorizationOf(req *oauth.AuthorizationRequest, allowed []string) *model.OAuthAuthorization {
	authorization := &model.OAuthAuthorization{
		ClientID: req.Client.ClientID, ClientName: req.Client.ClientName, ClientURI: nil, RedirectURI: req.RedirectURI,
		Scopes: req.Scopes, AllowedScopes: make([]string, 0, len(req.Scopes)),
	}
	if req.Client.ClientURI != "" {
		authorization.ClientURI = new(req.Client.ClientURI)
	}
	for _, scope := range req.Scopes {
		if slices.Contains(allowed, scope) {
			authorization.AllowedScopes = append(authorization.AllowedScopes, scope)
		}
	}

	return authorization
}

// oauthError は、認可のリクエストを確かめたときのエラーを、エラーコードにする。
// クライアントへ戻すエラー (*oauth.AuthorizationError) は、エラーにせず、呼び出し側が戻す URL として返す。
func oauthError(err error) error {
	switch {
	case errors.Is(err, oauth.ErrDisabled):
		return NewError(CodeNotFound, "MCP is disabled")
	case errors.Is(err, oauth.ErrInvalidAuthorizationRequest):
		return NewError(CodeBadUserInput, err.Error())
	case errors.Is(err, oauth.ErrInvalidScopeChoice):
		return NewError(CodeBadUserInput, "the chosen scopes are not requested or not allowed")
	default:
		return err
	}
}
