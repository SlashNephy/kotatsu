package dramarecord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
)

const (
	// simklWebURL は、Simkl の画面のオリジン。利用者を認可の画面へ送り出す。認可の応答の iss もこの値になる。
	simklWebURL = "https://simkl.com"
	// simklAPIURL は、Simkl の API のオリジン。トークンの引き換えと更新、取り消しもここで行う。
	simklAPIURL = "https://api.simkl.com"
	// simklAppName は、Simkl がすべての問い合わせに求める app-name。
	simklAppName = "kotatsu"
	// simklScope は、求める権限。見た話を記録するので、書き込みの権限 (media:write) が要る。
	simklScope = "media:read media:write"
	// simklWriteScope は、書き込みの権限。トークンの応答の scope に、これがあることを確認する。
	simklWriteScope = "media:write"
	// paramClientID は、アプリを示す client_id の名前。
	paramClientID = "client_id"
)

// Simkl は、Simkl への記録先 (#1649)。Simkl の AUTH V2 (OAuth 2.0 と PKCE) で連携し、見た話は /sync/history に送る。
type Simkl struct {
	client *http.Client
	webURL string
	apiURL string
}

var _ Destination = new(Simkl)

// NewSimkl は Simkl を返す。baseURL が空でなければ、画面と API のすべての問い合わせをそのオリジンへ向ける (検証用)。
func NewSimkl(client *http.Client, baseURL string) *Simkl {
	if baseURL != "" {
		base := strings.TrimRight(baseURL, "/")

		return &Simkl{client: client, webURL: base, apiURL: base}
	}

	return &Simkl{client: client, webURL: simklWebURL, apiURL: simklAPIURL}
}

func (s *Simkl) Name() string { return DestinationSimkl }

func (s *Simkl) Issuer() string { return s.webURL }

func (s *Simkl) AuthCodeURL(app *App, redirectURI, state, verifier string) string {
	query := url.Values{
		"response_type": {"code"}, paramClientID: {app.ClientID}, "redirect_uri": {redirectURI}, "scope": {simklScope},
		"state": {state}, "code_challenge": {oauth2.S256ChallengeFromVerifier(verifier)}, "code_challenge_method": {"S256"},
	}

	return s.webURL + "/oauth2/authorize?" + query.Encode()
}

func (s *Simkl) Exchange(ctx context.Context, app *App, redirectURI, code, verifier string) (*Token, error) {
	token, err := s.token(ctx, app, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI}, "code_verifier": {verifier},
	})
	if err != nil {
		return nil, err
	}

	return token, nil
}

func (s *Simkl) Refresh(ctx context.Context, app *App, refreshToken string) (*Token, error) {
	token, err := s.token(ctx, app, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}})
	if err != nil {
		var status *statusError
		// 更新のトークンが拒否された (invalid_grant など) か、アプリのシークレットが拒否されたら、連携し直すまで変わらない。
		if errors.As(err, &status) && (status.code == http.StatusBadRequest || status.code == http.StatusUnauthorized) {
			return nil, fmt.Errorf("%w: %w", ErrReauthorizationRequired, err)
		}

		return nil, err
	}
	// Simkl の更新のトークンは回さない (同じ値を返す) が、返さなければ、前のものを使い続ける。
	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}

	return token, nil
}

func (s *Simkl) AccountName(ctx context.Context, app *App, accessToken string) (string, error) {
	var settings struct {
		User struct {
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := s.call(ctx, app, accessToken, http.MethodGet, "/users/settings", nil, nil, &settings); err != nil {
		return "", err
	}
	if settings.User.Name == "" {
		return "", fmt.Errorf("%w: the account has no name", ErrPermanent)
	}

	return settings.User.Name, nil
}

func (s *Simkl) Revoke(ctx context.Context, app *App, token string) error {
	form := url.Values{paramClientID: {app.ClientID}, "client_secret": {app.ClientSecret}, "token": {token}}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL+"/oauth2/revoke", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create the revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	res, err := s.client.Do(req)
	if err != nil {
		return errors.New("failed to send the revoke request")
	}
	_ = res.Body.Close()
	// 取り消しのエンドポイントは、取り消せたかによらず 200 を返す (RFC 7009)。ほかの状態コードだけを失敗とする。
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("the revoke endpoint returned %d", res.StatusCode)
	}

	return nil
}

// historyEpisode と historySeason は、/sync/history に送る話とシーズン。
type historyEpisode struct {
	Number    int32  `json:"number"`
	WatchedAt string `json:"watched_at"`
}

type historySeason struct {
	Number   int32            `json:"number"`
	Episodes []historyEpisode `json:"episodes"`
}

type historyShow struct {
	IDs     map[string]string `json:"ids"`
	Seasons []historySeason   `json:"seasons"`
}

// historyResponse は、/sync/history の応答。
type historyResponse struct {
	Added struct {
		Statuses []struct {
			Response struct {
				Status any `json:"status"`
			} `json:"response"`
		} `json:"statuses"`
	} `json:"added"`
	NotFound struct {
		Shows    []json.RawMessage `json:"shows"`
		Episodes []struct {
			Season  *int32 `json:"season"`
			Number  *int32 `json:"number"`
			Episode *int32 `json:"episode"`
		} `json:"episodes"`
	} `json:"not_found"`
}

func (s *Simkl) RecordEpisodes(ctx context.Context, app *App, accessToken string, seriesID int32, episodes []*Episode) (*Recorded, error) {
	seasons := []historySeason{}
	for _, episode := range episodes {
		watched := historyEpisode{Number: episode.Number, WatchedAt: episode.WatchedAt.UTC().Format(time.RFC3339)}
		index := slices.IndexFunc(seasons, func(s historySeason) bool { return s.Number == episode.Season })
		if index < 0 {
			seasons = append(seasons, historySeason{Number: episode.Season, Episodes: []historyEpisode{}})
			index = len(seasons) - 1
		}
		seasons[index].Episodes = append(seasons[index].Episodes, watched)
	}
	tmdb := strconv.FormatInt(int64(seriesID), 10)
	body := map[string]any{"shows": []historyShow{{IDs: map[string]string{"tmdb": tmdb}, Seasons: seasons}}}
	var decoded historyResponse
	if err := s.call(ctx, app, accessToken, http.MethodPost, "/sync/history", nil, body, &decoded); err != nil {
		return nil, err
	}
	recorded := &Recorded{URL: s.redirectURL(app, tmdb), NotFound: []EpisodeKey{}, Status: ""}
	if len(decoded.NotFound.Shows) > 0 {
		for _, episode := range episodes {
			recorded.NotFound = append(recorded.NotFound, EpisodeKey{Season: episode.Season, Number: episode.Number})
		}
		recorded.URL = ""

		return recorded, nil
	}
	for _, missing := range decoded.NotFound.Episodes {
		number := missing.Number
		if number == nil {
			number = missing.Episode
		}
		if missing.Season != nil && number != nil {
			recorded.NotFound = append(recorded.NotFound, EpisodeKey{Season: *missing.Season, Number: *number})
		}
	}
	// 話が見つからなかったと示したのに、どの話かを読めなければ、送ったすべての話を見つからなかったものとする。
	if len(decoded.NotFound.Episodes) > 0 && len(recorded.NotFound) == 0 {
		for _, episode := range episodes {
			recorded.NotFound = append(recorded.NotFound, EpisodeKey{Season: episode.Season, Number: episode.Number})
		}
	}
	for _, status := range decoded.Added.Statuses {
		// 見直しの記録では、状態が数で返ることがある (1 が watching)。文字列のときだけ使う。
		if text, ok := status.Response.Status.(string); ok && text != "" {
			recorded.Status = normalizeStatus(text)

			break
		}
	}

	return recorded, nil
}

// activitiesResponse は、/sync/activities の応答。
type activitiesResponse struct {
	All     string `json:"all"`
	TVShows struct {
		RemovedFromList string `json:"removed_from_list"`
	} `json:"tv_shows"`
}

func (s *Simkl) Activities(ctx context.Context, app *App, accessToken string) (*Activities, error) {
	var decoded activitiesResponse
	if err := s.call(ctx, app, accessToken, http.MethodGet, "/sync/activities", nil, nil, &decoded); err != nil {
		return nil, err
	}

	return &Activities{All: decoded.All, RemovedFromList: decoded.TVShows.RemovedFromList}, nil
}

// allItemsResponse は、/sync/all-items の応答。作品の ID は、数か文字列で返る。
type allItemsResponse struct {
	Shows []struct {
		Status string `json:"status"`
		Show   struct {
			IDs map[string]any `json:"ids"`
		} `json:"show"`
	} `json:"shows"`
}

func (s *Simkl) Watchlist(ctx context.Context, app *App, accessToken, dateFrom string) ([]*WatchlistItem, error) {
	// 既定の形で取得する。作品ごとの状態と、TMDB の ID を含む外部の ID を返す。
	query := url.Values{}
	if dateFrom != "" {
		query.Set("date_from", dateFrom)
	}
	var decoded allItemsResponse
	if err := s.call(ctx, app, accessToken, http.MethodGet, "/sync/all-items/shows", query, nil, &decoded); err != nil {
		return nil, err
	}
	items := make([]*WatchlistItem, 0, len(decoded.Shows))
	for _, show := range decoded.Shows {
		simklID := intID(show.Show.IDs["simkl"])
		status := normalizeStatus(show.Status)
		if simklID <= 0 || status == "" {
			continue
		}
		tmdb := intID(show.Show.IDs["tmdb"])
		if tmdb > 0 && tmdb <= 1<<31-1 {
			items = append(items, &WatchlistItem{SimklID: simklID, TMDBID: int32(tmdb), Status: status})
		} else {
			items = append(items, &WatchlistItem{SimklID: simklID, TMDBID: 0, Status: status})
		}
	}

	return items, nil
}

func (s *Simkl) WatchlistIDs(ctx context.Context, app *App, accessToken string) ([]int64, error) {
	var decoded allItemsResponse
	if err := s.call(ctx, app, accessToken, http.MethodGet, "/sync/all-items/shows", url.Values{"extended": {"simkl_ids_only"}}, nil, &decoded); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(decoded.Shows))
	for _, show := range decoded.Shows {
		if id := intID(show.Show.IDs["simkl"]); id > 0 {
			ids = append(ids, id)
		}
	}

	return ids, nil
}

// tokenResponse は、トークンのエンドポイントの応答。
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
}

// token は、トークンのエンドポイントに grant を送り、トークンを返す。クライアントの認証は、シークレットを本文に入れて行う。
// エラーには、応答の本文を入れず、状態コードとエラーコードだけを入れる。
func (s *Simkl) token(ctx context.Context, app *App, form url.Values) (*Token, error) {
	form.Set(paramClientID, app.ClientID)
	form.Set("client_secret", app.ClientSecret)
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create the token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	res, err := s.client.Do(req)
	if err != nil {
		return nil, errors.New("failed to send the token request")
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read the token response: %w", err)
	}
	var decoded tokenResponse
	_ = json.Unmarshal(raw, &decoded)
	switch {
	case res.StatusCode == http.StatusTooManyRequests:
		return nil, &RateLimitError{RetryAfter: retryAfter(res.Header.Get("Retry-After"), time.Now())}
	case res.StatusCode >= http.StatusInternalServerError:
		return nil, fmt.Errorf("the token endpoint returned %d", res.StatusCode)
	case res.StatusCode >= http.StatusBadRequest:
		return nil, fmt.Errorf("%w: %w", ErrPermanent, &statusError{code: res.StatusCode, reason: "the token endpoint returned " + shorten(decoded.Error)})
	}
	// 引き換えでは、更新のトークンがなければ、7 日で連携が切れる。
	if decoded.AccessToken == "" || (decoded.RefreshToken == "" && form.Get("grant_type") == "authorization_code") {
		return nil, fmt.Errorf("%w: the token endpoint returned no token", ErrPermanent)
	}
	// 書き込みの権限がないトークンでは、記録できない。綴りを間違えた scope は、Simkl が黙って読み取りだけにする。
	if !slices.Contains(strings.Fields(decoded.Scope), simklWriteScope) {
		return nil, fmt.Errorf("%w: the token cannot write to the library (scope %q)", ErrPermanent, shorten(decoded.Scope))
	}
	expiresIn := time.Duration(decoded.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = 7 * 24 * time.Hour
	}

	return &Token{AccessToken: decoded.AccessToken, RefreshToken: decoded.RefreshToken, ExpiresIn: expiresIn}, nil
}

// redirectURL は、TMDB の作品 ID tmdb の、Simkl の作品のページへ移るリンクを返す。Simkl の ID を引かずに、作品のページへのリンクを作る。
func (s *Simkl) redirectURL(app *App, tmdb string) string {
	query := url.Values{"to": {"simkl"}, "tmdb": {tmdb}, "type": {"tv"}, paramClientID: {app.ClientID}, "app-name": {simklAppName}, "app-version": {appVersion()}}

	return s.apiURL + "/redirect?" + query.Encode()
}

// normalizeStatus は、Simkl のウォッチリストの状態を、StatusWatching などにする。古いアプリの notinteresting は dropped と同じ。
// 知らない状態なら空を返す。
func normalizeStatus(status string) string {
	switch status {
	case StatusWatching, StatusPlanToWatch, StatusHold, StatusCompleted, StatusDropped:
		return status
	case "notinteresting":
		return StatusDropped
	default:
		return ""
	}
}

// intID は、数か文字列で返る ID を、数にする。読めなければ 0 を返す。
func intID(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0
		}

		return n
	default:
		return 0
	}
}

// appVersion は、Simkl がすべての問い合わせに求める app-version を返す。
func appVersion() string {
	return buildinfo.Get().Version
}

// statusError は、記録先が 4xx を返したことを表す。
type statusError struct {
	code   int
	reason string
}

func (e *statusError) Error() string { return e.reason }

// apiError は、Simkl の API のエラーの応答。
type apiError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// call は、Simkl の API に、アクセストークン accessToken で問い合わせ、応答を out に読み込む。
// Simkl は、すべての問い合わせに client_id、app-name、app-version を求める。
// 401 は ErrUnauthorized を、429 は RateLimitError を、ほかの 4xx は ErrPermanent を包んで返す。5xx と届かないときは、送り直すエラーを返す。
// エラーには、トークンも応答の本文も入れない。
func (s *Simkl) call(ctx context.Context, app *App, accessToken, method, path string, query url.Values, body, out any) error {
	if query == nil {
		query = url.Values{}
	}
	query.Set(paramClientID, app.ClientID)
	query.Set("app-name", simklAppName)
	query.Set("app-version", appVersion())
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to encode the request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, s.apiURL+path+"?"+query.Encode(), reader)
	if err != nil {
		return fmt.Errorf("failed to create the request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	res, err := s.client.Do(req)
	if err != nil {
		// url.Error は URL を含むが、URL に載せるのは公開の client_id だけで、トークンは載せていない。
		return fmt.Errorf("failed to send the request to %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read the response of %s: %w", path, err)
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w: %s returned 401", ErrUnauthorized, path)
	case res.StatusCode == http.StatusTooManyRequests:
		return &RateLimitError{RetryAfter: retryAfter(res.Header.Get("Retry-After"), time.Now())}
	case res.StatusCode >= http.StatusInternalServerError:
		return fmt.Errorf("%s returned %d", path, res.StatusCode)
	case res.StatusCode >= http.StatusBadRequest:
		var decoded apiError
		if json.Unmarshal(raw, &decoded) == nil && decoded.Error != "" {
			return fmt.Errorf("%w: %s returned %d %s", ErrPermanent, path, res.StatusCode, shorten(decoded.Error))
		}

		return fmt.Errorf("%w: %s returned %d", ErrPermanent, path, res.StatusCode)
	}
	if len(raw) > maxResponseBytes {
		return fmt.Errorf("the response of %s is too large", path)
	}
	// 空のウォッチリストは、{} か空の本文で返る。
	if len(bytes.TrimSpace(raw)) == 0 || out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("failed to decode the response of %s: %w", path, err)
	}

	return nil
}
