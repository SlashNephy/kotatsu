// Package simkltest は、テストで見たドラマの記録とウォッチリストの取り込みを確かめる、偽の Simkl を提供する (#1649)。
//
// Simkl の AUTH V2 (認可コードフローと PKCE、更新のトークン、取り消し) と、/users/settings、/sync/history、/sync/activities、
// /sync/all-items/shows をまねる。本物と同じく、更新すると前のアクセストークンをすぐに無効にし、すべての問い合わせに client_id、
// app-name、app-version を求める。認可のエンドポイントは、人の操作をまねず、その場でコードを作って redirect_uri に戻す。
package simkltest

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 応答の項目の名前。
const (
	keyCode  = "code"
	keyError = "error"
	keyIDs   = "ids"
	keyShow  = "show"
)

// AccessTokenTTL は、発行するアクセストークンの期限の長さ。本物と同じ 7 日。
const AccessTokenTTL = 7 * 24 * time.Hour

// Item は、ウォッチリストの 1 作品。
type Item struct {
	SimklID int64
	// TMDBID は、作品の TMDB の ID。0 なら、ID を返さない。
	TMDBID int32
	Status string
	// UpdatedAt は、ウォッチリストで最後に変わった日時。date_from で差分を返すのに使う。
	UpdatedAt time.Time
}

// Watched は、/sync/history で受け取った 1 話。
type Watched struct {
	TMDBID    string
	Season    int32
	Episode   int32
	WatchedAt string
}

// Server は、偽の Simkl。画面 (認可) と API を 1 つのオリジンで受ける。
type Server struct {
	ClientID     string
	ClientSecret string
	// Account は、/users/settings が返すアカウントの名前。
	Account string

	mu sync.Mutex
	// Scope は、発行するトークンの権限。空なら media:read media:write。
	Scope string
	// Items は、ウォッチリスト。
	Items []*Item
	// Activity と RemovedActivity は、/sync/activities が返す all と tv_shows.removed_from_list。
	Activity        string
	RemovedActivity string
	// UnknownSeries は、/sync/history で見つからないものとする TMDB の作品の ID。
	UnknownSeries []string
	// UnknownEpisodes は、/sync/history で見つからないものとする話 ("S1E3" の形)。
	UnknownEpisodes []string
	// FailNext は、次の API の問い合わせ (認可とトークンを除く) に返す状態コード。0 なら返さない。1 回だけ使う。
	FailNext int
	// RejectRefresh なら、トークンの更新を invalid_grant で断る。
	RejectRefresh bool
	// History は、受け取った見た話。
	History []*Watched
	// Requests は、受け取った API の問い合わせのパス (認可とトークンを含む)。
	Requests []string
	// Revoked は、取り消された更新のトークン。
	Revoked []string
	// Refreshes は、更新した回数。
	Refreshes int

	// redirectURIs は、アプリに登録したリダイレクト URI。本物と同じく、登録したものにだけ戻す。
	redirectURIs []string
	now          func() time.Time
	server       *httptest.Server
	codes        map[string]*grant
	access       map[string]*token
	refresh      map[string]string
	sequence     int
}

type grant struct {
	challenge   string
	redirectURI string
	scope       string
}

type token struct {
	refresh   string
	expiresAt time.Time
}

// New は、偽の Simkl を空いているポートで起動して返す。Close で止める。redirectURIs は、アプリに登録したリダイレクト URI。
func New(clientID, clientSecret string, redirectURIs ...string) *Server {
	s := newServer(clientID, clientSecret, redirectURIs)
	s.server = httptest.NewServer(s.Handler())

	return s
}

// NewOn は、偽の Simkl を listener で起動して返す。別のプロセスとして立てる偽の Simkl (internal/cmd/fakesimkl) が使う。
func NewOn(listener net.Listener, clientID, clientSecret string, redirectURIs ...string) *Server {
	s := newServer(clientID, clientSecret, redirectURIs)
	s.server = httptest.NewUnstartedServer(s.Handler())
	_ = s.server.Listener.Close()
	s.server.Listener = listener
	s.server.Start()

	return s
}

func newServer(clientID, clientSecret string, redirectURIs []string) *Server {
	return &Server{
		ClientID: clientID, ClientSecret: clientSecret, Account: "simkl-user", mu: sync.Mutex{}, Scope: "", Items: []*Item{},
		Activity: "2026-01-01T00:00:00Z", RemovedActivity: "2026-01-01T00:00:00Z", UnknownSeries: []string{}, UnknownEpisodes: []string{},
		FailNext: 0, RejectRefresh: false, History: []*Watched{}, Requests: []string{}, Revoked: []string{}, Refreshes: 0,
		redirectURIs: redirectURIs, now: time.Now, server: nil, codes: map[string]*grant{}, access: map[string]*token{}, refresh: map[string]string{}, sequence: 0,
	}
}

// URL は、偽の Simkl のオリジン。
func (s *Server) URL() string { return s.server.URL }

// Close は、偽の Simkl を止める。
func (s *Server) Close() { s.server.Close() }

// SetNow は、トークンの期限を決める時刻を変える。
func (s *Server) SetNow(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

// Lock と Unlock は、公開した項目をテストから変えるときに使う。
func (s *Server) Lock()   { s.mu.Lock() }
func (s *Server) Unlock() { s.mu.Unlock() }

// ExpireAccessTokens は、発行したアクセストークンをすべて期限切れにする。
func (s *Server) ExpireAccessTokens() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.access {
		t.expiresAt = time.Time{}
	}
}

// Handler は、偽の Simkl のハンドラーを返す。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /oauth2/authorize", s.authorize)
	mux.HandleFunc("POST /oauth2/token", s.token)
	mux.HandleFunc("POST /oauth2/revoke", s.revoke)
	mux.HandleFunc("GET /users/settings", s.api(s.settings))
	mux.HandleFunc("POST /sync/history", s.api(s.history))
	mux.HandleFunc("GET /sync/activities", s.api(s.activities))
	mux.HandleFunc("GET /sync/all-items/shows", s.api(s.allItems))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.Requests = append(s.Requests, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// 本物と同じく、アプリに登録したリダイレクト URI と 1 文字ずつ同じものにだけ戻す。戻す先は、登録した値から作る。
	index := slices.Index(s.redirectURIs, q.Get("redirect_uri"))
	if index < 0 || q.Get("client_id") != s.ClientID || q.Get("response_type") != keyCode ||
		q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		http.Error(w, "invalid_request", http.StatusBadRequest)

		return
	}
	registered := s.redirectURIs[index]
	redirectURI, err := url.Parse(registered)
	if err != nil {
		http.Error(w, "invalid_request", http.StatusBadRequest)

		return
	}
	code := random()
	s.mu.Lock()
	s.codes[code] = &grant{challenge: q.Get("code_challenge"), redirectURI: registered, scope: q.Get("scope")}
	s.mu.Unlock()
	values := redirectURI.Query()
	values.Set(keyCode, code)
	values.Set("state", q.Get("state"))
	values.Set("iss", s.server.URL)
	redirectURI.RawQuery = values.Encode()
	// 本物と同じく、認可を始めたときの redirect_uri に戻す。
	w.Header().Set("Location", redirectURI.String())
	w.WriteHeader(http.StatusFound)
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request")

		return
	}
	if r.PostForm.Get("client_id") != s.ClientID ||
		subtle.ConstantTimeCompare([]byte(r.PostForm.Get("client_secret")), []byte(s.ClientSecret)) != 1 {
		oauthError(w, http.StatusUnauthorized, "invalid_client")

		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		code := r.PostForm.Get(keyCode)
		g, ok := s.codes[code]
		// 本物と同じく、コードは引き換えに失敗しても使えなくなる。
		delete(s.codes, code)
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if !ok || g.redirectURI != r.PostForm.Get("redirect_uri") || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			oauthError(w, http.StatusBadRequest, "invalid_grant")

			return
		}
		refresh := "simkl_rt_" + random()
		s.issue(w, refresh, "")
	case "refresh_token":
		refresh := r.PostForm.Get("refresh_token")
		previous, ok := s.refresh[refresh]
		if !ok || s.RejectRefresh {
			oauthError(w, http.StatusBadRequest, "invalid_grant")

			return
		}
		s.Refreshes++
		s.issue(w, refresh, previous)
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type")
	}
}

// issue は、更新のトークン refresh に、新しいアクセストークンを発行する。前のアクセストークン previous は、すぐに使えなくする。
func (s *Server) issue(w http.ResponseWriter, refresh, previous string) {
	delete(s.access, previous)
	s.sequence++
	access := fmt.Sprintf("simkl_at_%d_%s", s.sequence, random())
	s.access[access] = &token{refresh: refresh, expiresAt: s.now().Add(AccessTokenTTL)}
	s.refresh[refresh] = access
	scope := s.Scope
	if scope == "" {
		scope = "media:read media:write"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": access, "token_type": "Bearer", "expires_in": int(AccessTokenTTL.Seconds()), "refresh_token": refresh, "scope": scope,
	})
}

func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusOK)

		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	revoked := r.PostForm.Get("token")
	s.Revoked = append(s.Revoked, revoked)
	if access, ok := s.refresh[revoked]; ok {
		delete(s.access, access)
		delete(s.refresh, revoked)
	}
	w.WriteHeader(http.StatusOK)
}

// api は、API の問い合わせを、アプリの印 (client_id、app-name、app-version) とアクセストークンを確かめてから handler に渡す。
func (s *Server) api(handler func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("client_id") != s.ClientID || q.Get("app-name") == "" || q.Get("app-version") == "" || r.Header.Get("User-Agent") == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{keyError: "client_id_failed", keyCode: http.StatusBadRequest})

			return
		}
		s.mu.Lock()
		access, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		t, known := s.access[access]
		valid := ok && known && s.now().Before(t.expiresAt)
		fail := s.FailNext
		s.FailNext = 0
		s.mu.Unlock()
		if !valid {
			writeJSON(w, http.StatusUnauthorized, map[string]any{keyError: "user_token_failed", keyCode: http.StatusUnauthorized})

			return
		}
		if fail != 0 {
			if fail == http.StatusTooManyRequests {
				w.Header().Set("Retry-After", "120")
			}
			writeJSON(w, fail, map[string]any{keyError: "injected", keyCode: fail})

			return
		}
		handler(w, r)
	}
}

func (s *Server) settings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": map[string]any{"name": s.Account}, "account": map[string]any{"id": 1, "type": "free"}})
}

type historyRequest struct {
	Shows []struct {
		IDs     map[string]string `json:"ids"`
		Seasons []struct {
			Number   int32 `json:"number"`
			Episodes []struct {
				Number    int32  `json:"number"`
				WatchedAt string `json:"watched_at"`
			} `json:"episodes"`
		} `json:"seasons"`
	} `json:"shows"`
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	var body historyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{keyError: "bad_body"})

		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	notFoundShows := []any{}
	notFoundEpisodes := []any{}
	statuses := []any{}
	added := 0
	for _, show := range body.Shows {
		tmdb := show.IDs["tmdb"]
		if slices.Contains(s.UnknownSeries, tmdb) {
			notFoundShows = append(notFoundShows, map[string]any{keyIDs: show.IDs})

			continue
		}
		for _, season := range show.Seasons {
			for _, episode := range season.Episodes {
				key := fmt.Sprintf("S%dE%d", season.Number, episode.Number)
				if slices.Contains(s.UnknownEpisodes, key) {
					notFoundEpisodes = append(notFoundEpisodes, map[string]any{"season": season.Number, "number": episode.Number})

					continue
				}
				s.History = append(s.History, &Watched{TMDBID: tmdb, Season: season.Number, Episode: episode.Number, WatchedAt: episode.WatchedAt})
				added++
			}
		}
		// 本物と同じく、見た記録を付けた作品を「見ている」に入れる。
		statuses = append(statuses, map[string]any{"request": show.IDs, "response": map[string]any{"status": "watching", "simkl_type": "tv"}})
		var id int32
		if _, err := fmt.Sscan(tmdb, &id); err == nil && added > 0 && !slices.ContainsFunc(s.Items, func(i *Item) bool { return i.TMDBID == id }) {
			s.sequence++
			s.Items = append(s.Items, &Item{SimklID: int64(100000 + s.sequence), TMDBID: id, Status: "watching", UpdatedAt: s.now()})
			s.Activity = s.now().UTC().Format(time.RFC3339)
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"added":     map[string]any{"episodes": added, "statuses": statuses},
		"not_found": map[string]any{"shows": notFoundShows, "episodes": notFoundEpisodes, "movies": []any{}},
	})
}

func (s *Server) activities(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"all": s.Activity, "tv_shows": map[string]any{"all": s.Activity, "removed_from_list": s.RemovedActivity},
	})
}

func (s *Server) allItems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var since time.Time
	if from := q.Get("date_from"); from != "" {
		parsed, err := time.Parse(time.RFC3339, from)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{keyError: "wrong_parameter"})

			return
		}
		since = parsed
	}
	idsOnly := q.Get("extended") == "simkl_ids_only"
	s.mu.Lock()
	defer s.mu.Unlock()
	shows := []any{}
	for _, item := range s.Items {
		if !since.IsZero() && !item.UpdatedAt.After(since) {
			continue
		}
		ids := map[string]any{"simkl": item.SimklID, "slug": "show-" + strconv.FormatInt(item.SimklID, 10)}
		if idsOnly {
			shows = append(shows, map[string]any{keyShow: map[string]any{keyIDs: ids}})

			continue
		}
		if item.TMDBID > 0 {
			ids["tmdb"] = strconv.Itoa(int(item.TMDBID))
		}
		shows = append(shows, map[string]any{"status": item.Status, keyShow: map[string]any{"title": keyShow, keyIDs: ids}})
	}
	if len(shows) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{})

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shows": shows})
}

func oauthError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{keyError: code, "error_description": code})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func random() string {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)

	return base64.RawURLEncoding.EncodeToString(raw)
}
