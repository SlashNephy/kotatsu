package graph

import (
	"context"
	"errors"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/oauth"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// ApproveOAuthAuthorization is the resolver for the approveOAuthAuthorization field.
func (r *mutationResolver) ApproveOAuthAuthorization(ctx context.Context, query string, scopes []string) (string, error) {
	if r.oauth == nil {
		return "", errOAuthUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return "", err
	}
	// 同意すると、クライアントがこのユーザーとして読み書きできるようになるので、API トークンの発行と同じく、直前の再認証を求める (設計書 9.5)。
	if err := auth.RequireRecentAuthentication(ctx, r.clock); err != nil {
		return "", err
	}
	values, origin, err := oauthQuery(ctx, query)
	if err != nil {
		return "", err
	}
	redirect, err := r.oauth.Approve(ctx, actorOf(ctx, principal), principal, origin, values, scopes)
	if authErr := (*oauth.AuthorizationError)(nil); errors.As(err, &authErr) {
		return authErr.RedirectURL, nil
	}
	if err != nil {
		return "", oauthError(err)
	}

	return redirect, nil
}

// DenyOAuthAuthorization is the resolver for the denyOAuthAuthorization field.
func (r *mutationResolver) DenyOAuthAuthorization(ctx context.Context, query string) (string, error) {
	if r.oauth == nil {
		return "", errOAuthUnavailable
	}
	if _, err := viewer(ctx); err != nil {
		return "", err
	}
	values, origin, err := oauthQuery(ctx, query)
	if err != nil {
		return "", err
	}
	redirect, err := r.oauth.Deny(ctx, origin, values)
	if authErr := (*oauth.AuthorizationError)(nil); errors.As(err, &authErr) {
		return authErr.RedirectURL, nil
	}
	if err != nil {
		return "", oauthError(err)
	}

	return redirect, nil
}

// DeleteOAuthGrant is the resolver for the deleteOAuthGrant field.
func (r *mutationResolver) DeleteOAuthGrant(ctx context.Context, id string) (string, error) {
	if r.oauth == nil {
		return "", errOAuthUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return "", err
	}
	grantID, err := parseOAuthGrantID(id)
	if err != nil {
		return "", err
	}
	// 取り消しは、権限を広げず、乗っ取られた後の被害を抑える操作なので、再認証を求めない (設計書 9.5)。
	if err := r.oauth.DeleteGrant(ctx, actorOf(ctx, principal), principal.UserID, grantID); err != nil {
		return "", oauthError(err)
	}

	return id, nil
}

// OauthGrants is the resolver for the oauthGrants field.
func (r *queryResolver) OauthGrants(ctx context.Context) ([]*model.OAuthGrant, error) {
	if r.oauth == nil {
		return nil, errOAuthUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	grants, err := r.oauth.ListGrants(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	result := make([]*model.OAuthGrant, 0, len(grants))
	for _, grant := range grants {
		result = append(result, oauthGrantOf(grant))
	}

	return result, nil
}

// OauthAuthorization is the resolver for the oauthAuthorization field.
func (r *queryResolver) OauthAuthorization(ctx context.Context, query string) (*model.OAuthAuthorizationCheck, error) {
	if r.oauth == nil {
		return nil, errOAuthUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	values, origin, err := oauthQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	req, err := r.oauth.Prepare(ctx, origin, values)
	if authErr := (*oauth.AuthorizationError)(nil); errors.As(err, &authErr) {
		return &model.OAuthAuthorizationCheck{Authorization: nil, ErrorRedirectURL: &authErr.RedirectURL}, nil
	}
	if err != nil {
		return nil, oauthError(err)
	}

	return &model.OAuthAuthorizationCheck{
		Authorization: oauthAuthorizationOf(req, oauth.AllowedScopes(principal.Role)), ErrorRedirectURL: nil,
	}, nil
}
