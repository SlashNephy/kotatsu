package streamapi

import (
	"errors"
	"net/http"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

// Authorizer は、映像のリクエストを、セッションか署名付き URL で認可する (設計書 9.6)。
type Authorizer struct {
	signer *stream.Signer
	users  *auth.URLUsers
	clock  clock.Clock
}

// NewAuthorizer は Authorizer を返す。
func NewAuthorizer(signer *stream.Signer, users *auth.URLUsers, clk clock.Clock) *Authorizer {
	return &Authorizer{signer: signer, users: users, clock: clk}
}

// Middleware は、視聴を許されたユーザーのリクエストだけ next に渡すミドルウェアを返す。
// ログイン中のセッション (Sessions.Middleware が context に持たせる) があれば、そのユーザーで認可する。
// なければ、リクエストのパスに対する署名を確かめ、URL を発行したユーザーで認可する。
// 発行した後に無効にしたユーザーの URL は、期限の前でも使えない。
// DB に届かない間は、DB が止まる前に引けたユーザーに限って、覚えた結果で認可する (#1084)。
func (a *Authorizer) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal := auth.PrincipalFrom(r.Context()); principal != nil {
			if !principal.Role.Can(auth.PermissionWatch) {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)

				return
			}
			next.ServeHTTP(w, r)

			return
		}

		userID, err := a.signer.Verify(r.URL.Path, r.URL.Query(), a.clock.Now())
		if err != nil {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

			return
		}
		role, err := a.users.Role(r.Context(), userID)
		if errors.Is(err, auth.ErrUserUnusable) {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

			return
		}
		if errors.Is(err, auth.ErrDatabaseUnavailable) {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)

			return
		}
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

			return
		}
		if !role.Can(auth.PermissionWatch) {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)

			return
		}
		next.ServeHTTP(w, r)
	})
}
