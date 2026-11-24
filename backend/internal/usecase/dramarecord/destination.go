// Package dramarecord は、Kotatsu で見たドラマを Simkl に記録し、Simkl のウォッチリストを取り込む (#1649、設計書 3.9)。
//
// アニメの記録 (animerecord) と同じく、管理者が記録先に登録したアプリで、利用者が自分のアカウントを OAuth で繋ぐ。
// 作品と話は、番組と録画に結びついた TMDB の作品とシーズンと話 (#1651) で指す。アニメの記録先はしょぼいカレンダーの作品と話数で
// 記録するので、記録先の interface は分け、ドラマの記録先は Destination の裏に置く。
package dramarecord

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
)

const (
	// DestinationSimkl は、記録先の名前。DB の destination のカラムと同じ。
	DestinationSimkl = "simkl"
	// requestTimeout は、記録先への 1 回の問い合わせの時間の上限。
	requestTimeout = 20 * time.Second
	// maxResponseBytes は、記録先の応答を読む量の上限。ウォッチリストの全件 (作品ごとに数百バイト) でも収まる大きさにする。
	maxResponseBytes = 16 << 20
	// maxErrorMessageRunes は、記録先が返したエラーの文を、通知や画面に残す長さの上限。
	maxErrorMessageRunes = 200
)

var (
	// ErrPermanent は、送り直しても変わらないこと (作品や話が見つからない、トークンが使えないなど) を表す。
	ErrPermanent = errors.New("the request cannot be completed")
	// ErrUnknownDestination は、知らない記録先であることを表す。
	ErrUnknownDestination = errors.New("unknown destination")
	// ErrUnauthorized は、記録先がアクセストークンを断った (401) ことを表す。トークンを更新してから送り直す。
	ErrUnauthorized = errors.New("the destination rejected the access token")
	// ErrReauthorizationRequired は、更新のトークンが断られ、利用者が連携し直すまで問い合わせられないことを表す。ErrPermanent を包む。
	ErrReauthorizationRequired = fmt.Errorf("%w: the connection must be authorized again", ErrPermanent)
	// ErrRateLimited は、記録先が問い合わせの多すぎること (429) を返したことを表す。
	ErrRateLimited = errors.New("the destination is rate limiting the requests")
)

// App は、管理者が記録先に登録したアプリ。
type App struct {
	ClientID     string
	ClientSecret string
}

// Token は、記録先が発行したトークン。
type Token struct {
	AccessToken  string
	RefreshToken string
	// ExpiresIn は、アクセストークンが切れるまでの長さ。
	ExpiresIn time.Duration
}

// Episode は、記録する 1 話。TMDB のシーズンと話の番号で指す。
type Episode struct {
	Season int32
	Number int32
	// WatchedAt は、見たと判定した日時。
	WatchedAt time.Time
}

// EpisodeKey は、シーズンと話の番号の組。
type EpisodeKey struct {
	Season int32
	Number int32
}

// Recorded は、記録した結果。
type Recorded struct {
	// URL は、記録した作品の、記録先のページ。分からなければ空。
	URL string
	// NotFound は、記録先が見つけられなかった話。作品が見つからなければ、送ったすべての話。
	NotFound []EpisodeKey
	// Status は、記録先が作品を置いたウォッチリストの状態 (watching など)。分からなければ空。
	Status string
}

// Activities は、ウォッチリストが変わったかを確認する、記録先の最後に変わった日時。記録先が返した文字列のまま持つ。
type Activities struct {
	// All は、すべての変更のうち、最も新しいもの。差分を取得するときの date_from に渡す。
	All string
	// RemovedFromList は、テレビの作品をウォッチリストから外した、最も新しい日時。
	RemovedFromList string
}

// WatchlistItem は、ウォッチリストの 1 作品。
type WatchlistItem struct {
	// SimklID は、記録先の作品 ID。
	SimklID int64
	// TMDBID は、TMDB の作品 ID。記録先が持っていなければ 0。
	TMDBID int32
	// Status は、ウォッチリストの状態 (StatusWatching など)。
	Status string
}

// ウォッチリストの状態 (drama_watchlist_imports の status)。Simkl の状態の名前と同じ。
const (
	StatusWatching    = "watching"
	StatusPlanToWatch = "plantowatch"
	StatusHold        = "hold"
	StatusCompleted   = "completed"
	StatusDropped     = "dropped"
)

// Destination は、見たドラマを記録する先。
type Destination interface {
	// Name は、記録先の名前 (DestinationSimkl など)。
	Name() string
	// Issuer は、認可の応答の iss に入る、記録先の認可サーバーの識別子。別のサーバーの応答を取り違えないよう、戻ってきたときに比較する。
	Issuer() string
	// AuthCodeURL は、利用者を送り出す、記録先の認可のエンドポイントの URL を返す。PKCE (S256) を使う。
	AuthCodeURL(app *App, redirectURI, state, verifier string) string
	// Exchange は、認可コードをトークンと引き換える。書き込める権限がなければ、ErrPermanent を包んで返す。
	Exchange(ctx context.Context, app *App, redirectURI, code, verifier string) (*Token, error)
	// Refresh は、更新のトークンで、新しいアクセストークンを取得する。更新のトークンが断られたら ErrReauthorizationRequired を返す。
	Refresh(ctx context.Context, app *App, refreshToken string) (*Token, error)
	// AccountName は、トークンの持ち主の、記録先でのアカウントの名前を返す。
	AccountName(ctx context.Context, app *App, accessToken string) (string, error)
	// Revoke は、トークンを無効にする。
	Revoke(ctx context.Context, app *App, token string) error
	// RecordEpisodes は、TMDB の作品 seriesID の話 episodes を見たことを、1 回の問い合わせで記録する。
	RecordEpisodes(ctx context.Context, app *App, accessToken string, seriesID int32, episodes []*Episode) (*Recorded, error)
	// Activities は、ウォッチリストが最後に変わった日時を返す。
	Activities(ctx context.Context, app *App, accessToken string) (*Activities, error)
	// Watchlist は、テレビの作品のウォッチリストを返す。dateFrom が空でなければ、その後に変わった作品だけを返す。
	Watchlist(ctx context.Context, app *App, accessToken, dateFrom string) ([]*WatchlistItem, error)
	// WatchlistIDs は、テレビの作品のウォッチリストにある、すべての作品の記録先の ID を返す。外した作品を見分けるのに使う。
	WatchlistIDs(ctx context.Context, app *App, accessToken string) ([]int64, error)
}

// NewHTTPClient は、記録先に問い合わせる HTTP クライアントを返す。
// 問い合わせ先は決まったホストだけなので、リダイレクトはたどらない (ほかのホストへ、トークンを付けて向け直させない)。
func NewHTTPClient(recorder *netactivity.Recorder, purpose netactivity.Purpose) *http.Client {
	return netactivity.NewClient(recorder, purpose, netactivity.ClientOptions{
		Transport: nil, Timeout: requestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	})
}

// RateLimitError は、記録先が 429 を返したことを表す。ErrRateLimited を包む。
type RateLimitError struct {
	// RetryAfter は、記録先が Retry-After で示した、問い合わせ直すまでの長さ。示さなかったか、読めなければ 0。
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("the destination returned 429 (retry after %s)", e.RetryAfter)
	}

	return "the destination returned 429"
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

// shorten は、記録先が返したエラーの文を、通知に残せる長さに切り詰める。
func shorten(message string) string {
	runes := []rune(strings.TrimSpace(message))
	if len(runes) > maxErrorMessageRunes {
		return string(runes[:maxErrorMessageRunes]) + "…"
	}

	return string(runes)
}
