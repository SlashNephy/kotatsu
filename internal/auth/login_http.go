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

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginResponse は、パスワードを確かめた後の応答。next は、次に確かめるもの。
type loginResponse struct {
	Next string `json:"next"`
	// Methods は、使える第二要素の種類 (totp、passkey、recovery_code)。登録したものがなければ空。
	Methods   []string  `json:"methods"`
	ExpiresAt time.Time `json:"expiresAt"`
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
func (l *Logins) PasswordHandler(db Database, sessions *Sessions, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeLoginError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "only POST is allowed")

			return
		}
		if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
			writeLoginError(w, http.StatusUnsupportedMediaType, "BAD_REQUEST", "Content-Type must be application/json")

			return
		}
		var req loginRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBody)).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
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
			l.logger.ErrorContext(r.Context(), "failed to log in", slog.Any("error", err))
			writeLoginError(w, http.StatusInternalServerError, "INTERNAL", "internal error")

			return
		}

		http.SetCookie(w, sessions.namedCookie(r, ChallengeCookieName, challengeCookiePath, token, challenge.ExpiresAt))
		// 第二要素の登録と確認は #33 で作る。今は、どの種類も登録できないので空にする。
		writeJSON(w, http.StatusOK, loginResponse{Next: "second_factor", Methods: []string{}, ExpiresAt: challenge.ExpiresAt})
	})
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
