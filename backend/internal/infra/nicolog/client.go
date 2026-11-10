// Package nicolog は、NICOLOG (ニコニコ生放送のアニメのコメントのアーカイブ) に問い合わせる (#1363、設計書 7.4)。
//
// NICOLOG は個人が運営するサービスなので、問い合わせは控えめにする。
package nicolog

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
)

const (
	// DefaultURL は、NICOLOG の URL。
	DefaultURL = "https://nicolog.oyasumi.today"
	// RootPath は、コメントのファイルを置いたフォルダ。
	RootPath = "/nico-live-anime"
	// requestTimeout は、1 回の問い合わせの時間の上限。
	requestTimeout = 30 * time.Second
	// maxResponseBytes は、読むレスポンスの大きさの上限。コメントの XML は、1 話で数 MB ほど。
	maxResponseBytes = 64 << 20
	// vposPerSecond は、vpos の 1 秒あたりの値。
	vposPerSecond = 100
)

// ErrUnexpectedResponse は、NICOLOG が想定と違う応答を返したことを表す。
var ErrUnexpectedResponse = errors.New("nicolog returned an unexpected response")

// Entry は、フォルダの中のファイルかフォルダ。
type Entry struct {
	Name  string
	IsDir bool
}

// Comment は、上映会のコメント 1 件。Time は、上映会の開始からの秒。
type Comment struct {
	Time    float64
	Mail    string
	Content string
	UserID  string
}

// Options は、Client の設定。
type Options struct {
	HTTPClient *http.Client
	// URL は、NICOLOG の URL。空なら DefaultURL。
	URL string
}

// Client は、NICOLOG に問い合わせる。
type Client struct {
	http *http.Client
	url  string
}

// NewHTTPClient は、NICOLOG への問い合わせに使う HTTP クライアントを返す。
func NewHTTPClient(recorder *netactivity.Recorder) *http.Client {
	return netactivity.NewClient(recorder, netactivity.PurposeNicologComments, netactivity.ClientOptions{Transport: nil, Timeout: requestTimeout, CheckRedirect: checkRedirect})
}

// checkRedirect は、ほかのホストへのリダイレクトを断る。NICOLOG の応答に従って、内部のネットワークへ接続させないためである。
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 0 && req.URL.Host != via[0].URL.Host {
		return fmt.Errorf("%w: redirected to another host", ErrUnexpectedResponse)
	}

	return nil
}

// NewClient は Client を返す。opts.HTTPClient は NewHTTPClient で作る。
func NewClient(opts *Options) *Client {
	c := &Client{http: opts.HTTPClient, url: DefaultURL}
	if opts.URL != "" {
		c.url = strings.TrimRight(opts.URL, "/")
	}

	return c
}

type apiResponse[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type listData struct {
	Content []struct {
		Name  string `json:"name"`
		IsDir bool   `json:"is_dir"`
	} `json:"content"`
}

type getData struct {
	RawURL string `json:"raw_url"`
}

// List は、フォルダ path の中身を返す。
func (c *Client) List(ctx context.Context, path string) ([]*Entry, error) {
	var body apiResponse[listData]
	if err := c.post(ctx, "/api/fs/list", map[string]any{"path": path, "page": 1, "per_page": 0}, &body); err != nil {
		return nil, err
	}
	entries := make([]*Entry, 0, len(body.Data.Content))
	for _, item := range body.Data.Content {
		entries = append(entries, &Entry{Name: item.Name, IsDir: item.IsDir})
	}

	return entries, nil
}

// Comments は、ファイル path のコメントを返す。
func (c *Client) Comments(ctx context.Context, path string) ([]*Comment, error) {
	var body apiResponse[getData]
	if err := c.post(ctx, "/api/fs/get", map[string]any{"path": path}, &body); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(body.Data.RawURL, c.url+"/") {
		return nil, fmt.Errorf("%w: the file is not served by nicolog", ErrUnexpectedResponse)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, body.Data.RawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build the request for nicolog: %w", err)
	}
	raw, err := c.do(req)
	if err != nil {
		return nil, err
	}

	return parseComments(raw)
}

func (c *Client) post(ctx context.Context, path string, payload, v any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode the request for nicolog: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+path, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("failed to build the request for nicolog: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	raw, err := c.do(req)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%w: %w", ErrUnexpectedResponse, err)
	}
	var head apiResponse[json.RawMessage]
	if err := json.Unmarshal(raw, &head); err != nil {
		return fmt.Errorf("%w: %w", ErrUnexpectedResponse, err)
	}
	if head.Code != http.StatusOK {
		return fmt.Errorf("%w: code %d", ErrUnexpectedResponse, head.Code)
	}

	return nil
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	res, err := c.http.Do(req)
	if err != nil {
		// *url.Error は URL の全文を含むので、その中の理由だけを返す。
		if urlErr, ok := errors.AsType[*neturl.Error](err); ok {
			err = urlErr.Err
		}

		return nil, fmt.Errorf("failed to request nicolog: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %s", ErrUnexpectedResponse, res.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read the response from nicolog: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return nil, fmt.Errorf("%w: the response is too large", ErrUnexpectedResponse)
	}

	return raw, nil
}

type packet struct {
	Chats []struct {
		Vpos    string `xml:"vpos,attr"`
		Mail    string `xml:"mail,attr"`
		UserID  string `xml:"user_id,attr"`
		Content string `xml:",chardata"`
	} `xml:"chat"`
}

// parseComments は、コメントの XML を読む。vpos の読めないコメントは除く。
func parseComments(raw []byte) ([]*Comment, error) {
	var body packet
	if err := xml.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnexpectedResponse, err)
	}
	comments := make([]*Comment, 0, len(body.Chats))
	for _, chat := range body.Chats {
		vpos, err := strconv.ParseFloat(chat.Vpos, 64)
		if err != nil {
			continue
		}
		comments = append(comments, &Comment{Time: vpos / vposPerSecond, Mail: chat.Mail, Content: chat.Content, UserID: chat.UserID})
	}

	return comments, nil
}
