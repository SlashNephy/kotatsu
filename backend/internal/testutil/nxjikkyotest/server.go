// Package nxjikkyotest は、テストで使う NX-Jikkyo の WebSocket の偽物を提供する。
package nxjikkyotest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const (
	fieldContent = "content"
	fieldData    = "data"
	fieldType    = "type"
	fieldMail    = "mail"
	fieldUserID  = "user_id"
)

// Post は、受け取った投稿。
type Post struct {
	Text   string
	Cookie string
}

// Server は、NX-Jikkyo の偽物。
type Server struct {
	// URL は、NXJikkyoURL に渡す URL。
	URL string

	mu sync.Mutex
	// watchCookies は、視聴セッションごとの NX-User-ID の Cookie の値。接続の数を数えるのにも使う。
	watchCookies []string
	watches      map[*websocket.Conn]struct{}
	comments     map[*websocket.Conn]struct{}
	posts        []Post
	keepSeats    int
	pongs        int
	// MessageServer は、空でなければ room で知らせるコメントセッションの接続先にする。
	messageServer string
	// PostError は、空でなければ投稿にこのエラーのコードを返す。
	postError string
	// backlog は、コメントセッションを開いたときに rf:0 より前に送るコメント。
	backlog []string
	// keepIntervalSec は、seat で知らせる keepSeat の間隔 (秒)。
	keepIntervalSec float64
	server          *httptest.Server
	closed          chan struct{}
}

// New は Server を返す。テストの終わりに閉じる。
func New(t *testing.T) *Server {
	t.Helper()

	s := &Server{
		URL: "", mu: sync.Mutex{}, watchCookies: nil, watches: map[*websocket.Conn]struct{}{}, comments: map[*websocket.Conn]struct{}{}, posts: nil, keepSeats: 0, pongs: 0,
		messageServer: "", postError: "", backlog: nil, keepIntervalSec: 1, server: nil, closed: make(chan struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/channels/{channel}/ws/watch", s.serveWatch)
	mux.HandleFunc("/api/v1/channels/{channel}/ws/comment", s.serveComment)
	s.server = httptest.NewServer(mux)
	s.URL = s.server.URL
	t.Cleanup(func() {
		close(s.closed)
		s.server.CloseClientConnections()
		s.server.Close()
	})

	return s
}

// SetMessageServer は、room で知らせるコメントセッションの接続先を変える。
func (s *Server) SetMessageServer(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageServer = uri
}

// SetKeepInterval は、seat で知らせる keepSeat の間隔 (秒) を変える。
func (s *Server) SetKeepInterval(seconds float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keepIntervalSec = seconds
}

// SetPostError は、投稿に返すエラーのコードを変える。
func (s *Server) SetPostError(code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.postError = code
}

// SetBacklog は、コメントセッションを開いたときに rf:0 より前に送るコメントを変える。
func (s *Server) SetBacklog(contents ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backlog = contents
}

// Chat は、コメントセッションを開いているすべての接続にコメントを送る。書き込まれた時刻は付けない。
func (s *Server) Chat(ctx context.Context, content, userID string) {
	s.broadcastChat(ctx, map[string]any{fieldContent: content, fieldMail: "184", fieldUserID: userID})
}

// ChatAt は、書き込まれた時刻 date (Unix 秒) と dateUsec (マイクロ秒) を、本物の NX-Jikkyo と同じく数で付けたコメントを送る (#1515)。
func (s *Server) ChatAt(ctx context.Context, content, userID string, date, dateUsec int64) {
	s.broadcastChat(ctx, map[string]any{fieldContent: content, fieldMail: "184", fieldUserID: userID, "date": date, "date_usec": dateUsec})
}

// DropAll は、すべての接続を切る。
func (s *Server) DropAll() {
	s.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(s.watches)+len(s.comments))
	for conn := range s.watches {
		conns = append(conns, conn)
	}
	for conn := range s.comments {
		conns = append(conns, conn)
	}
	s.mu.Unlock()
	for _, conn := range conns {
		_ = conn.CloseNow()
	}
}

// WatchCookies は、これまでの視聴セッションの Cookie の値を返す。
func (s *Server) WatchCookies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string{}, s.watchCookies...)
}

// CommentSessions は、開いているコメントセッションの数を返す。
func (s *Server) CommentSessions() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.comments)
}

// Posts は、受け取った投稿を返す。
func (s *Server) Posts() []Post {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]Post{}, s.posts...)
}

// KeepSeats は、受け取った keepSeat の数を返す。Pongs は、受け取った pong の数を返す。
func (s *Server) KeepSeats() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.keepSeats
}

// Pongs は、受け取った pong の数を返す。
func (s *Server) Pongs() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.pongs
}

// UserIDOf は、Cookie の値から偽物が作るユーザーの ID を返す。
func UserIDOf(cookie string) string {
	return "user:" + cookie
}

func cookieValue(r *http.Request) string {
	cookie, err := r.Cookie("NX-User-ID")
	if err != nil {
		return ""
	}

	return cookie.Value
}

func send(ctx context.Context, conn *websocket.Conn, message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return conn.Write(ctx, websocket.MessageText, data)
}

func (s *Server) keepInterval() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.keepIntervalSec
}

func (s *Server) serveWatch(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()
	cookie := cookieValue(r)
	s.mu.Lock()
	s.watchCookies = append(s.watchCookies, cookie)
	s.watches[conn] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.watches, conn)
		s.mu.Unlock()
	}()
	ctx := r.Context()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var message struct {
			Type string `json:"type"`
			Data struct {
				Text string `json:"text"`
			} `json:"data"`
		}
		if json.Unmarshal(data, &message) != nil {
			continue
		}
		if !s.handleWatch(ctx, conn, r, cookie, message.Type, message.Data.Text) {
			return
		}
	}
}

func (s *Server) handleWatch(ctx context.Context, conn *websocket.Conn, r *http.Request, cookie, kind, text string) bool {
	switch kind {
	case "startWatching":
		uri := s.commentURI(r)
		for _, message := range []any{
			map[string]any{fieldType: "seat", fieldData: map[string]any{"keepIntervalSec": s.keepInterval()}},
			map[string]any{fieldType: "ping"},
			map[string]any{fieldType: "room", fieldData: map[string]any{
				"messageServer": map[string]any{"uri": uri}, "threadId": "1", "yourPostKey": UserIDOf(cookie), "vposBaseTime": "2026-11-09T00:00:00+09:00",
			}},
		} {
			if send(ctx, conn, message) != nil {
				return false
			}
		}
	case "keepSeat":
		s.mu.Lock()
		s.keepSeats++
		s.mu.Unlock()
	case "pong":
		s.mu.Lock()
		s.pongs++
		s.mu.Unlock()
	case "postComment":
		s.mu.Lock()
		code := s.postError
		if code == "" {
			s.posts = append(s.posts, Post{Text: text, Cookie: cookie})
		}
		s.mu.Unlock()
		if code != "" {
			return send(ctx, conn, map[string]any{fieldType: "error", fieldData: map[string]any{"code": code}}) == nil
		}
		// 配信と投稿の結果の順は保証されないので、厳しいほうの順 (配信が先) にする。
		s.Chat(ctx, text, UserIDOf(cookie))
		if send(ctx, conn, map[string]any{fieldType: "postCommentResult"}) != nil {
			return false
		}
	}

	return true
}

func (s *Server) commentURI(r *http.Request) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.messageServer != "" {
		return s.messageServer
	}

	return "ws://" + r.Host + strings.TrimSuffix(r.URL.Path, "watch") + "comment"
}

func (s *Server) serveComment(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()
	ctx := r.Context()
	// 最初のメッセージ (thread と ping の配列) を待つ。
	if _, _, err := conn.Read(ctx); err != nil {
		return
	}
	s.mu.Lock()
	backlog := append([]string{}, s.backlog...)
	s.mu.Unlock()
	for _, content := range backlog {
		if send(ctx, conn, map[string]any{"chat": map[string]any{fieldContent: content, fieldMail: "", fieldUserID: "old"}}) != nil {
			return
		}
	}
	// rf:0 を送る前に、開いているコメントセッションに数える (#1622)。クライアントは rf:0 を受けて接続を知らせるので、
	// 送った後に数えると、知らせを受けたテストが CommentSessions を確かめたり Chat を送ったりするのに、数えるのが間に合わないことがある。
	// Chat を送るテストは、接続の知らせを待ってから送るので、Chat が rf:0 より先に届くことはない。
	s.mu.Lock()
	s.comments[conn] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.comments, conn)
		s.mu.Unlock()
	}()
	if send(ctx, conn, map[string]any{"ping": map[string]any{fieldContent: "rf:0"}}) != nil {
		return
	}
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

// broadcastChat は、コメントセッションを開いているすべての接続に chat を送る。
func (s *Server) broadcastChat(ctx context.Context, chat map[string]any) {
	data, err := json.Marshal(chat)
	if err != nil {
		return
	}
	data = []byte(`{"chat":` + string(data) + `}`)
	s.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(s.comments))
	for conn := range s.comments {
		conns = append(conns, conn)
	}
	s.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Write(ctx, websocket.MessageText, data)
	}
}
