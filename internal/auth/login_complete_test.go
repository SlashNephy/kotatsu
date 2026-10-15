package auth_test

import (
	"encoding/base32"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/totp"
)

// passwordStep は、パスワードを確かめ、第二要素の確認を待つトークンを返す。
func (f *loginFixture) passwordStep(t *testing.T) string {
	t.Helper()

	token, _, err := f.logins.Password(t.Context(), f.db, loginName, loginPhrase, f.client)
	require.NoError(t, err)

	return token
}

// enrollAtLogin は、ログインの途中で TOTP を登録してログインを完了させ、秘密キーと結果を返す。
func (f *loginFixture) enrollAtLogin(t *testing.T) ([]byte, *auth.LoginResult) {
	t.Helper()

	token := f.passwordStep(t)
	enrollment, err := f.logins.BeginEnrollment(t.Context(), f.db, token)
	require.NoError(t, err)
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	require.NoError(t, err)
	result, err := f.logins.CompleteEnrollment(t.Context(), f.db, token, f.totpCode(t, secret), f.client)
	require.NoError(t, err)

	return secret, result
}

func (f *loginFixture) totpCode(t *testing.T, secret []byte) string {
	t.Helper()

	code, err := totp.Code(secret, totp.Step(f.clock.Now()), totp.Digits)
	require.NoError(t, err)

	return code
}

// TestEnrollAtLogin は、第二要素のないユーザーが、パスワードの確認の後に TOTP を登録してログインできることを確かめる。
func TestEnrollAtLogin(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	_, result := f.enrollAtLogin(t)
	assert.NotEmpty(t, result.Token)
	assert.Equal(t, f.user.ID, result.Session.UserID)
	assert.Len(t, result.RecoveryCodes, auth.RecoveryCodeCount)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM sessions WHERE user_id = $1", f.user.ID))
	assert.Zero(t, f.count(t, "SELECT count(*) FROM login_challenges"))
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.login' AND summary->>'method' = 'totp'"))

	// 登録した後は、パスワードを知っているだけでは、登録し直せない。
	_, err := f.logins.BeginEnrollment(t.Context(), f.db, f.passwordStep(t))
	require.ErrorIs(t, err, auth.ErrSecondFactorEnrolled)
}

// TestLoginWithTOTP は、TOTP でログインを完了させ、失敗が多すぎたらパスワードの確認からやり直させることを確かめる。
func TestLoginWithTOTP(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	secret, _ := f.enrollAtLogin(t)
	f.clock.Advance(totp.Period)

	token := f.passwordStep(t)
	for range 4 {
		_, err := f.logins.CompleteWithTOTP(t.Context(), f.db, token, "000000", f.client)
		require.ErrorIs(t, err, auth.ErrInvalidSecondFactor)
	}
	// 5 回目の失敗で、待っているログインを消す。正しいコードでも、もう使えない。
	_, err := f.logins.CompleteWithTOTP(t.Context(), f.db, token, "000000", f.client)
	require.ErrorIs(t, err, auth.ErrInvalidSecondFactor)
	_, err = f.logins.CompleteWithTOTP(t.Context(), f.db, token, f.totpCode(t, secret), f.client)
	require.ErrorIs(t, err, auth.ErrLoginChallengeInvalid)
	assert.Equal(t, 5, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.login_failed' AND summary->>'method' = 'totp'"))

	token = f.passwordStep(t)
	result, err := f.logins.CompleteWithTOTP(t.Context(), f.db, token, f.totpCode(t, secret), f.client)
	require.NoError(t, err)
	assert.Empty(t, result.RecoveryCodes)
	assert.Equal(t, 2, f.count(t, "SELECT count(*) FROM sessions WHERE user_id = $1", f.user.ID))
}

// TestSecondFactorFailuresThrottle は、パスワードの確認からやり直しても、第二要素の失敗を数え続け、
// 続けば待たせることを確かめる。パスワードを知っている人に、第二要素のコードを総当たりさせない。
func TestSecondFactorFailuresThrottle(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	secret, _ := f.enrollAtLogin(t)
	f.clock.Advance(totp.Period)

	// 5 回までは待たせない。待っているログインは 5 回の失敗で消えるので、パスワードの確認からやり直す。
	token := f.passwordStep(t)
	for range 5 {
		_, err := f.logins.CompleteWithTOTP(t.Context(), f.db, token, "000000", f.client)
		require.ErrorIs(t, err, auth.ErrInvalidSecondFactor)
	}
	token = f.passwordStep(t)
	_, err := f.logins.CompleteWithTOTP(t.Context(), f.db, token, "000000", f.client)
	require.ErrorIs(t, err, auth.ErrInvalidSecondFactor)

	// 6 回目の失敗で待たせる。待っている間は、第二要素もパスワードも試させない。
	_, err = f.logins.CompleteWithTOTP(t.Context(), f.db, token, f.totpCode(t, secret), f.client)
	throttled, ok := errors.AsType[*auth.ThrottledError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, time.Second, throttled.RetryAfter)
	_, _, err = f.logins.Password(t.Context(), f.db, loginName, loginPhrase, f.client)
	require.ErrorIs(t, err, auth.ErrThrottled)

	// 待ち時間が過ぎれば、第二要素まで確かめてログインでき、アカウントの失敗の回数は消える。
	f.clock.Advance(throttled.RetryAfter)
	_, err = f.logins.CompleteWithTOTP(t.Context(), f.db, token, f.totpCode(t, secret), f.client)
	require.NoError(t, err)
	assert.Zero(t, f.count(t, "SELECT count(*) FROM login_throttles WHERE key LIKE 'account:%' AND failures > 0"))
}

func TestLoginWithRecoveryCode(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	_, enrolled := f.enrollAtLogin(t)

	_, err := f.logins.CompleteWithRecoveryCode(t.Context(), f.db, f.passwordStep(t), enrolled.RecoveryCodes[0], f.client)
	require.NoError(t, err)
	// 同じコードは、二度と使えない。
	_, err = f.logins.CompleteWithRecoveryCode(t.Context(), f.db, f.passwordStep(t), enrolled.RecoveryCodes[0], f.client)
	require.ErrorIs(t, err, auth.ErrInvalidSecondFactor)
}

func TestLoginChallengeExpires(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	secret, _ := f.enrollAtLogin(t)
	token := f.passwordStep(t)
	f.clock.Advance(auth.DefaultChallengeTTL + time.Second)

	_, err := f.logins.CompleteWithTOTP(t.Context(), f.db, token, f.totpCode(t, secret), f.client)
	require.ErrorIs(t, err, auth.ErrLoginChallengeInvalid)
}

// TestLoginHandlers は、Cookie を受け渡しながら、パスワード、TOTP の登録、確認の順にログインできることを確かめる。
func TestLoginHandlers(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	// Cookie の期限は時計から決まる。過去の時刻のままだと、Cookie の入れ物が期限切れとして捨てる。
	f.clock.Advance(time.Since(f.clock.Now()))
	client := func(*http.Request) *auth.ClientInfo { return f.client }
	mux := http.NewServeMux()
	mux.Handle("POST /auth/login", f.logins.PasswordHandler(f.db, client))
	mux.Handle("POST /auth/login/totp", f.logins.TOTPHandler(f.db, client))
	mux.Handle("POST /auth/login/totp/enrollment", f.logins.EnrollmentHandler(f.db))
	mux.Handle("POST /auth/login/totp/enrollment/confirm", f.logins.EnrollmentConfirmHandler(f.db, client))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	httpClient := &http.Client{Jar: jar}

	post := func(path, body string, v any) int {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+path, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", jsonType)
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		defer func() {
			_ = resp.Body.Close()
		}()
		if v != nil {
			require.NoError(t, json.NewDecoder(resp.Body).Decode(v))
		}

		return resp.StatusCode
	}
	cookie := func(name string) string {
		for _, c := range jar.Cookies(mustParseURL(t, server.URL+"/auth/")) {
			if c.Name == name {
				return c.Value
			}
		}

		return ""
	}

	// 確認を待つトークンがなければ、登録を始められない。
	assert.Equal(t, http.StatusUnauthorized, post("/auth/login/totp/enrollment", "{}", nil))

	var login struct {
		Next string `json:"next"`
	}
	require.Equal(t, http.StatusOK, post("/auth/login", `{"username":"Kotatsu","password":"`+loginPhrase+`"}`, &login))
	assert.Equal(t, "enroll_second_factor", login.Next)
	assert.NotEmpty(t, cookie(auth.ChallengeCookieName))

	var enrollment struct {
		Secret string `json:"secret"`
		URI    string `json:"uri"`
	}
	require.Equal(t, http.StatusOK, post("/auth/login/totp/enrollment", "{}", &enrollment))
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	require.NoError(t, err)

	var failed struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.Equal(t, http.StatusUnauthorized, post("/auth/login/totp/enrollment/confirm", `{"code":"000000"}`, &failed))
	assert.Equal(t, "INVALID_SECOND_FACTOR", failed.Error.Code)
	assert.Empty(t, cookie(auth.CookieName))

	var completed struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	require.Equal(t, http.StatusOK, post("/auth/login/totp/enrollment/confirm", `{"code":"`+f.totpCode(t, secret)+`"}`, &completed))
	assert.Len(t, completed.RecoveryCodes, auth.RecoveryCodeCount)
	assert.NotEmpty(t, cookie(auth.CookieName), "the session cookie must be set")
	assert.Empty(t, cookie(auth.ChallengeCookieName), "the challenge cookie must be cleared")

	// 2 回目からは、登録した TOTP で確かめる。
	f.clock.Advance(totp.Period)
	require.Equal(t, http.StatusOK, post("/auth/login", `{"username":"Kotatsu","password":"`+loginPhrase+`"}`, &login))
	assert.Equal(t, "second_factor", login.Next)
	require.Equal(t, http.StatusOK, post("/auth/login/totp", `{"code":"`+f.totpCode(t, secret)+`"}`, &completed))
	assert.Empty(t, completed.RecoveryCodes)

	// 第二要素の失敗が続いて待たせている間は、429 を返す。
	f.clock.Advance(totp.Period)
	for range 2 {
		require.Equal(t, http.StatusOK, post("/auth/login", `{"username":"Kotatsu","password":"`+loginPhrase+`"}`, &login))
		for range 3 {
			require.Equal(t, http.StatusUnauthorized, post("/auth/login/totp", `{"code":"000000"}`, nil))
		}
	}
	require.Equal(t, http.StatusTooManyRequests, post("/auth/login/totp", `{"code":"000000"}`, &failed))
	assert.Equal(t, "TOO_MANY_ATTEMPTS", failed.Error.Code)
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	u, err := url.Parse(raw)
	require.NoError(t, err)

	return u
}
