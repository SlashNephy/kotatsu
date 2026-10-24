package jikkyoclient

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

	"golang.org/x/sync/singleflight"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

const (
	// DefaultNXJikkyoURL は、実況のチャンネルの一覧を返す NX-Jikkyo の URL。
	DefaultNXJikkyoURL = "https://nx-jikkyo.tsukumijima.net"
	// DefaultKakologURL は、過去ログを返すニコニコ実況 過去ログ API の URL (設計書 7.4)。
	DefaultKakologURL = "https://jikkyo.tsukumijima.net"
	// channelsTTL は、チャンネルの一覧を使い回す時間。一覧は、放送中のスレッドの勢いと視聴者数も持つので (#490)、
	// 1 分で取り直す。取り直す間も前の一覧を返すので、待たせない。
	channelsTTL = time.Minute
	// staleWait は、一覧が古くなっていたときに、取り直すのを待つ時間。NX-Jikkyo が応答しなくても、番組表や視聴の画面を長く待たせない。
	staleWait = 500 * time.Millisecond
	// channelsRetryAfter は、チャンネルの一覧を取れなかったときに、取り直すまで待つ時間。
	// 番組表を開くたびに、落ちている NX-Jikkyo に問い合わせないようにする。
	channelsRetryAfter = time.Minute
	// requestTimeout は、NX-Jikkyo と過去ログ API への 1 回の要求の時間の上限。過去ログ API は、長い時間帯で数秒かかる。
	requestTimeout = 30 * time.Second
	// maxResponseBytes は、応答の大きさの上限。過去ログ API は、1 日分で 10 MB 近くになる。
	maxResponseBytes = 256 << 20
	userAgent        = "Kotatsu-Jikkyo/1"
	// threadActive は、放送中のスレッドの状態。
	threadActive = "ACTIVE"
)

// ErrKakolog は、過去ログ API が、取得できなかった理由を返したこと。
var ErrKakolog = errors.New("the kakolog API returned an error")

// Channel は、NX-Jikkyo の実況のチャンネル。
type Channel struct {
	// ID は、jk1 などの ID。
	ID   string
	Name string
	// Activity は、放送中のスレッドの勢いと視聴者数 (#490)。放送中のスレッドがなければ nil。
	Activity *Activity
}

// Activity は、実況のチャンネルの、放送中のスレッドの盛り上がり。
type Activity struct {
	// Force は、勢い (1 分あたりのコメントの数)。
	Force int
	// Viewers は、見ている人の数。
	Viewers int
}

// Comment は、過去ログのコメント。
type Comment struct {
	// At は、コメントを投稿した時刻。
	At time.Time
	// Mail は、コメントの色や位置などのコマンド (184 shita big yellow など)。
	Mail    string
	Content string
	// UserID は、投稿したユーザーの匿名の ID。ミュートに使う (#488)。
	UserID string
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

	// refreshes は、チャンネルの一覧の取得を、同時に 1 つにまとめる。
	refreshes singleflight.Group

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
		refreshes: singleflight.Group{}, mu: sync.Mutex{}, channels: nil, fetchedAt: time.Time{}, attemptedAt: time.Time{}, fetchErr: nil,
	}
}

// Channel は、NX-Jikkyo が今扱っているチャンネルのうち、ID が id のものを返す。扱っていなければ nil を返す。
// チャンネルの一覧は、しばらく使い回す。
//
// 一覧が古くなっていたら、取り直しを staleWait まで待ち、間に合わなければ前に取った一覧を返す。一覧を一度も取れていなければ、取れるまで待つ。
// 取得はロックの外で行い、同時の取得は 1 つにまとめる。NX-Jikkyo が応答しなくても、ほかの要求を止めない。
func (c *Client) Channel(ctx context.Context, id string) (*Channel, error) {
	c.mu.Lock()
	now := c.clock.Now()
	stale := c.channels == nil || now.Sub(c.fetchedAt) >= channelsTTL
	retry := c.fetchErr == nil || now.Sub(c.attemptedAt) >= channelsRetryAfter
	channels, fetchErr := c.channels, c.fetchErr
	c.mu.Unlock()

	var refreshed <-chan singleflight.Result
	if stale && retry {
		refreshed = c.refreshChannels(ctx, now)
	}
	if channels != nil {
		if refreshed != nil {
			// 勢いが古くならないよう、取り直しを少しだけ待つ。間に合わなければ、前に取った一覧を返す。
			if fresh, ok := awaitRefresh(ctx, refreshed); ok {
				return fresh[id], nil
			}
		}

		return channels[id], nil
	}
	if refreshed == nil {
		return nil, fetchErr
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-refreshed:
		if result.Err != nil {
			return nil, result.Err
		}

		refreshedChannels, _ := result.Val.(map[string]*Channel)

		return refreshedChannels[id], nil
	}
}

// awaitRefresh は、取り直した一覧を staleWait まで待つ。取り直せなかったか、間に合わなければ false を返す。
func awaitRefresh(ctx context.Context, refreshed <-chan singleflight.Result) (map[string]*Channel, bool) {
	timer := time.NewTimer(staleWait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, false
	case <-timer.C:
		return nil, false
	case result := <-refreshed:
		channels, ok := result.Val.(map[string]*Channel)

		return channels, result.Err == nil && ok
	}
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
				UserID   string `json:"user_id"`
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
			At: time.Unix(seconds, micros*int64(time.Microsecond)), Mail: packet.Chat.Mail, Content: packet.Chat.Content, UserID: packet.Chat.UserID,
		})
	}
	slices.SortStableFunc(comments, func(a, b *Comment) int { return a.At.Compare(b.At) })

	return comments, nil
}

// refreshChannels は、チャンネルの一覧を取り直し始め、その結果を受け取るチャネルを返す。取り直している最中なら、それに相乗りする。
// 取得は、呼び出した要求を取りやめても続ける。ほかの要求も、その結果を待っているからである。
func (c *Client) refreshChannels(ctx context.Context, now time.Time) <-chan singleflight.Result {
	return c.refreshes.DoChan("channels", func() (any, error) {
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
		defer cancel()
		channels, err := c.fetchChannels(fetchCtx)
		c.mu.Lock()
		defer c.mu.Unlock()
		c.attemptedAt, c.fetchErr = now, err
		if err != nil {
			return nil, err
		}
		c.channels, c.fetchedAt = channels, now

		return channels, nil
	})
}

func (c *Client) fetchChannels(ctx context.Context) (map[string]*Channel, error) {
	var body []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Threads []struct {
			Status      string `json:"status"`
			JikkyoForce int    `json:"jikkyo_force"`
			Viewers     int    `json:"viewers"`
		} `json:"threads"`
	}
	if err := c.getJSON(ctx, c.nxJikkyoURL+"/api/v1/channels", &body); err != nil {
		return nil, fmt.Errorf("failed to fetch the jikkyo channels: %w", err)
	}
	channels := make(map[string]*Channel, len(body))
	for _, channel := range body {
		var activity *Activity
		for _, thread := range channel.Threads {
			if thread.Status == threadActive {
				activity = &Activity{Force: thread.JikkyoForce, Viewers: thread.Viewers}

				break
			}
		}
		channels[channel.ID] = &Channel{ID: channel.ID, Name: channel.Name, Activity: activity}
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
