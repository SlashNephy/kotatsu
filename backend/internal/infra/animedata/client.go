// Package animedata は、アニメの番組と作品の結びつきに使う、連携先の情報を GitHub から取得する (設計書 3.8)。
//
// サービスとしょぼいカレンダー、Annict のチャンネルの対応は saya-definitions から、
// しょぼいカレンダーの TID と Annict、AniList、MyAnimeList の作品 ID の対応は arm から取得する。
// どれも 1 日ごとに取得し直すので、ETag を送り、変わっていなければ読み直さない。
package animedata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
)

const (
	// DefaultSayaDefinitionsURL は、saya-definitions のチャンネルの定義。
	DefaultSayaDefinitionsURL = "https://raw.githubusercontent.com/SlashNephy/saya-definitions/master/definitions.json"
	// DefaultArmURL は、arm の作品 ID の対応。
	DefaultArmURL = "https://raw.githubusercontent.com/kawaiioverflow/arm/master/arm.json"
	// requestTimeout は、1 回の取得の時間の上限。arm は数 MB ある。
	requestTimeout = 2 * time.Minute
	// maxResponseBytes は、読むレスポンスの大きさの上限。arm は 2026 年 11 月で約 2 MB。
	maxResponseBytes = 64 << 20
)

// ErrNotModified は、前に取得したときから内容が変わっていない (304) ことを表す。
var ErrNotModified = errors.New("not modified")

// Channel は、saya-definitions のチャンネル 1 つ。
type Channel struct {
	Name string `json:"name"`
	// Type は、GR、BS、CS のどれか。
	Type       string  `json:"type"`
	ServiceIDs []int64 `json:"serviceIds"`
	// NetworkID は、BS と CS では ARIB のネットワーク ID。地上波では地域で違うので、意味を持たない (15 が入っている)。
	NetworkID int64 `json:"networkId"`
	// SyobocalID は、しょぼいカレンダーの ChID。対応がなければ nil。
	SyobocalID *int32 `json:"syobocalId"`
	// AnnictID は、Annict のチャンネルの ID。対応がなければ nil。
	AnnictID *int32 `json:"annictId"`
}

// Work は、arm の作品 1 つ。どの ID も、対応がなければ nil。
type Work struct {
	SyobocalTID *int32 `json:"syobocal_tid"`
	AnnictID    *int32 `json:"annict_id"`
	AniListID   *int32 `json:"anilist_id"`
	MALID       *int32 `json:"mal_id"`
}

// Options は Client の設定。空の項目は既定値を使う。
type Options struct {
	HTTPClient         *http.Client
	SayaDefinitionsURL string
	ArmURL             string
}

// Client は、連携先の情報を取得する。
type Client struct {
	http               *http.Client
	sayaDefinitionsURL string
	armURL             string
}

// NewClient は Client を返す。opts が nil なら、既定値を使う。
func NewClient(opts *Options) *Client {
	c := &Client{http: http.DefaultClient, sayaDefinitionsURL: DefaultSayaDefinitionsURL, armURL: DefaultArmURL}
	if opts == nil {
		return c
	}
	if opts.HTTPClient != nil {
		c.http = opts.HTTPClient
	}
	c.sayaDefinitionsURL = valueOr(opts.SayaDefinitionsURL, c.sayaDefinitionsURL)
	c.armURL = valueOr(opts.ArmURL, c.armURL)

	return c
}

// SayaDefinitions は、saya-definitions のチャンネルの一覧と ETag を返す。
// etag が前に取得したときの ETag で、内容が変わっていなければ ErrNotModified を返す。
func (c *Client) SayaDefinitions(ctx context.Context, etag string) ([]*Channel, string, error) {
	var body struct {
		Channels []*Channel `json:"channels"`
	}
	newETag, err := c.fetch(ctx, c.sayaDefinitionsURL, etag, &body)
	if err != nil {
		return nil, "", err
	}
	if body.Channels == nil {
		return nil, "", errors.New("saya-definitions has no channels")
	}

	return body.Channels, newETag, nil
}

// Arm は、arm の作品の一覧と ETag を返す。
// etag が前に取得したときの ETag で、内容が変わっていなければ ErrNotModified を返す。
func (c *Client) Arm(ctx context.Context, etag string) ([]*Work, string, error) {
	works := []*Work{}
	newETag, err := c.fetch(ctx, c.armURL, etag, &works)
	if err != nil {
		return nil, "", err
	}

	return works, newETag, nil
}

// fetch は、url の JSON を v に読み、ETag を返す。
func (c *Client) fetch(ctx context.Context, url, etag string, v any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build the request: %w", err)
	}
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get %s: %w", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotModified {
		return "", ErrNotModified
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get %s: unexpected status %s", url, res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", url, err)
	}
	if len(body) > maxResponseBytes {
		return "", fmt.Errorf("%s is larger than %d bytes", url, maxResponseBytes)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return "", fmt.Errorf("failed to decode %s: %w", url, err)
	}

	return res.Header.Get("ETag"), nil
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}
