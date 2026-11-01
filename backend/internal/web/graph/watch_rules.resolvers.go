package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// CreateWatchRule is the resolver for the createWatchRule field.
func (r *mutationResolver) CreateWatchRule(ctx context.Context, input model.WatchRuleInput) (*model.WatchRule, error) {
	return r.saveWatchRule(ctx, nil, &input)
}

// UpdateWatchRule is the resolver for the updateWatchRule field.
func (r *mutationResolver) UpdateWatchRule(ctx context.Context, id string, input model.WatchRuleInput) (*model.WatchRule, error) {
	return r.saveWatchRule(ctx, &id, &input)
}

// SetWatchRuleEnabled is the resolver for the setWatchRuleEnabled field.
func (r *mutationResolver) SetWatchRuleEnabled(ctx context.Context, id string, enabled bool) (*model.WatchRule, error) {
	return r.setWatchRuleEnabled(ctx, id, enabled)
}

// DeleteWatchRule is the resolver for the deleteWatchRule field.
func (r *mutationResolver) DeleteWatchRule(ctx context.Context, id string) (string, error) {
	return r.deleteWatchRule(ctx, id)
}

// WatchRules is the resolver for the watchRules field.
func (r *queryResolver) WatchRules(ctx context.Context) ([]*model.WatchRule, error) {
	return r.listWatchRules(ctx)
}
