package jikkyoapi_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/jikkyoclient"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/testutil/nxjikkyotest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/jikkyo"
	"github.com/SlashNephy/kotatsu/internal/web/jikkyoapi"
)

const (
	liveTimeout = 5 * time.Second
	liveTick    = 5 * time.Millisecond
	liveSecret  = "test-secret"
	statusKey   = "status"
	commentKey  = "comment"
	resultKey   = "postResult"
	connected   = "connected"
)

func alwaysUsable(context.Context, uuid.UUID) (bool, error) { return true, nil }

type liveFixture struct {
	pool   *pgxpool.Pool
	nx     *nxjikkyotest.Server
	server *httptest.Server
	alice  uuid.UUID
	bob    uuid.UUID
}

func newLiveFixture(t *testing.T) *liveFixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	nx := nxjikkyotest.New(t)
	table, err := jikkyo.NewTable(nil)
	require.NoError(t, err)
	client := jikkyoclient.NewClient(&jikkyoclient.ClientOptions{
		NXJikkyoURL: nx.URL, KakologURL: nx.URL, HTTPClient: httpclienttest.New(t), Clock: nil, LiveRetryDelays: []time.Duration{10 * time.Millisecond},
	})
	handler := jikkyoapi.NewLiveHandler(&jikkyoapi.LiveOptions{
		Pool: pool, Table: table, Client: client, Relay: jikkyo.NewRelayWithGrace(client, 10*time.Millisecond), Sessions: alwaysUsable,
		Secret: liveSecret, Clock: new(clock.Real), Logger: slog.New(slog.DiscardHandler), RevalidateInterval: 10 * time.Millisecond,
	})
	f := &liveFixture{pool: pool, nx: nx, server: nil, alice: id.New(), bob: id.New()}
	for _, user := range []struct {
		id   uuid.UUID
		name string
	}{{f.alice, "alice"}, {f.bob, "bob"}} {
		_, err = pool.Exec(t.Context(), `INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES ($1, $2, $2, 'member', now(), now())`, user.id, user.name)
		require.NoError(t, err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET "+jikkyoapi.LivePath, handler)
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := uuid.Parse(r.Header.Get("X-Test-User"))
		if err != nil {
			mux.ServeHTTP(w, r)

			return
		}
		mux.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), &auth.Principal{
			UserID: user, Role: domain.RoleMember, SessionID: uuid.Nil(), AuthenticatedAt: time.Now(), OAuthClientID: "",
		})))
	}))
	t.Cleanup(func() {
		f.server.CloseClientConnections()
		f.server.Close()
	})
	f.setSystem(t, true)
	f.setUser(t, f.alice, true)
	f.setUser(t, f.bob, true)

	return f
}

func (f *liveFixture) setSystem(t *testing.T, enabled bool) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO system_settings (id, recording_defaults, archive_retention_days, comment_sources, updated_at)
		VALUES (1, '{}', 30, CASE WHEN $1::boolean THEN ARRAY['nx_jikkyo'] ELSE '{}'::text[] END, now()) ON CONFLICT (id) DO UPDATE SET comment_sources = EXCLUDED.comment_sources`, enabled)
	require.NoError(t, err)
}

func (f *liveFixture) setUser(t *testing.T, user uuid.UUID, enabled bool) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO user_settings (user_id, comment_sources, updated_at) VALUES ($1, CASE WHEN $2::boolean THEN ARRAY['nx_jikkyo'] ELSE '{}'::text[] END, now())
		ON CONFLICT (user_id) DO UPDATE SET comment_sources = EXCLUDED.comment_sources`, user, enabled)
	require.NoError(t, err)
}

type liveMessage struct {
	Type    string `json:"type"`
	Kind    string `json:"kind"`
	Content string `json:"content"`
	UserID  string `json:"userId"`
	ID      int64  `json:"id"`
	Error   string `json:"error"`
}

// dial は、ユーザー user として接続する。
func (f *liveFixture) dial(t *testing.T, user uuid.UUID) *websocket.Conn {
	t.Helper()

	url := "ws" + strings.TrimPrefix(f.server.URL, "http") + "/internal/media/jikkyo/" + jkNHK + "/ws"
	conn, response, err := websocket.Dial(t.Context(), url, &websocket.DialOptions{HTTPClient: httpclienttest.New(t), HTTPHeader: http.Header{"X-Test-User": {user.String()}}})
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.CloseNow() })

	return conn
}

// read は、種類が kinds のどれかのメッセージが届くまで読む。
func read(t *testing.T, conn *websocket.Conn, kinds ...string) *liveMessage {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), liveTimeout)
	defer cancel()
	for {
		_, data, err := conn.Read(ctx)
		require.NoError(t, err)
		var message liveMessage
		require.NoError(t, json.Unmarshal(data, &message))
		if slices.Contains(kinds, message.Type) {
			return &message
		}
	}
}

func waitLiveConnected(t *testing.T, conn *websocket.Conn) {
	t.Helper()

	for read(t, conn, statusKey).Kind != connected {
		continue
	}
}

func post(t *testing.T, conn *websocket.Conn, id int, text string) *liveMessage {
	t.Helper()

	data, err := json.Marshal(map[string]any{"type": "post", "id": id, "text": text})
	require.NoError(t, err)
	require.NoError(t, conn.Write(t.Context(), websocket.MessageText, data))

	return read(t, conn, resultKey)
}

// TestLiveHandlerOptIn は、実況を使えないときは NX-Jikkyo につながないことを確かめる (設計書 0.3、7.4)。
func TestLiveHandlerOptIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		system     bool
		user       bool
		anonymous  bool
		path       string
		wantStatus int
	}{
		{name: "全体の設定で切っている", system: false, user: true, path: jkNHK, wantStatus: http.StatusNotFound},
		{name: "ユーザーの設定で切っている", system: true, user: false, path: jkNHK, wantStatus: http.StatusNotFound},
		{name: "対応表にないチャンネル", system: true, user: true, path: "evil", wantStatus: http.StatusNotFound},
		{name: "ログインしていない", system: true, user: true, anonymous: true, path: jkNHK, wantStatus: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newLiveFixture(t)
			f.setSystem(t, tt.system)
			f.setUser(t, f.alice, tt.user)
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+"/internal/media/jikkyo/"+tt.path+"/ws", http.NoBody)
			require.NoError(t, err)
			if !tt.anonymous {
				request.Header.Set("X-Test-User", f.alice.String())
			}
			response, err := httpclienttest.New(t).Do(request)
			require.NoError(t, err)
			t.Cleanup(func() { _ = response.Body.Close() })

			assert.Equal(t, tt.wantStatus, response.StatusCode)
			assert.Empty(t, f.nx.WatchCookies())
		})
	}
}

func TestLiveHandlerRelaysComments(t *testing.T) {
	t.Parallel()

	f := newLiveFixture(t)
	alice := f.dial(t, f.alice)
	waitLiveConnected(t, alice)
	bob := f.dial(t, f.bob)
	waitLiveConnected(t, bob)

	f.nx.Chat(t.Context(), "こんにちは", "someone")

	for _, conn := range []*websocket.Conn{alice, bob} {
		message := read(t, conn, commentKey)
		assert.Equal(t, "こんにちは", message.Content)
		assert.Equal(t, "someone", message.UserID)
	}
	assert.Len(t, f.nx.WatchCookies(), 1, "受信は、視聴者の間で 1 本の接続を共有する")
}

func TestLiveHandlerPost(t *testing.T) {
	t.Parallel()

	f := newLiveFixture(t)
	alice := f.dial(t, f.alice)
	waitLiveConnected(t, alice)
	bob := f.dial(t, f.bob)
	waitLiveConnected(t, bob)

	result := post(t, alice, 1, "やあ")

	assert.Equal(t, int64(1), result.ID)
	assert.Empty(t, result.Error)
	cookie := jikkyo.PosterCookie(liveSecret, f.alice)
	assert.Equal(t, []nxjikkyotest.Post{{Text: "やあ", Cookie: cookie}}, f.nx.Posts(), "投稿は、ユーザーごとの Cookie を付ける")
	// ほかの視聴者には届き、投稿者には返さない (画面が先に流すので、二重になる)。
	assert.Equal(t, nxjikkyotest.UserIDOf(cookie), read(t, bob, commentKey).UserID)
	f.nx.Chat(t.Context(), "次", "someone")
	assert.Equal(t, "次", read(t, alice, commentKey).Content, "投稿者に自分の投稿が届かず、次のコメントが届く")
}

func TestLiveHandlerPostValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
	}{
		{name: "空", text: "   "},
		{name: "長すぎる", text: strings.Repeat("あ", 76)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newLiveFixture(t)
			conn := f.dial(t, f.alice)
			waitLiveConnected(t, conn)

			assert.Equal(t, "COMMENT_INVALID", post(t, conn, 1, tt.text).Error)
			assert.Empty(t, f.nx.Posts())
		})
	}
}

func TestLiveHandlerRefusesRapidPosts(t *testing.T) {
	t.Parallel()

	f := newLiveFixture(t)
	conn := f.dial(t, f.alice)
	waitLiveConnected(t, conn)

	require.Empty(t, post(t, conn, 1, "一度目").Error)
	assert.Equal(t, "COMMENT_TOO_FAST", post(t, conn, 2, "二度目").Error)
	assert.Len(t, f.nx.Posts(), 1, "NX-Jikkyo の連投の制限に当たらないよう、ここで止める")
}

// TestLiveHandlerRefusesRapidPostsAcrossConnections は、接続を増やしても投稿の間隔を縮められないことを確かめる。
func TestLiveHandlerRefusesRapidPostsAcrossConnections(t *testing.T) {
	t.Parallel()

	f := newLiveFixture(t)
	first := f.dial(t, f.alice)
	waitLiveConnected(t, first)
	second := f.dial(t, f.alice)
	waitLiveConnected(t, second)
	other := f.dial(t, f.bob)
	waitLiveConnected(t, other)

	require.Empty(t, post(t, first, 1, "一度目").Error)
	assert.Equal(t, "COMMENT_TOO_FAST", post(t, second, 2, "別の接続から").Error)
	assert.Empty(t, post(t, other, 3, "ほかの人").Error, "制限はユーザーごとなので、ほかの人は投稿できる")
}

func TestLiveHandlerClosesWhenDisabled(t *testing.T) {
	t.Parallel()

	f := newLiveFixture(t)
	conn := f.dial(t, f.alice)
	waitLiveConnected(t, conn)

	f.setUser(t, f.alice, false)

	ctx, cancel := context.WithTimeout(t.Context(), liveTimeout)
	defer cancel()
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			assert.Equal(t, websocket.StatusPolicyViolation, websocket.CloseStatus(err))

			break
		}
	}
	require.Eventually(t, func() bool { return f.nx.CommentSessions() == 0 }, liveTimeout, liveTick, "最後の視聴者が抜けたら、NX-Jikkyo との接続も閉じる")
}
