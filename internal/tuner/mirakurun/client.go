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
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/config"
)

const (
	// PriorityHeader は、Mirakurun と mirakc がチューナーの優先度を受け取るヘッダー。
	PriorityHeader = "X-Mirakurun-Priority"

	defaultUserAgent      = "kotatsu"
	defaultRequestTimeout = 30 * time.Second
	defaultDetectInterval = 10 * time.Minute
	// maxErrorBodySize は、エラーの応答から読む本文の上限。
	maxErrorBodySize = 64 << 10
	// maxLogoSize は、ロゴの画像として読む本文の上限。放送のロゴは数 KB に収まる。
	maxLogoSize = 1 << 20
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
	// DetectInterval は、サーバーの種類を判定し直す間隔。0 なら 10 分。
	DetectInterval time.Duration
	// StreamIdleTimeout は、ストリームのデータが届かないまま待つ時間の上限。0 なら DefaultStreamIdleTimeout。
	StreamIdleTimeout time.Duration
	// StreamOpenTimeout は、ストリームを開くときに、応答の頭を待つ時間の上限。0 なら DefaultStreamOpenTimeout。
	StreamOpenTimeout time.Duration
	Clock             clock.Clock
}

// Client は Mirakurun 互換のチューナーサーバーのクライアント。複数の goroutine から使ってよい。
type Client struct {
	baseURL        *url.URL
	username       string
	password       config.Secret
	userAgent      string
	httpClient     *http.Client
	requestTimeout time.Duration
	detectInterval time.Duration
	idleTimeout    time.Duration
	openTimeout    time.Duration
	clock          clock.Clock

	mu sync.Mutex
	// server は、最後に判定したサーバーの種類。detectedAt から detectInterval が過ぎたら判定し直す。
	server     *ServerInfo
	detectedAt time.Time
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

var _ error = new(APIError)

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
	detectInterval := opts.DetectInterval
	if detectInterval == 0 {
		detectInterval = defaultDetectInterval
	}
	idleTimeout := opts.StreamIdleTimeout
	if idleTimeout == 0 {
		idleTimeout = DefaultStreamIdleTimeout
	}
	openTimeout := opts.StreamOpenTimeout
	if openTimeout == 0 {
		openTimeout = DefaultStreamOpenTimeout
	}
	clk := opts.Clock
	if clk == nil {
		clk = new(clock.Real)
	}

	return &Client{
		baseURL:        baseURL,
		username:       username,
		password:       password,
		userAgent:      userAgent,
		httpClient:     httpClient,
		requestTimeout: requestTimeout,
		detectInterval: detectInterval,
		idleTimeout:    idleTimeout,
		openTimeout:    openTimeout,
		clock:          clk,
		mu:             sync.Mutex{},
		server:         nil,
		detectedAt:     time.Time{},
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

// ServerDate は、/api/version の応答の Date ヘッダーから、チューナーサーバーの時計の時刻を返す (設計書 3.7)。
// Date ヘッダーは秒の精度なので、比べるときは 1 秒程度の誤差を見込む。
func (c *Client) ServerDate(ctx context.Context) (time.Time, error) {
	var version Version
	header, err := c.get(ctx, "api/version", nil, &version)
	if err != nil {
		return time.Time{}, err
	}
	date, err := http.ParseTime(header.Get("Date"))
	if err != nil {
		return time.Time{}, fmt.Errorf("the tuner server returned no valid Date header: %w", err)
	}

	return date.UTC(), nil
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

// OnAirProgram は、mirakc の /api/onair が返す、サービスで放送中の番組と次の番組。
// /api/programs と違い、EIT[schedule] ではなく EIT[p/f] から作るので、延長や繰り上げがすぐに反映される。
type OnAirProgram struct {
	// ServiceID は Mirakurun のサービス ID (networkId × 100000 + serviceId)。
	ServiceID int64 `json:"serviceId"`
	// Current は放送中の番組。放送していなければ nil。
	Current *Program `json:"current"`
	// Next は次の番組。なければ nil。
	Next *Program `json:"next"`
}

// OnAirPrograms は、mirakc の /api/onair を取得する (設計書 5.1)。
// /api/onair を持たないサーバー (Mirakurun と Mahiron) では、取得せずに空のスライスを返す。
func (c *Client) OnAirPrograms(ctx context.Context) ([]*OnAirProgram, error) {
	server, err := c.Server(ctx)
	if err != nil {
		return nil, err
	}
	if !server.OnAir {
		return []*OnAirProgram{}, nil
	}

	programs := []*OnAirProgram{}
	if err := c.getJSON(ctx, "api/onair", nil, &programs); err != nil {
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
// opts が nil なら、スクランブルを解除せず、優先度 0 で開く。
// チューナーを確保できなければ、StatusCode が 503 の *APIError を返す。
func (c *Client) OpenServiceStream(ctx context.Context, serviceID int64, opts *StreamOptions) (io.ReadCloser, error) {
	if opts == nil {
		opts = &StreamOptions{Decode: false, Priority: 0}
	}
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

// ServiceLogo は、サービス ID (networkId × 100000 + serviceId) のロゴの画像を返す。Mirakurun 互換のサーバーは PNG を返す。
// サービスがなければ StatusCode が 404、ロゴをまだ受信していなければ 503 の *APIError を返す。
func (c *Client) ServiceLogo(ctx context.Context, serviceID int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	req, err := c.newRequest(ctx, "api/services/"+strconv.FormatInt(serviceID, 10)+"/logo", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request %s %s: %w", req.Method, req.URL.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxLogoSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read the response of %s %s: %w", req.Method, req.URL.Redacted(), err)
	}
	if len(data) > maxLogoSize {
		return nil, fmt.Errorf("the logo of %s %s exceeds %d bytes", req.Method, req.URL.Redacted(), maxLogoSize)
	}

	return data, nil
}

// openStream は、ストリームのリクエストを送り、2xx なら本文を返す。
// 本文は、データが idleTimeout の間届かなければ閉じ、ErrStreamStalled を返す。接続を保ったまま送らなくなったことに気づくため (#537)。
// 応答の頭が openTimeout までに届かなければ、要求をやめて ErrStreamOpenTimeout を返す (#412)。
// ストリームは何時間も続くので、要求全体には期限を付けず、応答の頭を待つ間だけ見張る。
func (c *Client) openStream(req *http.Request) (io.ReadCloser, error) {
	ctx, cancel := context.WithCancelCause(req.Context())
	timer := time.AfterFunc(c.openTimeout, func() { cancel(ErrStreamOpenTimeout) })
	resp, err := c.httpClient.Do(req.WithContext(ctx))
	// 応答が届いた直後に上限が来たら、ストリームは使えないので、届かなかったときと同じく扱う。
	if !timer.Stop() && err == nil {
		_ = resp.Body.Close()
		err = context.Cause(ctx)
	}
	if err != nil {
		cancel(nil)
		if errors.Is(context.Cause(ctx), ErrStreamOpenTimeout) {
			return nil, fmt.Errorf("failed to request %s %s: %w", req.Method, req.URL.Redacted(), ErrStreamOpenTimeout)
		}

		return nil, fmt.Errorf("failed to request %s %s: %w", req.Method, req.URL.Redacted(), err)
	}
	if err := checkResponse(resp); err != nil {
		_ = resp.Body.Close()
		cancel(nil)

		return nil, err
	}

	return newIdleReader(resp.Body, c.idleTimeout, func() { cancel(nil) }), nil
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, v any) error {
	_, err := c.get(ctx, path, query, v)

	return err
}

// get は JSON の API を呼び、応答を v に読み込んで、応答のヘッダーを返す。
func (c *Client) get(ctx context.Context, path string, query url.Values, v any) (http.Header, error) {
	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	req, err := c.newRequest(ctx, path, query)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request %s %s: %w", req.Method, req.URL.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return nil, fmt.Errorf("failed to decode the response of %s %s: %w", req.Method, req.URL.Redacted(), err)
	}

	return resp.Header, nil
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
