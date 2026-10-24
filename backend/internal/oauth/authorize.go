package oauth

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// エラーコード (RFC 6749 4.1.2.1、5.2、RFC 8707 2)。
const (
	errorInvalidRequest          = "invalid_request"
	errorUnsupportedResponseType = "unsupported_response_type"
	errorInvalidScope            = "invalid_scope"
	errorInvalidTarget           = "invalid_target"
	errorAccessDenied            = "access_denied"
)

var (
	// ErrInvalidAuthorizationRequest は、認可のリクエストが正しくなく、しかもクライアントへ戻せないことを表す。
	// client_id かリダイレクト URI を確かめられないときは、どこへ戻すかが分からないので、画面にエラーを出す (RFC 6749 4.1.2.1)。
	ErrInvalidAuthorizationRequest = errors.New("invalid authorization request")
	// ErrInvalidScopeChoice は、同意の画面で選んだスコープが、求められたものか、ロールに許すものでないことを表す。
	ErrInvalidScopeChoice = errors.New("the chosen scopes are not requested or not allowed")
)

// AuthorizationError は、クライアントへ戻して伝える認可のエラー。RedirectURL に、エラーを付けて戻す。
type AuthorizationError struct {
	Code        string
	Description string
	RedirectURL string
}

func (e *AuthorizationError) Error() string {
	return fmt.Sprintf("authorization error %s: %s", e.Code, e.Description)
}

// AuthorizationRequest は、確かめた認可のリクエスト。
type AuthorizationRequest struct {
	Client      *ClientMetadata
	RedirectURI string
	State       string
	// Scopes は、求められたスコープのうち、Kotatsu のもの。
	Scopes        []string
	CodeChallenge string
	Resource      string
	// Issuer は、認可のレスポンスに付ける iss (RFC 9207)。
	Issuer string
}

// Prepare は、認可のエンドポイントに届いたクエリを確かめる。同意の画面を出す前と、同意したときに呼ぶ。
// クエリは、ログインした後で確かめる。ログインしていない経路から、クライアントのメタデータの取得を起こさせないためである。
// client_id かリダイレクト URI が正しくなければ ErrInvalidAuthorizationRequest を、
// ほかのパラメーターが正しくなければ、クライアントへ戻す *AuthorizationError を返す。
func (s *Server) Prepare(ctx context.Context, origin string, query url.Values) (*AuthorizationRequest, error) {
	if err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	for key, values := range query {
		if len(values) > 1 {
			// 同じパラメーターを 2 回送ってはならない (RFC 6749 3.1)。どちらを使うかで、確かめた値と使う値が食い違わないようにする。
			return nil, fmt.Errorf("%w: duplicated parameter %s", ErrInvalidAuthorizationRequest, key)
		}
	}
	clientID, redirectURI := query.Get("client_id"), query.Get("redirect_uri")
	if _, err := ParseClientID(clientID); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidAuthorizationRequest, err)
	}
	if redirectURI == "" {
		return nil, fmt.Errorf("%w: redirect_uri is missing", ErrInvalidAuthorizationRequest)
	}
	client, err := s.fetcher.Fetch(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidAuthorizationRequest, err)
	}
	if !MatchRedirectURI(client.RedirectURIs, redirectURI) {
		return nil, fmt.Errorf("%w: redirect_uri is not registered", ErrInvalidAuthorizationRequest)
	}

	issuer := origin
	req := &AuthorizationRequest{
		Client: client, RedirectURI: redirectURI, State: query.Get("state"), Scopes: parseScopes(query.Get("scope")),
		CodeChallenge: query.Get("code_challenge"), Resource: ResourceURL(origin), Issuer: issuer,
	}
	fail := func(code, description string) (*AuthorizationRequest, error) {
		return nil, &AuthorizationError{
			Code: code, Description: description, RedirectURL: errorRedirect(redirectURI, code, description, req.State, issuer),
		}
	}
	switch {
	case query.Get("response_type") != responseTypeCode:
		return fail(errorUnsupportedResponseType, "response_type must be code")
	case query.Get("code_challenge_method") != CodeChallengeMethodS256 || !ValidCodeChallenge(req.CodeChallenge):
		return fail(errorInvalidRequest, "PKCE with S256 is required")
	case !sameResource(query.Get("resource"), req.Resource):
		return fail(errorInvalidTarget, "resource must be "+req.Resource)
	case len(req.Scopes) == 0:
		return fail(errorInvalidScope, "no supported scope is requested")
	}

	return req, nil
}

// Approve は、ログイン中のユーザーが同意したので、認可コードを発行し、クライアントへ戻す URL を返す。
// chosen は、同意の画面で選んだスコープ。求められたもので、ロールに許すものでなければならない。
func (s *Server) Approve(
	ctx context.Context, actor *audit.Actor, principal *auth.Principal, origin string, query url.Values, chosen []string,
) (string, error) {
	req, err := s.Prepare(ctx, origin, query)
	if err != nil {
		return "", err
	}
	allowed := AllowedScopes(principal.Role)
	for _, scope := range chosen {
		if !slices.Contains(req.Scopes, scope) || !slices.Contains(allowed, scope) {
			return "", fmt.Errorf("%w: %s", ErrInvalidScopeChoice, scope)
		}
	}
	// 並びと重なりは、メタデータの順にそろえる。
	scopes := make([]string, 0, len(chosen))
	for _, scope := range supportedScopes() {
		if slices.Contains(chosen, scope) {
			scopes = append(scopes, scope)
		}
	}
	if len(scopes) == 0 {
		return "", ErrInvalidScopeChoice
	}

	code, err := newSecret(codeMark)
	if err != nil {
		return "", err
	}
	now := s.clock.Now()
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		err := sqlc.New(tx).CreateOAuthAuthorizationCode(ctx, sqlc.CreateOAuthAuthorizationCodeParams{
			CodeHash: hashSecret(code), UserID: principal.UserID, ClientID: req.Client.ClientID, ClientName: req.Client.ClientName,
			RedirectUri: req.RedirectURI, CodeChallenge: req.CodeChallenge, Scopes: scopes, Resource: req.Resource,
			Now: now, ExpiresAt: now.Add(AuthorizationCodeLifetime),
		})
		if err != nil {
			return fmt.Errorf("failed to save the authorization code: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: actionGrantCreate, TargetType: targetOAuthClient, TargetID: req.Client.ClientID,
			Summary: map[string]any{"client_name": req.Client.ClientName, "scopes": scopes}, SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
		})
	})
	if err != nil {
		return "", err
	}

	return successRedirect(req.RedirectURI, code, req.State, req.Issuer), nil
}

// Deny は、ログイン中のユーザーが断ったので、クライアントへ access_denied を戻す URL を返す。
func (s *Server) Deny(ctx context.Context, origin string, query url.Values) (string, error) {
	req, err := s.Prepare(ctx, origin, query)
	if err != nil {
		return "", err
	}

	return errorRedirect(req.RedirectURI, errorAccessDenied, "the user denied the request", req.State, req.Issuer), nil
}

// successRedirect は、認可コードを付けて、クライアントへ戻す URL を返す。
func successRedirect(redirectURI, code, state, issuer string) string {
	params := url.Values{"code": {code}, "iss": {issuer}}
	if state != "" {
		params.Set("state", state)
	}

	return withQuery(redirectURI, params)
}

// errorRedirect は、エラーを付けて、クライアントへ戻す URL を返す。エラーにも iss を付ける (RFC 9207 2)。
func errorRedirect(redirectURI, code, description, state, issuer string) string {
	params := url.Values{"error": {code}, "error_description": {description}, "iss": {issuer}}
	if state != "" {
		params.Set("state", state)
	}

	return withQuery(redirectURI, params)
}

// withQuery は、redirectURI のクエリに params を足した URL を返す。もとのクエリは残す (RFC 6749 3.1.2)。
func withQuery(redirectURI string, params url.Values) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return redirectURI
	}
	query := u.Query()
	maps.Copy(query, params)
	u.RawQuery = query.Encode()

	return u.String()
}
