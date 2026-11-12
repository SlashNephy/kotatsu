package mirakurun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
)

const (
	// PriorityHeader は、Mirakurun と mirakc がチューナーの優先度を受け取るヘッダー。
	PriorityHeader = "X-Mirakurun-Priority"
	// TunerUserIDHeader は、Mirakurun、mirakc、Mahiron が、ストリームの応答で、チューナーの利用者の ID を返すヘッダー。
	// /api/tuners の users[].id と同じ値になる (#1467)。
	TunerUserIDHeader = "X-Mirakurun-Tuner-User-ID"

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
	// StreamIdleTimeout は、放送のストリームのデータが届かないまま待つ時間の上限。0 なら DefaultStreamIdleTimeout。イベントストリームには使わない。
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

var _ tunerserver.Client = new(Client)

// APIError は、チューナーサーバーが 2xx 以外の状態を返したことを表す。
// errors.Is で tunerserver.ErrRefused に当たる。呼び出した API と状態によっては、
// tunerserver.ErrTunerUnavailable のような、状態の意味を表す sentinel にも当たる。
type APIError struct {
	Method     string
	URL        string
	StatusCode int
	// Reason は、Mirakurun のエラー応答の reason。読めなければ本文の先頭をそのまま持つ。
	Reason string
	// cause は、状態の意味を表す tunerserver の sentinel。当たるものがなければ nil。
	cause error
}

var _ error = new(APIError)

func (e *APIError) Error() string {
	return fmt.Sprintf("tuner server responded %d to %s %s: %s", e.StatusCode, e.Method, e.URL, e.Reason)
}

// Unwrap は、状態の意味を表す sentinel を返す。当たるものがなければ tunerserver.ErrRefused を返す。
// tunerserver の sentinel は、どれも tunerserver.ErrRefused に当たる。
func (e *APIError) Unwrap() error {
	if e.cause == nil {
		return tunerserver.ErrRefused
	}

	return e.cause
}

// causeOf は、API ごとに、状態を tunerserver の sentinel に読み替える。当たるものがなければ nil を返す。
type causeOf func(status int) error

// streamCause は、ストリームを開くときの状態の意味を返す。503 はチューナーを確保できなかったこと、404 はサービスかチャンネルがないこと。
func streamCause(status int) error {
	switch status {
	case http.StatusServiceUnavailable:
		return tunerserver.ErrTunerUnavailable
	case http.StatusNotFound:
		return tunerserver.ErrNotFound
	default:
		return nil
	}
}

// logoCause は、ロゴを取るときの状態の意味を返す。Mirakurun は、ロゴをまだ受信していなければ 503 を返す。
func logoCause(status int) error {
	switch status {
	case http.StatusServiceUnavailable:
		return tunerserver.ErrLogoUnavailable
	case http.StatusNotFound:
		return tunerserver.ErrNotFound
	default:
		return nil
	}
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
func (c *Client) Channels(ctx context.Context) ([]*tunerserver.Channel, error) {
	channels := []*channel{}
	if err := c.getJSON(ctx, "api/channels", nil, &channels); err != nil {
		return nil, err
	}

	return convert(channels, (*channel).toTunerServer), nil
}

// Services は /api/services を取得する。
func (c *Client) Services(ctx context.Context) ([]*tunerserver.Service, error) {
	services := []*service{}
	if err := c.getJSON(ctx, "api/services", nil, &services); err != nil {
		return nil, err
	}

	return convert(services, (*service).toTunerServer), nil
}

// Programs は /api/programs を取得する。query が nil なら全件を取得する。
func (c *Client) Programs(ctx context.Context, query *tunerserver.ProgramsQuery) ([]*tunerserver.Program, error) {
	values := url.Values{}
	if query != nil {
		for key, v := range map[string]int{"networkId": query.NetworkID, "serviceId": query.ServiceID, "eventId": query.EventID} {
			if v != 0 {
				values.Set(key, strconv.Itoa(v))
			}
		}
	}

	programs := []*program{}
	if err := c.getJSON(ctx, "api/programs", values, &programs); err != nil {
		return nil, err
	}

	return convert(programs, (*program).toTunerServer), nil
}

// OnAirPrograms は、mirakc の /api/onair を取得する (設計書 5.1)。
// /api/programs と違い、EIT[schedule] ではなく EIT[p/f] から作るので、延長や繰り上げがすぐに反映される。
// /api/onair を持たないサーバー (Mirakurun と Mahiron) では、取得せずに空のスライスを返す。
func (c *Client) OnAirPrograms(ctx context.Context) ([]*tunerserver.OnAirProgram, error) {
	server, err := c.Server(ctx)
	if err != nil {
		return nil, err
	}
	if !server.OnAir {
		return []*tunerserver.OnAirProgram{}, nil
	}

	programs := []*onAirProgram{}
	if err := c.getJSON(ctx, "api/onair", nil, &programs); err != nil {
		return nil, err
	}

	return convert(programs, (*onAirProgram).toTunerServer), nil
}

// Tuners は /api/tuners を取得する。
func (c *Client) Tuners(ctx context.Context) ([]*tunerserver.Tuner, error) {
	tuners := []*tuner{}
	if err := c.getJSON(ctx, "api/tuners", nil, &tuners); err != nil {
		return nil, err
	}

	return convert(tuners, (*tuner).toTunerServer), nil
}

// OpenServiceStream は、サービス ID (networkId × 100000 + serviceId) のストリームを開く。
// 返す io.ReadCloser は、読み終えたら必ず閉じる。ctx が終わると、ストリームも閉じる。
// opts が nil なら、スクランブルを解除せず、優先度 0 で開く。
// チューナーを確保できなければ (503)、tunerserver.ErrTunerUnavailable に当たる *APIError を返す。
// サービスがなければ (404)、tunerserver.ErrNotFound に当たる *APIError を返す。
func (c *Client) OpenServiceStream(ctx context.Context, serviceID int64, opts *tunerserver.StreamOptions) (io.ReadCloser, error) {
	if opts == nil {
		opts = &tunerserver.StreamOptions{Decode: false, Priority: 0}
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

	return c.openStream(req, c.idleTimeout, streamCause)
}

// OpenChannelStream は、チャンネル種別 channelType (GR、BS など) のチャンネル channel のストリームを、
// そのチャンネルのすべてのサービスを含んだまま開く (#830)。返す io.ReadCloser は、読み終えたら必ず閉じる。
// opts が nil なら、スクランブルを解除せず、優先度 0 で開く。エラーは OpenServiceStream と同じく返す。
func (c *Client) OpenChannelStream(ctx context.Context, channelType, channel string, opts *tunerserver.StreamOptions) (io.ReadCloser, error) {
	if opts == nil {
		opts = &tunerserver.StreamOptions{Decode: false, Priority: 0}
	}
	values := url.Values{}
	if opts.Decode {
		values.Set("decode", "1")
	}
	req, err := c.newRequest(ctx, "api/channels/"+url.PathEscape(channelType)+"/"+url.PathEscape(channel)+"/stream", values)
	if err != nil {
		return nil, err
	}
	req.Header.Set(PriorityHeader, strconv.Itoa(opts.Priority))

	return c.openStream(req, c.idleTimeout, streamCause)
}

// ServiceLogo は、サービス ID (networkId × 100000 + serviceId) のロゴの画像を返す。Mirakurun 互換のサーバーは PNG を返す。
// サービスがなければ (404) tunerserver.ErrNotFound に、ロゴをまだ受信していなければ (503) tunerserver.ErrLogoUnavailable に当たる
// *APIError を返す。
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

	if err := checkResponse(resp, logoCause); err != nil {
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
// 本文は、データが idleTimeout の間届かなければ閉じ、tunerserver.ErrStreamStalled を返す。接続を保ったまま送らなくなったことに気づくため (#537)。
// idleTimeout が 0 なら見張らない。イベントストリームは、変化がなければ何分でも黙るので、黙っても止まったとはみなせない (#954)。
// 応答の頭が openTimeout までに届かなければ、要求をやめて tunerserver.ErrStreamOpenTimeout を返す (#412)。
// ストリームは何時間も続くので、要求全体には期限を付けず、応答の頭を待つ間だけ見張る。
// 2xx 以外の応答は、cause で状態の意味を読み替えた *APIError にする。
func (c *Client) openStream(req *http.Request, idleTimeout time.Duration, cause causeOf) (io.ReadCloser, error) {
	ctx, cancel := context.WithCancelCause(req.Context())
	timer := time.AfterFunc(c.openTimeout, func() { cancel(tunerserver.ErrStreamOpenTimeout) })
	resp, err := c.httpClient.Do(req.WithContext(ctx))
	// Stop が false なら、上限が来て、要求をやめ始めている。やめる処理が走り終えるのを待たずに、時間切れとして扱う。
	// 応答が届いた直後に上限が来たときも、ストリームは使えないので、届かなかったときと同じく扱う。
	timedOut := !timer.Stop()
	if timedOut && err == nil {
		_ = resp.Body.Close()
		err = tunerserver.ErrStreamOpenTimeout
	}
	if err != nil {
		cancel(nil)
		if timedOut {
			return nil, fmt.Errorf("failed to request %s %s: %w", req.Method, req.URL.Redacted(), tunerserver.ErrStreamOpenTimeout)
		}

		return nil, fmt.Errorf("failed to request %s %s: %w", req.Method, req.URL.Redacted(), err)
	}
	if err := checkResponse(resp, cause); err != nil {
		_ = resp.Body.Close()
		cancel(nil)

		return nil, err
	}

	userID := resp.Header.Get(TunerUserIDHeader)
	if idleTimeout == 0 {
		return &Stream{ReadCloser: &releasingBody{ReadCloser: resp.Body, release: func() { cancel(nil) }}, userID: userID}, nil
	}

	return &Stream{ReadCloser: newIdleReader(resp.Body, idleTimeout, func() { cancel(nil) }), userID: userID}, nil
}

// Stream は、開いたストリームの本文。チューナーサーバーが返した、チューナーの利用者の ID も持つ。
type Stream struct {
	io.ReadCloser

	userID string
}

var (
	_ io.ReadCloser                   = new(Stream)
	_ tunerserver.TunerUserIdentifier = new(Stream)
)

// TunerUserID は、このストリームのチューナーの利用者の ID を返す。チューナーサーバーが返さなければ空 (#1467)。
func (s *Stream) TunerUserID() string {
	return s.userID
}

// releasingBody は、閉じたときに、ストリームの要求の context を手放す本文。
type releasingBody struct {
	io.ReadCloser

	release func()
}

var _ io.ReadCloser = new(releasingBody)

func (b *releasingBody) Close() error {
	defer b.release()

	return b.ReadCloser.Close()
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

	if err := checkResponse(resp, nil); err != nil {
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

// checkResponse は、2xx 以外の応答を *APIError にする。状態の意味は cause で読み替える。cause は nil でもよい。
func checkResponse(resp *http.Response, cause causeOf) error {
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

	apiErr := &APIError{
		Method:     resp.Request.Method,
		URL:        resp.Request.URL.Redacted(),
		StatusCode: resp.StatusCode,
		Reason:     reason,
		cause:      nil,
	}
	if cause != nil {
		apiErr.cause = cause(resp.StatusCode)
	}

	return apiErr
}
