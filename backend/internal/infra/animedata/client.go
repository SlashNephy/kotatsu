// Package animedata は、アニメの番組と作品の結びつきに使う、連携先の情報を返す (設計書 3.8)。
//
// サービスとしょぼいカレンダー、Annict のチャンネルの対応は、バイナリに埋め込んだ定義 (channels.json) を返す (#1589)。
// しょぼいカレンダーの TID と Annict、AniList、MyAnimeList の作品 ID の対応は、arm を GitHub から取得する。
// どれも 1 日ごとに問い合わせ直すので、ETag を比べ、変わっていなければ読み直さない。
package animedata

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
)

const (
	// DefaultArmURL は、arm の作品 ID の対応。
	DefaultArmURL = "https://raw.githubusercontent.com/kawaiioverflow/arm/master/arm.json"
	// requestTimeout は、1 回の取得の時間の上限。arm は数 MB ある。
	requestTimeout = 2 * time.Minute
	// maxResponseBytes は、読むレスポンスの大きさの上限。arm は 2026 年 11 月で約 2 MB。
	maxResponseBytes = 64 << 20
)

// ErrNotModified は、前に取得したときから内容が変わっていない (304) ことを表す。
var ErrNotModified = errors.New("not modified")

// channelDefinitions は、サービスとしょぼいカレンダー、Annict のチャンネルの対応 (#1589)。
// SlashNephy/saya-definitions (MIT) の definitions.json から、Kotatsu が使う項目だけを残し、系列局を局ごとに分けたもの。
//
//go:embed channels.json
var channelDefinitions []byte

// Channel は、チャンネルの定義 1 つ。地上波の系列局は、局ごとに 1 つ。
type Channel struct {
	Name string `json:"name"`
	// Type は、GR、BS、CS のどれか。
	Type       string  `json:"type"`
	ServiceIDs []int64 `json:"serviceIds"`
	// NetworkID は、BS と CS では ARIB のネットワーク ID。地上波では地域で違うので、意味を持たない (15 が入っている)。
	NetworkID int64 `json:"networkId"`
	// SyobocalID は、しょぼいカレンダーの ChID。対応がなければ nil。
	SyobocalID *int32 `json:"syobocalId"`
	// AnnictID は、Annict のチャンネルの ID。しょぼいカレンダーの ChID と同じとは限らない。対応がなければ nil。
	AnnictID *int32 `json:"annictId"`
	// NicojkID は、ニコニコ実況のチャンネルの番号。系列局は、キー局の番号を持つ。対応がなければ nil。
	NicojkID *int32 `json:"nicojkId"`
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
	HTTPClient *http.Client
	ArmURL     string
}

// Client は、連携先の情報を取得する。
type Client struct {
	http   *http.Client
	armURL string
}

// NewClient は Client を返す。opts が nil なら、既定値を使う。
func NewClient(opts *Options) *Client {
	c := &Client{http: netactivity.NewClient(nil, netactivity.PurposeAnimeMappings, netactivity.ClientOptions{Transport: nil, Timeout: 0, CheckRedirect: nil}), armURL: DefaultArmURL}
	if opts == nil {
		return c
	}
	if opts.HTTPClient != nil {
		c.http = opts.HTTPClient
	}
	c.armURL = valueOr(opts.ArmURL, c.armURL)

	return c
}

// ChannelDefinitions は、埋め込んだチャンネルの定義の一覧と、その ETag (中身のハッシュ) を返す。
// etag が前に返した ETag と同じなら、ErrNotModified を返す。Kotatsu を更新して定義が変わったときだけ、取り込み直させる。
func (c *Client) ChannelDefinitions(_ context.Context, etag string) ([]*Channel, string, error) {
	return parseChannelDefinitions(channelDefinitions, etag)
}

func parseChannelDefinitions(data []byte, etag string) ([]*Channel, string, error) {
	sum := sha256.Sum256(data)
	newETag := "sha256:" + hex.EncodeToString(sum[:])
	if etag == newETag {
		return nil, "", ErrNotModified
	}
	var body struct {
		Channels []*Channel `json:"channels"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, "", fmt.Errorf("failed to decode the channel definitions: %w", err)
	}
	if body.Channels == nil {
		return nil, "", errors.New("the channel definitions have no channels")
	}

	return body.Channels, newETag, nil
}

// Arm は、arm の作品の一覧と ETag を返す。
// etag が前に取得したときの ETag で、内容が変わっていなければ ErrNotModified を返す。
func (c *Client) Arm(ctx context.Context, etag string) ([]*Work, string, error) {
	works := []*Work{}
	newETag, err := c.fetch(ctx, "arm", c.armURL, etag, &works)
	if err != nil {
		return nil, "", err
	}

	return works, newETag, nil
}

// fetch は、url の JSON を v に読み、ETag を返す。
// url は設定で資格情報を含みうるので、エラーには url ではなく、取り込み元の名前 name を書く。
func (c *Client) fetch(ctx context.Context, name, url, etag string, v any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build the request for %s", name)
	}
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	res, err := c.http.Do(req)
	if err != nil {
		// *url.Error は URL の全文を含むので、その中の理由だけを返す。
		if urlErr, ok := errors.AsType[*neturl.Error](err); ok {
			err = urlErr.Err
		}

		return "", fmt.Errorf("failed to get %s: %w", name, err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotModified {
		return "", ErrNotModified
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get %s: unexpected status %s", name, res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", name, err)
	}
	if len(body) > maxResponseBytes {
		return "", fmt.Errorf("%s is larger than %d bytes", name, maxResponseBytes)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return "", fmt.Errorf("failed to decode %s: %w", name, err)
	}

	return res.Header.Get("ETag"), nil
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}
