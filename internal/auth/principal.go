package auth

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// Principal は、リクエストを送ってきたログイン中のユーザー。
type Principal struct {
	UserID uuid.UUID
	Role   Role
	// SessionID は、認証に使ったセッションの ID。
	SessionID uuid.UUID
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
