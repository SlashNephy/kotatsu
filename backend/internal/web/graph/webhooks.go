package graph

import (
	"context"
	"errors"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/webhook"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// webhookType は、Webhook の ID に付ける型の名前。
const webhookType = "Webhook"

// errWebhooksUnavailable は、Webhook の管理に使う部品が設定されていないことを表す。
var errWebhooksUnavailable = errors.New("webhook management is not configured")

// webhookAdministrator は、ログイン中のユーザーが管理者で、直近 10 分以内に認証したことを確かめ、操作者を返す (設計書 9.5、9.7)。
func (r *Resolver) webhookAdministrator(ctx context.Context) (*audit.Actor, error) {
	if r.webhooks == nil {
		return nil, errWebhooksUnavailable
	}
	_, actor, err := r.recentAdministrator(ctx, "only administrators can manage webhooks")

	return actor, err
}

// listWebhooks は、Webhook を作った順に返す。読むだけなので、再認証は求めない。
func (r *Resolver) listWebhooks(ctx context.Context) ([]*model.Webhook, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}
	if r.webhooks == nil {
		return nil, errWebhooksUnavailable
	}
	webhooks, err := r.webhooks.List(ctx, r.db)
	if err != nil {
		return nil, err
	}
	result := make([]*model.Webhook, 0, len(webhooks))
	for _, w := range webhooks {
		result = append(result, webhookModel(w))
	}

	return result, nil
}

func (r *Resolver) createWebhook(ctx context.Context, input *model.CreateWebhookInput) (*model.CreateWebhookPayload, error) {
	actor, err := r.webhookAdministrator(ctx)
	if err != nil {
		return nil, err
	}
	created, secret, err := r.webhooks.Create(ctx, r.db, actor, &webhook.Input{
		Name: input.Name, URL: input.URL, Format: input.Format, Kinds: input.Kinds, Enabled: input.Enabled,
	})
	if err != nil {
		return nil, webhookError(err)
	}

	return &model.CreateWebhookPayload{Webhook: webhookModel(created), Secret: secret}, nil
}

func (r *Resolver) updateWebhook(ctx context.Context, globalID string, input *model.UpdateWebhookInput) (*model.Webhook, error) {
	actor, err := r.webhookAdministrator(ctx)
	if err != nil {
		return nil, err
	}
	webhookID, err := parseWebhookID(globalID)
	if err != nil {
		return nil, err
	}
	changes := &webhook.Changes{Name: input.Name, URL: input.URL, Format: nil, Kinds: nil, Enabled: input.Enabled}
	if input.Format != nil {
		changes.Format = input.Format
	}
	if input.Kinds != nil {
		changes.Kinds = input.Kinds
	}
	updated, err := r.webhooks.Update(ctx, r.db, actor, webhookID, changes)
	if err != nil {
		return nil, webhookError(err)
	}

	return webhookModel(updated), nil
}

func (r *Resolver) deleteWebhook(ctx context.Context, globalID string) (string, error) {
	actor, err := r.webhookAdministrator(ctx)
	if err != nil {
		return "", err
	}
	webhookID, err := parseWebhookID(globalID)
	if err != nil {
		return "", err
	}
	if err := r.webhooks.Delete(ctx, r.db, actor, webhookID); err != nil {
		return "", webhookError(err)
	}

	return globalID, nil
}

func parseWebhookID(globalID string) (uuid.UUID, error) {
	return parseTypedID(globalID, webhookType, "invalid webhook ID")
}

// webhookError は、Webhook のエラーを、エラーコードを持つエラーにする。
func webhookError(err error) error {
	switch {
	case errors.Is(err, webhook.ErrInvalidWebhook):
		return NewError(CodeBadUserInput, err.Error())
	case errors.Is(err, webhook.ErrNotFound):
		return NewError(CodeNotFound, "webhook not found")
	default:
		return err
	}
}

func webhookModel(w *webhook.Webhook) *model.Webhook {
	return &model.Webhook{
		ID: GlobalID(webhookType, w.ID.String()), Name: w.Name, URL: w.URL,
		Format: w.Format, Kinds: w.Kinds,
		Enabled: w.Enabled, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
	}
}
