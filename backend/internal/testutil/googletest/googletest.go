// Package googletest は、テストと検証のためのモックの Google (OAuth 2.0 の認可とトークン、Drive API の読み取り) を提供する (#1902)。
//
// 認可のエンドポイントは、人の操作をまねずに、その場でコードを作って redirect_uri に戻す。トークンの API は、クライアントの資格、
// redirect_uri、PKCE (S256) の code_verifier を確認する。Drive API は、Bearer のアクセストークンを確認し、files.list、files.get、
// alt=media と Range による読み出し、drives.list、about.get を返す。上限に当たった応答 (429 と 403) を差し込める。
package googletest

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DriveScope は、Kotatsu が求めるスコープ。
	DriveScope = "https://www.googleapis.com/auth/drive.readonly"
	// FolderMIMEType は、フォルダの MIME タイプ。
	FolderMIMEType = "application/vnd.google-apps.folder"
	// MyDriveID は、マイドライブの根元の別名。
	MyDriveID = "root"
	// SharedDriveRootName は、files.get が共有ドライブの根元に返す名前。共有ドライブの名前ではない。
	SharedDriveRootName = "Drive"
	// keyCode と keyName は、JSON の項目の名前。
	keyCode = "code"
	keyName = "name"
)

// File は、モックの Drive のファイルかフォルダ。
type File struct {
	ID       string
	Name     string
	MIMEType string
	// Parent は、親のフォルダの ID。マイドライブの直下なら root、共有ドライブの直下なら共有ドライブの ID。
	Parent       string
	Content      []byte
	ModifiedTime time.Time
	Trashed      bool
}

// Drive は、モックの共有ドライブ。
type Drive struct {
	ID   string
	Name string
}

// Fault は、Drive API に差し込む失敗。
type Fault string

const (
	// FaultRateLimit は、429 を返す。
	FaultRateLimit Fault = "rate_limit"
	// FaultUserRateLimit は、403 の userRateLimitExceeded を返す。
	FaultUserRateLimit Fault = "user_rate_limit"
	// FaultDownloadQuota は、403 の downloadQuotaExceeded を返す。
	FaultDownloadQuota Fault = "download_quota"
	// FaultLongRateLimit は、429 を、Retry-After を 30 秒にして返す。
	FaultLongRateLimit Fault = "long_rate_limit"
	// FaultIgnoreRange は、alt=media の Range を無視して、頭から 200 で返す。
	FaultIgnoreRange Fault = "ignore_range"
	// FaultUnauthorized は、アクセストークンの期限が切れたとして 401 を返す。
	FaultUnauthorized Fault = "unauthorized"
	// FaultCut は、alt=media の本文を、決まった大きさで切る (つながりが切れたのをまねる)。
	FaultCut Fault = "cut"
)

// Server は、モックの Google。
type Server struct {
	ClientID     string
	ClientSecret string
	// RedirectURIs は、クライアントに登録したリダイレクト URI。本物と同じく、認可の画面はこの中のどれかにだけ戻す。
	RedirectURIs []string
	// Account は、about.get が返すメールアドレス。
	Account string
	// AccessTokenTTL は、アクセストークンの期限。
	AccessTokenTTL time.Duration
	// OmitRefreshToken は、認可コードの引き換えで、リフレッシュトークンを返さないかどうか。
	OmitRefreshToken bool
	// CutAfter は、FaultCut で本文を切る大きさ。
	CutAfter int

	mu       sync.Mutex
	codes    map[string]*grant
	refresh  map[string]bool
	access   map[string]time.Time
	files    map[string]*File
	drives   []*Drive
	faults   []Fault
	requests []string
	ranges   []string
	refreshN int
	revoked  []string
	now      func() time.Time
}

type grant struct {
	challenge   string
	redirectURI string
	scope       string
}

// New は、クライアント clientID と clientSecret を登録したモックの Google を返す。
func New(clientID, clientSecret string) *Server {
	return &Server{
		ClientID: clientID, ClientSecret: clientSecret, RedirectURIs: []string{}, Account: "admin@example.com",
		AccessTokenTTL: time.Hour, OmitRefreshToken: false, CutAfter: 1024,
		mu: sync.Mutex{}, codes: map[string]*grant{}, refresh: map[string]bool{}, access: map[string]time.Time{},
		files: map[string]*File{
			// マイドライブの根元は、本物と同じく、別名 root で引けるフォルダにする。
			MyDriveID: {
				ID: MyDriveID, Name: "My Drive", MIMEType: FolderMIMEType, Parent: "", Content: nil,
				ModifiedTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Trashed: false,
			},
		},
		drives: []*Drive{}, faults: []Fault{}, requests: []string{}, ranges: []string{},
		refreshN: 0, revoked: []string{}, now: time.Now,
	}
}

// SetNow は、トークンの期限を決める時計を差し替える。
func (s *Server) SetNow(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

// AddFile は、ファイルかフォルダを足す。
func (s *Server) AddFile(f *File) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[f.ID] = f
}

// AddDrive は、共有ドライブを足す。
func (s *Server) AddDrive(d *Drive) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drives = append(s.drives, d)
	// 共有ドライブの根元は、本物と同じく、共有ドライブの ID で引けるフォルダにする。本物の files.get は、共有ドライブの名前でなく、
	// 決まった名前 (日本語では「ドライブ」) を返すので、それをまねる (#1902)。
	s.files[d.ID] = &File{
		ID: d.ID, Name: SharedDriveRootName, MIMEType: FolderMIMEType, Parent: "", Content: nil,
		ModifiedTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Trashed: false,
	}
}

// Inject は、次の Drive API の呼び出しに、失敗 faults を順に差し込む。
func (s *Server) Inject(faults ...Fault) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults = append(s.faults, faults...)
}

// Requests は、Drive API に届いたリクエストのパスを、届いた順に返す。
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.requests)
}

// Ranges は、alt=media に届いた Range ヘッダーを、届いた順に返す。
func (s *Server) Ranges() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.ranges)
}

// Refreshes は、リフレッシュトークンでアクセストークンを作り直した回数を返す。
func (s *Server) Refreshes() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.refreshN
}

// Revoked は、取り消されたトークンを返す。
func (s *Server) Revoked() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.revoked)
}

// RevokeAll は、発行したリフレッシュトークンをすべて使えなくする (利用者が Google でアクセスを取り消したのをまねる)。
func (s *Server) RevokeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.refresh)
	clear(s.access)
}

// ExpireAccessTokens は、発行したアクセストークンをすべて期限切れにする。
func (s *Server) ExpireAccessTokens() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.access)
}

// IssueAccessToken は、認可を経ずにアクセストークンを発行する。Drive API のクライアントだけを確認するテストで使う。
func (s *Server) IssueAccessToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.issueAccess()
}

// Handler は、モックの Google の HTTP ハンドラーを返す。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /o/oauth2/v2/auth", s.authorize)
	mux.HandleFunc("POST /token", s.token)
	mux.HandleFunc("POST /revoke", s.revoke)
	mux.HandleFunc("GET /drive/v3/about", s.drive(s.about))
	mux.HandleFunc("GET /drive/v3/drives", s.drive(s.listDrives))
	mux.HandleFunc("GET /drive/v3/drives/{id}", s.drive(s.getDrive))
	mux.HandleFunc("GET /drive/v3/files", s.drive(s.listFiles))
	mux.HandleFunc("GET /drive/v3/files/{id}", s.drive(s.getFile))

	return mux
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// 本物の Google と同じく、登録したリダイレクト URI と 1 文字ずつ同じものにだけ戻す。戻す先は、登録した値から作る。
	index := slices.Index(s.RedirectURIs, q.Get("redirect_uri"))
	switch {
	case q.Get("client_id") != s.ClientID:
		http.Error(w, "unknown client", http.StatusBadRequest)
	case index < 0:
		http.Error(w, "redirect_uri_mismatch", http.StatusBadRequest)
	case q.Get("response_type") != keyCode || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "":
		http.Error(w, "invalid request", http.StatusBadRequest)
	case q.Get("access_type") != "offline":
		http.Error(w, "access_type must be offline", http.StatusBadRequest)
	case !slices.Contains(strings.Fields(q.Get("scope")), DriveScope):
		http.Error(w, "invalid scope", http.StatusBadRequest)
	default:
		registered := s.RedirectURIs[index]
		target, err := url.Parse(registered)
		if err != nil {
			http.Error(w, "invalid redirect_uri", http.StatusBadRequest)

			return
		}
		code := random()
		s.mu.Lock()
		s.codes[code] = &grant{challenge: q.Get("code_challenge"), redirectURI: registered, scope: q.Get("scope")}
		s.mu.Unlock()
		target.RawQuery = url.Values{keyCode: {code}, "state": {q.Get("state")}, "scope": {q.Get("scope")}}.Encode()
		w.Header().Set("Location", target.String())
		w.WriteHeader(http.StatusFound)
	}
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, "invalid_request")

		return
	}
	if r.PostForm.Get("client_id") != s.ClientID || r.PostForm.Get("client_secret") != s.ClientSecret {
		oauthError(w, "invalid_client")

		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		g, ok := s.codes[r.PostForm.Get("code")]
		delete(s.codes, r.PostForm.Get("code"))
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if !ok || g.redirectURI != r.PostForm.Get("redirect_uri") || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			oauthError(w, "invalid_grant")

			return
		}
		body := map[string]any{"access_token": s.issueAccess(), "expires_in": int(s.AccessTokenTTL.Seconds()), "token_type": "Bearer", "scope": g.scope}
		if !s.OmitRefreshToken {
			refresh := random()
			s.refresh[refresh] = true
			body["refresh_token"] = refresh
		}
		writeJSON(w, body)
	case "refresh_token":
		if !s.refresh[r.PostForm.Get("refresh_token")] {
			oauthError(w, "invalid_grant")

			return
		}
		s.refreshN++
		writeJSON(w, map[string]any{"access_token": s.issueAccess(), "expires_in": int(s.AccessTokenTTL.Seconds()), "token_type": "Bearer"})
	default:
		oauthError(w, "unsupported_grant_type")
	}
}

// issueAccess は、アクセストークンを発行する。s.mu を持って呼ぶ。
func (s *Server) issueAccess() string {
	token := random()
	s.access[token] = s.now().Add(s.AccessTokenTTL)

	return token
}

func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, "invalid_request")

		return
	}
	token := r.PostForm.Get("token")
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.refresh, token)
	delete(s.access, token)
	s.revoked = append(s.revoked, token)
	w.WriteHeader(http.StatusOK)
}

// drive は、Drive API のハンドラーに、アクセストークンの確認と、差し込んだ失敗を付ける。
func (s *Server) drive(next func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r.URL.Path)
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		expiry, ok := s.access[token]
		valid := ok && s.now().Before(expiry)
		var fault Fault
		if valid && len(s.faults) > 0 {
			fault, s.faults = s.faults[0], s.faults[1:]
		}
		s.mu.Unlock()
		switch {
		case !valid:
			apiError(w, http.StatusUnauthorized, "authError")
		case fault == FaultRateLimit:
			w.Header().Set("Retry-After", "1")
			apiError(w, http.StatusTooManyRequests, "rateLimitExceeded")
		case fault == FaultLongRateLimit:
			w.Header().Set("Retry-After", "30")
			apiError(w, http.StatusTooManyRequests, "rateLimitExceeded")
		case fault == FaultUnauthorized:
			apiError(w, http.StatusUnauthorized, "authError")
		case fault == FaultIgnoreRange:
			r.Header.Del("Range")
			next(w, r)
		case fault == FaultUserRateLimit:
			apiError(w, http.StatusForbidden, "userRateLimitExceeded")
		case fault == FaultDownloadQuota:
			apiError(w, http.StatusForbidden, "downloadQuotaExceeded")
		case fault == FaultCut:
			next(&cutWriter{ResponseWriter: w, left: s.CutAfter}, r)
		default:
			next(w, r)
		}
	}
}

// cutWriter は、決まった大きさを書いたら、それ以上を書かずに破棄する。本文の長さが足りなくなり、受け取る側では途中で切れる。
type cutWriter struct {
	http.ResponseWriter

	left int
}

func (c *cutWriter) Write(p []byte) (int, error) {
	if c.left <= 0 {
		return 0, http.ErrHandlerTimeout
	}
	n := min(len(p), c.left)
	c.left -= n
	if _, err := c.ResponseWriter.Write(p[:n]); err != nil {
		return 0, fmt.Errorf("failed to write: %w", err)
	}
	if n < len(p) {
		return n, http.ErrHandlerTimeout
	}

	return n, nil
}

func (s *Server) about(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"user": map[string]any{"displayName": "Admin", "emailAddress": s.Account}})
}

func (s *Server) listDrives(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	drives := make([]map[string]any, 0, len(s.drives))
	for _, d := range s.drives {
		drives = append(drives, map[string]any{"id": d.ID, keyName: d.Name, "createdTime": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	}
	writeJSON(w, map[string]any{"drives": drives})
}

func (s *Server) getDrive(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.drives {
		if d.ID == r.PathValue("id") {
			writeJSON(w, map[string]any{"id": d.ID, keyName: d.Name, "createdTime": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})

			return
		}
	}
	apiError(w, http.StatusNotFound, "notFound")
}

// parentsQuery は、files.list の q のうち、親を指定する部分。
var parentsQuery = regexp.MustCompile(`^'([^']+)' in parents and trashed = false$`)

// driveOf は、ファイル f のある共有ドライブの ID を返す。マイドライブなら空。s.mu を持って呼ぶ。
func (s *Server) driveOf(f *File) string {
	for range 64 {
		for _, d := range s.drives {
			if f.ID == d.ID {
				return d.ID
			}
		}
		parent, ok := s.files[f.Parent]
		if !ok {
			return ""
		}
		f = parent
	}

	return ""
}

// searchQuery は、files.list の q のうち、名前で探す部分。
var searchQuery = regexp.MustCompile(`^name contains '((?:[^'\\]|\\.)*)' and mimeType != '` + regexp.QuoteMeta(FolderMIMEType) + `' and trashed = false$`)

// searchFiles は、名前に語を含むファイルを、大文字と小文字を区別せずに探す。共有ドライブを指定されたら、その中だけを探す。
// 1 件ずつのページに分ける。
func (s *Server) searchFiles(w http.ResponseWriter, q url.Values, match []string) {
	word := strings.ToLower(strings.NewReplacer(`\'`, `'`, `\\`, `\`).Replace(match[1]))
	s.mu.Lock()
	defer s.mu.Unlock()
	found := []*File{}
	for _, f := range s.files {
		if f.Trashed || f.MIMEType == FolderMIMEType || !strings.Contains(strings.ToLower(f.Name), word) {
			continue
		}
		drive := s.driveOf(f)
		if (q.Get("corpora") == "drive" && drive != q.Get("driveId")) || (q.Get("corpora") == "user" && drive != "") {
			continue
		}
		found = append(found, f)
	}
	slices.SortFunc(found, func(a, b *File) int { return strings.Compare(a.ID, b.ID) })
	start, _ := strconv.Atoi(q.Get("pageToken"))
	page := map[string]any{"files": []any{}}
	if start < len(found) {
		page["files"] = []any{s.fileJSON(found[start])}
		if start+1 < len(found) {
			page["nextPageToken"] = strconv.Itoa(start + 1)
		}
	}
	writeJSON(w, page)
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if search := searchQuery.FindStringSubmatch(q.Get("q")); search != nil {
		s.searchFiles(w, q, search)

		return
	}
	match := parentsQuery.FindStringSubmatch(q.Get("q"))
	if match == nil || q.Get("supportsAllDrives") != "true" || q.Get("includeItemsFromAllDrives") != "true" {
		apiError(w, http.StatusBadRequest, "invalidQuery")

		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	children := []*File{}
	for _, f := range s.files {
		if f.Parent == match[1] && !f.Trashed {
			children = append(children, f)
		}
	}
	// 同じ名前が並んでも、ページをまたいで順が変わらないよう、ID でも並べる。
	slices.SortFunc(children, func(a, b *File) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}

		return strings.Compare(a.ID, b.ID)
	})
	// 1 件ずつのページに分けて、ページをたどることを確かめられるようにする。
	start, _ := strconv.Atoi(q.Get("pageToken"))
	page := map[string]any{"files": []any{}}
	if start < len(children) {
		page["files"] = []any{s.fileJSON(children[start])}
		if start+1 < len(children) {
			page["nextPageToken"] = strconv.Itoa(start + 1)
		}
	}
	writeJSON(w, page)
}

func (s *Server) getFile(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	f, ok := s.files[r.PathValue("id")]
	var body map[string]any
	if ok {
		body = s.fileJSON(f)
	}
	s.mu.Unlock()
	if !ok {
		apiError(w, http.StatusNotFound, "notFound")

		return
	}
	if r.URL.Query().Get("alt") != "media" {
		writeJSON(w, body)

		return
	}
	s.mu.Lock()
	s.ranges = append(s.ranges, r.Header.Get("Range"))
	s.mu.Unlock()
	start, end, ok := parseRange(r.Header.Get("Range"), int64(len(f.Content)))
	if !ok {
		w.Header().Set("Content-Range", "bytes */"+strconv.Itoa(len(f.Content)))
		apiError(w, http.StatusRequestedRangeNotSatisfiable, "requestedRangeNotSatisfiable")

		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	if r.Header.Get("Range") != "" {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(f.Content)))
		w.WriteHeader(http.StatusPartialContent)
	}
	_, _ = w.Write(f.Content[start : end+1])
}

// parseRange は、Range (bytes=N- か bytes=N-M) を読む。なければ全体にする。
func parseRange(header string, size int64) (int64, int64, bool) {
	if header == "" {
		return 0, size - 1, size > 0
	}
	spec, ok := strings.CutPrefix(header, "bytes=")
	if !ok {
		return 0, 0, false
	}
	first, last, ok := strings.Cut(spec, "-")
	if !ok {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(first, 10, 64)
	if err != nil || start >= size {
		return 0, 0, false
	}
	end := size - 1
	if last != "" {
		if end, err = strconv.ParseInt(last, 10, 64); err != nil || end < start {
			return 0, 0, false
		}
		end = min(end, size-1)
	}

	return start, end, true
}

// fileJSON は、ファイルの情報を返す。親と、ファイルのある共有ドライブも付ける。s.mu を持って呼ぶ。
func (s *Server) fileJSON(f *File) map[string]any {
	body := map[string]any{
		"id": f.ID, keyName: f.Name, "mimeType": f.MIMEType, "modifiedTime": f.ModifiedTime, "trashed": f.Trashed,
		"parents": []string{}, "driveId": s.driveOf(f),
	}
	if f.Parent != "" {
		body["parents"] = []string{f.Parent}
	}
	if f.MIMEType != FolderMIMEType {
		body["size"] = strconv.Itoa(len(f.Content))
	}

	return body
}

func apiError(w http.ResponseWriter, status int, reason string) {
	writeStatusJSON(w, status, map[string]any{
		"error": map[string]any{keyCode: status, "message": reason, "errors": []any{map[string]any{"reason": reason}}},
	})
}

func oauthError(w http.ResponseWriter, code string) {
	status := http.StatusBadRequest
	if code == "invalid_client" {
		status = http.StatusUnauthorized
	}
	writeStatusJSON(w, status, map[string]any{"error": code})
}

func writeJSON(w http.ResponseWriter, body any) {
	writeStatusJSON(w, http.StatusOK, body)
}

func writeStatusJSON(w http.ResponseWriter, status int, body any) {
	data, err := json.Marshal(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func random() string {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)

	return base64.RawURLEncoding.EncodeToString(raw)
}
