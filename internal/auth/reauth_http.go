package auth

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"
)

type reauthRequest struct {
	Password string `json:"password"`
	// Method は、パスワードと組み合わせる第二要素の種類 (totp か recovery_code)。
	Method SecondFactor `json:"method"`
	Code   string       `json:"code"`
}

type reauthResponse struct {
	// AuthenticatedAt は、更新したセッションの認証の日時。これから 10 分の間、重要な操作ができる。
	AuthenticatedAt time.Time `json:"authenticatedAt"`
}

// ReauthenticateHandler は、ログイン中のセッションのまま、パスワードと第二要素で再認証するハンドラーを返す
// (POST /auth/reauth)。
func (l *Logins) ReauthenticateHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req reauthRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		principal, ok := requirePrincipal(w, r)
		if !ok {
			return
		}
		if req.Password == "" || req.Method == "" || req.Code == "" {
			writeLoginError(w, http.StatusBadRequest, "BAD_REQUEST", "password, method and code are required")

			return
		}
		authenticatedAt, err := l.Reauthenticate(r.Context(), db, principal, req.Password, req.Method, req.Code, client(r))
		if throttled, ok := errors.AsType[*ThrottledError](err); ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(throttled.RetryAfter.Seconds()))))
			writeLoginError(w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", "too many attempts; try again later")

			return
		}
		switch {
		case err == nil:
			writeJSON(w, http.StatusOK, reauthResponse{AuthenticatedAt: authenticatedAt})
		case errors.Is(err, ErrUnsupportedSecondFactor):
			writeLoginError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		case errors.Is(err, ErrInvalidCredentials):
			writeLoginError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid password")
		case errors.Is(err, ErrInvalidSecondFactor):
			writeLoginError(w, http.StatusUnauthorized, "INVALID_SECOND_FACTOR", "invalid code")
		case errors.Is(err, ErrUnauthenticated):
			writeLoginError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "log in first")
		default:
			l.internalError(w, r, err)
		}
	})
}

// PasskeyReauthenticationHandler は、ログイン中のユーザーに、パスキーでの再認証を始めさせるハンドラーを返す
// (POST /auth/reauth/passkey)。
func (l *Logins) PasskeyReauthenticationHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireJSONPost(w, r) {
			return
		}
		principal, ok := requirePrincipal(w, r)
		if !ok {
			return
		}
		assertion, err := l.BeginPasskeyReauthentication(r.Context(), db, principal, client(r))
		if err != nil {
			if !writePasskeyError(w, err) {
				l.internalError(w, r, err)
			}

			return
		}
		http.SetCookie(w, l.sessions.namedCookie(r, WebAuthnCookieName, challengeCookiePath, assertion.Token, assertion.ExpiresAt))
		writeJSON(w, http.StatusOK, assertionResponse{Options: assertion.Options, ExpiresAt: assertion.ExpiresAt})
	})
}

// PasskeyReauthenticationConfirmHandler は、パスキーの応答を確かめ、セッションの認証の日時を更新するハンドラーを返す
// (POST /auth/reauth/passkey/confirm)。
func (l *Logins) PasskeyReauthenticationConfirmHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodePasskeyRequest(w, r)
		if !ok {
			return
		}
		principal, ok := requirePrincipal(w, r)
		if !ok {
			return
		}
		token, ok := ceremonyToken(w, r)
		if !ok {
			return
		}
		authenticatedAt, err := l.CompletePasskeyReauthentication(r.Context(), db, principal, token, req.Credential, client(r))
		// 手続きは、確かめられなくても使用済みになる。
		http.SetCookie(w, l.sessions.namedCookie(r, WebAuthnCookieName, challengeCookiePath, "", time.Unix(0, 0)))
		switch {
		case err == nil:
			writeJSON(w, http.StatusOK, reauthResponse{AuthenticatedAt: authenticatedAt})
		case errors.Is(err, ErrUnauthenticated):
			writeLoginError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "log in first")
		case writePasskeyError(w, err):
		default:
			l.internalError(w, r, err)
		}
	})
}

// requirePrincipal は、ログイン中のユーザーを返す。ログインしていなければ、401 を書いて false を返す。
func requirePrincipal(w http.ResponseWriter, r *http.Request) (*Principal, bool) {
	principal := PrincipalFrom(r.Context())
	if principal == nil {
		writeLoginError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "log in first")

		return nil, false
	}

	return principal, true
}
