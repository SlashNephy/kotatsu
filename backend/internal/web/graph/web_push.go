package graph

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
	"github.com/SlashNephy/kotatsu/internal/webpush"
)

// webPushSubscriptionType は、WebPushSubscription の Global ID の型名。
const webPushSubscriptionType = "WebPushSubscription"

var errWebPushUnavailable = errors.New("web push is not configured")

// webPushPublicKey は、VAPID の公開鍵を base64url にして返す。
func (r *Resolver) webPushPublicKey(ctx context.Context) (string, error) {
	if _, err := viewer(ctx); err != nil {
		return "", err
	}
	if r.webPush == nil {
		return "", errWebPushUnavailable
	}
	keys, err := r.webPush.Keys(ctx, r.db)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(keys.PublicKey()), nil
}

// webPushSubscriptions は、ログイン中のユーザーの購読を返す。
func (r *Resolver) webPushSubscriptions(ctx context.Context) ([]*model.WebPushSubscription, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if r.webPush == nil {
		return nil, errWebPushUnavailable
	}
	subscriptions, err := r.webPush.List(ctx, r.db, principal.UserID)
	if err != nil {
		return nil, err
	}
	result := make([]*model.WebPushSubscription, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		result = append(result, webPushSubscriptionOf(subscription))
	}

	return result, nil
}

// subscribeWebPush は、ログイン中のユーザーの端末で、Web Push を購読する。
func (r *Resolver) subscribeWebPush(ctx context.Context, input *model.WebPushSubscriptionInput) (*model.WebPushSubscription, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if r.webPush == nil {
		return nil, errWebPushUnavailable
	}
	p256dh, err := decodeKey(input.P256dh)
	if err != nil {
		return nil, NewError(CodeBadUserInput, "p256dh must be base64url")
	}
	authSecret, err := decodeKey(input.Auth)
	if err != nil {
		return nil, NewError(CodeBadUserInput, "auth must be base64url")
	}
	userAgent, _ := ctx.Value(userAgentKey{}).(string)
	subscription := &webpush.Subscription{
		ID: uuid.Nil, Endpoint: input.Endpoint, P256DH: p256dh, Auth: authSecret, UserAgent: userAgent, CreatedAt: r.clock.Now(), UpdatedAt: r.clock.Now(),
	}
	if err := r.webPush.Subscribe(ctx, r.db, principal.UserID, subscription); err != nil {
		if errors.Is(err, webpush.ErrInvalidSubscription) {
			return nil, NewError(CodeBadUserInput, err.Error())
		}

		return nil, err
	}
	subscriptions, err := r.webPush.List(ctx, r.db, principal.UserID)
	if err != nil {
		return nil, err
	}
	for _, saved := range subscriptions {
		if saved.Endpoint == input.Endpoint {
			return webPushSubscriptionOf(saved), nil
		}
	}

	return nil, errors.New("the saved web push subscription was not found")
}

// unsubscribeWebPush は、ログイン中のユーザーの購読を解除する。
func (r *Resolver) unsubscribeWebPush(ctx context.Context, endpoint string) (bool, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return false, err
	}
	if r.webPush == nil {
		return false, errWebPushUnavailable
	}

	return r.webPush.Unsubscribe(ctx, r.db, principal.UserID, endpoint)
}

// decodeKey は、PushSubscription の鍵を読む。toJSON() は詰め物のない base64url を返すが、詰め物があっても受け付ける。
func decodeKey(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(value, "="))
}

func webPushSubscriptionOf(subscription *webpush.Subscription) *model.WebPushSubscription {
	return &model.WebPushSubscription{
		ID:        GlobalID(webPushSubscriptionType, subscription.ID.String()),
		Endpoint:  subscription.Endpoint,
		UserAgent: subscription.UserAgent,
		CreatedAt: subscription.CreatedAt,
	}
}
