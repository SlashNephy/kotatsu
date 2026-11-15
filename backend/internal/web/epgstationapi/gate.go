package epgstationapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/oauth"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

// Gate は、管理者が EPGStation 互換の API を有効にしているかを返す (#1769)。*settings.URLGate が満たす。
type Gate interface {
	EPGStationAPIEnabled(ctx context.Context) (bool, error)
}

var _ Gate = new(settings.URLGate)

// TokenAuthenticator は、API トークンから、発行したユーザーを引く。*auth.APITokens が満たす。
type TokenAuthenticator interface {
	Authenticate(ctx context.Context, token string) (*auth.Principal, error)
}

var _ TokenAuthenticator = new(auth.APITokens)

// DeviceAuthenticator は、デバイス認可で端末に発行したアクセストークンから、承認したユーザーを引く (#1527)。*oauth.Server が満たす。
type DeviceAuthenticator interface {
	AuthenticateDevice(ctx context.Context, token, resource string) (*auth.Principal, *oauth.Grant, error)
	DeviceGrantRole(ctx context.Context, grantID uuid.UUID) (domain.Role, error)
}

var _ DeviceAuthenticator = new(oauth.Server)

// Credentials は、EPGStation 互換の API の Authorization の Bearer のトークンを確かめる (設計書 9.8)。
// API トークンと、デバイス認可で端末に発行したアクセストークン (設計書 9.9) を受け付ける。
type Credentials struct {
	tokens TokenAuthenticator
	// devices は、端末のトークンを確かめる。nil なら、端末のトークンを受け付けない。
	devices DeviceAuthenticator
}

// NewCredentials は Credentials を返す。
func NewCredentials(tokens TokenAuthenticator, devices DeviceAuthenticator) *Credentials {
	return &Credentials{tokens: tokens, devices: devices}
}

// bearer は、Authorization の Bearer のトークンを返す。
func bearer(r *http.Request) (string, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")

	return strings.TrimSpace(token), ok
}

// Middleware は、Bearer のトークンを確かめ、そのユーザーを context に持たせるミドルウェアを返す。
// トークンがなければ、そのまま next を呼ぶ (権限は、後ろの auth.RequirePermission が確かめる)。トークンが違えば 401 を返す。
// 端末のアクセストークンの印 (kotatsu_at_) が付いたものは、端末のトークンとしてだけ確かめ、違っても API トークンとしては確かめない。
// ほかの認証へ回すと、取り消された端末が、取り消されたことに気づけないからである。
func (c *Credentials) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearer(r)
		if !ok {
			next.ServeHTTP(w, r)

			return
		}
		ctx := r.Context()
		var principal *auth.Principal
		var err error
		if oauth.IsAccessToken(token) {
			var grant *oauth.Grant
			principal, grant, err = c.authenticateDevice(r, token)
			ctx = oauth.WithDeviceGrant(ctx, grant)
		} else {
			principal, err = c.tokens.Authenticate(ctx, token)
		}
		if errors.Is(err, auth.ErrUnauthenticated) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="kotatsu"`)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

			return
		}
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(ctx, principal)))
	})
}

func (c *Credentials) authenticateDevice(r *http.Request, token string) (*auth.Principal, *oauth.Grant, error) {
	if c.devices == nil {
		return nil, nil, auth.ErrUnauthenticated
	}

	return c.devices.AuthenticateDevice(r.Context(), token, oauth.EPGStationResourceURL(oauth.Origin(r)))
}

// stillAllowed は、開いたままの配信を続けてよいかを返す。token と grant は、配信を始めたリクエストのもの。
// 端末のトークンなら (grant が nil でなければ)、アクセストークンの期限ではなく、連携を取り消したかと、ユーザーを無効にしたかを見る。
// 配信の途中でアクセストークンの期限が来ても止めないためである。確かめられないとき (DB に届かないなど) は true を返す。
func (c *Credentials) stillAllowed(ctx context.Context, token string, grant *oauth.Grant) bool {
	var role domain.Role
	var err error
	if grant != nil && c.devices != nil {
		role, err = c.devices.DeviceGrantRole(ctx, grant.ID)
	} else {
		var principal *auth.Principal
		principal, err = c.tokens.Authenticate(ctx, token)
		if err == nil {
			role = principal.Role
		}
	}
	if errors.Is(err, auth.ErrUnauthenticated) {
		return false
	}

	return err != nil || role.Can(domain.PermissionWatch)
}

// RequireEnabled は、管理者が EPGStation 互換の API を有効にしている間だけ、next を呼ぶ (#1769、設計書 9.8)。
// 無効の間は、API があることを知らせないよう、トークンを確かめる前に 404 を返す。
// DB に届かず、覚えた設定もなければ 503 を返す。
func RequireEnabled(gate Gate, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enabled, err := gate.EPGStationAPIEnabled(r.Context())
		switch {
		case errors.Is(err, settings.ErrDatabaseUnavailable):
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		case err != nil:
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		case !enabled:
			http.NotFound(w, r)
		default:
			next.ServeHTTP(w, r)
		}
	})
}

// StopWhenRevoked は、ライブの TS を流している間、管理者が EPGStation 互換の API を無効にしたか、API トークンか端末の連携を
// 取り消したか、発行したユーザーがライブ視聴を見られなくなったら、流しているストリームも止める (#1769、#1527)。
// 確かめられないとき (DB に届かないなど) は、流し続ける。
func StopWhenRevoked(gate Gate, credentials *Credentials, next http.Handler) http.Handler {
	return StopWhenRevokedEvery(gate, credentials, next, httpserver.RevocationInterval)
}

// StopWhenRevokedEvery は、StopWhenRevoked と同じく止める。確かめ直す間隔を interval にする。
// credentials の Middleware を通ったリクエストを受け取る。
func StopWhenRevokedEvery(gate Gate, credentials *Credentials, next http.Handler, interval time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, _ := bearer(r)
		grant := oauth.DeviceGrantFrom(r.Context())
		httpserver.StopWhenRevoked(w, r, next, interval, func(ctx context.Context) bool {
			enabled, err := gate.EPGStationAPIEnabled(ctx)
			if err == nil && !enabled {
				return false
			}

			return credentials.stillAllowed(ctx, token, grant)
		})
	})
}
