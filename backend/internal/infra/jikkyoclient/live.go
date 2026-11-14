package jikkyoclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
)

const (
	// maxLiveMessageBytes は、NX-Jikkyo から受け取る WebSocket のメッセージの大きさの上限。
	maxLiveMessageBytes = 1 << 20
	// liveWriteTimeout は、NX-Jikkyo へメッセージを書くのを待つ時間の上限。
	liveWriteTimeout = 10 * time.Second
	// livePostTimeout は、投稿の結果を待つ時間の上限。視聴セッションがつながるまでの時間を含む。
	livePostTimeout = 10 * time.Second
	// defaultKeepSeatInterval は、NX-Jikkyo が keepSeat の間隔を知らせなかったときの間隔。
	defaultKeepSeatInterval = 30 * time.Second
	// minKeepSeatInterval と maxKeepSeatInterval は、NX-Jikkyo が知らせた keepSeat の間隔として受け入れる範囲。
	minKeepSeatInterval = time.Second
	maxKeepSeatInterval = 10 * time.Minute
	// nxUserIDCookie は、NX-Jikkyo がユーザーの ID を作るのに使う Cookie の名前。
	nxUserIDCookie = "NX-User-ID"

	liveFieldType    = "type"
	liveFieldData    = "data"
	liveFieldContent = "content"
	liveMessagePing  = "ping"

	reasonConnectionFailed = "CONNECTION_FAILED"
	reasonConnectionClosed = "CONNECTION_CLOSED"
	reasonError            = "ERROR"
	reasonDisconnected     = "DISCONNECTED"
)

// defaultLiveRetryDelays は、NX-Jikkyo との接続が切れてからつなぎ直すまでの間隔を返す。続けて切れるほど延ばす。
func defaultLiveRetryDelays() []time.Duration {
	return []time.Duration{3 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second}
}

// ErrLivePostRejected は、NX-Jikkyo が投稿を拒んだこと。
var ErrLivePostRejected = errors.New("NX-Jikkyo rejected the comment")

// ErrLiveNotConnected は、NX-Jikkyo につながっていないので投稿できないこと。
var ErrLiveNotConnected = errors.New("not connected to NX-Jikkyo")

//go:generate go tool go-enum --values --nocomments

// LiveStatusKind は、NX-Jikkyo との接続の状態。
//
// ENUM(
// connecting
// connected
// disconnected
// )
type LiveStatusKind string

// LiveStatus は、NX-Jikkyo との接続の状態の変化。
type LiveStatus struct {
	Kind LiveStatusKind
	// Reason は、切れた理由のコード。RetryIn は、つなぎ直すまでの秒。どちらも LiveStatusKindDisconnected のときだけ入れる。
	Reason  string
	RetryIn int
}

// LiveComment は、NX-Jikkyo から届いたコメント。
type LiveComment struct {
	Content string
	Mail    string
	UserID  string
	// PostedAt は、コメントが書き込まれた時刻 (NX-Jikkyo の date と date_usec、#1515)。届かなければ零値。
	PostedAt time.Time
}

// LiveOptions は、LiveSession の設定。
type LiveOptions struct {
	ChannelID string
	// Cookie は、投稿者を見分ける NX-User-ID の値。空なら送らない。
	Cookie string
	// Receive は、コメントセッションを開いてコメントを受け取るかどうか。投稿だけなら false にする。
	Receive bool
	// OnComment と OnStatus は、セッションのゴルーチンから順に呼ぶ。nil でもよい。
	OnComment func(*LiveComment)
	OnStatus  func(*LiveStatus)
}

// LiveSession は、NX-Jikkyo の視聴セッション (と、受け取るならコメントセッション) を保つ。切れたら、間隔を延ばしながらつなぎ直す。
type LiveSession struct {
	client *Client
	opts   LiveOptions
	cancel context.CancelFunc
	done   chan struct{}

	mu sync.Mutex
	// ready は、投稿できるようになったら閉じる。つなぎ直すたびに作り直す。
	ready chan struct{}
	// watch は、今の視聴セッション。vposBase は、投稿の vpos の基準の時刻。yourPostKey は、NX-Jikkyo がこの接続に割り当てた ID。
	watch       *websocket.Conn
	vposBase    time.Time
	yourPostKey string
	// postLock は、投稿を 1 つずつにする。pending は、結果を待っている投稿。
	postLock chan struct{}
	pending  chan error
}

// OpenLive は、セッションを始める。Close か parent の取り消しで止める。
func (c *Client) OpenLive(parent context.Context, opts *LiveOptions) *LiveSession {
	ctx, cancel := context.WithCancel(parent)
	s := &LiveSession{
		client: c, opts: *opts, cancel: cancel, done: make(chan struct{}),
		mu: sync.Mutex{}, ready: make(chan struct{}), watch: nil, vposBase: time.Time{}, yourPostKey: "",
		postLock: make(chan struct{}, 1), pending: nil,
	}
	go s.run(ctx)

	return s
}

// Close は、セッションを止める。
func (s *LiveSession) Close() {
	s.cancel()
}

// Wait は、セッションのゴルーチンが終わるまで待つ。
func (s *LiveSession) Wait() {
	<-s.done
}

// YourPostKey は、NX-Jikkyo がこの接続に割り当てた ID (投稿のコメントの user_id と同じ値) を返す。つながる前は空を返す。
func (s *LiveSession) YourPostKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.yourPostKey
}

// Ready は、投稿できるようになるまで待つ。
func (s *LiveSession) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, livePostTimeout)
	defer cancel()
	s.mu.Lock()
	ready := s.ready
	s.mu.Unlock()
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return ErrLiveNotConnected
	}
}

// Post は、コメントを匿名で投稿し、結果を待つ。つながるのも少し待つ。
func (s *LiveSession) Post(ctx context.Context, text string) error {
	ctx, cancel := context.WithTimeout(ctx, livePostTimeout)
	defer cancel()
	select {
	case s.postLock <- struct{}{}:
		defer func() { <-s.postLock }()
	case <-ctx.Done():
		return ErrLiveNotConnected
	}

	s.mu.Lock()
	ready := s.ready
	s.mu.Unlock()
	select {
	case <-ready:
	case <-ctx.Done():
		return ErrLiveNotConnected
	}

	result := make(chan error, 1)
	s.mu.Lock()
	watch := s.watch
	vpos := s.client.clock.Now().Sub(s.vposBase).Milliseconds() / 10
	s.pending = result
	s.mu.Unlock()
	defer s.forgetPost(result)
	if watch == nil {
		return ErrLiveNotConnected
	}
	if err := writeLive(ctx, watch, map[string]any{liveFieldType: "postComment", liveFieldData: map[string]any{
		"text": text, "color": "white", "position": "naka", "size": "medium", "vpos": vpos, "isAnonymous": true,
	}}); err != nil {
		return fmt.Errorf("failed to send the comment: %w", err)
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ErrLiveNotConnected
	}
}

func (s *LiveSession) run(ctx context.Context) {
	defer close(s.done)
	attempt := 0
	for {
		s.status(&LiveStatus{Kind: LiveStatusKindConnecting, Reason: "", RetryIn: 0})
		reason, connected := s.connect(ctx)
		if ctx.Err() != nil {
			return
		}
		if connected {
			attempt = 0
		}
		delays := s.client.liveRetryDelays
		delay := delays[min(attempt, len(delays)-1)]
		attempt++
		s.status(&LiveStatus{Kind: LiveStatusKindDisconnected, Reason: reason, RetryIn: int(delay.Round(time.Second) / time.Second)})
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()

			return
		case <-timer.C:
		}
	}
}

// liveFailureError は、接続を諦めた理由のコード。
type liveFailureError string

func (f liveFailureError) Error() string { return string(f) }

// connect は、1 回分の接続を、切れるまで保つ。切れた理由のコードと、一度でもつながったかどうかを返す。
func (s *LiveSession) connect(parent context.Context) (string, bool) {
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	watch, err := s.dial(ctx, s.client.liveURL(s.opts.ChannelID, "watch"))
	if err != nil {
		return reasonConnectionFailed, false
	}
	defer func() { _ = watch.CloseNow() }()
	watch.SetReadLimit(maxLiveMessageBytes)
	s.setWatch(watch)
	defer s.clearWatch()

	var routines sync.WaitGroup
	defer routines.Wait()
	defer cancel(nil)
	if err := writeLive(ctx, watch, map[string]any{liveFieldType: "startWatching", liveFieldData: map[string]any{"reconnect": false}}); err != nil {
		return reasonConnectionClosed, false
	}

	connection := &liveConnection{
		session: s, cancel: cancel, watch: watch, routines: &routines,
		cancelKeepSeat: nil, cancelComment: nil, mu: sync.Mutex{}, connected: false,
	}
	for {
		_, data, err := watch.Read(ctx)
		if err != nil {
			if failure, ok := errors.AsType[liveFailureError](context.Cause(ctx)); ok {
				return string(failure), connection.isConnected()
			}

			return reasonConnectionClosed, connection.isConnected()
		}
		var message liveWatchMessage
		if err := json.Unmarshal(data, &message); err != nil {
			continue
		}
		if reason := connection.handle(ctx, &message); reason != "" {
			return reason, connection.isConnected()
		}
	}
}

// liveWatchMessage は、視聴セッションで届くメッセージ。
type liveWatchMessage struct {
	Type string `json:"type"`
	Data struct {
		KeepIntervalSec *float64 `json:"keepIntervalSec"`
		ThreadID        string   `json:"threadId"`
		YourPostKey     string   `json:"yourPostKey"`
		VposBaseTime    string   `json:"vposBaseTime"`
		Code            string   `json:"code"`
		Reason          string   `json:"reason"`
	} `json:"data"`
}

// liveConnection は、1 回分の接続の状態。
type liveConnection struct {
	session  *LiveSession
	cancel   context.CancelCauseFunc
	watch    *websocket.Conn
	routines *sync.WaitGroup

	cancelKeepSeat context.CancelFunc
	cancelComment  context.CancelFunc

	mu        sync.Mutex
	connected bool
}

func (c *liveConnection) isConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.connected
}

// markConnected は、つながったことを記録して知らせる。
func (c *liveConnection) markConnected() {
	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()
	c.session.status(&LiveStatus{Kind: LiveStatusKindConnected, Reason: "", RetryIn: 0})
}

// handle は、視聴セッションのメッセージを処理する。接続をやめるなら、その理由のコードを返す。
func (c *liveConnection) handle(ctx context.Context, message *liveWatchMessage) string {
	switch message.Type {
	case "seat":
		c.startKeepSeat(ctx, message.Data.KeepIntervalSec)
	case liveMessagePing:
		if err := writeLive(ctx, c.watch, map[string]any{liveFieldType: "pong"}); err != nil {
			return reasonConnectionClosed
		}
	case "room":
		return c.handleRoom(ctx, message)
	case "postCommentResult":
		c.session.finishPost(nil)
	case "error":
		code := message.Data.Code
		if isLivePostError(code) && c.session.finishPost(fmt.Errorf("%w: %s", ErrLivePostRejected, code)) {
			return ""
		}
		if code == "" {
			return reasonError
		}

		return code
	case "disconnect":
		if message.Data.Reason == "" {
			return reasonDisconnected
		}

		return message.Data.Reason
	}

	return ""
}

func (c *liveConnection) handleRoom(parent context.Context, message *liveWatchMessage) string {
	base, err := time.Parse(time.RFC3339Nano, message.Data.VposBaseTime)
	if err != nil {
		base = c.session.client.clock.Now()
	}
	c.session.setRoom(base, message.Data.YourPostKey)
	if !c.session.opts.Receive {
		c.markConnected()

		return ""
	}
	// room が届き直したら (スレッドが変わったときなど)、前のコメントセッションを閉じてから開き直す。
	if c.cancelComment != nil {
		c.cancelComment()
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancelComment = cancel
	c.routines.Go(func() {
		c.runComment(ctx, message.Data.ThreadID, message.Data.YourPostKey)
	})

	return ""
}

// keepSeatInterval は、NX-Jikkyo が知らせた keepSeat の間隔を返す。
// 外部から届く値なので、換算後の値を確かめる。あふれて負になった値を time.NewTicker に渡すと、panic でプロセスごと落ちる。
func keepSeatInterval(seconds *float64) time.Duration {
	if seconds == nil {
		return defaultKeepSeatInterval
	}
	interval := time.Duration(*seconds * float64(time.Second))
	if interval < minKeepSeatInterval || interval > maxKeepSeatInterval {
		return defaultKeepSeatInterval
	}

	return interval
}

func (c *liveConnection) startKeepSeat(parent context.Context, seconds *float64) {
	interval := keepSeatInterval(seconds)
	if c.cancelKeepSeat != nil {
		c.cancelKeepSeat()
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancelKeepSeat = cancel
	c.routines.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := writeLive(ctx, c.watch, map[string]any{liveFieldType: "keepSeat"}); err != nil {
					c.cancel(liveFailureError(reasonConnectionClosed))

					return
				}
			}
		}
	})
}

// liveCommentMessage は、コメントセッションで届くメッセージ。
type liveCommentMessage struct {
	Ping *struct {
		Content string `json:"content"`
	} `json:"ping"`
	Chat *struct {
		Content  string  `json:"content"`
		Mail     string  `json:"mail"`
		UserID   string  `json:"user_id"`
		Date     flexInt `json:"date"`
		DateUsec flexInt `json:"date_usec"`
	} `json:"chat"`
}

// runComment は、コメントセッションを保ち、届いたコメントを渡す。切れたら、接続全体をやめて、つなぎ直しに任せる。
func (c *liveConnection) runComment(ctx context.Context, threadID, threadKey string) {
	lost := func() {
		// room が届き直して閉じたのなら、新しいコメントセッションが引き継ぐ。
		if ctx.Err() == nil {
			c.cancel(liveFailureError(reasonConnectionClosed))
		}
	}
	// 接続先は、視聴セッションで知らされた URI を使わず、自分で組み立てる。外部から届く値で接続先を決めないためである。
	comment, err := c.session.dial(ctx, c.session.client.liveURL(c.session.opts.ChannelID, "comment"))
	if err != nil {
		lost()

		return
	}
	defer func() { _ = comment.CloseNow() }()
	comment.SetReadLimit(maxLiveMessageBytes)
	if err := writeLive(ctx, comment, liveHandshake(threadID, threadKey)); err != nil {
		lost()

		return
	}
	// 最初に届く直近のコメントは、もう流れ終えたものなので流さない。rf:0 の ping で、届き終えたと分かる。
	caughtUp := false
	for {
		_, data, err := comment.Read(ctx)
		if err != nil {
			lost()

			return
		}
		var message liveCommentMessage
		if err := json.Unmarshal(data, &message); err != nil {
			continue
		}
		if message.Ping != nil && message.Ping.Content == "rf:0" {
			caughtUp = true
			c.markConnected()

			continue
		}
		if !caughtUp || message.Chat == nil {
			continue
		}
		if handler := c.session.opts.OnComment; handler != nil {
			handler(&LiveComment{
				Content: message.Chat.Content, Mail: message.Chat.Mail, UserID: message.Chat.UserID,
				PostedAt: postedAt(int64(message.Chat.Date), int64(message.Chat.DateUsec)),
			})
		}
	}
}

// liveHandshake は、コメントセッションを開いたときに送るメッセージを返す。直近の 100 件を求め、rf:0 の ping で、求めた分を送り終えたことを知らせてもらう。
func liveHandshake(threadID, threadKey string) []any {
	return []any{
		map[string]any{liveMessagePing: map[string]any{liveFieldContent: "rs:0"}},
		map[string]any{liveMessagePing: map[string]any{liveFieldContent: "ps:0"}},
		map[string]any{"thread": map[string]any{"version": "20061206", "thread": threadID, "threadkey": threadKey, "user_id": "", "res_from": -100}},
		map[string]any{liveMessagePing: map[string]any{liveFieldContent: "pf:0"}},
		map[string]any{liveMessagePing: map[string]any{liveFieldContent: "rf:0"}},
	}
}

func (s *LiveSession) status(status *LiveStatus) {
	if handler := s.opts.OnStatus; handler != nil {
		handler(status)
	}
}

func (s *LiveSession) setWatch(watch *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watch = watch
}

// setRoom は、投稿に要る値を覚えて、投稿できるようにする。
func (s *LiveSession) setRoom(base time.Time, yourPostKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vposBase = base
	s.yourPostKey = yourPostKey
	select {
	case <-s.ready:
	default:
		close(s.ready)
	}
}

// clearWatch は、切れた接続を忘れ、待っている投稿を失敗させる。
func (s *LiveSession) clearWatch() {
	s.finishPost(ErrLiveNotConnected)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watch = nil
	select {
	case <-s.ready:
		s.ready = make(chan struct{})
	default:
	}
}

// finishPost は、結果を待っている投稿に err を返す。待っている投稿がなければ false を返す。
func (s *LiveSession) finishPost(err error) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		return false
	}
	s.pending <- err
	s.pending = nil

	return true
}

// forgetPost は、結果を待つのをやめた投稿を忘れる。
func (s *LiveSession) forgetPost(result chan error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == result {
		s.pending = nil
	}
}

// dial は、NX-Jikkyo の WebSocket につなぐ。
func (s *LiveSession) dial(ctx context.Context, uri string) (*websocket.Conn, error) {
	header := http.Header{"User-Agent": {userAgent}}
	if s.opts.Cookie != "" {
		header.Set("Cookie", nxUserIDCookie+"="+s.opts.Cookie)
	}

	conn, response, err := websocket.Dial(netactivity.WithPurpose(ctx, netactivity.PurposeJikkyoComments), uri, &websocket.DialOptions{HTTPClient: s.client.http, HTTPHeader: header})
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NX-Jikkyo: %w", err)
	}

	return conn, nil
}

func writeLive(ctx context.Context, conn *websocket.Conn, message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to encode the message: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, liveWriteTimeout)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("failed to write the message: %w", err)
	}

	return nil
}

// isLivePostError は、エラーのコードが、投稿の失敗を表すかどうかを返す。
func isLivePostError(code string) bool {
	return strings.HasPrefix(code, "COMMENT_") || code == "INVALID_MESSAGE"
}

// liveBase は、NX-Jikkyo の WebSocket の接続先の URL を返す。
func (c *Client) liveBase() *url.URL {
	base, err := url.Parse(c.nxJikkyoURL)
	if err != nil {
		return new(url.URL)
	}
	if base.Scheme == "https" {
		base.Scheme = "wss"
	} else {
		base.Scheme = "ws"
	}

	return base
}

func (c *Client) liveURL(channelID, kind string) string {
	return c.liveBase().JoinPath("api", "v1", "channels", channelID, "ws", kind).String()
}

// postedAt は、コメントの date (Unix 秒) と date_usec (マイクロ秒) から、書き込まれた時刻を返す。date がなければ零値。
func postedAt(date, micros int64) time.Time {
	if date <= 0 {
		return time.Time{}
	}

	return time.Unix(date, micros*int64(time.Microsecond))
}

// flexInt は、数でも、数を書いた文字列でも読める整数。NX-Jikkyo は、同じ項目を、ライブでは数で、過去ログでは文字列で返す。
// 読めない値は 0 にし、コメントそのものは捨てない。
type flexInt int64

func (n *flexInt) UnmarshalJSON(data []byte) error {
	*n = 0
	if value, err := strconv.ParseInt(strings.Trim(string(data), `"`), 10, 64); err == nil {
		*n = flexInt(value)
	}

	return nil
}
