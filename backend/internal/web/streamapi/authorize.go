package streamapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

// Authorizer は、映像と画像のリクエストを、セッションか署名付き URL のどちらか一方で認可する (設計書 9.1、9.6、#1605)。
// 画面が Cookie で使う経路はセッションだけで、外部プレイヤーに渡す /api/streams/* は署名付き URL だけで認可する。
// 1 つの経路で両方を受けると、前段の認証のプロキシから、署名付き URL だけを外せないため。
type Authorizer struct {
	signer *stream.Signer
	users  *auth.URLUsers
	gate   URLGate
	clock  clock.Clock
}

// URLGate は、管理者が外部プレイヤー用の署名付き URL を有効にしているかを返す (#1755)。*settings.URLGate が満たす。
type URLGate interface {
	ExternalPlayerURLsEnabled(ctx context.Context) (bool, error)
}

var _ URLGate = new(settings.URLGate)

// NewAuthorizer は Authorizer を返す。
func NewAuthorizer(signer *stream.Signer, users *auth.URLUsers, gate URLGate, clk clock.Clock) *Authorizer {
	return &Authorizer{signer: signer, users: users, gate: gate, clock: clk}
}

// Session は、ログイン中のセッション (Sessions.Middleware が context に持たせる) のユーザーが視聴を許されていれば、
// リクエストを next に渡すミドルウェアを返す。署名付き URL は受け付けない。
func (a *Authorizer) Session(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal := auth.PrincipalFrom(r.Context())
		if principal == nil {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

			return
		}
		if !principal.Role.Can(domain.PermissionWatch) {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)

			return
		}
		next.ServeHTTP(w, r)
	})
}

// Signed は、リクエストのパスに対する署名を確かめ、URL を発行したユーザーが視聴を許されていれば、
// リクエストを next に渡すミドルウェアを返す。セッションは見ない。
// 発行した後に無効にしたユーザーの URL は、期限の前でも使えない。
// 管理者が外部プレイヤー用の URL を無効にしている間は、期限の前の URL も断る (#1755)。
// DB に届かない間は、DB が止まる前に引けたユーザーと設定に限って、覚えた結果で認可する (#1084)。
func (a *Authorizer) Signed(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := a.signer.Verify(r.URL.Path, r.URL.Query(), a.clock.Now())
		if err != nil {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

			return
		}
		enabled, err := a.gate.ExternalPlayerURLsEnabled(r.Context())
		if errors.Is(err, settings.ErrDatabaseUnavailable) {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)

			return
		}
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

			return
		}
		if !enabled {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)

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
		if !role.Can(domain.PermissionWatch) {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)

			return
		}
		next.ServeHTTP(w, r)
	})
}
