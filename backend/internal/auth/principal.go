package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// RecentAuthenticationWindow は、重要な操作の前に求める再認証の新しさ (設計書 9.5)。
const RecentAuthenticationWindow = 10 * time.Minute

var (
	// ErrUnauthenticated は、ログインしていないことを表す。
	ErrUnauthenticated = errors.New("unauthenticated")
	// ErrReauthenticationRequired は、重要な操作の前に、もう一度認証する必要があることを表す。
	ErrReauthenticationRequired = errors.New("reauthentication required")
)

// Principal は、リクエストを送ってきたログイン中のユーザー。
type Principal struct {
	UserID uuid.UUID
	Role   Role
	// SessionID は、認証に使ったセッションの ID。
	SessionID uuid.UUID
	// AuthenticatedAt は、このセッションで最後にパスワードなどで認証した日時。
	AuthenticatedAt time.Time
	// OAuthClientID は、MCP のクライアントのトークンで認証したときの、クライアントの ID (#964)。ほかの認証では空。
	OAuthClientID string
}

// RequireRecentAuthentication は、パスワードの変更、第二要素の変更、ユーザーの管理のような重要な操作の前に呼ぶ (設計書 9.5)。
// 直近 RecentAuthenticationWindow 以内に認証していなければ ErrReauthenticationRequired を返す。
// 受け取った側は、もう一度認証してもらってから、操作をやり直してもらう。
func RequireRecentAuthentication(ctx context.Context, clk clock.Clock) error {
	principal := PrincipalFrom(ctx)
	if principal == nil {
		return ErrUnauthenticated
	}
	if clk.Now().Sub(principal.AuthenticatedAt) > RecentAuthenticationWindow {
		return ErrReauthenticationRequired
	}

	return nil
}

type principalKey struct{}

// WithPrincipal は、principal を持たせた context を返す。
func WithPrincipal(ctx context.Context, principal *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

// PrincipalFrom は、context に持たせたログイン中のユーザーを返す。未ログインなら nil を返す。
func PrincipalFrom(ctx context.Context) *Principal {
	principal, _ := ctx.Value(principalKey{}).(*Principal)

	return principal
}

// RequirePermission は、ログイン中のユーザーのロールが p を許されているときだけ next を呼ぶミドルウェアを返す。
// 未ログインなら 401、許されていなければ 403 を返す。
// ログイン中のユーザーは、手前のミドルウェア (Sessions.Middleware) が context に持たせておく。
func RequirePermission(p Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := PrincipalFrom(r.Context())
			if principal == nil {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

				return
			}
			if !principal.Role.Can(p) {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)

				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
