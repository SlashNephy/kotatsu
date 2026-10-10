package auth

import (
	"errors"
	"net/http"
	"net/url"
	"time"
)

type oidcStatusResponse struct {
	// Enabled は、OIDC でのログインを受け付けるかどうか。画面は、これを見てボタンを出す。
	Enabled bool `json:"enabled"`
	// DisplayName は、ボタンに出す IdP の名前。
	DisplayName string `json:"displayName"`
}

type oidcBeginResponse struct {
	// AuthorizationURL は、ブラウザーを送り出す IdP の認可のエンドポイント。
	AuthorizationURL string `json:"authorizationUrl"`
}

// StatusHandler は、OIDC でのログインを受け付けるかを返すハンドラーを返す (GET /auth/oidc)。
func (o *OIDC) StatusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !o.Enabled() {
			writeJSON(w, http.StatusOK, oidcStatusResponse{Enabled: false, DisplayName: ""})

			return
		}
		writeJSON(w, http.StatusOK, oidcStatusResponse{Enabled: true, DisplayName: o.DisplayName()})
	})
}

// BeginHandler は、OIDC でのログインを始め、IdP の認可のエンドポイントを返すハンドラーを返す (POST /auth/oidc)。
// 画面は、返した URL にブラウザーを送り出す。CSRF を防ぐため、ほかの /auth/* と同じく、同じオリジンの JSON の POST にする。
func (o *OIDC) BeginHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireJSONPost(w, r) {
			return
		}
		login, err := o.Begin(r.Context(), db, client(r))
		switch {
		case err == nil:
		case errors.Is(err, ErrOIDCDisabled):
			writeLoginError(w, http.StatusNotFound, "OIDC_DISABLED", "OIDC login is not configured")

			return
		case errors.Is(err, ErrOIDCUnavailable):
			o.logger.WarnContext(r.Context(), "the OIDC provider is unavailable", "error", err)
			writeLoginError(w, http.StatusServiceUnavailable, "OIDC_UNAVAILABLE", "the identity provider is unavailable")

			return
		default:
			writeInternalError(w, r, o.logger, "failed to begin the OIDC login", err)

			return
		}
		http.SetCookie(w, o.sessions.namedCookie(r, OIDCCookieName, challengeCookiePath, login.Token, login.ExpiresAt))
		writeJSON(w, http.StatusOK, oidcBeginResponse{AuthorizationURL: login.AuthorizationURL})
	})
}

// CallbackHandler は、IdP から戻ってきたログインを確かめ、セッションを作るハンドラーを返す (GET /auth/oidc/callback)。
//
// IdP からのリダイレクトなので、Origin を確かめられない。代わりに、Cookie のトークンと state で、
// この端末が始めたログインであることを確かめる。結果は、画面へのリダイレクトで伝える。
// 失敗したときは、/login?error=<理由> に戻す。理由には、トークンなどの中身を入れない。
func (o *OIDC) CallbackHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ログインの途中の状態は、確かめられなくても使用済みになる。
		http.SetCookie(w, o.sessions.namedCookie(r, OIDCCookieName, challengeCookiePath, "", time.Unix(0, 0)))
		cookie, err := r.Cookie(OIDCCookieName)
		if err != nil || cookie.Value == "" {
			redirectLoginError(w, r, "oidc_expired")

			return
		}
		query := r.URL.Query()
		result, err := o.Complete(r.Context(), db, cookie.Value, &OIDCCallback{
			State: query.Get("state"), Code: query.Get("code"), Error: query.Get("error"),
		}, client(r))
		if err != nil {
			reason := oidcErrorReason(err)
			if reason == "oidc_failed" {
				o.logger.ErrorContext(r.Context(), "failed to complete the OIDC login", "error", err)
			} else {
				o.logger.InfoContext(r.Context(), "rejected an OIDC login", "error", err)
			}
			redirectLoginError(w, r, reason)

			return
		}
		o.sessions.SetCookie(w, r, result.Token, result.Session)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
}

// oidcErrorReason は、OIDC でのログインのエラーを、画面に伝える理由にする。
func oidcErrorReason(err error) string {
	switch {
	case errors.Is(err, ErrOIDCLoginExpired):
		return "oidc_expired"
	case errors.Is(err, ErrOIDCStateMismatch), errors.Is(err, ErrOIDCTokenInvalid):
		return "oidc_invalid"
	case errors.Is(err, ErrOIDCAccessDenied):
		return "oidc_denied"
	case errors.Is(err, ErrOIDCUsernameTaken):
		return "oidc_username_taken"
	case errors.Is(err, ErrOIDCUnavailable), errors.Is(err, ErrOIDCDisabled):
		return "oidc_unavailable"
	default:
		return "oidc_failed"
	}
}

func redirectLoginError(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/login?"+url.Values{"error": {reason}}.Encode(), http.StatusSeeOther)
}
