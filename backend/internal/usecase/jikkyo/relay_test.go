package jikkyo_test

import (
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/jikkyoclient"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/testutil/nxjikkyotest"
	"github.com/SlashNephy/kotatsu/internal/usecase/jikkyo"
)

const (
	relayTimeout = 5 * time.Second
	relayTick    = 5 * time.Millisecond
)

func newRelay(t *testing.T, nx *nxjikkyotest.Server, grace time.Duration) *jikkyo.Relay {
	t.Helper()

	client := jikkyoclient.NewClient(&jikkyoclient.ClientOptions{
		NXJikkyoURL: nx.URL, KakologURL: nx.URL, HTTPClient: httpclienttest.New(t), Clock: nil, LiveRetryDelays: []time.Duration{10 * time.Millisecond},
	})

	return jikkyo.NewRelayWithGrace(client, grace)
}

// nextComment は、次のコメントを待って本文を返す。状態のイベントは読み飛ばす。
func nextComment(t *testing.T, subscription *jikkyo.Subscription) string {
	t.Helper()

	timeout := time.After(relayTimeout)
	for {
		select {
		case event := <-subscription.Events:
			if event.Comment != nil {
				return event.Comment.Content
			}
		case <-timeout:
			require.Fail(t, "timed out waiting for a comment")

			return ""
		}
	}
}

// waitConnected は、接続できたという状態が届くまで待つ。
func waitConnected(t *testing.T, subscription *jikkyo.Subscription) {
	t.Helper()

	timeout := time.After(relayTimeout)
	for {
		select {
		case event := <-subscription.Events:
			if event.Status != nil && event.Status.Kind == jikkyoclient.LiveConnected {
				return
			}
		case <-timeout:
			require.Fail(t, "timed out waiting for the connection")

			return
		}
	}
}

func TestRelaySharesOneUpstream(t *testing.T) {
	t.Parallel()

	nx := nxjikkyotest.New(t)
	relay := newRelay(t, nx, time.Hour)
	first := relay.Subscribe(t.Context(), "jk1")
	t.Cleanup(first.Close)
	waitConnected(t, first)
	// あとから来た視聴者にも、今の状態を知らせる。
	second := relay.Subscribe(t.Context(), "jk1")
	t.Cleanup(second.Close)
	waitConnected(t, second)

	nx.Chat(t.Context(), "みんなに", "someone")

	assert.Equal(t, "みんなに", nextComment(t, first))
	assert.Equal(t, "みんなに", nextComment(t, second))
	assert.Len(t, nx.WatchCookies(), 1, "同じチャンネルの視聴者で、上流の接続を共有する")
}

func TestRelayClosesAfterGrace(t *testing.T) {
	t.Parallel()

	nx := nxjikkyotest.New(t)
	relay := newRelay(t, nx, 10*time.Millisecond)
	subscription := relay.Subscribe(t.Context(), "jk1")
	waitConnected(t, subscription)

	subscription.Close()

	require.Eventually(t, func() bool { return nx.CommentSessions() == 0 }, relayTimeout, relayTick)
}

func TestRelayKeepsUpstreamWithinGrace(t *testing.T) {
	t.Parallel()

	nx := nxjikkyotest.New(t)
	relay := newRelay(t, nx, time.Hour)
	first := relay.Subscribe(t.Context(), "jk1")
	waitConnected(t, first)
	first.Close()

	second := relay.Subscribe(t.Context(), "jk1")
	t.Cleanup(second.Close)
	waitConnected(t, second)

	assert.Len(t, nx.WatchCookies(), 1, "猶予の間に戻ったら、つなぎ直さない")
}

func TestPosterCookie(t *testing.T) {
	t.Parallel()

	userA := uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	userB := uuid.MustParse("00000000-0000-0000-0000-00000000000b")

	first, second := jikkyo.PosterCookie("secret", userA), jikkyo.PosterCookie("secret", userA)
	assert.Equal(t, first, second, "同じユーザーなら、再接続しても同じ値にする")
	assert.NotEqual(t, jikkyo.PosterCookie("secret", userA), jikkyo.PosterCookie("secret", userB))
	assert.NotEqual(t, jikkyo.PosterCookie("secret", userA), jikkyo.PosterCookie("other", userA))
	assert.NotContains(t, jikkyo.PosterCookie("secret", userA), userA.String())
}
