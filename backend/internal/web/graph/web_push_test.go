package graph_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	webPushPublicKey     = `{ webPushPublicKey }`
	webPushSubscriptions = `{ webPushSubscriptions { id endpoint userAgent } }`
	subscribeWebPush     = `mutation($input: WebPushSubscriptionInput!) { subscribeWebPush(input: $input) { id endpoint userAgent } }`
	unsubscribeWebPush   = `mutation($endpoint: String!) { unsubscribeWebPush(endpoint: $endpoint) }`
	pushEndpoint         = "https://push.example/send/kotatsu"
	varEndpoint          = "endpoint"
)

type webPushSubscriptionNode struct {
	ID        string `json:"id"`
	Endpoint  string `json:"endpoint"`
	UserAgent string `json:"userAgent"`
}

// browserSubscription は、ブラウザの PushSubscription.toJSON() と同じ形の購読を作る。
func browserSubscription(t *testing.T, endpoint string) map[string]any {
	t.Helper()

	private, err := ecdh.P256().GenerateKey(rand.Reader)
	require.NoError(t, err)
	authSecret := make([]byte, 16)
	_, err = rand.Read(authSecret)
	require.NoError(t, err)

	return map[string]any{
		varEndpoint: endpoint,
		"p256dh":    base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()),
		"auth":      base64.RawURLEncoding.EncodeToString(authSecret),
	}
}

func (f *fixture) webPushSubscriptions(t *testing.T) []webPushSubscriptionNode {
	t.Helper()

	var got struct {
		WebPushSubscriptions []webPushSubscriptionNode `json:"webPushSubscriptions"`
	}
	require.Empty(t, f.query(t, webPushSubscriptions, nil, &got))

	return got.WebPushSubscriptions
}

// TestWebPush は、どのロールのユーザーも、VAPID の公開鍵を得て、自分の端末で購読し、解除できることを確かめる (設計書 6.2)。
func TestWebPush(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	viewer := f.as(t, "yuzu", auth.RoleViewer)

	var key struct {
		WebPushPublicKey string `json:"webPushPublicKey"`
	}
	require.Empty(t, viewer.query(t, webPushPublicKey, nil, &key))
	publicKey, err := base64.RawURLEncoding.DecodeString(key.WebPushPublicKey)
	require.NoError(t, err)
	_, err = ecdh.P256().NewPublicKey(publicKey)
	require.NoError(t, err, "the key is a P-256 public key the browser can use as applicationServerKey")

	var subscribed struct {
		SubscribeWebPush webPushSubscriptionNode `json:"subscribeWebPush"`
	}
	require.Empty(t, viewer.query(t, subscribeWebPush, map[string]any{varInput: browserSubscription(t, pushEndpoint)}, &subscribed))
	assert.Equal(t, pushEndpoint, subscribed.SubscribeWebPush.Endpoint)
	assert.Equal(t, testUserAgent, subscribed.SubscribeWebPush.UserAgent)
	assert.Equal(t, []webPushSubscriptionNode{subscribed.SubscribeWebPush}, viewer.webPushSubscriptions(t))
	// 購読は、ユーザーごとに持つ。
	assert.Empty(t, f.webPushSubscriptions(t))

	// 同じ端末で別のユーザーがログインして購読し直したら、そのユーザーの購読に移る。
	require.Empty(t, f.query(t, subscribeWebPush, map[string]any{varInput: browserSubscription(t, pushEndpoint)}, nil))
	assert.Empty(t, viewer.webPushSubscriptions(t))
	assert.Len(t, f.webPushSubscriptions(t), 1)

	var unsubscribed struct {
		UnsubscribeWebPush bool `json:"unsubscribeWebPush"`
	}
	require.Empty(t, viewer.query(t, unsubscribeWebPush, map[string]any{varEndpoint: pushEndpoint}, &unsubscribed))
	assert.False(t, unsubscribed.UnsubscribeWebPush, "another user's subscription is not removed")
	require.Empty(t, f.query(t, unsubscribeWebPush, map[string]any{varEndpoint: pushEndpoint}, &unsubscribed))
	assert.True(t, unsubscribed.UnsubscribeWebPush)
	assert.Empty(t, f.webPushSubscriptions(t))
}

// TestSubscribeWebPushRejectsInvalidInput は、正しくない購読を BAD_USER_INPUT で拒むことを確かめる。
func TestSubscribeWebPushRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		modify func(input map[string]any)
	}{
		{name: "http の送り先", modify: func(input map[string]any) { input[varEndpoint] = "http://push.example/send" }},
		{name: "base64url でない鍵", modify: func(input map[string]any) { input["p256dh"] = "not base64!" }},
		{name: "base64url でない認証用の秘密", modify: func(input map[string]any) { input["auth"] = "not base64!" }},
		{name: "短い認証用の秘密", modify: func(input map[string]any) { input["auth"] = "AAAA" }},
	}
	f := newFixture(t, nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := browserSubscription(t, pushEndpoint)
			tt.modify(input)
			assert.Equal(t, []string{string(graph.CodeBadUserInput)}, f.query(t, subscribeWebPush, map[string]any{varInput: input}, nil))
		})
	}
}
