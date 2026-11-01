// Package databroadcastapi は、データ放送の文書の双方向の通信 (投票やプレゼントの応募など) を、tenban が中継する
// (#501、設計書 7.4)。
//
// CSP (設計書 10.4) で、ブラウザからは外部へ接続させないので、web-bml の通信の機能 (IP) を、ここへのリクエストにする。
// 管理者が全体の設定で有効にしたときだけ答え、無効なら 404 を返す。
//
// tenban が、文書の指す任意の宛先へ接続するので、LAN の中のサーバーへ接続させないよう、接続するときのアドレスで
// グローバルでないもの (プライベート、ループバック、リンクローカルなど) を断る。名前を引いた後のアドレスで確かめるので、
// DNS の応答を後から変えられたときや、リダイレクトの先にも効く。
package databroadcastapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/publicnet"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// RequestPath は、文書の HTTP のリクエスト (GET と、transmitTextDataOverIP の POST) を中継するエンドポイント。
	RequestPath = "/media/data-broadcast/requests"
	// ConnectivityPath は、文書がインターネットへ接続できるかを確かめる (confirmIPNetwork) エンドポイント。
	ConnectivityPath = "/media/data-broadcast/connectivity"

	// requestTimeout は、中継するリクエストの 1 回を待つ長さ (リダイレクトを含む)。
	requestTimeout = 10 * time.Second
	// maxRedirects は、中継するリクエストで追うリダイレクトの回数の上限。
	maxRedirects = 3
	// maxBodyBytes は、文書が送る本文の大きさの上限。web-bml が送るのは、文書の文字列を符号化した Denbun の 1 つだけ。
	maxBodyBytes = 64 << 10
	// maxRequestBytes は、画面が送る JSON の大きさの上限。本文を Base64 にした分と、URL の分を見込む。
	maxRequestBytes = 128 << 10
	// maxResponseBytes は、中継する応答の本文の大きさの上限。文書と、文書が読む画像や音声に足りる大きさにする。
	maxResponseBytes = 1 << 20

	// defaultConfirmTimeout は、接続を確かめるときに、文書が待つ長さを指定しなかったときの長さ。web-bml の既定と同じ。
	defaultConfirmTimeout = 4 * time.Second
	// maxConfirmTimeout は、接続を確かめるときに待つ長さの上限。
	maxConfirmTimeout = 10 * time.Second
	// defaultConfirmPort は、接続を確かめるときに接続するポート。ICMP は特権が要るので、どちらを求められても TCP で 80 番に接続する。
	defaultConfirmPort = "80"
	// maxHostnameLength は、接続を確かめる宛先の名前の長さの上限 (RFC 1035)。
	maxHostnameLength = 253
)

var (
	errUnsupportedScheme = errors.New("the URL must be http or https")
	errTooManyRedirects  = errors.New("too many redirects")
	errTooLarge          = errors.New("the response is too large")
	errMissingHost       = errors.New("the URL has no host")
	errInvalidMethod     = errors.New("the method must be GET or POST")
	errBodyWithGet       = errors.New("a GET request cannot have a body")
	errBodyTooLarge      = errors.New("the body is too large")
	errInvalidHost       = errors.New("the destination must be a host name or an IP address")
)

// Control は、接続する直前に、名前を引いた後のアドレスを確かめる。net.Dialer の Control と同じ形。
type Control func(network, address string, conn syscall.RawConn) error

// Request は、画面が送る、中継するリクエスト。
type Request struct {
	// Method は GET か POST。
	Method string `json:"method"`
	// URL は、文書が指す http か https の URL。
	URL string `json:"url"`
	// Body は、POST の本文 (JSON では Base64)。web-bml が作った application/x-www-form-urlencoded の Denbun をそのまま送る。
	Body []byte `json:"body"`
}

// Response は、中継した応答。上流の状態コードを、中継の成否と分けて返す。
type Response struct {
	Status       int    `json:"status"`
	ContentType  string `json:"contentType"`
	CacheControl string `json:"cacheControl"`
	// Body は、上流の応答の本文 (JSON では Base64)。
	Body []byte `json:"body"`
}

// ConnectivityRequest は、画面が送る、接続を確かめる宛先。
type ConnectivityRequest struct {
	// Destination は、宛先のホスト名か IP アドレス。
	Destination string `json:"destination"`
	// TimeoutMillis は、待つ長さ (ミリ秒)。0 以下なら既定の長さにし、maxConfirmTimeout を超えたら、そこで打ち切る。
	TimeoutMillis int `json:"timeoutMillis"`
}

// ConnectivityResponse は、接続を確かめた結果。web-bml の confirmIPNetwork の結果と同じ項目。
type ConnectivityResponse struct {
	Success            bool    `json:"success"`
	IPAddress          *string `json:"ipAddress"`
	ResponseTimeMillis *int64  `json:"responseTimeMillis"`
}

// Relay は、データ放送の文書の通信を中継する。
type Relay struct {
	enabled func(ctx context.Context) (bool, error)
	control Control
	// confirmPort は、接続を確かめるときに接続するポート。テストのほかは defaultConfirmPort。
	confirmPort string
	client      *http.Client
	clock       clock.Clock
	logger      *slog.Logger
}

// New は、全体の設定で有効にしたときだけ答え、グローバルなアドレスにだけ接続する Relay を返す。
func New(db *pgxpool.Pool, clk clock.Clock, logger *slog.Logger) *Relay {
	enabled := func(ctx context.Context) (bool, error) {
		system, err := settings.Load(ctx, db)
		if err != nil {
			return false, err
		}

		return system.DataBroadcastIPEnabled, nil
	}

	return newRelay(enabled, publicnet.Control, clk, logger)
}

func newRelay(enabled func(ctx context.Context) (bool, error), control Control, clk clock.Clock, logger *slog.Logger) *Relay {
	dialer := &net.Dialer{Timeout: requestTimeout, Control: control}
	transport := &http.Transport{
		// 環境変数のプロキシを使うと、プロキシから先のアドレスを確かめられないので、使わない。
		Proxy: nil, DialContext: dialer.DialContext, ForceAttemptHTTP2: true, TLSHandshakeTimeout: requestTimeout,
		ResponseHeaderTimeout: requestTimeout,
	}

	return &Relay{
		enabled: enabled, control: control, confirmPort: defaultConfirmPort, clock: clk, logger: logger,
		client: &http.Client{Transport: transport, Timeout: requestTimeout, CheckRedirect: checkRedirect, Jar: nil},
	}
}

// checkRedirect は、http か https のまま maxRedirects 回までのリダイレクトだけを追う。
// リダイレクトの先のアドレスも、接続するときに Control で確かめる。
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects {
		return errTooManyRedirects
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return errUnsupportedScheme
	}

	return nil
}

// RequestHandler は、文書の HTTP のリクエストを中継する (POST RequestPath)。
// 文書の GET も、画面からは POST で送らせる。どれも、ほかのオリジンからのリクエストを断る (RequireSameOrigin) ためである。
func (h *Relay) RequestHandler() http.Handler {
	return http.HandlerFunc(h.serveRequest)
}

// ConnectivityHandler は、文書がインターネットへ接続できるかを確かめる (POST ConnectivityPath)。
func (h *Relay) ConnectivityHandler() http.Handler {
	return http.HandlerFunc(h.serveConnectivity)
}

func (h *Relay) serveRequest(w http.ResponseWriter, r *http.Request) {
	principal := h.authorize(w, r)
	if principal == nil {
		return
	}
	var req Request
	if !decode(w, r, &req) {
		return
	}
	target, err := parseRequest(&req)
	if errors.Is(err, errBodyTooLarge) {
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)

		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	// 監査ログには残さず、宛先のホストだけをログに残す。文書が送るたびに残すと監査ログが埋まり、パスとクエリには、
	// 文書が入れた受信機の情報 (郵便番号など) が入ることがあるからである。
	logger := h.logger.With(
		slog.String("user_id", principal.UserID.String()), slog.String("method", req.Method), slog.String("host", target.Host),
	)
	resp, err := h.forward(r.Context(), req.Method, target, req.Body)
	if err != nil {
		fail(w, r, logger, err)

		return
	}
	logger.InfoContext(r.Context(), "relayed a data broadcast request", slog.Int("status", resp.Status))
	writeJSON(w, r, logger, resp)
}

func (h *Relay) serveConnectivity(w http.ResponseWriter, r *http.Request) {
	principal := h.authorize(w, r)
	if principal == nil {
		return
	}
	var req ConnectivityRequest
	if !decode(w, r, &req) {
		return
	}
	if !validHost(req.Destination) {
		http.Error(w, errInvalidHost.Error(), http.StatusBadRequest)

		return
	}
	timeout := time.Duration(req.TimeoutMillis) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultConfirmTimeout
	}
	timeout = min(timeout, maxConfirmTimeout)

	logger := h.logger.With(slog.String("user_id", principal.UserID.String()), slog.String("host", req.Destination))
	result := h.confirm(r.Context(), req.Destination, timeout)
	logger.InfoContext(r.Context(), "confirmed the connectivity for a data broadcast", slog.Bool("success", result.Success))
	writeJSON(w, r, logger, result)
}

// authorize は、ログイン中のユーザーで、管理者が中継を有効にしているかを確かめ、そのユーザーを返す。
// 確かめられなければ、応答を書いて nil を返す。
func (h *Relay) authorize(w http.ResponseWriter, r *http.Request) *auth.Principal {
	w.Header().Set("Cache-Control", "no-store")
	principal := auth.PrincipalFrom(r.Context())
	if principal == nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

		return nil
	}
	enabled, err := h.enabled(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "failed to load the system settings", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return nil
	}
	// 無効なら、MCP (設計書 9.9) と同じく、ないものとして答える。
	if !enabled {
		http.Error(w, "data broadcast communication is disabled", http.StatusNotFound)

		return nil
	}

	return principal
}

// fail は、中継に失敗したことを答える。グローバルでない宛先を断ったときは 403、上流に届かなかったときは 502 にする。
func fail(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	if errors.Is(err, publicnet.ErrNonPublicAddress) {
		logger.WarnContext(r.Context(), "refused a data broadcast request to a non-public address", slog.Any("error", err))
		http.Error(w, publicnet.ErrNonPublicAddress.Error(), http.StatusForbidden)

		return
	}
	if r.Context().Err() == nil {
		logger.WarnContext(r.Context(), "failed to relay a data broadcast request", slog.Any("error", err))
	}
	http.Error(w, "failed to reach the destination", http.StatusBadGateway)
}

// forward は、target へリクエストを送り、応答を返す。
func (h *Relay) forward(ctx context.Context, method string, target *url.URL, body []byte) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var reader io.Reader = http.NoBody
	if method == http.MethodPost {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("failed to build the request: %w", err)
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "ja")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := h.client.Do(req)
	if err != nil {
		// *url.Error は、パスとクエリを含む URL を文に入れる。ログに受信機の情報を残さないよう、URL を外す。
		if urlErr, ok := errors.AsType[*url.Error](err); ok {
			err = urlErr.Err
		}

		return nil, fmt.Errorf("failed to send the request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read the response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return nil, errTooLarge
	}

	return &Response{
		Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), CacheControl: resp.Header.Get("Cache-Control"),
		Body: data,
	}, nil
}

// confirm は、destination の 80 番に TCP で接続できるかを確かめる。グローバルでないアドレスには接続せず、失敗とする。
func (h *Relay) confirm(ctx context.Context, destination string, timeout time.Duration) *ConnectivityResponse {
	failed := &ConnectivityResponse{Success: false, IPAddress: nil, ResponseTimeMillis: nil}
	dialer := &net.Dialer{Timeout: timeout, Control: h.control}
	stopwatch := h.clock.StartStopwatch()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(destination, h.confirmPort))
	if err != nil {
		return failed
	}
	elapsed := stopwatch.Elapsed()
	remote := conn.RemoteAddr().String()
	_ = conn.Close()
	addrPort, err := netip.ParseAddrPort(remote)
	if err != nil {
		return failed
	}

	return &ConnectivityResponse{
		Success: true, IPAddress: new(addrPort.Addr().Unmap().String()), ResponseTimeMillis: new(elapsed.Milliseconds()),
	}
}

// parseRequest は、中継するリクエストを確かめ、送り先の URL を返す。
func parseRequest(req *Request) (*url.URL, error) {
	switch req.Method {
	case http.MethodGet:
		if len(req.Body) > 0 {
			return nil, errBodyWithGet
		}
	case http.MethodPost:
		if len(req.Body) > maxBodyBytes {
			return nil, errBodyTooLarge
		}
	default:
		return nil, errInvalidMethod
	}
	target, err := url.Parse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the URL: %w", err)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, errUnsupportedScheme
	}
	if target.Hostname() == "" {
		return nil, errMissingHost
	}
	// URL の利用者情報は、上流に Basic 認証として送られるので、落とす。
	target.User = nil

	return target, nil
}

// validHost は、destination が IP アドレスか、ホスト名に使える文字だけの名前かを返す。ポートやパスは受け付けない。
func validHost(destination string) bool {
	if _, err := netip.ParseAddr(destination); err == nil {
		return true
	}
	if destination == "" || len(destination) > maxHostnameLength {
		return false
	}

	return strings.IndexFunc(destination, func(c rune) bool {
		return (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '.'
	}) < 0
}

// decode は、本文の JSON を v に読む。読めなければ、応答を書いて false を返す。
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	err := json.NewDecoder(r.Body).Decode(v)
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		http.Error(w, "the request is too large", http.StatusRequestEntityTooLarge)

		return false
	case err != nil:
		http.Error(w, "the request must be a JSON object", http.StatusBadRequest)

		return false
	}

	return true
}

// writeJSON は、v を JSON で返す。書けなかったら (接続が切れたなど)、ログに残すだけにする。
func writeJSON[T Response | ConnectivityResponse](w http.ResponseWriter, r *http.Request, logger *slog.Logger, v *T) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.DebugContext(r.Context(), "failed to write the response", slog.Any("error", err))
	}
}
