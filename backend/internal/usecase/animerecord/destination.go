// Package animerecord は、Kotatsu で見たアニメを、Annict と AniList に記録する (#927)。
//
// 管理者が記録先に登録したアプリで、利用者が自分のアカウントを OAuth で繋ぐ。プレイヤーが実際に再生した区間を送り、
// 番組の長さの一定の割合を見たら、番組に結びついた話 (設計書 3.8) を、River のジョブで記録先に送る。
// 記録先は Destination の裏に置き、MyAnimeList なども後から足せるようにする。
package animerecord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
)

const (
	// DestinationAnnict と DestinationAniList は、記録先の名前。DB の destination の列と同じ。
	DestinationAnnict  = "annict"
	DestinationAniList = "anilist"
	// requestTimeout は、記録先への 1 回の問い合わせの時間の上限。
	requestTimeout = 20 * time.Second
	// maxResponseBytes は、記録先の応答を読む量の上限。エピソードの一覧でも収まる大きさにする。
	maxResponseBytes = 4 << 20
	// maxErrorMessageRunes は、記録先が返したエラーの文を、通知に残す長さの上限。
	maxErrorMessageRunes = 200
)

var (
	// ErrPermanent は、送り直しても記録できないこと (トークンが使えない、エピソードが見つからないなど) を表す。
	// River の再試行を待たずに、記録できなかったことにする。
	ErrPermanent = errors.New("the record cannot be sent")
	// ErrUnknownDestination は、知らない記録先であることを表す。
	ErrUnknownDestination = errors.New("unknown destination")
	// ErrRateLimited は、記録先が問い合わせの多すぎること (429) を返したことを表す。送り直すエラーとして扱う。
	ErrRateLimited = errors.New("the destination is rate limiting the requests")
)

// App は、管理者が記録先に登録したアプリ。
type App struct {
	ClientID     string
	ClientSecret string
}

// StatusRules は、ユーザーが選んだ、作品の状態を変える条件。既定ではどれも変えない。
type StatusRules struct {
	// WatchingOnFirst は、第 1 話 ([新]) を見たら「見てる」にするかどうか。
	WatchingOnFirst bool
	// WatchingOnEpisode は、最終話 ([終]) 以外を見たら「見てる」にするかどうか。
	WatchingOnEpisode bool
	// WatchedOnLast は、最終話を見たら「見た」にするかどうか。
	WatchedOnLast bool
}

// WorkStatus は、記録先によらない作品の状態。
type WorkStatus int

const (
	// WorkStatusNone は、状態を決めていないこと (ほかの状態も含む) を表す。
	WorkStatusNone WorkStatus = iota
	// WorkStatusWatching は「見てる」。
	WorkStatusWatching
	// WorkStatusWatched は「見た」。
	WorkStatusWatched
)

// NextStatus は、話を見たときに変える作品の状態を返す。変えなければ false を返す。
// 最終話なら「見た」に、第 1 話か最終話以外なら「見てる」にする。すでに「見た」の作品は「見てる」に戻さないが、
// 第 1 話 ([新]) なら戻す (分割 2 クールなど)。TVTestAnnictRecorder と同じ。
func NextStatus(rules *StatusRules, first, last bool, current WorkStatus) (WorkStatus, bool) {
	next := WorkStatusNone
	switch {
	case last && rules.WatchedOnLast:
		next = WorkStatusWatched
	case (first && rules.WatchingOnFirst) || (!last && rules.WatchingOnEpisode):
		next = WorkStatusWatching
	}
	if next == WorkStatusNone || next == current {
		return WorkStatusNone, false
	}
	if next == WorkStatusWatching && current == WorkStatusWatched && !first {
		return WorkStatusNone, false
	}

	return next, true
}

// Episode は、記録する 1 話。
type Episode struct {
	// Number は話数。話数のない放送 (劇場版など) では nil。
	Number *float64
	// Subtitle は、しょぼいカレンダーのサブタイトル。
	Subtitle *string
	// First と Last は、[新] と [終] の印。
	First bool
	Last  bool
}

// Recorded は、記録した結果。
type Recorded struct {
	// URL は、記録した作品かエピソードの、記録先のページ。
	URL string
	// FollowUp は、記録を付けた後に、作品の状態を変える。変えなければ nil。
	// 記録を付けたことを残してから呼び、失敗しても記録は付け直さない (付け直すと、記録が二重になる)。
	FollowUp func(ctx context.Context) error
}

// Destination は、見たアニメを記録する先。
type Destination interface {
	// Name は、記録先の名前 (DestinationAnnict など)。
	Name() string
	// AuthCodeURL は、利用者を送り出す、記録先の認可のエンドポイントの URL を返す。PKCE (S256) を使う。
	AuthCodeURL(app *App, redirectURI, state, verifier string) string
	// Exchange は、認可コードをアクセストークンと引き換える。
	Exchange(ctx context.Context, app *App, redirectURI, code, verifier string) (string, error)
	// AccountName は、トークンの持ち主の、記録先でのアカウントの名前を返す。
	AccountName(ctx context.Context, token string) (string, error)
	// Revoke は、トークンを無効にする。記録先が無効にする API を持たなければ、何もしない。
	Revoke(ctx context.Context, app *App, token string) error
	// WorkIDs は、作品の対応から、記録先の作品の ID を、使う順に返す。
	WorkIDs(works []*anime.Work) []int32
	// RecordEpisode は、作品 workIDs のどれかの、1 話を見たことを記録する。rules に従って作品の状態を変えるのに、
	// 記録とは別に送るものがあれば、Recorded.FollowUp に入れて返す。送り直しで二重にならないよう、記録を付けた後には失敗を返さない。
	RecordEpisode(ctx context.Context, token string, workIDs []int32, episode *Episode, rules *StatusRules) (*Recorded, error)
	// Wishlist は、トークンの持ち主が「見たい」「見てる」にした作品の 1 ページを返す (#928)。cursor が空なら、最初のページを返す。
	// 続きがなければ、WishlistPage.Next は空。問い合わせの間隔は、呼ぶ側で空ける。
	Wishlist(ctx context.Context, token, cursor string) (*WishlistPage, error)
}

// 見たいリストの作品の状態 (anime_wishlist_entries の status)。
const (
	// WishlistWannaWatch は「見たい」。AniList の PLANNING。
	WishlistWannaWatch = "wanna_watch"
	// WishlistWatching は「見てる」。AniList の CURRENT。
	WishlistWatching = "watching"
)

// WishlistEntry は、見たいリストの 1 作品。
type WishlistEntry struct {
	// WorkID は、記録先の作品 ID。
	WorkID int32
	// Status は、WishlistWannaWatch か WishlistWatching。
	Status string
	// Title は、記録先の作品名。
	Title string
}

// WishlistPage は、見たいリストの 1 ページ。
type WishlistPage struct {
	Entries []*WishlistEntry
	// Next は、続きを引くときに渡す cursor。続きがなければ空。
	Next string
}

// Endpoints は、記録先の URL。本物の記録先のほかには、管理者が設定で向け直したときだけ使う (検証用)。
type Endpoints struct {
	AuthURL    string
	TokenURL   string
	RevokeURL  string
	GraphQLURL string
}

// NewHTTPClient は、記録先に問い合わせる HTTP クライアントを返す。
// 問い合わせ先は決まったホストだけなので、リダイレクトはたどらない (ほかのホストへ、トークンを付けて向け直させない)。
func NewHTTPClient() *http.Client {
	return &http.Client{
		Transport: nil,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Jar:     nil,
		Timeout: requestTimeout,
	}
}

// oauth2Config は、記録先の OAuth のクライアントの設定を返す。
func oauth2Config(app *App, endpoints *Endpoints, redirectURI string, scopes []string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     app.ClientID,
		ClientSecret: app.ClientSecret,
		Endpoint:     oauth2.Endpoint{AuthURL: endpoints.AuthURL, DeviceAuthURL: "", TokenURL: endpoints.TokenURL, AuthStyle: oauth2.AuthStyleInParams},
		RedirectURL:  redirectURI,
		Scopes:       scopes,
	}
}

// exchange は、認可コードをアクセストークンと引き換える。エラーには、応答の本文を入れず、状態コードとエラーコードだけを入れる。
func exchange(ctx context.Context, client *http.Client, config *oauth2.Config, code, verifier string) (string, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, client)
	token, err := config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		var retrieve *oauth2.RetrieveError
		if errors.As(err, &retrieve) && retrieve.Response != nil {
			return "", fmt.Errorf("%w: the token endpoint returned %d %s", ErrPermanent, retrieve.Response.StatusCode, retrieve.ErrorCode)
		}

		return "", errors.New("failed to exchange the authorization code")
	}
	if token.AccessToken == "" {
		return "", fmt.Errorf("%w: the token endpoint returned no access token", ErrPermanent)
	}

	return token.AccessToken, nil
}

// RateLimitError は、記録先が 429 を返したことを表す。ErrRateLimited を包む。
type RateLimitError struct {
	// RetryAfter は、記録先が Retry-After で示した、問い合わせ直すまでの長さ。示さなかったか、読めなければ 0。
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("the GraphQL endpoint returned 429 (retry after %s)", e.RetryAfter)
	}

	return "the GraphQL endpoint returned 429"
}

func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// retryAfter は、Retry-After の値 (秒か HTTP の日時) を、now からの長さにする。なければ、読めなければ、過去なら 0 を返す。
func retryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return max(time.Duration(seconds)*time.Second, 0)
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(at.Sub(now), 0)
	}

	return 0
}

// graphQLResponse は、GraphQL の応答。
type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// postGraphQL は、トークン token で、GraphQL の query を送り、data を out に読み込む。
// 401 と 403、ほかの 4xx、GraphQL のエラーは送り直しても変わらないので、ErrPermanent を包んで返す。
// 429 (ErrRateLimited を包む) と 5xx、届かないときは、送り直すエラーを返す。エラーには、トークンも応答の本文も入れない。
func postGraphQL(ctx context.Context, client *http.Client, endpoint, token, query string, variables map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return fmt.Errorf("failed to encode the GraphQL request: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create the GraphQL request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	res, err := client.Do(req)
	if err != nil {
		// url.Error は URL を含むが、URL にトークンは載せていない。
		return fmt.Errorf("failed to send the GraphQL request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read the GraphQL response: %w", err)
	}
	switch {
	case res.StatusCode == http.StatusTooManyRequests:
		return &RateLimitError{RetryAfter: retryAfter(res.Header.Get("Retry-After"), time.Now())}
	case res.StatusCode >= http.StatusInternalServerError:
		return fmt.Errorf("the GraphQL endpoint returned %d", res.StatusCode)
	case res.StatusCode >= http.StatusBadRequest:
		var decoded graphQLResponse
		if json.Unmarshal(raw, &decoded) == nil && len(decoded.Errors) > 0 {
			return fmt.Errorf("%w: %d %s", ErrPermanent, res.StatusCode, shorten(decoded.Errors[0].Message))
		}

		return fmt.Errorf("%w: the GraphQL endpoint returned %d", ErrPermanent, res.StatusCode)
	}
	if len(raw) > maxResponseBytes {
		return errors.New("the GraphQL response is too large")
	}
	var decoded graphQLResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("failed to decode the GraphQL response: %w", err)
	}
	if len(decoded.Errors) > 0 {
		return fmt.Errorf("%w: %s", ErrPermanent, shorten(decoded.Errors[0].Message))
	}
	if err := json.Unmarshal(decoded.Data, out); err != nil {
		return fmt.Errorf("failed to decode the GraphQL data: %w", err)
	}

	return nil
}

// shorten は、記録先が返したエラーの文を、通知に残せる長さに切り詰める。
func shorten(message string) string {
	runes := []rune(strings.TrimSpace(message))
	if len(runes) > maxErrorMessageRunes {
		return string(runes[:maxErrorMessageRunes]) + "…"
	}

	return string(runes)
}
