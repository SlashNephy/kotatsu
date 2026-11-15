package oauth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/text/width"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clientinfo"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

// デバイス認可 (RFC 8628、#1527、設計書 9.9) の値。決め方は、設計書の付録 A にまとめる。
const (
	// DeviceAuthorizationPath は、デバイス認可のエンドポイントのパス。
	DeviceAuthorizationPath = "/oauth/device_authorization"
	// DeviceVerificationPath は、承認の画面のパス。
	DeviceVerificationPath = "/device"
	// EPGStationResourcePath は、端末のトークンを使える EPGStation 互換の API のパス (設計書 9.8)。
	EPGStationResourcePath = "/api/epgstation"
	// ScopeEPGStation は、EPGStation 互換の API (番組表、録画、ライブと録画の配信) を読むスコープ。
	ScopeEPGStation = "epgstation:read"

	// DeviceCodeLifetime は、コードの有効期間 (expires_in)。
	DeviceCodeLifetime = 10 * time.Minute
	// DevicePollInterval は、端末の問い合わせの間隔 (interval)。
	DevicePollInterval = 5 * time.Second
	// devicePollTolerance は、タイマーのずれのゆとり。前回からの間が、問い合わせの間隔からこれを引いた値より短ければ、slow_down を返す。
	devicePollTolerance = 2 * time.Second
	// MaxDeviceNameLength は、端末の名前の字数の上限。
	MaxDeviceNameLength = 32
	// maxDeviceClientIDLength は、端末が名乗る client_id の長さの上限。
	maxDeviceClientIDLength = 255
	// userCodeLength は、user_code の字数。
	userCodeLength = 8
	// userCodeAlphabet は、user_code に使う 32 字。英大文字と数字から、見間違えやすい 0、O、1、I を除く。
	// 字が 32 個なので、1 バイトの値を 32 で割った余りに偏りが出ない。
	userCodeAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	// userCodeGroupLength は、user_code をハイフンで区切るときの、前の組の字数。
	userCodeGroupLength = userCodeLength / 2
	// userCodeRetries は、user_code が一意の制約に当たったときに引き直す回数。
	userCodeRetries = 3
	// maxDeviceCodes は、有効なコードの数の、全体の上限。
	maxDeviceCodes = 100
	// maxDeviceCodesPerSource は、1 つの接続元 (IPv4 のアドレスか IPv6 の /64) が持てる有効なコードの数の上限。
	maxDeviceCodesPerSource = 5
	// ipv6SourcePrefixBits は、IPv6 の接続元をまとめて数える単位 (/64)。
	ipv6SourcePrefixBits = 64
	// pgUniqueViolation は、一意制約の違反を表す PostgreSQL のエラーコード。
	pgUniqueViolation = "23505"
	// deviceCodeMark は、device_code の頭に付ける印。
	deviceCodeMark = "kotatsu_dc_"
	// deviceCodeCleanupBatch は、コードを発行するときに、ついでに消す、期限を過ぎて時間の経ったコードの数の上限。
	deviceCodeCleanupBatch = 100

	grantTypeDeviceCode = "urn:ietf:params:oauth:grant-type:device_code"
	grantKindMCP        = "mcp"
	grantKindDevice     = "device"

	errorAuthorizationPending   = "authorization_pending"
	errorSlowDown               = "slow_down"
	errorExpiredToken           = "expired_token"
	errorTemporarilyUnavailable = "temporarily_unavailable"

	actionDeviceApprove = "oauth_device.approve"
	targetDevice        = "oauth_device"
)

var (
	// ErrDeviceDisabled は、管理者が EPGStation 互換の API を有効にしていないので、デバイス認可を使えないことを表す。
	ErrDeviceDisabled = errors.New("device authorization is disabled")
	// ErrInvalidUserCode は、user_code の字数か字が正しくないことを表す。
	ErrInvalidUserCode = errors.New("invalid user code")
	// ErrDeviceCodeNotFound は、user_code の有効なコードがないことを表す。
	ErrDeviceCodeNotFound = errors.New("device code not found")
	// ErrDeviceApprovedByOther は、ほかのユーザーが承認したコードを承認しようとしたことを表す。
	ErrDeviceApprovedByOther = errors.New("the device code is approved by another user")
	// ErrTokenCannotApprove は、トークンで認証したリクエストから承認しようとしたことを表す。
	ErrTokenCannotApprove = errors.New("a device cannot be approved with a token")

	// errUserCodeTaken は、引いた user_code が、ほかのコードと重なったことを表す。
	errUserCodeTaken = errors.New("user code is taken")
	// errTooManyDeviceCodes は、上限に達し、承認していないコードがないので空けられないことを表す。
	errTooManyDeviceCodes = errors.New("too many approved device codes")
)

// DeviceAuthorization は、承認の画面に出す、端末のコード。
type DeviceAuthorization struct {
	// DeviceName は、端末が名乗った名前。空なら、画面が既定の名前を出す。
	DeviceName string
	// UserCode は、ハイフンで区切った user_code。
	UserCode string
	// Approved は、このユーザーが承認済みかどうか。
	Approved bool
}

// deviceAuthorizationResponse は、デバイス認可のエンドポイントの応答 (RFC 8628 3.2)。
type deviceAuthorizationResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// deviceRequest は、確かめたデバイス認可のリクエスト。
type deviceRequest struct {
	clientID string
	name     string
	scopes   []string
	resource string
	source   netip.Prefix
}

// DeviceEnabled は、デバイス認可を使えるかどうかを返す。端末のトークンは EPGStation 互換の API にしか使えないので、
// 管理者がそれを有効にしているときだけ使わせる (設計書 9.8)。
func (s *Server) DeviceEnabled(ctx context.Context) (bool, error) {
	system, err := settings.Load(ctx, s.db)
	if err != nil {
		return false, err
	}

	return system.EPGStationAPIEnabled, nil
}

func (s *Server) requireDeviceEnabled(ctx context.Context) error {
	enabled, err := s.DeviceEnabled(ctx)
	if err != nil {
		return err
	}
	if !enabled {
		return ErrDeviceDisabled
	}

	return nil
}

// EPGStationResourceURL は、origin の EPGStation 互換の API の URL を返す。端末のトークンは、この URL 宛てにだけ発行する。
func EPGStationResourceURL(origin string) string {
	return strings.TrimSuffix(origin, "/") + EPGStationResourcePath
}

// DeviceAuthorizationHandler は、デバイス認可のエンドポイント (POST /oauth/device_authorization、RFC 8628 3.1) のハンドラーを返す。
// 資格なしで呼べる。端末の名前 (device_name) と client_id を受け取り、device_code と user_code を返す。
func (s *Server) DeviceAuthorizationHandler(logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		err := s.requireDeviceEnabled(r.Context())
		if errors.Is(err, ErrDeviceDisabled) {
			http.NotFound(w, r)

			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "failed to load the EPGStation API setting", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)

			return
		}
		form, err := readForm(w, r)
		if err != nil {
			writeTokenError(w, &tokenError{code: errorInvalidRequest, description: err.Error()})

			return
		}
		req, err := parseDeviceRequest(r, form)
		if tokenErr := (*tokenError)(nil); errors.As(err, &tokenErr) {
			writeTokenError(w, tokenErr)

			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "failed to read the device authorization request", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

			return
		}
		resp, err := s.issueDeviceCode(r.Context(), req, origin(r))
		if errors.Is(err, errTooManyDeviceCodes) {
			// 全部承認済みで空けられないときだけ断る。承認済みのコードは、端末がすぐに受け取るので、間を置けば空く。
			w.Header().Set("Retry-After", strconv.Itoa(int(DevicePollInterval.Seconds())))
			writeErrorStatus(w, http.StatusTooManyRequests, &tokenError{
				code: errorTemporarilyUnavailable, description: "too many device codes are waiting to be picked up",
			})

			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "failed to issue a device code", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
}

// parseDeviceRequest は、デバイス認可のリクエストを確かめる。
func parseDeviceRequest(r *http.Request, form url.Values) (*deviceRequest, error) {
	clientID := form.Get("client_id")
	if clientID == "" || len(clientID) > maxDeviceClientIDLength || !utf8.ValidString(clientID) || strings.ContainsFunc(clientID, unicode.IsControl) {
		return nil, &tokenError{code: errorInvalidRequest, description: "client_id is required and must be printable"}
	}
	name := strings.TrimSpace(form.Get("device_name"))
	if utf8.RuneCountInString(name) > MaxDeviceNameLength || !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
		return nil, &tokenError{code: errorInvalidRequest, description: fmt.Sprintf("device_name must be at most %d printable characters", MaxDeviceNameLength)}
	}
	scopes := []string{ScopeEPGStation}
	if requested := strings.Fields(form.Get("scope")); len(requested) > 0 && !slices.Contains(requested, ScopeEPGStation) {
		// 端末に許すのは、EPGStation 互換の API のスコープだけ。MCP のスコープは、認可コードで求めてもらう。
		return nil, &tokenError{code: errorInvalidScope, description: "only " + ScopeEPGStation + " is supported for devices"}
	}
	source, err := sourceNetwork(r)
	if err != nil {
		return nil, err
	}

	return &deviceRequest{clientID: clientID, name: name, scopes: scopes, resource: EPGStationResourceURL(origin(r)), source: source}, nil
}

// sourceNetwork は、リクエストの接続元を、コードを数える単位にして返す。IPv4 ならアドレス、IPv6 なら /64 にする。
// 家には /64 以上が配られ、1 台でいくらでも名乗り替えられるからである。
func sourceNetwork(r *http.Request) (netip.Prefix, error) {
	var addr netip.Addr
	if client := clientinfo.From(r.Context()); client != nil {
		addr = client.IP
	} else if addrPort, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		addr = addrPort.Addr()
	}
	addr = addr.Unmap().WithZone("")
	if !addr.IsValid() {
		return netip.Prefix{}, errors.New("the request source is unknown")
	}
	if addr.Is4() {
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}

	return addr.Prefix(ipv6SourcePrefixBits)
}

// issueDeviceCode は、コードを発行する。上限に達していたら、まだ承認していないコードを古い方から失効させて空ける。
func (s *Server) issueDeviceCode(ctx context.Context, req *deviceRequest, issuer string) (*deviceAuthorizationResponse, error) {
	deviceCode, err := newSecret(deviceCodeMark)
	if err != nil {
		return nil, err
	}
	for range userCodeRetries + 1 {
		userCode, err := newUserCode()
		if err != nil {
			return nil, err
		}
		err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
			return s.insertDeviceCode(ctx, sqlc.New(tx), req, deviceCode, userCode)
		})
		if errors.Is(err, errUserCodeTaken) {
			continue
		}
		if err != nil {
			return nil, err
		}
		formatted := FormatUserCode(userCode)
		verification := strings.TrimSuffix(issuer, "/") + DeviceVerificationPath

		return &deviceAuthorizationResponse{
			DeviceCode: deviceCode, UserCode: formatted, VerificationURI: verification,
			VerificationURIComplete: verification + "?" + url.Values{"user_code": {formatted}}.Encode(),
			ExpiresIn:               int(DeviceCodeLifetime.Seconds()), Interval: int(DevicePollInterval.Seconds()),
		}, nil
	}

	return nil, errors.New("failed to draw a unique user code")
}

// insertDeviceCode は、上限を確かめて、コードを足す。数えてから足すまでを、ほかの発行と重ねない。
func (s *Server) insertDeviceCode(ctx context.Context, q *sqlc.Queries, req *deviceRequest, deviceCode, userCode string) error {
	now := s.clock.Now()
	if err := q.LockOAuthDeviceCodes(ctx); err != nil {
		return fmt.Errorf("failed to lock the device codes: %w", err)
	}
	// 失効させたコードを呼び続けられても行が増え続けないよう、期限を過ぎて時間の経ったものを、ついでに消す。
	if _, err := q.DeleteExpiredOAuthDeviceCodes(ctx, sqlc.DeleteExpiredOAuthDeviceCodesParams{
		ExpiredBefore: now.Add(-DeviceCodeLifetime), MaxCount: deviceCodeCleanupBatch,
	}); err != nil {
		return fmt.Errorf("failed to delete the expired device codes: %w", err)
	}
	counts, err := q.CountValidOAuthDeviceCodes(ctx, sqlc.CountValidOAuthDeviceCodesParams{SourceNetwork: req.source, Now: now})
	if err != nil {
		return fmt.Errorf("failed to count the device codes: %w", err)
	}
	total := counts.Total
	// 埋まっても断らず、まだ承認していないコードを古い方から失効させる。断ると、呼び続ける 1 台のせいで、誰もつなげなくなる。
	if counts.FromSource >= maxDeviceCodesPerSource {
		if err := expireOldestPending(ctx, q, now, &req.source); err != nil {
			return err
		}
		total--
	}
	if total >= maxDeviceCodes {
		if err := expireOldestPending(ctx, q, now, nil); err != nil {
			return err
		}
	}
	err = q.CreateOAuthDeviceCode(ctx, sqlc.CreateOAuthDeviceCodeParams{
		DeviceCodeHash: hashSecret(deviceCode), UserCode: userCode, ClientID: req.clientID, DeviceName: req.name,
		Scopes: req.scopes, Resource: req.resource, SourceNetwork: req.source, Now: now, ExpiresAt: now.Add(DeviceCodeLifetime),
	})
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return errUserCodeTaken
	}
	if err != nil {
		return fmt.Errorf("failed to save the device code: %w", err)
	}

	return nil
}

// expireOldestPending は、まだ承認していないいちばん古いコードを失効させる。source が nil でなければ、その接続元のものに限る。
// 失効させられるものがなければ errTooManyDeviceCodes を返す。
func expireOldestPending(ctx context.Context, q *sqlc.Queries, now time.Time, source *netip.Prefix) error {
	expired, err := q.ExpireOldestPendingOAuthDeviceCode(ctx, sqlc.ExpireOldestPendingOAuthDeviceCodeParams{Now: now, SourceNetwork: source})
	if err != nil {
		return fmt.Errorf("failed to expire a device code: %w", err)
	}
	if expired == 0 {
		return errTooManyDeviceCodes
	}

	return nil
}

// newUserCode は、userCodeAlphabet の字を userCodeLength 個並べた user_code を返す。
func newUserCode() (string, error) {
	raw := make([]byte, userCodeLength)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate a user code: %w", err)
	}
	code := make([]byte, userCodeLength)
	for i, b := range raw {
		code[i] = userCodeAlphabet[int(b)%len(userCodeAlphabet)]
	}

	return string(code), nil
}

// FormatUserCode は、正規化した user_code を、読みやすいよう、真ん中でハイフンで区切る。
func FormatUserCode(code string) string {
	if len(code) != userCodeLength {
		return code
	}

	return code[:userCodeGroupLength] + "-" + code[userCodeGroupLength:]
}

// NormalizeUserCode は、人が打った user_code を正規化する。小文字、全角の字、ハイフンの抜けや余り、空白を許す。
// 字数が違うか、使わない字を含めば ErrInvalidUserCode を返す。
func NormalizeUserCode(input string) (string, error) {
	folded := strings.ToUpper(width.Fold.String(input))
	var b strings.Builder
	for _, r := range folded {
		switch {
		case r == '-' || unicode.IsSpace(r):
			continue
		case r > unicode.MaxASCII || !strings.ContainsRune(userCodeAlphabet, r):
			return "", ErrInvalidUserCode
		}
		b.WriteRune(r)
	}
	if b.Len() != userCodeLength {
		return "", ErrInvalidUserCode
	}

	return b.String(), nil
}

// LookupDevice は、承認の画面に出すため、user_code のコードを引く。何も変えない。
func (s *Server) LookupDevice(ctx context.Context, principal *auth.Principal, userCode string) (*DeviceAuthorization, error) {
	if err := s.requireDeviceEnabled(ctx); err != nil {
		return nil, err
	}
	normalized, err := NormalizeUserCode(userCode)
	if err != nil {
		return nil, err
	}
	row, err := sqlc.New(s.db).GetOAuthDeviceCodeByUserCode(ctx, sqlc.GetOAuthDeviceCodeByUserCodeParams{UserCode: normalized, Now: s.clock.Now()})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDeviceCodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to look up the device code: %w", err)
	}

	return &DeviceAuthorization{
		DeviceName: row.DeviceName, UserCode: FormatUserCode(row.UserCode),
		Approved: row.ApprovedBy != nil && *row.ApprovedBy == principal.UserID,
	}, nil
}

// ApproveDevice は、ログイン中のユーザーが、user_code のコードを承認する。何度呼んでも同じで、トークンは端末が受け取るときに作る。
// ほかのユーザーが承認したコードなら ErrDeviceApprovedByOther を返す。承認したことは監査ログに残す。
// トークンで認証したリクエストからは承認させない。盗まれたトークンで、ほかの端末をつながせないためである。
func (s *Server) ApproveDevice(ctx context.Context, actor *audit.Actor, principal *auth.Principal, userCode string) (*DeviceAuthorization, error) {
	if principal.OAuthClientID != "" || principal.SessionID == uuid.Nil() {
		return nil, ErrTokenCannotApprove
	}
	if err := s.requireDeviceEnabled(ctx); err != nil {
		return nil, err
	}
	normalized, err := NormalizeUserCode(userCode)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	var approved *DeviceAuthorization
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		row, err := q.LockOAuthDeviceCodeByUserCode(ctx, sqlc.LockOAuthDeviceCodeByUserCodeParams{UserCode: normalized, Now: now})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDeviceCodeNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to look up the device code: %w", err)
		}
		approved = &DeviceAuthorization{DeviceName: row.DeviceName, UserCode: FormatUserCode(normalized), Approved: true}
		if row.ApprovedBy != nil {
			if *row.ApprovedBy != principal.UserID {
				return ErrDeviceApprovedByOther
			}

			// 同じユーザーがもう一度押しても、何も変えない。
			return nil
		}
		if err := q.ApproveOAuthDeviceCode(ctx, sqlc.ApproveOAuthDeviceCodeParams{
			UserID: &principal.UserID, Now: &now, DeviceCodeHash: row.DeviceCodeHash,
		}); err != nil {
			return fmt.Errorf("failed to approve the device code: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: actionDeviceApprove, TargetType: targetDevice, TargetID: row.ClientID,
			Summary:  map[string]any{"device_name": row.DeviceName, "user_code": approved.UserCode},
			SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
		})
	})
	if err != nil {
		return nil, err
	}

	return approved, nil
}

// devicePoll は、端末の問い合わせの結果。トランザクションを確定してから、エラーにして返す。
type devicePoll struct {
	resp *tokenResponse
	err  *tokenError
}

// exchangeDeviceCode は、端末の問い合わせ (RFC 8628 3.4) に答える。承認済みなら、その時点でトークンを作って 1 回だけ返し、
// コードを使用済みにする。トークンを受け取りの時点で作るので、平文のトークンを預かる場所が要らない。
// 断る手順がないので、access_denied は返さない。
func (s *Server) exchangeDeviceCode(ctx context.Context, form url.Values) (*tokenResponse, error) {
	deviceCode := form.Get("device_code")
	if deviceCode == "" {
		return nil, &tokenError{code: errorInvalidRequest, description: "device_code is required"}
	}
	var poll devicePoll
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		poll, err = s.pollDeviceCode(ctx, sqlc.New(tx), hashSecret(deviceCode), form.Get("client_id"))

		return err
	})
	if err != nil {
		return nil, err
	}
	if poll.err != nil {
		return nil, poll.err
	}

	return poll.resp, nil
}

func (s *Server) pollDeviceCode(ctx context.Context, q *sqlc.Queries, codeHash []byte, clientID string) (devicePoll, error) {
	now := s.clock.Now()
	row, err := q.LockOAuthDeviceCode(ctx, codeHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return devicePoll{resp: nil, err: &tokenError{code: errorInvalidGrant, description: "the device code is invalid"}}, nil
	}
	if err != nil {
		return devicePoll{}, fmt.Errorf("failed to look up the device code: %w", err)
	}
	switch {
	case row.UsedAt != nil:
		return devicePoll{resp: nil, err: &tokenError{code: errorInvalidGrant, description: "the device code is already used"}}, nil
	case clientID != row.ClientID:
		return devicePoll{resp: nil, err: &tokenError{code: errorInvalidGrant, description: "client_id does not match the device code"}}, nil
	case !now.Before(row.ExpiresAt):
		return devicePoll{resp: nil, err: &tokenError{code: errorExpiredToken, description: "the device code is expired"}}, nil
	}
	last := row.LastPolledAt
	if err := q.TouchOAuthDeviceCodePoll(ctx, sqlc.TouchOAuthDeviceCodePollParams{Now: &now, DeviceCodeHash: codeHash}); err != nil {
		return devicePoll{}, fmt.Errorf("failed to record the device poll: %w", err)
	}
	// 間隔を守っている端末にも返さないよう、タイマーのずれのゆとりを引いた値と比べる。
	if last != nil && now.Sub(*last) < DevicePollInterval-devicePollTolerance {
		return devicePoll{resp: nil, err: &tokenError{code: errorSlowDown, description: "polling too frequently"}}, nil
	}
	if row.ApprovedBy == nil {
		return devicePoll{resp: nil, err: &tokenError{code: errorAuthorizationPending, description: "the user has not approved the device yet"}}, nil
	}
	approver, err := q.GetOAuthDeviceApprover(ctx, *row.ApprovedBy)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return devicePoll{}, fmt.Errorf("failed to look up the approver: %w", err)
	}
	if errors.Is(err, pgx.ErrNoRows) || approver.DisabledAt != nil || !approver.Role.Can(domain.PermissionWatch) {
		return devicePoll{resp: nil, err: &tokenError{code: errorInvalidGrant, description: "the approving user is disabled"}}, nil
	}
	grantID := id.New()
	if err := q.CreateOAuthGrant(ctx, sqlc.CreateOAuthGrantParams{
		ID: grantID, UserID: *row.ApprovedBy, Kind: grantKindDevice, ClientID: row.ClientID, ClientName: row.DeviceName,
		Scopes: row.Scopes, Resource: row.Resource, Now: now,
	}); err != nil {
		return devicePoll{}, fmt.Errorf("failed to save the OAuth grant: %w", err)
	}
	resp, err := s.issue(ctx, q, grantID, grantScopes(grantKindDevice, row.Scopes, approver.Role))
	if err != nil {
		return devicePoll{}, err
	}
	if err := q.MarkOAuthDeviceCodeUsed(ctx, sqlc.MarkOAuthDeviceCodeUsedParams{Now: &now, DeviceCodeHash: codeHash}); err != nil {
		return devicePoll{}, fmt.Errorf("failed to mark the device code used: %w", err)
	}

	return devicePoll{resp: resp, err: nil}, nil
}

// AuthenticateDevice は、デバイス認可で発行したアクセストークンから、承認したユーザーと連携を返す。
// resource は、リクエストを受けた EPGStation 互換の API の URL。トークンが違うか、期限を過ぎたか、取り消したか、
// ユーザーを無効にしたか、ほかのオリジン宛てか、MCP のクライアントのトークンなら auth.ErrUnauthenticated を返す。
func (s *Server) AuthenticateDevice(ctx context.Context, token, resource string) (*auth.Principal, *Grant, error) {
	if !strings.HasPrefix(token, accessMark) {
		return nil, nil, auth.ErrUnauthenticated
	}
	err := s.requireDeviceEnabled(ctx)
	if errors.Is(err, ErrDeviceDisabled) {
		return nil, nil, auth.ErrUnauthenticated
	}
	if err != nil {
		return nil, nil, err
	}
	q := sqlc.New(s.db)
	now := s.clock.Now()
	row, err := q.GetOAuthAccessToken(ctx, sqlc.GetOAuthAccessTokenParams{TokenHash: hashSecret(token), Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, auth.ErrUnauthenticated
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to look up the access token: %w", err)
	}
	scopes := grantScopes(row.Kind, row.Scopes, row.Role)
	if row.Kind != grantKindDevice || !sameResource(resource, row.Resource) || !slices.Contains(scopes, ScopeEPGStation) {
		return nil, nil, auth.ErrUnauthenticated
	}
	if row.LastUsedAt == nil || now.Sub(*row.LastUsedAt) >= grantTouchInterval {
		if err := q.TouchOAuthGrant(ctx, sqlc.TouchOAuthGrantParams{ID: row.GrantID, Now: &now}); err != nil {
			return nil, nil, fmt.Errorf("failed to record the OAuth grant use: %w", err)
		}
	}
	principal := &auth.Principal{UserID: row.UserID, Role: row.Role, SessionID: uuid.Nil(), AuthenticatedAt: time.Time{}, OAuthClientID: row.ClientID}

	return principal, &Grant{ID: row.GrantID, ClientID: row.ClientID, ClientName: row.ClientName, Scopes: scopes}, nil
}

// DeviceGrantRole は、端末の連携が使えるなら、ユーザーの今のロールを返す。開いたままの配信を確かめ直すのに使う。
// アクセストークンの期限ではなく、連携を取り消したかと、ユーザーを無効にしたかを見る。配信の途中でアクセストークンの期限が来ても止めない。
// 使えなければ auth.ErrUnauthenticated を返す。
func (s *Server) DeviceGrantRole(ctx context.Context, grantID uuid.UUID) (domain.Role, error) {
	role, err := sqlc.New(s.db).GetActiveOAuthDeviceGrantRole(ctx, grantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrUnauthenticated
	}
	if err != nil {
		return "", fmt.Errorf("failed to look up the OAuth grant: %w", err)
	}

	return role, nil
}

// grantScopes は、連携のスコープを、ユーザーの今のロールに許すものに絞る。
// 端末の連携は、ライブ視聴と録画の再生を許すロールなら、そのまま使える。
func grantScopes(kind string, scopes []string, role domain.Role) []string {
	if kind != grantKindDevice {
		return narrowScopes(scopes, role)
	}
	if !role.Can(domain.PermissionWatch) || !slices.Contains(scopes, ScopeEPGStation) {
		return []string{}
	}

	return []string{ScopeEPGStation}
}

type deviceGrantKey struct{}

// WithDeviceGrant は、端末のトークンで認証した連携を持たせた context を返す。
func WithDeviceGrant(ctx context.Context, grant *Grant) context.Context {
	return context.WithValue(ctx, deviceGrantKey{}, grant)
}

// DeviceGrantFrom は、context に持たせた端末の連携を返す。端末のトークンで認証していなければ nil を返す。
func DeviceGrantFrom(ctx context.Context) *Grant {
	grant, _ := ctx.Value(deviceGrantKey{}).(*Grant)

	return grant
}

// IsAccessToken は、token が、認可サーバーが発行したアクセストークンの印を持つかどうかを返す。
func IsAccessToken(token string) bool {
	return strings.HasPrefix(token, accessMark)
}

// Origin は、リクエストが接続したオリジンを返す。リバースプロキシを経由したときは、信頼済みのプロキシが伝えた値にする。
func Origin(r *http.Request) string {
	return origin(r)
}
