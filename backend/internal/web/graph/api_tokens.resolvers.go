package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// CreateAPIToken is the resolver for the createApiToken field.
func (r *mutationResolver) CreateAPIToken(ctx context.Context, name string) (*model.CreatedAPIToken, error) {
	if r.apiTokens == nil {
		return nil, errAPITokensUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	// トークンは、ログインと同じく、そのユーザーとして読むことを許すので、直前の再認証を求める (設計書 9.5)。
	if err := auth.RequireRecentAuthentication(ctx, r.clock); err != nil {
		return nil, err
	}
	token, record, err := r.apiTokens.Create(ctx, actorOf(ctx, principal), principal.UserID, name)
	if err != nil {
		return nil, apiTokenError(err)
	}

	return &model.CreatedAPIToken{Token: token, APIToken: apiTokenOf(record)}, nil
}

// DeleteAPIToken is the resolver for the deleteApiToken field.
func (r *mutationResolver) DeleteAPIToken(ctx context.Context, id string) (string, error) {
	if r.apiTokens == nil {
		return "", errAPITokensUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return "", err
	}
	tokenID, err := parseAPITokenID(id)
	if err != nil {
		return "", err
	}
	if err := r.apiTokens.Delete(ctx, actorOf(ctx, principal), principal.UserID, tokenID); err != nil {
		return "", apiTokenError(err)
	}

	return id, nil
}

// APITokens is the resolver for the apiTokens field.
func (r *queryResolver) APITokens(ctx context.Context) ([]*model.APIToken, error) {
	if r.apiTokens == nil {
		return nil, errAPITokensUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	tokens, err := r.apiTokens.List(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	result := make([]*model.APIToken, 0, len(tokens))
	for _, token := range tokens {
		result = append(result, apiTokenOf(token))
	}

	return result, nil
}
