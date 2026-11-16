// Package tmdb は、TMDb (The Movie Database) の API に問い合わせる (#1651、設計書 3.9)。
//
// 問い合わせには、管理者が連携の画面で入れた API の読み取りアクセストークン (API Read Access Token) を Authorization ヘッダーで付ける。
// トークンは URL に入れず、エラーの文にも出さない。問い合わせ先は決まったホストだけなので、リダイレクトはたどらない
// (ほかのホストへ、トークンを付けて向け直させない)。問い合わせの間隔を空け、結果は呼ぶ側が DB にキャッシュする。
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
)

const (
	// DefaultURL は、TMDb の API のオリジン。
	DefaultURL = "https://api.themoviedb.org"
	// DefaultInterval は、問い合わせの間隔の下限 (付録 A)。TMDb は上限を公開していないが、控えめにする。
	DefaultInterval = 250 * time.Millisecond
	// Language は、作品名と話の名前を引く言語。日本の放送の番組名と照らし合わせるので、日本語で引く。
	Language = "ja-JP"
	// requestTimeout は、1 回の問い合わせの時間の上限。
	requestTimeout = 30 * time.Second
	// maxResponseBytes は、読むレスポンスの大きさの上限。
	maxResponseBytes = 4 << 20
	// maxRetryAfter は、429 の Retry-After を信じる長さの上限。
	maxRetryAfter = time.Hour
	// defaultRetryAfter は、429 が Retry-After を示さなかったときに待つ長さ。
	defaultRetryAfter = 10 * time.Second
)

var (
	// ErrUnexpectedResponse は、TMDb が想定しない応答を返したことを表す。
	ErrUnexpectedResponse = errors.New("unexpected response from TMDb")
	// ErrNotFound は、作品やシーズンが TMDb にないことを表す。
	ErrNotFound = errors.New("not found on TMDb")
	// ErrUnauthorized は、トークンが使えないことを表す。
	ErrUnauthorized = errors.New("the TMDb token was rejected")
)

// RateLimitedError は、TMDb が 429 を返したことを表す。RetryAfter だけ待ってから問い合わせ直す。
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("rate limited by TMDb; retry after %s", e.RetryAfter)
}

// SearchResult は、作品の検索の結果の作品 1 つ。
type SearchResult struct {
	ID           int32   `json:"id"`
	Name         string  `json:"name"`
	OriginalName string  `json:"originalName"`
	FirstAirDate string  `json:"firstAirDate"`
	GenreIDs     []int32 `json:"genreIds"`
}

// Season は、作品のシーズン 1 つ。
type Season struct {
	SeasonNumber int32  `json:"seasonNumber"`
	Name         string `json:"name"`
	AirDate      string `json:"airDate"`
	EpisodeCount int32  `json:"episodeCount"`
}

// Series は、作品 (TV) の名前とシーズンの一覧。
type Series struct {
	ID           int32
	Name         string
	OriginalName string
	FirstAirDate string
	Seasons      []*Season
}

// Episode は、シーズンの話 1 つ。
type Episode struct {
	EpisodeNumber int32  `json:"episodeNumber"`
	AirDate       string `json:"airDate"`
	Name          string `json:"name"`
}

// Options は Client の設定。空の項目は既定値を使う。
type Options struct {
	HTTPClient *http.Client
	// URL は、API のオリジン。空なら DefaultURL。
	URL string
	// Interval は、問い合わせの間隔の下限。
	Interval time.Duration
}

// Client は、TMDb の API に問い合わせる。問い合わせは、同じ Client を使う間、Interval 以上の間隔を空ける。
// トークンは管理者が画面で入れ直せるので、Client には持たせず、問い合わせごとに渡す。
type Client struct {
	http     *http.Client
	url      string
	interval time.Duration

	// mu は、問い合わせを 1 つずつにし、間隔を空ける。
	mu   sync.Mutex
	last time.Time
}

// NewHTTPClient は、通信を recorder に記録する HTTP クライアントを返す。リダイレクトはたどらない。
func NewHTTPClient(recorder *netactivity.Recorder) *http.Client {
	return netactivity.NewClient(recorder, netactivity.PurposeDramaLinking, netactivity.ClientOptions{
		Transport: nil, Timeout: requestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	})
}

// NewClient は Client を返す。
func NewClient(opts *Options) *Client {
	c := &Client{http: opts.HTTPClient, url: DefaultURL, interval: DefaultInterval, mu: sync.Mutex{}, last: time.Time{}}
	if c.http == nil {
		c.http = NewHTTPClient(nil)
	}
	if opts.URL != "" {
		c.url = strings.TrimRight(opts.URL, "/")
	}
	if opts.Interval > 0 {
		c.interval = opts.Interval
	}

	return c
}

// searchResponse は、/3/search/tv の応答。
type searchResponse struct {
	Results []struct {
		ID           int32   `json:"id"`
		Name         string  `json:"name"`
		OriginalName string  `json:"original_name"`
		FirstAirDate string  `json:"first_air_date"`
		GenreIDs     []int32 `json:"genre_ids"`
	} `json:"results"`
}

// SearchTV は、作品 (TV) を名前で探し、1 ページ目 (20 件まで) の結果を、TMDb の並び順で返す。
func (c *Client) SearchTV(ctx context.Context, token, query string) ([]*SearchResult, error) {
	var res searchResponse
	if err := c.get(ctx, token, "/3/search/tv", url.Values{"query": {query}, "include_adult": {"false"}}, &res); err != nil {
		return nil, err
	}
	results := make([]*SearchResult, 0, len(res.Results))
	for _, r := range res.Results {
		if r.ID <= 0 {
			continue
		}
		genres := r.GenreIDs
		if genres == nil {
			genres = []int32{}
		}
		results = append(results, &SearchResult{
			ID: r.ID, Name: r.Name, OriginalName: r.OriginalName, FirstAirDate: r.FirstAirDate, GenreIDs: genres,
		})
	}

	return results, nil
}

// seriesResponse は、/3/tv/{id} の応答。
type seriesResponse struct {
	ID           int32  `json:"id"`
	Name         string `json:"name"`
	OriginalName string `json:"original_name"`
	FirstAirDate string `json:"first_air_date"`
	Seasons      []struct {
		SeasonNumber int32  `json:"season_number"`
		Name         string `json:"name"`
		AirDate      string `json:"air_date"`
		EpisodeCount int32  `json:"episode_count"`
	} `json:"seasons"`
}

// TV は、作品 (TV) の名前とシーズンの一覧を返す。作品がなければ ErrNotFound を返す。
func (c *Client) TV(ctx context.Context, token string, id int32) (*Series, error) {
	var res seriesResponse
	if err := c.get(ctx, token, "/3/tv/"+strconv.Itoa(int(id)), url.Values{}, &res); err != nil {
		return nil, err
	}
	if res.ID != id {
		return nil, fmt.Errorf("%w: asked for the series %d but got %d", ErrUnexpectedResponse, id, res.ID)
	}
	series := &Series{ID: res.ID, Name: res.Name, OriginalName: res.OriginalName, FirstAirDate: res.FirstAirDate, Seasons: make([]*Season, 0, len(res.Seasons))}
	for _, s := range res.Seasons {
		series.Seasons = append(series.Seasons, &Season{SeasonNumber: s.SeasonNumber, Name: s.Name, AirDate: s.AirDate, EpisodeCount: s.EpisodeCount})
	}

	return series, nil
}

// seasonResponse は、/3/tv/{id}/season/{number} の応答。
type seasonResponse struct {
	Episodes []struct {
		EpisodeNumber int32  `json:"episode_number"`
		AirDate       string `json:"air_date"`
		Name          string `json:"name"`
	} `json:"episodes"`
}

// Season は、作品 id のシーズン number の話の一覧を返す。シーズンがなければ ErrNotFound を返す。
func (c *Client) Season(ctx context.Context, token string, id, number int32) ([]*Episode, error) {
	var res seasonResponse
	if err := c.get(ctx, token, fmt.Sprintf("/3/tv/%d/season/%d", id, number), url.Values{}, &res); err != nil {
		return nil, err
	}
	episodes := make([]*Episode, 0, len(res.Episodes))
	for _, e := range res.Episodes {
		if e.EpisodeNumber <= 0 {
			continue
		}
		episodes = append(episodes, &Episode{EpisodeNumber: e.EpisodeNumber, AirDate: e.AirDate, Name: e.Name})
	}

	return episodes, nil
}

// get は、path に query を付けて token で問い合わせ、JSON の応答を v に読む。
func (c *Client) get(ctx context.Context, token, path string, query url.Values, v any) error {
	if token == "" {
		return ErrUnauthorized
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := c.interval - time.Since(c.last); !c.last.IsZero() && wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()

			return ctx.Err()
		case <-timer.C:
		}
	}
	defer func() { c.last = time.Now() }()

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	query.Set("language", Language)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+path+"?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("failed to build the request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	res, err := c.http.Do(req)
	if err != nil {
		// url.Error は URL を含むが、トークンは URL に入れないので、そのまま返してよい。
		return fmt.Errorf("failed to query TMDb: %w", err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusTooManyRequests:
		return &RateLimitedError{RetryAfter: retryAfter(res.Header.Get("Retry-After"), time.Now())}
	case res.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case res.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("%w: status %s", ErrUnexpectedResponse, res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read the response from TMDb: %w", err)
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("%w: the response is too large", ErrUnexpectedResponse)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%w: %w", ErrUnexpectedResponse, err)
	}

	return nil
}

// retryAfter は、Retry-After (秒か HTTP の日時) を読み、待つ長さを返す。読めなければ既定の長さにし、maxRetryAfter で抑える。
func retryAfter(value string, now time.Time) time.Duration {
	wait := defaultRetryAfter
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		// 大きすぎる値を掛けてあふれさせないよう、先に上限で抑える。
		wait = time.Duration(min(seconds, int(maxRetryAfter/time.Second))) * time.Second
	} else if at, err := http.ParseTime(value); err == nil {
		wait = max(at.Sub(now), 0)
	}

	return min(wait, maxRetryAfter)
}
