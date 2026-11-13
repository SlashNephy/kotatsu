package jikkyoapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/jikkyoclient"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/jikkyo"
)

const (
	// LivePath は、ライブの実況のコメントを中継する WebSocket のパス。
	LivePath = "/internal/media/jikkyo/{channel}/ws"

	// maxPostRunes は、投稿できるコメントの長さの上限。画面の入力の上限と同じにする。
	maxPostRunes = 75
	// minPostInterval は、1 人のユーザーが続けて投稿するときの間隔の下限。
	// NX-Jikkyo は 0.5 秒以内の連投を黙って捨て、続くと接続が切れるまで投稿を受け付けなくなるので、ここで止める。
	minPostInterval = time.Second
	// maxClientMessageBytes は、画面から受け取るメッセージの大きさの上限。
	maxClientMessageBytes = 4 << 10
	// clientWriteTimeout は、画面へメッセージを書くのを待つ時間の上限。
	clientWriteTimeout = 10 * time.Second
	// defaultRevalidateInterval は、接続中のセッションと実況の設定を確かめ直す間隔。
	defaultRevalidateInterval = 15 * time.Second

	postErrorInvalid = "COMMENT_INVALID"
	postErrorTooFast = "COMMENT_TOO_FAST"
	postErrorFailed  = "COMMENT_POST_FAILED"
)

// SessionChecker は、接続中のセッションが今も使えるかを確かめる。
type SessionChecker func(ctx context.Context, sessionID uuid.UUID) (bool, error)

// LiveOptions は、LiveHandler の設定。
type LiveOptions struct {
	Pool     *pgxpool.Pool
	Table    *jikkyo.Table
	Client   *jikkyoclient.Client
	Relay    *jikkyo.Relay
	Sessions SessionChecker
	// Secret は、投稿者を見分ける値を作るアプリの秘密キー。
	Secret string
	Clock  clock.Clock
	Logger *slog.Logger
	// RevalidateInterval は、テストで短くする。0 なら既定の間隔にする。
	RevalidateInterval time.Duration
}

// LiveHandler は、ライブの実況のコメントの受信と投稿を、WebSocket で中継する (GET /internal/media/jikkyo/{channel}/ws、設計書 7.4)。
// ブラウザを NX-Jikkyo につながせないのは、視聴者の IP アドレスと見ているチャンネルを、第三者に渡さないためである。
// 全体の設定とユーザーの設定の両方で実況を有効にしていなければ、404 を返し、NX-Jikkyo にはつながない。
type LiveHandler struct {
	opts    LiveOptions
	queries *query.Queries

	mu sync.Mutex
	// lastPosts は、ユーザーごとの最後の投稿の時刻。接続を増やしても、投稿の間隔は縮められない。
	lastPosts map[uuid.UUID]time.Time
}

var _ http.Handler = new(LiveHandler)

// NewLiveHandler は LiveHandler を返す。
func NewLiveHandler(opts *LiveOptions) *LiveHandler {
	o := *opts
	if o.RevalidateInterval <= 0 {
		o.RevalidateInterval = defaultRevalidateInterval
	}

	return &LiveHandler{opts: o, queries: query.New(o.Pool), mu: sync.Mutex{}, lastPosts: map[uuid.UUID]time.Time{}}
}

type clientMessage struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
	Text string `json:"text"`
}

type serverMessage struct {
	Type string `json:"type"`
	// status のときの項目。
	Kind    string `json:"kind,omitempty"`
	Reason  string `json:"reason,omitempty"`
	RetryIn int    `json:"retryIn,omitempty"`
	// comment のときの項目。
	Content string `json:"content,omitempty"`
	Mail    string `json:"mail,omitempty"`
	UserID  string `json:"userId,omitempty"`
	// postResult のときの項目。
	ID    int64  `json:"id,omitempty"`
	Error string `json:"error,omitempty"`
}

func (h *LiveHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFrom(r.Context())
	if principal == nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

		return
	}
	if !principal.Role.Can(domain.PermissionWatch) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)

		return
	}
	access, err := h.queries.GetJikkyoAccess(r.Context(), principal.UserID)
	if err != nil {
		h.opts.Logger.ErrorContext(r.Context(), "failed to read whether jikkyo is enabled", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}
	channelID, known := h.opts.Table.Channel(r.PathValue("channel"))
	if !access.Allowed() || !known {
		http.NotFound(w, r)

		return
	}
	// 同じオリジンかどうかは、手前の RequireSameOriginUpgrade が、リバースプロキシを考えて確かめている。
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		h.opts.Logger.WarnContext(r.Context(), "failed to accept the jikkyo WebSocket", slog.Any("error", err))

		return
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(maxClientMessageBytes)

	viewer := &liveViewer{
		handler: h, conn: conn, principal: principal, channelID: channelID,
		poster: nil, ownUserID: atomic.Pointer[string]{}, posting: atomic.Bool{}, routines: sync.WaitGroup{},
	}
	viewer.serve(r.Context())
}

// reservePost は、ユーザーが今投稿してよいかを返し、よければ投稿の時刻を記録する。
func (h *LiveHandler) reservePost(userID uuid.UUID, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if last, ok := h.lastPosts[userID]; ok && now.Sub(last) < minPostInterval {
		return false
	}
	// 間隔を過ぎた行は、もう判定に使わないので捨てる。
	for user, last := range h.lastPosts {
		if now.Sub(last) >= minPostInterval {
			delete(h.lastPosts, user)
		}
	}
	h.lastPosts[userID] = now

	return true
}

// liveViewer は、1 つの WebSocket の接続。
type liveViewer struct {
	handler   *LiveHandler
	conn      *websocket.Conn
	principal *auth.Principal
	channelID string

	// poster は、この接続の投稿に使う NX-Jikkyo の接続。初めて投稿するときに開く。投稿は同時に 1 つなので、ロックは要らない。
	poster *jikkyoclient.LiveSession
	// ownUserID は、この接続の投稿者の、NX-Jikkyo での ID。自分の投稿は画面が先に流すので、届いても返さない。
	ownUserID atomic.Pointer[string]
	posting   atomic.Bool
	routines  sync.WaitGroup
}

func (v *liveViewer) serve(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	subscription := v.handler.opts.Relay.Subscribe(ctx, v.channelID)
	defer subscription.Close()
	defer func() {
		cancel()
		v.routines.Wait()
		if v.poster != nil {
			v.poster.Close()
		}
	}()

	v.routines.Go(func() { v.forward(ctx, cancel, subscription) })
	v.routines.Go(func() { v.revalidate(ctx, cancel) })

	for {
		_, data, err := v.conn.Read(ctx)
		if err != nil {
			return
		}
		var message clientMessage
		if err := json.Unmarshal(data, &message); err != nil || message.Type != "post" {
			continue
		}
		v.post(ctx, &message)
	}
}

// forward は、届いたイベントを画面へ書く。
func (v *liveViewer) forward(ctx context.Context, cancel context.CancelFunc, subscription *jikkyo.Subscription) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-subscription.Events:
			message := v.messageOf(event)
			if message == nil {
				continue
			}
			if err := v.write(ctx, message); err != nil {
				cancel()

				return
			}
		}
	}
}

func (v *liveViewer) messageOf(event *jikkyo.Event) *serverMessage {
	if event.Status != nil {
		return &serverMessage{
			Type: "status", Kind: string(event.Status.Kind), Reason: event.Status.Reason, RetryIn: event.Status.RetryIn,
			Content: "", Mail: "", UserID: "", ID: 0, Error: "",
		}
	}
	if own := v.ownUserID.Load(); own != nil && *own != "" && *own == event.Comment.UserID {
		return nil
	}

	return &serverMessage{
		Type: "comment", Kind: "", Reason: "", RetryIn: 0,
		Content: event.Comment.Content, Mail: event.Comment.Mail, UserID: event.Comment.UserID, ID: 0, Error: "",
	}
}

// revalidate は、接続中に、セッションと実況の設定を確かめ直す。ログアウトしたり、実況を切ったりしたら、外部との通信を続けない。
func (v *liveViewer) revalidate(ctx context.Context, cancel context.CancelFunc) {
	ticker := time.NewTicker(v.handler.opts.RevalidateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if v.stillAllowed(ctx) {
			continue
		}
		_ = v.conn.Close(websocket.StatusPolicyViolation, "jikkyo is no longer available")
		cancel()

		return
	}
}

// stillAllowed は、接続を続けてよいかどうかを返す。DB に届かないときは、確かめられないので続ける。
func (v *liveViewer) stillAllowed(ctx context.Context) bool {
	h := v.handler
	usable, err := h.opts.Sessions(ctx, v.principal.SessionID)
	if err != nil {
		h.opts.Logger.WarnContext(ctx, "failed to revalidate the session of a jikkyo connection", slog.Any("error", err))

		return true
	}
	if !usable {
		return false
	}
	access, err := h.queries.GetJikkyoAccess(ctx, v.principal.UserID)
	if err != nil {
		h.opts.Logger.WarnContext(ctx, "failed to read whether jikkyo is enabled", slog.Any("error", err))

		return true
	}

	return access.Allowed()
}

// post は、投稿を受け付ける。結果は、投稿を待たずに次のメッセージを読めるよう、別のゴルーチンで返す。
func (v *liveViewer) post(ctx context.Context, message *clientMessage) {
	text := strings.TrimSpace(message.Text)
	if text == "" || utf8.RuneCountInString(text) > maxPostRunes {
		v.writePostResult(ctx, message.ID, postErrorInvalid)

		return
	}
	if !v.posting.CompareAndSwap(false, true) {
		v.writePostResult(ctx, message.ID, postErrorTooFast)

		return
	}
	if !v.handler.reservePost(v.principal.UserID, v.handler.opts.Clock.Now()) {
		v.posting.Store(false)
		v.writePostResult(ctx, message.ID, postErrorTooFast)

		return
	}
	v.routines.Go(func() {
		defer v.posting.Store(false)
		v.writePostResult(ctx, message.ID, v.send(ctx, text))
	})
}

// send は、NX-Jikkyo へ投稿し、失敗の理由のコードを返す。成功なら空を返す。
func (v *liveViewer) send(ctx context.Context, text string) string {
	h := v.handler
	if v.poster == nil {
		v.poster = h.opts.Client.OpenLive(ctx, &jikkyoclient.LiveOptions{
			ChannelID: v.channelID, Cookie: jikkyo.PosterCookie(h.opts.Secret, v.principal.UserID), Receive: false, OnComment: nil, OnStatus: nil,
		})
	}
	// 共有の受信の接続から、投稿の結果より先に自分の投稿が届くことがあるので、投稿する前に自分の ID を覚える。
	if err := v.poster.Ready(ctx); err != nil {
		return postErrorFailed
	}
	own := v.poster.YourPostKey()
	v.ownUserID.Store(&own)
	err := v.poster.Post(ctx, text)
	if err == nil {
		return ""
	}
	if errors.Is(err, jikkyoclient.ErrLivePostRejected) {
		_, code, _ := strings.Cut(err.Error(), ": ")

		return code
	}
	if ctx.Err() == nil {
		h.opts.Logger.WarnContext(ctx, "failed to post a jikkyo comment", slog.String("channel", v.channelID), slog.Any("error", err))
	}

	return postErrorFailed
}

func (v *liveViewer) writePostResult(ctx context.Context, id int64, code string) {
	_ = v.write(ctx, &serverMessage{
		Type: "postResult", Kind: "", Reason: "", RetryIn: 0, Content: "", Mail: "", UserID: "", ID: id, Error: code,
	})
}

func (v *liveViewer) write(ctx context.Context, message *serverMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to encode the message: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, clientWriteTimeout)
	defer cancel()

	if err := v.conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("failed to write the message: %w", err)
	}

	return nil
}
