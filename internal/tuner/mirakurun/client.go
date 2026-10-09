package mirakurun

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
	"time"

	"github.com/SlashNephy/kotatsu/internal/config"
)

const (
	// PriorityHeader は、Mirakurun と mirakc がチューナーの優先度を受け取るヘッダー。
	PriorityHeader = "X-Mirakurun-Priority"

	defaultUserAgent      = "kotatsu"
	defaultRequestTimeout = 30 * time.Second
	// maxErrorBodySize は、エラーの応答から読む本文の上限。
	maxErrorBodySize = 64 << 10
)

// Options は Client の設定。
type Options struct {
	// URL はチューナーサーバーの URL (例: http://mirakurun:40772/)。
	URL string
	// Username と Password は BASIC 認証の資格情報。Username が空なら認証しない。
	Username string
	Password config.Secret
	// UserAgent は、チューナーサーバーに名乗る名前。Mirakurun はチューナーの利用者の表示に使う。空なら "kotatsu"。
	UserAgent string
	// HTTPClient は通信に使うクライアント。nil なら http.DefaultClient を使う。
	// サービスストリームは何時間も続くので、Timeout を設定したクライアントを渡してはいけない。
	HTTPClient *http.Client
	// RequestTimeout は、ストリーム以外の API の呼び出しにかける時間の上限。0 なら 30 秒。
	RequestTimeout time.Duration
}

// Client は Mirakurun 互換のチューナーサーバーのクライアント。複数の goroutine から使ってよい。
type Client struct {
	baseURL        *url.URL
	username       string
	password       config.Secret
	userAgent      string
	httpClient     *http.Client
	requestTimeout time.Duration
}

// APIError は、チューナーサーバーが 2xx 以外の状態を返したことを表す。
// サービスストリームを開くときの 503 はチューナーを確保できなかったことを表すので、呼び出し側は再試行できる。
type APIError struct {
	Method     string
	URL        string
	StatusCode int
	// Reason は、Mirakurun のエラー応答の reason。読めなければ本文の先頭をそのまま持つ。
	Reason string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("tuner server responded %d to %s %s: %s", e.StatusCode, e.Method, e.URL, e.Reason)
}

// New は Client を作る。
func New(opts *Options) (*Client, error) {
	baseURL, err := url.Parse(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the tuner server URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("tuner server URL must be http or https: %s", baseURL.Redacted())
	}
	if !strings.HasSuffix(baseURL.Path, "/") {
		baseURL.Path += "/"
	}

	username, password := opts.Username, opts.Password
	// URL に資格情報が含まれていれば、それも使う。ログに出さないよう、URL からは取り除いて持つ。
	if baseURL.User != nil {
		if username == "" {
			username = baseURL.User.Username()
			if p, ok := baseURL.User.Password(); ok {
				password = config.Secret(p)
			}
		}
		baseURL.User = nil
	}

	userAgent := opts.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	requestTimeout := opts.RequestTimeout
	if requestTimeout == 0 {
		requestTimeout = defaultRequestTimeout
	}

	return &Client{
		baseURL:        baseURL,
		username:       username,
		password:       password,
		userAgent:      userAgent,
		httpClient:     httpClient,
		requestTimeout: requestTimeout,
	}, nil
}

// Version は /api/version を取得する。
func (c *Client) Version(ctx context.Context) (*Version, error) {
	var version Version
	if err := c.getJSON(ctx, "api/version", nil, &version); err != nil {
		return nil, err
	}

	return &version, nil
}

// Channels は /api/channels を取得する。
func (c *Client) Channels(ctx context.Context) ([]*Channel, error) {
	channels := []*Channel{}
	if err := c.getJSON(ctx, "api/channels", nil, &channels); err != nil {
		return nil, err
	}

	return channels, nil
}

// Services は /api/services を取得する。
func (c *Client) Services(ctx context.Context) ([]*Service, error) {
	services := []*Service{}
	if err := c.getJSON(ctx, "api/services", nil, &services); err != nil {
		return nil, err
	}

	return services, nil
}

// ProgramsQuery は /api/programs の絞り込みの条件。0 の項目では絞り込まない。
type ProgramsQuery struct {
	NetworkID int
	ServiceID int
	EventID   int
}

// Programs は /api/programs を取得する。query が nil なら全件を取得する。
func (c *Client) Programs(ctx context.Context, query *ProgramsQuery) ([]*Program, error) {
	values := url.Values{}
	if query != nil {
		for key, v := range map[string]int{"networkId": query.NetworkID, "serviceId": query.ServiceID, "eventId": query.EventID} {
			if v != 0 {
				values.Set(key, strconv.Itoa(v))
			}
		}
	}

	programs := []*Program{}
	if err := c.getJSON(ctx, "api/programs", values, &programs); err != nil {
		return nil, err
	}

	return programs, nil
}

// Tuners は /api/tuners を取得する。
func (c *Client) Tuners(ctx context.Context) ([]*Tuner, error) {
	tuners := []*Tuner{}
	if err := c.getJSON(ctx, "api/tuners", nil, &tuners); err != nil {
		return nil, err
	}

	return tuners, nil
}

// StreamOptions は、サービスストリームを開くときの設定。
type StreamOptions struct {
	// Decode が true なら、チューナーサーバーにスクランブルを解除させる。
	Decode bool
	// Priority はチューナーの優先度。チューナーが足りないとき、優先度の低い利用者からチューナーを奪う。
	// 負の値なら、他の利用者からチューナーを奪わない。
	Priority int
}

// OpenServiceStream は、サービス ID (networkId × 100000 + serviceId) のストリームを開く。
// 返す io.ReadCloser は、読み終えたら必ず閉じる。ctx が終わると、ストリームも閉じる。
// チューナーを確保できなければ、StatusCode が 503 の *APIError を返す。
func (c *Client) OpenServiceStream(ctx context.Context, serviceID int64, opts *StreamOptions) (io.ReadCloser, error) {
	values := url.Values{}
	if opts.Decode {
		values.Set("decode", "1")
	}
	req, err := c.newRequest(ctx, "api/services/"+strconv.FormatInt(serviceID, 10)+"/stream", values)
	if err != nil {
		return nil, err
	}
	req.Header.Set(PriorityHeader, strconv.Itoa(opts.Priority))

	return c.openStream(req)
}

// openStream は、ストリームのリクエストを送り、2xx なら本文を返す。
func (c *Client) openStream(req *http.Request) (io.ReadCloser, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request %s %s: %w", req.Method, req.URL.Redacted(), err)
	}
	if err := checkResponse(resp); err != nil {
		_ = resp.Body.Close()

		return nil, err
	}

	return resp.Body, nil
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, v any) error {
	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	req, err := c.newRequest(ctx, path, query)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to request %s %s: %w", req.Method, req.URL.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkResponse(resp); err != nil {
		return err
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("failed to decode the response of %s %s: %w", req.Method, req.URL.Redacted(), err)
	}

	return nil
}

func (c *Client) newRequest(ctx context.Context, path string, query url.Values) (*http.Request, error) {
	u := c.baseURL.JoinPath(path)
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build a request to %s: %w", u.Redacted(), err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password.Value())
	}

	return req, nil
}

// checkResponse は、2xx 以外の応答を *APIError にする。
func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
	reason := strings.TrimSpace(string(body))
	var payload struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Reason != "" {
		reason = payload.Reason
	}

	return &APIError{
		Method:     resp.Request.Method,
		URL:        resp.Request.URL.Redacted(),
		StatusCode: resp.StatusCode,
		Reason:     reason,
	}
}

// IsStatus は、err が status の *APIError かどうかを返す。
func IsStatus(err error, status int) bool {
	var apiErr *APIError

	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}
