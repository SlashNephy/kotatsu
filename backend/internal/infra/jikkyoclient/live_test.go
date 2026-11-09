package jikkyoclient_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/jikkyoclient"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/testutil/nxjikkyotest"
)

const (
	liveTimeout = 5 * time.Second
	liveTick    = 5 * time.Millisecond
)

// liveRecorder は、セッションから届いたものを覚える。
type liveRecorder struct {
	mu       sync.Mutex
	comments []string
	statuses []jikkyoclient.LiveStatus
}

func (r *liveRecorder) options(receive bool, cookie string) *jikkyoclient.LiveOptions {
	return &jikkyoclient.LiveOptions{
		ChannelID: "jk1", Cookie: cookie, Receive: receive,
		OnComment: func(comment *jikkyoclient.LiveComment) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.comments = append(r.comments, comment.Content)
		},
		OnStatus: func(status *jikkyoclient.LiveStatus) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.statuses = append(r.statuses, *status)
		},
	}
}

func (r *liveRecorder) gotComments() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string{}, r.comments...)
}

func (r *liveRecorder) count(kind jikkyoclient.LiveStatusKind) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, status := range r.statuses {
		if status.Kind == kind {
			count++
		}
	}

	return count
}

func openLive(t *testing.T, nx *nxjikkyotest.Server, opts *jikkyoclient.LiveOptions) *jikkyoclient.LiveSession {
	t.Helper()

	client := jikkyoclient.NewClient(&jikkyoclient.ClientOptions{
		NXJikkyoURL: nx.URL, KakologURL: nx.URL, HTTPClient: httpclienttest.New(t), Clock: nil, LiveRetryDelays: []time.Duration{10 * time.Millisecond},
	})
	session := client.OpenLive(t.Context(), opts)
	t.Cleanup(func() {
		session.Close()
		session.Wait()
	})

	return session
}

func TestLiveSessionReceivesCommentsAfterCaughtUp(t *testing.T) {
	t.Parallel()

	nx := nxjikkyotest.New(t)
	nx.SetBacklog("もう流れた")
	recorder := new(liveRecorder)
	openLive(t, nx, recorder.options(true, ""))
	require.Eventually(t, func() bool { return nx.CommentSessions() == 1 }, liveTimeout, liveTick)
	require.Eventually(t, func() bool { return recorder.count(jikkyoclient.LiveConnected) == 1 }, liveTimeout, liveTick)

	nx.Chat(t.Context(), "こんにちは", "someone")

	require.Eventually(t, func() bool { return len(recorder.gotComments()) == 1 }, liveTimeout, liveTick)
	assert.Equal(t, []string{"こんにちは"}, recorder.gotComments())
	assert.Equal(t, []string{""}, nx.WatchCookies())
	// 座席を保ち、ping に答える。
	require.Eventually(t, func() bool { return nx.KeepSeats() > 0 && nx.Pongs() > 0 }, liveTimeout, liveTick)
}

// TestLiveSessionToleratesKeepInterval は、NX-Jikkyo が知らせる間隔がどんな値でも、落ちずにつながり続けることを確かめる。
func TestLiveSessionToleratesKeepInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		seconds float64
	}{
		{name: "換算するとあふれる", seconds: 1e300},
		{name: "負の値", seconds: -1},
		{name: "ゼロ", seconds: 0},
		{name: "短すぎる", seconds: 1e-12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nx := nxjikkyotest.New(t)
			nx.SetKeepInterval(tt.seconds)
			recorder := new(liveRecorder)
			openLive(t, nx, recorder.options(true, ""))

			require.Eventually(t, func() bool { return recorder.count(jikkyoclient.LiveConnected) == 1 }, liveTimeout, liveTick)
			assert.Zero(t, nx.KeepSeats(), "短すぎる間隔や不正な間隔では、既定の間隔で送る")
		})
	}
}

func TestLiveSessionRefusesForeignMessageServer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		uri  string
	}{
		{name: "別のホスト", uri: "ws://example.invalid/api/v1/channels/jk1/ws/comment"},
		{name: "別のパス", uri: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nx := nxjikkyotest.New(t)
			uri := tt.uri
			if uri == "" {
				uri = "ws://" + nx.URL[len("http://"):] + "/other"
			}
			nx.SetMessageServer(uri)
			recorder := new(liveRecorder)
			openLive(t, nx, recorder.options(true, ""))

			require.Eventually(t, func() bool { return recorder.count(jikkyoclient.LiveDisconnected) > 0 }, liveTimeout, liveTick)
			assert.Equal(t, 0, recorder.count(jikkyoclient.LiveConnected))
			assert.Equal(t, 0, nx.CommentSessions())
		})
	}
}

func TestLiveSessionReconnects(t *testing.T) {
	t.Parallel()

	nx := nxjikkyotest.New(t)
	recorder := new(liveRecorder)
	openLive(t, nx, recorder.options(true, ""))
	require.Eventually(t, func() bool { return recorder.count(jikkyoclient.LiveConnected) == 1 }, liveTimeout, liveTick)

	nx.DropAll()

	require.Eventually(t, func() bool { return recorder.count(jikkyoclient.LiveConnected) == 2 }, liveTimeout, liveTick)
	assert.Positive(t, recorder.count(jikkyoclient.LiveDisconnected))
}

func TestLiveSessionPost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		postError string
		wantErr   error
		wantPosts []nxjikkyotest.Post
	}{
		{name: "Cookie を付けて投稿する", wantPosts: []nxjikkyotest.Post{{Text: "やあ", Cookie: "poster"}}},
		{name: "拒まれたら理由を返す", postError: "COMMENT_POST_NOT_ALLOWED", wantErr: jikkyoclient.ErrLivePostRejected, wantPosts: []nxjikkyotest.Post{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nx := nxjikkyotest.New(t)
			nx.SetPostError(tt.postError)
			session := openLive(t, nx, new(liveRecorder).options(false, "poster"))

			err := session.Post(t.Context(), "やあ")

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Contains(t, err.Error(), tt.postError)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantPosts, nx.Posts())
			assert.Equal(t, 0, nx.CommentSessions(), "投稿だけの接続は、コメントセッションを開かない")
			assert.Equal(t, nxjikkyotest.UserIDOf("poster"), session.YourPostKey())
		})
	}
}
