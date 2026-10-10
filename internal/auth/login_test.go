package auth_test

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/argon2"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/password"
)

const (
	browserUA   = "Mozilla/5.0"
	unknownUser = "unknown user"
	jsonType    = "application/json"
	loginName   = "Kotatsu"
	loginPhrase = "mikan-kotatsu-neko-2026!"
)

// loginFixture は、パスワードを設定したユーザーを 1 人持つ DB と、偽の時計で動く Logins。
type loginFixture struct {
	db     *pgxpool.Pool
	clock  *clocktest.Fake
	logins *auth.Logins
	user   *auth.User
	client *auth.ClientInfo
}

func newLoginFixture(t *testing.T) *loginFixture {
	t.Helper()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	encoded, err := password.Hash(loginPhrase)
	require.NoError(t, err)
	user, err := auth.NewUsers(clk).Create(t.Context(), db, &auth.NewUser{
		Username: loginName, DisplayName: "こたつ", Role: auth.RoleMember, PasswordHash: &encoded,
	})
	require.NoError(t, err)

	return &loginFixture{
		db:    db,
		clock: clk,
		logins: auth.NewLogins(&auth.LoginsOptions{
			Clock: clk, Recorder: audit.NewRecorder(clk), Logger: slog.New(slog.DiscardHandler), ChallengeTTL: 0,
		}),
		user:   user,
		client: &auth.ClientInfo{UserAgent: browserUA, SourceIP: netip.MustParseAddr("192.168.1.10")},
	}
}

func (f *loginFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()

	var n int
	require.NoError(t, f.db.QueryRow(t.Context(), query, args...).Scan(&n))

	return n
}

// TestPasswordLogin は、パスワードを確かめても、セッションを作らずに、第二要素の確認を待つことを確かめる。
func TestPasswordLogin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		username string
		phrase   string
		prepare  func(t *testing.T, f *loginFixture)
		wantErr  error
	}{
		{name: "correct password", username: loginName, phrase: loginPhrase},
		{name: "username is case-insensitive", username: strings.ToLower(loginName), phrase: loginPhrase},
		{name: "wrong password", username: loginName, phrase: "mikan-kotatsu-neko-2025!", wantErr: auth.ErrInvalidCredentials},
		{name: unknownUser, username: "nobody", phrase: loginPhrase, wantErr: auth.ErrInvalidCredentials},
		{
			name: "disabled user", username: loginName, phrase: loginPhrase, wantErr: auth.ErrInvalidCredentials,
			prepare: func(t *testing.T, f *loginFixture) {
				t.Helper()
				require.NoError(t, auth.NewUsers(f.clock).Disable(t.Context(), f.db, f.user.ID))
			},
		},
		{
			name: "user without a password", username: loginName, phrase: loginPhrase, wantErr: auth.ErrInvalidCredentials,
			prepare: func(t *testing.T, f *loginFixture) {
				t.Helper()
				_, err := f.db.Exec(t.Context(), "UPDATE users SET password_hash = NULL WHERE id = $1", f.user.ID)
				require.NoError(t, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newLoginFixture(t)
			if tt.prepare != nil {
				tt.prepare(t, f)
			}
			token, challenge, err := f.logins.Password(t.Context(), f.db, tt.username, tt.phrase, f.client)
			assert.Zero(t, f.count(t, "SELECT count(*) FROM sessions"), "a password alone must not create a session")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.login_failed' AND source_ip = '192.168.1.10'"))
				assert.Zero(t, f.count(t, "SELECT count(*) FROM login_challenges"))

				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, token)
			assert.Equal(t, f.user.ID, challenge.UserID)
			assert.Equal(t, testNow().Add(auth.DefaultChallengeTTL), challenge.ExpiresAt)
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM login_challenges WHERE user_id = $1 AND token_hash = sha256($2::bytea)", f.user.ID, []byte(token)))
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.password_verified' AND actor_user_id = $1", f.user.ID))
		})
	}
}

// TestPasswordLoginThrottles は、失敗が続くと待ち時間が倍々に延び、上限で止まり、成功すると数え直すことを確かめる。
func TestPasswordLoginThrottles(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	wrong := func() error {
		_, _, err := f.logins.Password(t.Context(), f.db, loginName, "wrong-password", f.client)

		return err
	}

	// 5 回までは待たせない。
	for range 5 {
		require.ErrorIs(t, wrong(), auth.ErrInvalidCredentials)
	}
	// それを超えると、1 秒、2 秒、4 秒と待たせる。待たせている間の試行は、失敗として数えない。
	const throttledAttempts = 12
	waits := make([]time.Duration, 0, throttledAttempts)
	for range throttledAttempts {
		require.ErrorIs(t, wrong(), auth.ErrInvalidCredentials)
		err := wrong()
		throttled, ok := errors.AsType[*auth.ThrottledError](err)
		require.True(t, ok, "got %v", err)
		waits = append(waits, throttled.RetryAfter)
		f.clock.Advance(throttled.RetryAfter)
	}
	assert.Equal(t, []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second,
		64 * time.Second, 128 * time.Second, 256 * time.Second, 512 * time.Second, 15 * time.Minute, 15 * time.Minute,
	}, waits)

	// 待ち時間が過ぎれば、正しいパスワードでログインでき、アカウントの失敗の回数は消える。
	_, _, err := f.logins.Password(t.Context(), f.db, loginName, loginPhrase, f.client)
	require.NoError(t, err)
	require.ErrorIs(t, wrong(), auth.ErrInvalidCredentials)
}

// TestPasswordLoginThrottlesByIP は、ユーザー名を変えながら試しても、IP 単位で待たせることを確かめる。
func TestPasswordLoginThrottlesByIP(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	for i := range 20 {
		_, _, err := f.logins.Password(t.Context(), f.db, fmt.Sprintf("user%d", i), loginPhrase, f.client)
		require.ErrorIs(t, err, auth.ErrInvalidCredentials)
	}
	_, _, err := f.logins.Password(t.Context(), f.db, "user20", loginPhrase, f.client)
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
	_, _, err = f.logins.Password(t.Context(), f.db, loginName, loginPhrase, f.client)
	require.ErrorIs(t, err, auth.ErrThrottled)

	// 別の IP からは、待たずに試せる。
	other := &auth.ClientInfo{UserAgent: browserUA, SourceIP: netip.MustParseAddr("192.168.1.11")}
	_, _, err = f.logins.Password(t.Context(), f.db, loginName, loginPhrase, other)
	require.NoError(t, err)
}

// TestStaleLoginThrottlesAreDeleted は、数え直す時間を過ぎた制限の行が、次の失敗のときに消えることを確かめる。
// 存在しないユーザー名の行は、成功で消えることがないので、溜まり続けないようにする。
func TestStaleLoginThrottlesAreDeleted(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	_, _, err := f.logins.Password(t.Context(), f.db, "nobody", "x", f.client)
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM login_throttles WHERE key = 'account:nobody'"))

	f.clock.Advance(25 * time.Hour)
	_, _, err = f.logins.Password(t.Context(), f.db, "someone", "x", f.client)
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
	assert.Zero(t, f.count(t, "SELECT count(*) FROM login_throttles WHERE key = 'account:nobody'"))
	assert.Equal(t, 1, f.count(t, "SELECT failures FROM login_throttles WHERE key = 'ip:192.168.1.10'"))
}

// TestPasswordLoginRehashes は、古いパラメーターのハッシュでログインすると、今のパラメーターで作り直すことを確かめる。
func TestPasswordLoginRehashes(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	salt := []byte("0123456789abcdef")
	old := fmt.Sprintf("$argon2id$v=19$m=8192,t=1,p=1$%s$%s", base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(argon2.IDKey([]byte(loginPhrase), salt, 1, 8*1024, 1, 32)))
	_, err := f.db.Exec(t.Context(), "UPDATE users SET password_hash = $1 WHERE id = $2", old, f.user.ID)
	require.NoError(t, err)

	_, _, err = f.logins.Password(t.Context(), f.db, loginName, loginPhrase, f.client)
	require.NoError(t, err)

	var encoded string
	require.NoError(t, f.db.QueryRow(t.Context(), "SELECT password_hash FROM users WHERE id = $1", f.user.ID).Scan(&encoded))
	assert.True(t, strings.HasPrefix(encoded, "$argon2id$v=19$m=19456,t=2,p=1$"))
	verification, err := password.Verify(loginPhrase, encoded)
	require.NoError(t, err)
	assert.Equal(t, &password.Verification{OK: true, NeedsRehash: false}, verification)
}

func TestPasswordHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		method      string
		contentType string
		body        string
		// throttle は、送る前に、失敗を重ねて待たせるかどうか。
		throttle   bool
		wantStatus int
		wantCode   string
	}{
		{name: "correct password", method: http.MethodPost, contentType: jsonType, body: `{"username":"Kotatsu","password":"` + loginPhrase + `"}`, wantStatus: http.StatusOK},
		{name: "wrong password", method: http.MethodPost, contentType: jsonType, body: `{"username":"Kotatsu","password":"x"}`, wantStatus: http.StatusUnauthorized, wantCode: "INVALID_CREDENTIALS"},
		{name: unknownUser, method: http.MethodPost, contentType: jsonType, body: `{"username":"nobody","password":"x"}`, wantStatus: http.StatusUnauthorized, wantCode: "INVALID_CREDENTIALS"},
		{
			name: "throttled", method: http.MethodPost, contentType: jsonType, body: `{"username":"Kotatsu","password":"x"}`,
			throttle: true, wantStatus: http.StatusTooManyRequests, wantCode: "TOO_MANY_ATTEMPTS",
		},
		{name: "missing password", method: http.MethodPost, contentType: jsonType, body: `{"username":"Kotatsu"}`, wantStatus: http.StatusBadRequest, wantCode: "BAD_REQUEST"},
		{name: "form post", method: http.MethodPost, contentType: "application/x-www-form-urlencoded", body: "username=Kotatsu", wantStatus: http.StatusUnsupportedMediaType, wantCode: "BAD_REQUEST"},
		{name: "GET", method: http.MethodGet, wantStatus: http.StatusMethodNotAllowed, wantCode: "METHOD_NOT_ALLOWED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newLoginFixture(t)
			if tt.throttle {
				for range 6 {
					_, _, _ = f.logins.Password(t.Context(), f.db, loginName, "x", f.client)
				}
			}
			sessions := auth.NewSessions(&auth.SessionsOptions{Clock: f.clock})
			handler := f.logins.PasswordHandler(f.db, sessions, func(*http.Request) *auth.ClientInfo { return f.client })
			req := httptest.NewRequestWithContext(t.Context(), tt.method, "/auth/login", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			body, err := io.ReadAll(rec.Body)
			require.NoError(t, err)

			assert.Equal(t, tt.wantStatus, rec.Code, string(body))
			assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			if tt.wantCode != "" {
				assert.Contains(t, string(body), `"code":"`+tt.wantCode+`"`)
				assert.Empty(t, rec.Result().Cookies())
			}
			if tt.throttle {
				assert.Equal(t, "1", rec.Header().Get("Retry-After"))
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			assert.JSONEq(t, `{"next":"second_factor","methods":[],"expiresAt":"2026-10-09T21:05:00Z"}`, string(body))
			cookies := rec.Result().Cookies()
			require.Len(t, cookies, 1)
			assert.Equal(t, auth.ChallengeCookieName, cookies[0].Name)
			assert.Equal(t, "/auth/", cookies[0].Path)
			assert.True(t, cookies[0].HttpOnly)
			assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
		})
	}
}

func TestSetPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		phrase string
		want   password.Reason
	}{
		{name: "strong password", phrase: "neko-to-mikan-no-fuyu-2026"},
		{name: "too short", phrase: "mikan77", want: password.ReasonTooShort},
		{name: "contains the user name", phrase: "kotatsu123", want: password.ReasonTooWeak},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newLoginFixture(t)
			policy, err := password.NewPolicy(&password.PolicyOptions{HIBP: nil, StrengthTimeout: 10 * time.Minute, Logger: slog.New(slog.DiscardHandler)})
			require.NoError(t, err)
			passwords := auth.NewPasswords(f.clock, audit.NewRecorder(f.clock), policy)

			err = passwords.Set(t.Context(), f.db, audit.System("cli"), f.user.ID, tt.phrase)
			if tt.want != "" {
				rejected, ok := errors.AsType[*password.RejectedError](err)
				require.True(t, ok, "got %v", err)
				assert.Equal(t, tt.want, rejected.Reason)
				// 拒否したら、元のパスワードのままになる。
				_, _, err = f.logins.Password(t.Context(), f.db, loginName, loginPhrase, f.client)
				require.NoError(t, err)

				return
			}
			require.NoError(t, err)
			_, _, err = f.logins.Password(t.Context(), f.db, loginName, tt.phrase, f.client)
			require.NoError(t, err)
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.password_set' AND target_id = $1", f.user.ID.String()))
		})
	}

	f := newLoginFixture(t)
	policy, err := password.NewPolicy(&password.PolicyOptions{HIBP: nil, StrengthTimeout: 10 * time.Minute, Logger: slog.New(slog.DiscardHandler)})
	require.NoError(t, err)
	_, err = auth.UserIDByUsername(t.Context(), f.db, "nobody")
	require.ErrorIs(t, err, auth.ErrUserNotFound)
	err = auth.NewPasswords(f.clock, audit.NewRecorder(f.clock), policy).Set(t.Context(), f.db, audit.System("cli"), id.New(), loginPhrase)
	require.ErrorIs(t, err, auth.ErrUserNotFound)
}
