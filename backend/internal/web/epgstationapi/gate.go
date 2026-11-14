package epgstationapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
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

// StopWhenRevoked は、ライブの TS を流している間、管理者が EPGStation 互換の API を無効にしたか、API トークンを取り消したか、
// 発行したユーザーがライブ視聴を見られなくなったら、流しているストリームも止める (#1769)。
// 確かめられないとき (DB に届かないなど) は、流し続ける。
func StopWhenRevoked(gate Gate, tokens TokenAuthenticator, next http.Handler) http.Handler {
	return StopWhenRevokedEvery(gate, tokens, next, httpserver.RevocationInterval)
}

// StopWhenRevokedEvery は、StopWhenRevoked と同じく止める。確かめ直す間隔を interval にする。
func StopWhenRevokedEvery(gate Gate, tokens TokenAuthenticator, next http.Handler, interval time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		httpserver.StopWhenRevoked(w, r, next, interval, func(ctx context.Context) bool {
			enabled, err := gate.EPGStationAPIEnabled(ctx)
			if err == nil && !enabled {
				return false
			}
			principal, err := tokens.Authenticate(ctx, token)
			if errors.Is(err, auth.ErrUnauthenticated) {
				return false
			}

			return err != nil || principal.Role.Can(domain.PermissionWatch)
		})
	})
}
