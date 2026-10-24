package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

// maxPasskeyBody は、認証器の応答を含むリクエストの本文の大きさの上限。attestation の証明書を含んでも収まる大きさにする。
const maxPasskeyBody = 64 << 10

// creationResponse は、パスキーの登録の開始の応答。Options を navigator.credentials.create の publicKey に渡す。
type creationResponse struct {
	Options   *protocol.PublicKeyCredentialCreationOptions `json:"options"`
	ExpiresAt time.Time                                    `json:"expiresAt"`
}

// assertionResponse は、パスキーでのログインの開始の応答。Options を navigator.credentials.get の publicKey に渡す。
type assertionResponse struct {
	Options   *protocol.PublicKeyCredentialRequestOptions `json:"options"`
	ExpiresAt time.Time                                   `json:"expiresAt"`
}

// passkeyRequest は、認証器の応答 (PublicKeyCredential を JSON にしたもの) を送るリクエスト。
type passkeyRequest struct {
	Credential json.RawMessage `json:"credential"`
	// Name は、登録するパスキーの名前。登録のときだけ使う。
	Name string `json:"name"`
}

type passkeyResponse struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	RPID       string     `json:"rpId"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

// RegistrationHandler は、ログイン中のユーザーに、パスキーの登録を始めさせるハンドラーを返す (POST /auth/passkey/registration)。
// 第二要素を変える操作なので、直前の再認証を求める (設計書 9.5)。
func (p *Passkeys) RegistrationHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireJSONPost(w, r) {
			return
		}
		principal, ok := p.recentPrincipal(w, r)
		if !ok {
			return
		}
		creation, err := p.BeginRegistration(r.Context(), db, principal.UserID, client(r))
		if p.writeError(w, r, err) {
			return
		}
		http.SetCookie(w, p.sessions.namedCookie(r, WebAuthnCookieName, challengeCookiePath, creation.Token, creation.ExpiresAt))
		writeJSON(w, http.StatusOK, creationResponse{Options: creation.Options, ExpiresAt: creation.ExpiresAt})
	})
}

// RegistrationConfirmHandler は、認証器の応答を確かめ、パスキーを登録するハンドラーを返す
// (POST /auth/passkey/registration/confirm)。
func (p *Passkeys) RegistrationConfirmHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodePasskeyRequest(w, r)
		if !ok {
			return
		}
		principal, ok := p.recentPrincipal(w, r)
		if !ok {
			return
		}
		token, ok := ceremonyToken(w, r)
		if !ok {
			return
		}
		info := client(r)
		actor := audit.UserFrom(principal.UserID, info.SourceIP, info.UserAgent)
		passkey, err := p.FinishRegistration(r.Context(), db, actor, principal.UserID, token, req.Name, req.Credential, info)
		// 名前が違うだけなら、手続きは使っていないので、Cookie を残して直させる。
		if !errors.Is(err, ErrInvalidPasskeyName) {
			http.SetCookie(w, p.sessions.namedCookie(r, WebAuthnCookieName, challengeCookiePath, "", time.Unix(0, 0)))
		}
		if p.writeError(w, r, err) {
			return
		}
		writeJSON(w, http.StatusCreated, passkeyResponse{
			ID: passkey.ID, Name: passkey.Name, RPID: passkey.RPID, CreatedAt: passkey.CreatedAt, LastUsedAt: passkey.LastUsedAt,
		})
	})
}

// recentPrincipal は、ログイン中で、直前に再認証したユーザーを返す。そうでなければ、エラーを書いて false を返す。
func (p *Passkeys) recentPrincipal(w http.ResponseWriter, r *http.Request) (*Principal, bool) {
	err := RequireRecentAuthentication(r.Context(), p.clock)
	switch {
	case errors.Is(err, ErrUnauthenticated):
		writeLoginError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "log in first")

		return nil, false
	case errors.Is(err, ErrReauthenticationRequired):
		writeLoginError(w, http.StatusForbidden, "REAUTHENTICATION_REQUIRED", "reauthentication required")

		return nil, false
	case err != nil:
		writeInternalError(w, r, p.logger, "failed to register a passkey", err)

		return nil, false
	}

	return PrincipalFrom(r.Context()), true
}

// writeError は、パスキーの登録のエラーを書く。エラーがなければ false を返す。
func (p *Passkeys) writeError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if !writePasskeyError(w, err) {
		writeInternalError(w, r, p.logger, "failed to register a passkey", err)
	}

	return true
}

// PasskeyLoginHandler は、パスキーだけでのログインを始めるハンドラーを返す (POST /auth/passkey/login)。
func (l *Logins) PasskeyLoginHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireJSONPost(w, r) {
			return
		}
		assertion, err := l.BeginPasskeyLogin(r.Context(), db, client(r))
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

// PasskeyLoginConfirmHandler は、認証器の応答を確かめ、パスキーだけでのログインを完了させるハンドラーを返す
// (POST /auth/passkey/login/confirm)。
func (l *Logins) PasskeyLoginConfirmHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodePasskeyRequest(w, r)
		if !ok {
			return
		}
		token, ok := ceremonyToken(w, r)
		if !ok {
			return
		}
		result, err := l.CompleteWithPasskey(r.Context(), db, token, req.Credential, client(r))
		// 手続きは、確かめられなくても使用済みになる。
		http.SetCookie(w, l.sessions.namedCookie(r, WebAuthnCookieName, challengeCookiePath, "", time.Unix(0, 0)))
		if err != nil {
			if !writePasskeyError(w, err) {
				l.internalError(w, r, err)
			}

			return
		}
		l.sessions.SetCookie(w, r, result.Token, result.Session)
		writeJSON(w, http.StatusOK, completedResponse{RecoveryCodes: result.RecoveryCodes})
	})
}

// PasskeySecondFactorHandler は、パスワードを確かめた後の第二要素として、パスキーでの確認を始めるハンドラーを返す
// (POST /auth/login/passkey)。
func (l *Logins) PasskeySecondFactorHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireJSONPost(w, r) {
			return
		}
		token, ok := l.challengeToken(w, r)
		if !ok {
			return
		}
		assertion, err := l.BeginPasskeySecondFactor(r.Context(), db, token, client(r))
		if err != nil && writePasskeyError(w, err) {
			return
		}
		if l.writeSecondFactorError(w, r, err) {
			return
		}
		writeJSON(w, http.StatusOK, assertionResponse{Options: assertion.Options, ExpiresAt: assertion.ExpiresAt})
	})
}

// PasskeySecondFactorConfirmHandler は、パスキーの応答を確かめ、パスワードでのログインを完了させるハンドラーを返す
// (POST /auth/login/passkey/confirm)。
func (l *Logins) PasskeySecondFactorConfirmHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodePasskeyRequest(w, r)
		if !ok {
			return
		}
		token, ok := l.challengeToken(w, r)
		if !ok {
			return
		}
		result, err := l.CompleteWithPasskeySecondFactor(r.Context(), db, token, req.Credential, client(r))
		if err != nil && writePasskeyError(w, err) {
			return
		}
		if l.writeSecondFactorError(w, r, err) {
			return
		}
		l.sessions.SetCookie(w, r, result.Token, result.Session)
		http.SetCookie(w, l.sessions.namedCookie(r, ChallengeCookieName, challengeCookiePath, "", time.Unix(0, 0)))
		writeJSON(w, http.StatusOK, completedResponse{RecoveryCodes: result.RecoveryCodes})
	})
}

// writePasskeyError は、パスキーに固有のエラーを書く。固有のエラーでなければ、何も書かずに false を返す。
func writePasskeyError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, ErrPasskeyUnavailable):
		writeLoginError(w, http.StatusBadRequest, "PASSKEY_UNAVAILABLE", err.Error())
	case errors.Is(err, ErrInvalidPasskey):
		writeLoginError(w, http.StatusUnauthorized, "INVALID_PASSKEY", "the passkey could not be verified")
	case errors.Is(err, ErrWebAuthnCeremonyInvalid):
		writeLoginError(w, http.StatusUnauthorized, "PASSKEY_CEREMONY_EXPIRED", "start over the passkey ceremony")
	case errors.Is(err, ErrTooManyPasskeys):
		writeLoginError(w, http.StatusConflict, "TOO_MANY_PASSKEYS", "too many passkeys")
	case errors.Is(err, ErrInvalidPasskeyName):
		writeLoginError(w, http.StatusBadRequest, "INVALID_PASSKEY_NAME", "the name must be 1 to 64 characters")
	case errors.Is(err, ErrNoPasskey):
		writeLoginError(w, http.StatusBadRequest, "NO_PASSKEY", "no passkey is registered")
	default:
		return false
	}

	return true
}

// ceremonyToken は、パスキーの手続きのトークンを Cookie から取り出す。なければ 401 を返す。
func ceremonyToken(w http.ResponseWriter, r *http.Request) (string, bool) {
	cookie, err := r.Cookie(WebAuthnCookieName)
	if err != nil || cookie.Value == "" {
		writeLoginError(w, http.StatusUnauthorized, "PASSKEY_CEREMONY_EXPIRED", "start over the passkey ceremony")

		return "", false
	}

	return cookie.Value, true
}

// decodePasskeyRequest は、認証器の応答を含むリクエストを読む。読めなければ、エラーを書いて false を返す。
func decodePasskeyRequest(w http.ResponseWriter, r *http.Request) (*passkeyRequest, bool) {
	var req passkeyRequest
	if !decodeJSONWithin(w, r, &req, maxPasskeyBody) {
		return nil, false
	}
	if len(req.Credential) == 0 {
		writeLoginError(w, http.StatusBadRequest, "BAD_REQUEST", "credential is required")

		return nil, false
	}

	return &req, true
}
