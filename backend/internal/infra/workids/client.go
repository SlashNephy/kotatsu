// Package workids は、作品 ID の対応を自前で作るために、Annict と AniList の GraphQL API に問い合わせる (#1223、設計書 3.8)。
// Annict からは作品のしょぼいカレンダーの TID と MyAnimeList の ID を、AniList からは MyAnimeList の ID に対応する AniList の ID を引く。
package workids

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

	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
)

const (
	annictGraphQLURL  = "https://api.annict.com/graphql"
	aniListGraphQLURL = "https://graphql.anilist.co"
	// requestTimeout は、1 回の問い合わせの時間の上限。
	requestTimeout = 30 * time.Second
	// maxResponseBytes は、応答を読む量の上限。1 ページ (50 件) の作品なら十分に収まる。
	maxResponseBytes = 4 << 20
	// AnnictPageSize は、Annict の作品を 1 回に引く数。
	AnnictPageSize = 50
	// AniListBatchSize は、AniList に 1 回に問い合わせる MyAnimeList の ID の数。AniList の 1 ページの上限。
	AniListBatchSize = 50
)

var (
	// ErrUnauthorized は、Annict がトークンを受け付けなかったことを表す。
	ErrUnauthorized = errors.New("the access token was rejected")
	// ErrRateLimited は、問い合わせが多すぎること (429) を表す。
	ErrRateLimited = errors.New("the API is rate limiting the requests")
)

// RateLimitError は、429 を返されたことを表す。ErrRateLimited を包む。
type RateLimitError struct {
	// RetryAfter は、Retry-After で示された、問い合わせ直すまでの長さ。示されなければ 0。
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("the API returned 429 (retry after %s)", e.RetryAfter)
}

func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// AnnictWork は、Annict の作品。
type AnnictWork struct {
	AnnictID int32
	// SyobocalTID は、しょぼいカレンダーの TID。Annict が持たなければ nil。
	SyobocalTID *int32
	// MALAnimeID は、MyAnimeList の作品 ID。Annict が持たないか、数でなければ nil。
	MALAnimeID *int32
}

// AnnictPage は、Annict の作品の 1 ページ。
type AnnictPage struct {
	Works []*AnnictWork
	// Next は、続きを引くときに渡す cursor。続きがなければ空。
	Next string
}

// Options は Client の設定。空の URL は、本物の API を使う。
type Options struct {
	HTTPClient *http.Client
	// AnnictBaseURL と AniListBaseURL は、検証用に API を向け直すときの URL。GraphQL のエンドポイントは、その /graphql。
	AnnictBaseURL  string
	AniListBaseURL string
}

// Client は、Annict と AniList の GraphQL API に問い合わせる。
type Client struct {
	client     *http.Client
	annictURL  string
	aniListURL string
}

// NewClient は Client を返す。
func NewClient(opts *Options) *Client {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{
			Transport: nil, Jar: nil, Timeout: requestTimeout,
			// 問い合わせ先は決まったホストだけなので、リダイレクトはたどらない (ほかのホストへ、トークンを付けて向け直させない)。
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	annictURL, aniListURL := annictGraphQLURL, aniListGraphQLURL
	if opts.AnnictBaseURL != "" {
		annictURL = strings.TrimRight(opts.AnnictBaseURL, "/") + "/graphql"
	}
	if opts.AniListBaseURL != "" {
		aniListURL = strings.TrimRight(opts.AniListBaseURL, "/") + "/graphql"
	}

	return &Client{client: client, annictURL: annictURL, aniListURL: aniListURL}
}

// AnnictWorks は、Annict の作品の 1 ページを、作った順に返す。seasons (2026-autumn の形) を渡せば、その時期の作品に絞る。
// cursor が空なら、最初のページを返す。
func (c *Client) AnnictWorks(ctx context.Context, token string, seasons []string, cursor string) (*AnnictPage, error) {
	const query = `query ($seasons: [String!], $first: Int, $after: String) {
  searchWorks(seasons: $seasons, orderBy: { field: CREATED_AT, direction: ASC }, first: $first, after: $after) {
    nodes { annictId syobocalTid malAnimeId }
    pageInfo { hasNextPage endCursor }
  } }`
	variables := map[string]any{"seasons": nil, "first": AnnictPageSize, "after": nil}
	if len(seasons) > 0 {
		variables["seasons"] = seasons
	}
	if cursor != "" {
		variables["after"] = cursor
	}
	var data struct {
		SearchWorks *struct {
			Nodes []*struct {
				AnnictID    int32   `json:"annictId"`
				SyobocalTID *int32  `json:"syobocalTid"`
				MALAnimeID  *string `json:"malAnimeId"`
			} `json:"nodes"`
			PageInfo struct {
				HasNextPage bool    `json:"hasNextPage"`
				EndCursor   *string `json:"endCursor"`
			} `json:"pageInfo"`
		} `json:"searchWorks"`
	}
	if err := c.post(ctx, c.annictURL, token, query, variables, &data); err != nil {
		return nil, err
	}
	page := &AnnictPage{Works: []*AnnictWork{}, Next: ""}
	if data.SearchWorks == nil {
		return page, nil
	}
	for _, node := range data.SearchWorks.Nodes {
		if node == nil {
			continue
		}
		work := &AnnictWork{AnnictID: node.AnnictID, SyobocalTID: positive(node.SyobocalTID), MALAnimeID: nil}
		if node.MALAnimeID != nil {
			if id, err := strconv.ParseInt(strings.TrimSpace(*node.MALAnimeID), 10, 32); err == nil && id > 0 {
				work.MALAnimeID = new(int32(id))
			}
		}
		page.Works = append(page.Works, work)
	}
	if data.SearchWorks.PageInfo.HasNextPage && data.SearchWorks.PageInfo.EndCursor != nil {
		page.Next = *data.SearchWorks.PageInfo.EndCursor
	}

	return page, nil
}

// AniListIDsByMAL は、MyAnimeList の作品 ID (AniListBatchSize 件まで) に対応する、AniList の作品 ID を返す。見つからない ID は含めない。
// AniList の公開 API なので、トークンは使わない。
func (c *Client) AniListIDsByMAL(ctx context.Context, malIDs []int32) (map[int32]int32, error) {
	if len(malIDs) > AniListBatchSize {
		return nil, fmt.Errorf("too many MyAnimeList IDs: %d", len(malIDs))
	}
	const query = `query ($ids: [Int], $perPage: Int) { Page(perPage: $perPage) { media(idMal_in: $ids, type: ANIME) { id idMal } } }`
	var data struct {
		Page *struct {
			Media []*struct {
				ID    int32  `json:"id"`
				IDMal *int32 `json:"idMal"`
			} `json:"media"`
		} `json:"Page"`
	}
	if err := c.post(ctx, c.aniListURL, "", query, map[string]any{"ids": malIDs, "perPage": AniListBatchSize}, &data); err != nil {
		return nil, err
	}
	result := map[int32]int32{}
	if data.Page == nil {
		return result, nil
	}
	for _, media := range data.Page.Media {
		if media != nil && media.IDMal != nil {
			// 1 つの MyAnimeList の作品に複数の AniList の作品が対応するときは、小さい ID を使う。
			if current, ok := result[*media.IDMal]; !ok || media.ID < current {
				result[*media.IDMal] = media.ID
			}
		}
	}

	return result, nil
}

// graphQLResponse は、GraphQL の応答。
type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// post は、GraphQL の query を送り、data を out に読み込む。token が空でなければ、Bearer で付ける。
// エラーには、トークンも応答の本文も入れない。
func (c *Client) post(ctx context.Context, endpoint, token, query string, variables map[string]any, out any) error {
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
	// Annict の利用の目安に従い、User-Agent にアプリ名と連絡先を入れる。
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send the GraphQL request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read the GraphQL response: %w", err)
	}
	switch {
	case res.StatusCode == http.StatusTooManyRequests:
		return &RateLimitError{RetryAfter: retryAfter(res.Header.Get("Retry-After"))}
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %d", ErrUnauthorized, res.StatusCode)
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("the GraphQL endpoint returned %d", res.StatusCode)
	case len(raw) > maxResponseBytes:
		return errors.New("the GraphQL response is too large")
	}
	var decoded graphQLResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("failed to decode the GraphQL response: %w", err)
	}
	if len(decoded.Errors) > 0 {
		return fmt.Errorf("the GraphQL endpoint returned an error: %s", decoded.Errors[0].Message)
	}
	if err := json.Unmarshal(decoded.Data, out); err != nil {
		return fmt.Errorf("failed to decode the GraphQL data: %w", err)
	}

	return nil
}

// retryAfter は、Retry-After の秒数を長さにする。読めなければ 0 を返す。
func retryAfter(value string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds < 0 {
		return 0
	}

	return time.Duration(seconds) * time.Second
}

// positive は、0 より大きい数だけを返す。
func positive(value *int32) *int32 {
	if value == nil || *value <= 0 {
		return nil
	}

	return value
}
