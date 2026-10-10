package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"strconv"
	"time"
)

// maxLoginBody は、ログインのリクエストの本文の大きさの上限。
const maxLoginBody = 4 << 10

// 次に求めるもの。
const (
	// nextSecondFactor は、登録済みの第二要素で確かめること。
	nextSecondFactor = "second_factor"
	// nextEnrollSecondFactor は、第二要素がないので、TOTP を登録すること (設計書 9.3)。
	nextEnrollSecondFactor = "enroll_second_factor"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginResponse は、パスワードを確かめた後の応答。next は、次に求めるもの。
type loginResponse struct {
	Next string `json:"next"`
	// Methods は、使える第二要素の種類 (totp、recovery_code)。登録したものがなければ空。
	Methods   []SecondFactor `json:"methods"`
	ExpiresAt time.Time      `json:"expiresAt"`
}

type codeRequest struct {
	Code string `json:"code"`
}

type enrollmentResponse struct {
	Secret    string    `json:"secret"`
	URI       string    `json:"uri"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// completedResponse は、ログインを完了させた応答。RecoveryCodes は、ログインの途中で TOTP を登録したときだけ入る。
type completedResponse struct {
	RecoveryCodes []string `json:"recoveryCodes"`
}

type loginError struct {
	Error loginErrorBody `json:"error"`
}

type loginErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PasswordHandler は、パスワードでのログインを受け付けるハンドラーを返す (POST /auth/login)。
//
// パスワードを確かめると、第二要素の確認を待つトークンを Cookie (kotatsu_login) に入れて返す。
// セッションは、第二要素を確かめるまで作らない。client は、リクエストから送信元を求める。
func (l *Logins) PasswordHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Username == "" || req.Password == "" {
			writeLoginError(w, http.StatusBadRequest, "BAD_REQUEST", "username and password are required")

			return
		}

		token, challenge, err := l.Password(r.Context(), db, req.Username, req.Password, client(r))
		if throttled, ok := errors.AsType[*ThrottledError](err); ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(throttled.RetryAfter.Seconds()))))
			writeLoginError(w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", "too many login attempts; try again later")

			return
		}
		if errors.Is(err, ErrInvalidCredentials) {
			writeLoginError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid username or password")

			return
		}
		if err != nil {
			l.internalError(w, r, err)

			return
		}
		methods, err := l.SecondFactorMethods(r.Context(), db, challenge)
		if err != nil {
			l.internalError(w, r, err)

			return
		}

		http.SetCookie(w, l.sessions.namedCookie(r, ChallengeCookieName, challengeCookiePath, token, challenge.ExpiresAt))
		next := nextSecondFactor
		if len(methods) == 0 {
			next = nextEnrollSecondFactor
		}
		writeJSON(w, http.StatusOK, loginResponse{Next: next, Methods: methods, ExpiresAt: challenge.ExpiresAt})
	})
}

// TOTPHandler は、TOTP のコードでログインを完了させるハンドラーを返す (POST /auth/login/totp)。
func (l *Logins) TOTPHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return l.completeHandler(func(r *http.Request, token, code string) (*LoginResult, error) {
		return l.CompleteWithTOTP(r.Context(), db, token, code, client(r))
	})
}

// RecoveryCodeHandler は、リカバリーコードでログインを完了させるハンドラーを返す (POST /auth/login/recovery-code)。
func (l *Logins) RecoveryCodeHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return l.completeHandler(func(r *http.Request, token, code string) (*LoginResult, error) {
		return l.CompleteWithRecoveryCode(r.Context(), db, token, code, client(r))
	})
}

// EnrollmentConfirmHandler は、ログインの途中で登録する TOTP のコードを確かめ、ログインを完了させるハンドラーを返す
// (POST /auth/login/totp/enrollment/confirm)。リカバリーコードを、この応答でだけ返す。
func (l *Logins) EnrollmentConfirmHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return l.completeHandler(func(r *http.Request, token, code string) (*LoginResult, error) {
		return l.CompleteEnrollment(r.Context(), db, token, code, client(r))
	})
}

// EnrollmentHandler は、第二要素を 1 つも登録していないユーザーに、ログインの途中で TOTP の登録を始めさせるハンドラーを返す
// (POST /auth/login/totp/enrollment)。秘密キーと otpauth URI を返す。
func (l *Logins) EnrollmentHandler(db Database) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireJSONPost(w, r) {
			return
		}
		token, ok := l.challengeToken(w, r)
		if !ok {
			return
		}
		enrollment, err := l.BeginEnrollment(r.Context(), db, token)
		if l.writeSecondFactorError(w, r, err) {
			return
		}
		writeJSON(w, http.StatusOK, enrollmentResponse{Secret: enrollment.Secret, URI: enrollment.URI, ExpiresAt: enrollment.ExpiresAt})
	})
}

// completeHandler は、第二要素のコードを受け取り、complete でログインを完了させ、セッションの Cookie を返すハンドラーを返す。
func (l *Logins) completeHandler(complete func(r *http.Request, token, code string) (*LoginResult, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req codeRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Code == "" {
			writeLoginError(w, http.StatusBadRequest, "BAD_REQUEST", "code is required")

			return
		}
		token, ok := l.challengeToken(w, r)
		if !ok {
			return
		}
		result, err := complete(r, token, req.Code)
		if l.writeSecondFactorError(w, r, err) {
			return
		}

		l.sessions.SetCookie(w, r, result.Token, result.Session)
		http.SetCookie(w, l.sessions.namedCookie(r, ChallengeCookieName, challengeCookiePath, "", time.Unix(0, 0)))
		writeJSON(w, http.StatusOK, completedResponse{RecoveryCodes: result.RecoveryCodes})
	})
}

// challengeToken は、第二要素の確認を待つトークンを Cookie から取り出す。なければ 401 を返す。
func (l *Logins) challengeToken(w http.ResponseWriter, r *http.Request) (string, bool) {
	cookie, err := r.Cookie(ChallengeCookieName)
	if err != nil || cookie.Value == "" {
		writeLoginError(w, http.StatusUnauthorized, "LOGIN_REQUIRED", "verify the password first")

		return "", false
	}

	return cookie.Value, true
}

// writeSecondFactorError は、第二要素の確認のエラーを書く。エラーがなければ false を返す。
func (l *Logins) writeSecondFactorError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrLoginChallengeInvalid):
		http.SetCookie(w, l.sessions.namedCookie(r, ChallengeCookieName, challengeCookiePath, "", time.Unix(0, 0)))
		writeLoginError(w, http.StatusUnauthorized, "LOGIN_REQUIRED", "verify the password again")
	case errors.Is(err, ErrInvalidSecondFactor):
		writeLoginError(w, http.StatusUnauthorized, "INVALID_SECOND_FACTOR", "invalid code")
	case errors.Is(err, ErrSecondFactorEnrolled):
		writeLoginError(w, http.StatusConflict, "SECOND_FACTOR_ENROLLED", "a second factor is already enrolled")
	case errors.Is(err, ErrNoTOTPEnrollment):
		writeLoginError(w, http.StatusBadRequest, "NO_TOTP_ENROLLMENT", "start the TOTP enrollment first")
	default:
		l.internalError(w, r, err)
	}

	return true
}

func (l *Logins) internalError(w http.ResponseWriter, r *http.Request, err error) {
	writeInternalError(w, r, l.logger, "failed to log in", err)
}

// writeInternalError は、内部のエラーをログに残し、詳細を隠して 500 を返す。
func writeInternalError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, message string, err error) {
	logger.ErrorContext(r.Context(), message, slog.Any("error", err))
	writeLoginError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
}

// requireJSONPost は、POST の JSON でなければ、エラーを書いて false を返す。フォームの送信による CSRF を防ぐ。
func requireJSONPost(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeLoginError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "only POST is allowed")

		return false
	}
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		writeLoginError(w, http.StatusUnsupportedMediaType, "BAD_REQUEST", "Content-Type must be application/json")

		return false
	}

	return true
}

// decodeJSON は、POST の JSON の本文を v に読む。読めなければ、エラーを書いて false を返す。
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if !requireJSONPost(w, r) {
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBody)).Decode(v); err != nil {
		writeLoginError(w, http.StatusBadRequest, "BAD_REQUEST", "the body must be a JSON object")

		return false
	}

	return true
}

func writeLoginError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, loginError{Error: loginErrorBody{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// ヘッダーは送ってしまったので、これ以上は伝えられない。
		return
	}
}
