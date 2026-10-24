package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// CreateWebhook is the resolver for the createWebhook field.
func (r *mutationResolver) CreateWebhook(ctx context.Context, input model.CreateWebhookInput) (*model.CreateWebhookPayload, error) {
	return r.createWebhook(ctx, &input)
}

// UpdateWebhook is the resolver for the updateWebhook field.
func (r *mutationResolver) UpdateWebhook(ctx context.Context, id string, input model.UpdateWebhookInput) (*model.Webhook, error) {
	return r.updateWebhook(ctx, id, &input)
}

// DeleteWebhook is the resolver for the deleteWebhook field.
func (r *mutationResolver) DeleteWebhook(ctx context.Context, id string) (string, error) {
	return r.deleteWebhook(ctx, id)
}

// Webhooks is the resolver for the webhooks field.
func (r *queryResolver) Webhooks(ctx context.Context) ([]*model.Webhook, error) {
	return r.listWebhooks(ctx)
}
