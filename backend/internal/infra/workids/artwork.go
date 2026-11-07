package workids

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
)

// maxJikanResponseBytes は、Jikan の応答を読む量の上限。作品 1 件の情報なら十分に収まる。
const maxJikanResponseBytes = 1 << 20

// ErrNotFound は、作品が見つからないことを表す。
var ErrNotFound = errors.New("the work was not found")

// AniListCoversByMAL は、MyAnimeList の作品 ID (AniListBatchSize 件まで) に対応する、AniList の作品の高画質の画像 (coverImage.extraLarge。
// なければ large) の URL を返す (#1315)。見つからない ID と、画像のない作品は含めない。AniList の公開 API なので、トークンは使わない。
// 1 つの MyAnimeList の作品に複数の作品が対応するときは、AniList の ID の小さい作品の画像を使う。
func (c *Client) AniListCoversByMAL(ctx context.Context, malIDs []int32) (map[int32]string, error) {
	if len(malIDs) > AniListBatchSize {
		return nil, fmt.Errorf("too many MyAnimeList IDs: %d", len(malIDs))
	}
	const query = `query ($ids: [Int], $page: Int, $perPage: Int) {
  Page(page: $page, perPage: $perPage) {
    media(idMal_in: $ids, type: ANIME, sort: ID) { id idMal coverImage { extraLarge large } }
    pageInfo { hasNextPage }
  } }`
	result := map[int32]string{}
	for page := 1; page <= maxAniListPages; page++ {
		var data struct {
			Page *struct {
				Media []*struct {
					ID         int32  `json:"id"`
					IDMal      *int32 `json:"idMal"`
					CoverImage *struct {
						ExtraLarge *string `json:"extraLarge"`
						Large      *string `json:"large"`
					} `json:"coverImage"`
				} `json:"media"`
				PageInfo struct {
					HasNextPage bool `json:"hasNextPage"`
				} `json:"pageInfo"`
			} `json:"Page"`
		}
		variables := map[string]any{"ids": malIDs, "page": page, "perPage": AniListBatchSize}
		if err := c.post(ctx, c.aniListURL, "", query, variables, &data); err != nil {
			return nil, err
		}
		if data.Page == nil {
			return result, nil
		}
		for _, media := range data.Page.Media {
			if media == nil || media.IDMal == nil || media.CoverImage == nil {
				continue
			}
			// ID の順に引くので、先に見つけた (ID の小さい) 作品の画像を使う。
			if _, ok := result[*media.IDMal]; ok {
				continue
			}
			if url := firstNonEmpty(media.CoverImage.ExtraLarge, media.CoverImage.Large); url != "" {
				result[*media.IDMal] = url
			}
		}
		if !data.Page.PageInfo.HasNextPage {
			return result, nil
		}
	}

	return nil, errors.New("too many AniList works for the MyAnimeList IDs")
}

// JikanImage は、Jikan (MyAnimeList の非公式 API) から、MyAnimeList の作品 malID の大きい画像の URL を返す (#1315)。
// 作品がなければ ErrNotFound を、画像がなければ空を返す。
func (c *Client) JikanImage(ctx context.Context, malID int32) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.jikanURL+"/anime/"+strconv.Itoa(int(malID)), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create the Jikan request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	res, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send the Jikan request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	switch {
	case res.StatusCode == http.StatusTooManyRequests:
		return "", &RateLimitError{RetryAfter: retryAfter(res.Header.Get("Retry-After"))}
	case res.StatusCode == http.StatusNotFound:
		return "", ErrNotFound
	case res.StatusCode != http.StatusOK:
		return "", fmt.Errorf("jikan returned %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxJikanResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("failed to read the Jikan response: %w", err)
	}
	if len(raw) > maxJikanResponseBytes {
		return "", errors.New("the Jikan response is too large")
	}
	var body struct {
		Data *struct {
			Images map[string]*struct {
				LargeImageURL *string `json:"large_image_url"`
				ImageURL      *string `json:"image_url"`
			} `json:"images"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", fmt.Errorf("failed to decode the Jikan response: %w", err)
	}
	if body.Data == nil {
		return "", ErrNotFound
	}
	// JPEG を先に選ぶ。WebP は MyAnimeList が変換したもので、JPEG と同じ絵である。
	for _, format := range []string{"jpg", "webp"} {
		if images := body.Data.Images[format]; images != nil {
			if url := firstNonEmpty(images.LargeImageURL, images.ImageURL); url != "" {
				return url, nil
			}
		}
	}

	return "", nil
}

// firstNonEmpty は、空でない最初の値を、前後の空白を除いて返す。どれも空なら空を返す。
func firstNonEmpty(values ...*string) string {
	for _, value := range values {
		if value != nil && strings.TrimSpace(*value) != "" {
			return strings.TrimSpace(*value)
		}
	}

	return ""
}
