package jikkyo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/clock"
)

const (
	// DefaultNXJikkyoURL は、実況のチャンネルの一覧を返す NX-Jikkyo の URL。
	DefaultNXJikkyoURL = "https://nx-jikkyo.tsukumijima.net"
	// DefaultKakologURL は、過去ログを返すニコニコ実況 過去ログ API の URL (設計書 7.4)。
	DefaultKakologURL = "https://jikkyo.tsukumijima.net"
	// channelsTTL は、チャンネルの一覧を使い回す時間。チャンネルが増減するのはまれなので、長めにする。
	channelsTTL = time.Hour
	// channelsRetryAfter は、チャンネルの一覧を取れなかったときに、取り直すまで待つ時間。
	// 番組表を開くたびに、落ちている NX-Jikkyo に問い合わせないようにする。
	channelsRetryAfter = time.Minute
	// requestTimeout は、NX-Jikkyo と過去ログ API への 1 回の要求の時間の上限。過去ログ API は、長い時間帯で数秒かかる。
	requestTimeout = 30 * time.Second
	// maxResponseBytes は、応答の大きさの上限。過去ログ API は、1 日分で 10 MB 近くになる。
	maxResponseBytes = 256 << 20
	userAgent        = "Kotatsu-Jikkyo/1"
)

// ErrKakolog は、過去ログ API が、取得できなかった理由を返したこと。
var ErrKakolog = errors.New("the kakolog API returned an error")

// Channel は、NX-Jikkyo の実況のチャンネル。
type Channel struct {
	// ID は、jk1 などの ID。
	ID   string
	Name string
}

// Comment は、過去ログのコメント。
type Comment struct {
	// At は、コメントを投稿した時刻。
	At time.Time
	// Mail は、コメントの色や位置などのコマンド (184 shita big yellow など)。
	Mail    string
	Content string
}

// ClientOptions は、Client の設定。
type ClientOptions struct {
	// NXJikkyoURL と KakologURL は、テストで差し替える。空なら既定の URL を使う。
	NXJikkyoURL string
	KakologURL  string
	HTTPClient  *http.Client
	Clock       clock.Clock
}

// Client は、NX-Jikkyo のチャンネルの一覧と、過去ログ API から過去ログを取る。
type Client struct {
	nxJikkyoURL string
	kakologURL  string
	http        *http.Client
	clock       clock.Clock

	mu sync.Mutex
	// channels は、取得したチャンネルの一覧。fetchedAt は、一覧を取得できた時刻。
	channels  map[string]*Channel
	fetchedAt time.Time
	// attemptedAt と fetchErr は、最後に取得を試みた時刻と、そのときのエラー。
	attemptedAt time.Time
	fetchErr    error
}

// NewClient は Client を返す。
func NewClient(opts *ClientOptions) *Client {
	var o ClientOptions
	if opts != nil {
		o = *opts
	}
	if o.NXJikkyoURL == "" {
		o.NXJikkyoURL = DefaultNXJikkyoURL
	}
	if o.KakologURL == "" {
		o.KakologURL = DefaultKakologURL
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: requestTimeout}
	}
	if o.Clock == nil {
		o.Clock = new(clock.Real)
	}

	return &Client{
		nxJikkyoURL: o.NXJikkyoURL, kakologURL: o.KakologURL, http: o.HTTPClient, clock: o.Clock,
		mu: sync.Mutex{}, channels: nil, fetchedAt: time.Time{}, attemptedAt: time.Time{}, fetchErr: nil,
	}
}

// Channel は、NX-Jikkyo が今扱っているチャンネルのうち、ID が id のものを返す。扱っていなければ nil を返す。
// チャンネルの一覧は、しばらく使い回す。
func (c *Client) Channel(ctx context.Context, id string) (*Channel, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock.Now()
	stale := c.channels == nil || now.Sub(c.fetchedAt) >= channelsTTL
	retry := c.fetchErr == nil || now.Sub(c.attemptedAt) >= channelsRetryAfter
	if stale && retry {
		channels, err := c.fetchChannels(ctx)
		// 要求を取りやめたせいで取れなかったときは、NX-Jikkyo が落ちているとはみなさず、次の要求で取り直す。
		if err != nil && ctx.Err() != nil {
			return nil, err
		}
		c.attemptedAt, c.fetchErr = now, err
		if err == nil {
			c.channels, c.fetchedAt = channels, now
		}
	}
	// 取り直せなくても、前に取った一覧があれば使う。
	if c.channels == nil {
		return nil, c.fetchErr
	}

	return c.channels[id], nil
}

// Comments は、実況のチャンネル id の、from から to までの過去ログを、投稿した時刻の順に返す。
func (c *Client) Comments(ctx context.Context, id string, from, to time.Time) ([]*Comment, error) {
	query := url.Values{
		"starttime": {strconv.FormatInt(from.Unix(), 10)},
		"endtime":   {strconv.FormatInt(to.Unix(), 10)},
		"format":    {"json"},
	}
	var body struct {
		Error  string `json:"error"`
		Packet []struct {
			Chat struct {
				Date     string `json:"date"`
				DateUsec string `json:"date_usec"`
				Mail     string `json:"mail"`
				Content  string `json:"content"`
			} `json:"chat"`
		} `json:"packet"`
	}
	if err := c.getJSON(ctx, c.kakologURL+"/api/kakolog/"+url.PathEscape(id)+"?"+query.Encode(), &body); err != nil {
		return nil, fmt.Errorf("failed to fetch the jikkyo comments: %w", err)
	}
	// 過去ログ API は、取得できない理由を 200 で返す。
	if body.Error != "" {
		return nil, fmt.Errorf("%w: %s", ErrKakolog, body.Error)
	}
	comments := make([]*Comment, 0, len(body.Packet))
	for _, packet := range body.Packet {
		seconds, err := strconv.ParseInt(packet.Chat.Date, 10, 64)
		if err != nil {
			continue
		}
		// date_usec がないコメントもあるので、そのときは秒の単位で扱う。
		micros, _ := strconv.ParseInt(packet.Chat.DateUsec, 10, 64)
		comments = append(comments, &Comment{
			At: time.Unix(seconds, micros*int64(time.Microsecond)), Mail: packet.Chat.Mail, Content: packet.Chat.Content,
		})
	}
	slices.SortStableFunc(comments, func(a, b *Comment) int { return a.At.Compare(b.At) })

	return comments, nil
}

func (c *Client) fetchChannels(ctx context.Context) (map[string]*Channel, error) {
	var body []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.getJSON(ctx, c.nxJikkyoURL+"/api/v1/channels", &body); err != nil {
		return nil, fmt.Errorf("failed to fetch the jikkyo channels: %w", err)
	}
	channels := make(map[string]*Channel, len(body))
	for _, channel := range body {
		channels[channel.ID] = &Channel{ID: channel.ID, Name: channel.Name}
	}

	return channels, nil
}

func (c *Client) getJSON(ctx context.Context, rawURL string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create a request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send a request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// 本文を読み切って、接続を使い回せるようにする。
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))

		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(v); err != nil {
		return fmt.Errorf("failed to decode the response: %w", err)
	}

	return nil
}
