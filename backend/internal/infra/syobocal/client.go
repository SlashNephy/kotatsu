// Package syobocal は、しょぼいカレンダーの db.php に問い合わせる (設計書 3.8)。
//
// しょぼいカレンダーは個人が運営するサービスなので、問い合わせは控えめにする。
// 番組ごとではなく、チャンネルと期間ごとにまとめて引き、問い合わせの間隔を空ける。結果は呼ぶ側が DB にキャッシュする。
package syobocal

import (
	"context"
	"encoding/xml"
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
)

const (
	// DefaultURL は、しょぼいカレンダーの db.php。
	DefaultURL = "https://cal.syoboi.jp/db.php"
	// DefaultInterval は、問い合わせの間隔の下限 (付録 A)。
	DefaultInterval = 2 * time.Second
	// requestTimeout は、1 回の問い合わせの時間の上限。
	requestTimeout = 30 * time.Second
	// maxResponseBytes は、読むレスポンスの大きさの上限。ProgLookup は 1 回に 5,000 件までしか返さない。
	maxResponseBytes = 16 << 20
	// timeLayout は、db.php の日時の書き方。日本時間で書く。
	timeLayout = "2006-01-02 15:04:05"
	// rangeLayout は、db.php に渡す期間の書き方。日本時間で書く。
	rangeLayout = "20060102_150405"
	// FlagNew、FlagFinal、FlagRerun は、放送時間データのフラグ (https://docs.cal.syoboi.jp/spec/proginfo-flag/)。
	FlagNew   = 0x2
	FlagFinal = 0x4
	// FlagRerun は、再放送の印 (#1233)。
	FlagRerun = 0x8
	// MaxChannelsPerRequest は、1 回の ProgLookup に並べるチャンネルの数の上限。URL を長くしすぎず、5,000 件の上限にも当たらないようにする。
	MaxChannelsPerRequest = 10
)

// ErrUnexpectedResponse は、db.php が想定しない応答を返したことを表す。
var ErrUnexpectedResponse = errors.New("unexpected response from syobocal")

// tokyoOffset は、日本時間の UTC からのずれ (秒)。日本は 1951 年から夏時間を使っていない。
const tokyoOffset = 9 * 60 * 60

// Tokyo は、しょぼいカレンダーの日時のタイムゾーン (日本時間) を返す。表示のタイムゾーンの設定には従わない。
func Tokyo() *time.Location {
	return time.FixedZone("JST", tokyoOffset)
}

// Program は、しょぼいカレンダーの放送時間データ (ProgItem) 1 つ。
type Program struct {
	PID       int32
	TID       int32
	ChannelID int32
	// StartAt と EndAt は、StTime と EdTime。StOffset はまだ足していない。
	StartAt time.Time
	EndAt   time.Time
	// OffsetSeconds は、StOffset (秒)。実際の放送は、StartAt と EndAt をこれだけずらしたもの。
	OffsetSeconds int32
	// Count は、話数 (Count)。話数のない放送 (特番など) では空。
	Count string
	// SubTitle は、放送時間データのサブタイトル。複数話をまとめて放送するときは "#1～#2" のように書かれる。
	SubTitle string
	// STSubTitle は、タイトルのサブタイトルの表から引いた、その話のサブタイトル (JOIN=SubTitles)。
	STSubTitle string
	Flag       int32
	Deleted    bool
	LastUpdate time.Time
}

// Title は、しょぼいカレンダーのタイトル (TitleItem) 1 つ。
type Title struct {
	TID        int32
	Title      string
	LastUpdate time.Time
}

// Options は Client の設定。空の項目は既定値を使う。
type Options struct {
	HTTPClient *http.Client
	URL        string
	// Interval は、問い合わせの間隔の下限。
	Interval time.Duration
}

// Client は、しょぼいカレンダーに問い合わせる。問い合わせは、同じ Client を使う間、Interval 以上の間隔を空ける。
type Client struct {
	http     *http.Client
	url      string
	interval time.Duration

	// mu は、問い合わせを 1 つずつにし、間隔を空ける。
	mu   sync.Mutex
	last time.Time
}

// NewClient は Client を返す。opts が nil なら既定値を使う。
func NewClient(opts *Options) *Client {
	c := &Client{http: http.DefaultClient, url: DefaultURL, interval: DefaultInterval, mu: sync.Mutex{}, last: time.Time{}}
	if opts == nil {
		return c
	}
	if opts.HTTPClient != nil {
		c.http = opts.HTTPClient
	}
	if opts.URL != "" {
		c.url = opts.URL
	}
	if opts.Interval > 0 {
		c.interval = opts.Interval
	}

	return c
}

// progLookupResponse は、ProgLookup の応答。
type progLookupResponse struct {
	Result struct {
		Code    int    `xml:"Code"`
		Message string `xml:"Message"`
	} `xml:"Result"`
	Items []struct {
		PID        int32  `xml:"PID"`
		TID        int32  `xml:"TID"`
		ChID       int32  `xml:"ChID"`
		StTime     string `xml:"StTime"`
		EdTime     string `xml:"EdTime"`
		StOffset   int32  `xml:"StOffset"`
		Count      string `xml:"Count"`
		SubTitle   string `xml:"SubTitle"`
		STSubTitle string `xml:"STSubTitle"`
		Flag       int32  `xml:"Flag"`
		Deleted    int32  `xml:"Deleted"`
		LastUpdate string `xml:"LastUpdate"`
	} `xml:"ProgItems>ProgItem"`
}

// ProgLookup は、チャンネル channelIDs で、from から to までと放送時間が重なる放送時間データを返す。
// db.php は「Range0 < EdTime AND StTime < Range1」で探す。削除された放送時間データも返すので、呼ぶ側で Deleted を見る。
func (c *Client) ProgLookup(ctx context.Context, channelIDs []int32, from, to time.Time) ([]*Program, error) {
	if len(channelIDs) == 0 {
		return []*Program{}, nil
	}
	ids := make([]string, 0, len(channelIDs))
	for _, id := range channelIDs {
		ids = append(ids, strconv.FormatInt(int64(id), 10))
	}
	query := url.Values{
		"Command": {"ProgLookup"},
		"ChID":    {strings.Join(ids, ",")},
		"Range":   {from.In(Tokyo()).Format(rangeLayout) + "-" + to.In(Tokyo()).Format(rangeLayout)},
		"JOIN":    {"SubTitles"},
	}
	var body progLookupResponse
	if err := c.get(ctx, query, &body); err != nil {
		return nil, err
	}
	if body.Result.Code != http.StatusOK {
		return nil, fmt.Errorf("%w: ProgLookup returned %d %s", ErrUnexpectedResponse, body.Result.Code, body.Result.Message)
	}

	programs := make([]*Program, 0, len(body.Items))
	for _, item := range body.Items {
		start, err := parseTime(item.StTime)
		if err != nil {
			return nil, err
		}
		end, err := parseTime(item.EdTime)
		if err != nil {
			return nil, err
		}
		// LastUpdate は、照合には使わない。読めなければ空にする。
		lastUpdate, err := parseTime(item.LastUpdate)
		if err != nil {
			lastUpdate = time.Time{}
		}
		programs = append(programs, &Program{
			PID: item.PID, TID: item.TID, ChannelID: item.ChID, StartAt: start, EndAt: end,
			OffsetSeconds: item.StOffset, Count: strings.TrimSpace(item.Count),
			SubTitle: strings.TrimSpace(item.SubTitle), STSubTitle: strings.TrimSpace(item.STSubTitle),
			Flag: item.Flag, Deleted: item.Deleted != 0, LastUpdate: lastUpdate,
		})
	}

	return programs, nil
}

// titleLookupResponse は、TitleLookup の応答。
type titleLookupResponse struct {
	Result struct {
		Code    int    `xml:"Code"`
		Message string `xml:"Message"`
	} `xml:"Result"`
	Items []struct {
		TID        int32  `xml:"TID"`
		Title      string `xml:"Title"`
		LastUpdate string `xml:"LastUpdate"`
	} `xml:"TitleItems>TitleItem"`
}

// TitleLookup は、since より後に更新されたタイトルの、TID とタイトル名を返す。since が零値なら、すべてのタイトルを返す。
func (c *Client) TitleLookup(ctx context.Context, since time.Time) ([]*Title, error) {
	query := url.Values{
		"Command": {"TitleLookup"},
		"TID":     {"*"},
		"Fields":  {"TID,Title,LastUpdate"},
	}
	if !since.IsZero() {
		query.Set("LastUpdate", since.In(Tokyo()).Format(rangeLayout)+"-")
	}
	var body titleLookupResponse
	if err := c.get(ctx, query, &body); err != nil {
		return nil, err
	}
	if body.Result.Code != http.StatusOK {
		return nil, fmt.Errorf("%w: TitleLookup returned %d %s", ErrUnexpectedResponse, body.Result.Code, body.Result.Message)
	}
	titles := make([]*Title, 0, len(body.Items))
	for _, item := range body.Items {
		lastUpdate, err := parseTime(item.LastUpdate)
		if err != nil {
			lastUpdate = time.Time{}
		}
		titles = append(titles, &Title{TID: item.TID, Title: strings.TrimSpace(item.Title), LastUpdate: lastUpdate})
	}

	return titles, nil
}

// get は、db.php に query で問い合わせ、XML を v に読む。前の問い合わせから Interval が過ぎるまで待つ。
func (c *Client) get(ctx context.Context, query url.Values, v any) error {
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("failed to build the request: %w", err)
	}
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to query syobocal: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %s", ErrUnexpectedResponse, res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read the response from syobocal: %w", err)
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("%w: larger than %d bytes", ErrUnexpectedResponse, maxResponseBytes)
	}
	if err := xml.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%w: %w", ErrUnexpectedResponse, err)
	}

	return nil
}

// parseTime は、db.php の日本時間の日時を読む。
func parseTime(value string) (time.Time, error) {
	parsed, err := time.ParseInLocation(timeLayout, strings.TrimSpace(value), Tokyo())
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid time %q", ErrUnexpectedResponse, value)
	}

	return parsed, nil
}
