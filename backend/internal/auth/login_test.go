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
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/argon2"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/password"
)

const (
	browserUA = "Mozilla/5.0"
	testHost  = "kotatsu.example"
	otherName = "other"
	// 表のケースの名前と、パスワードを表す文字列 (JSON のキー、監査ログの方式)。
	caseWrongPassword = "wrong password"
	caseNoUV          = "without user verification"
	caseDisabledUser  = "disabled user"
	passwordText      = "password"
	// テストのユーザーの名前と、規定に当たる弱いパスワード。
	mikanName  = "mikan"
	mikanLabel = "みかん"
	nekoName   = "neko"
	rootName   = "root"
	weakPhrase = "kotatsukotatsu"
	// nameText は、名前を表す文字列 (JSON のキー、OIDC の claim)。
	nameText    = "name"
	unknownUser = "unknown user"
	jsonType    = "application/json"
	loginName   = "Kotatsu"
	loginPhrase = "mikan-kotatsu-neko-2026!"
)

// loginFixture は、パスワードを設定したユーザーを 1 人持つ DB と、偽の時計で動く Logins。
type loginFixture struct {
	db       *pgxpool.Pool
	clock    *clocktest.Fake
	sessions *auth.Sessions
	factors  *auth.SecondFactors
	logins   *auth.Logins
	user     *auth.User
	client   *auth.ClientInfo
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

	sessions := auth.NewSessions(&auth.SessionsOptions{Clock: clk})
	factors, err := auth.NewSecondFactors(clk, audit.NewRecorder(clk), appKeyMaterial)
	require.NoError(t, err)

	return &loginFixture{
		db:       db,
		clock:    clk,
		sessions: sessions,
		factors:  factors,
		logins: auth.NewLogins(&auth.LoginsOptions{
			Clock: clk, Recorder: audit.NewRecorder(clk), Logger: slog.New(slog.DiscardHandler),
			Sessions: sessions, SecondFactors: factors, ChallengeTTL: 0,
		}),
		user:   user,
		client: &auth.ClientInfo{UserAgent: browserUA, SourceIP: netip.MustParseAddr("192.168.1.10"), Host: testHost, Secure: true},
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
		{name: caseWrongPassword, username: loginName, phrase: "mikan-kotatsu-neko-2025!", wantErr: auth.ErrInvalidCredentials},
		{name: unknownUser, username: "nobody", phrase: loginPhrase, wantErr: auth.ErrInvalidCredentials},
		{
			name: caseDisabledUser, username: loginName, phrase: loginPhrase, wantErr: auth.ErrInvalidCredentials,
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

	// 待ち時間が過ぎれば、正しいパスワードで第二要素の確認に進める。
	// アカウントの失敗の回数は、第二要素まで確かめ終えるまで消さないので、次の失敗でまた待たせる。
	_, _, err := f.logins.Password(t.Context(), f.db, loginName, loginPhrase, f.client)
	require.NoError(t, err)
	require.ErrorIs(t, wrong(), auth.ErrInvalidCredentials)
	_, throttled := errors.AsType[*auth.ThrottledError](wrong())
	assert.True(t, throttled, "the account failures should not be reset by the password alone")
}

// TestConcurrentPasswordLoginsAreThrottled は、同時に送った試行も、試行回数の制限を素通りしないことを確かめる (#542)。
// 同じアカウントか IP の試行は 1 つずつ確かめるので、パスワードを確かめるのは、待たせない回数と、待たせ始める 1 回までになる。
func TestConcurrentPasswordLoginsAreThrottled(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	const attempts = 30
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	start := make(chan struct{})
	for range attempts {
		wg.Go(func() {
			<-start
			_, _, err := f.logins.Password(t.Context(), f.db, loginName, "wrong-password", f.client)
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)

	verified := 0
	for err := range results {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			verified++

			continue
		}
		require.ErrorIs(t, err, auth.ErrThrottled)
	}
	// 待たせない 5 回と、待たせ始める 6 回目まで。
	assert.LessOrEqual(t, verified, 6)
	assert.Equal(t, verified, f.count(t, `SELECT failures FROM login_throttles WHERE key = $1`, "account:kotatsu"))
}

// TestPasswordVerificationsAreLimited は、パスワードを同時に確かめる数に上限があり、順番を待ちきれない試行は、
// 確かめずに待たせることを確かめる (#542)。順番が空けば、また確かめる。
func TestPasswordVerificationsAreLimited(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	limited := auth.NewLogins(&auth.LoginsOptions{
		Clock: f.clock, Recorder: audit.NewRecorder(f.clock), Logger: slog.New(slog.DiscardHandler),
		Sessions: nil, SecondFactors: nil, ChallengeTTL: 0,
		MaxConcurrentVerifications: 1, VerificationWait: 50 * time.Millisecond,
	})
	release := limited.OccupyVerifications(1)
	_, _, err := limited.Password(t.Context(), f.db, loginName, "wrong-password", f.client)
	require.ErrorIs(t, err, auth.ErrThrottled)
	// 確かめていないので、失敗として数えない。
	assert.Zero(t, f.count(t, `SELECT count(*) FROM login_throttles`))

	release()
	_, _, err = limited.Password(t.Context(), f.db, loginName, "wrong-password", f.client)
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
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
	other := &auth.ClientInfo{UserAgent: browserUA, SourceIP: netip.MustParseAddr("192.168.1.11"), Host: testHost, Secure: true}
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
		{name: caseWrongPassword, method: http.MethodPost, contentType: jsonType, body: `{"username":"Kotatsu","password":"x"}`, wantStatus: http.StatusUnauthorized, wantCode: "INVALID_CREDENTIALS"},
		{name: unknownUser, method: http.MethodPost, contentType: jsonType, body: `{"username":"nobody","password":"x"}`, wantStatus: http.StatusUnauthorized, wantCode: "INVALID_CREDENTIALS"},
		{
			name: "throttled", method: http.MethodPost, contentType: jsonType, body: `{"username":"Kotatsu","password":"x"}`,
			throttle: true, wantStatus: http.StatusTooManyRequests, wantCode: "TOO_MANY_ATTEMPTS",
		},
		{name: "missing password", method: http.MethodPost, contentType: jsonType, body: `{"username":"Kotatsu"}`, wantStatus: http.StatusBadRequest, wantCode: badRequest},
		{name: "form post", method: http.MethodPost, contentType: "application/x-www-form-urlencoded", body: "username=Kotatsu", wantStatus: http.StatusUnsupportedMediaType, wantCode: badRequest},
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
			handler := f.logins.PasswordHandler(f.db, func(*http.Request) *auth.ClientInfo { return f.client })
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
			// 第二要素を登録していないので、TOTP の登録を求める。
			assert.JSONEq(t, `{"next":"enroll_second_factor","methods":[],"expiresAt":"2026-10-09T21:05:00Z"}`, string(body))
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
			passwords := auth.NewPasswords(f.clock, audit.NewRecorder(f.clock), newTestPolicy(t))

			err := passwords.Set(t.Context(), f.db, audit.System("cli"), f.user.ID, tt.phrase, nil)
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
	policy := newTestPolicy(t)
	_, err := auth.UserIDByUsername(t.Context(), f.db, "nobody")
	require.ErrorIs(t, err, auth.ErrUserNotFound)
	err = auth.NewPasswords(f.clock, audit.NewRecorder(f.clock), policy).Set(t.Context(), f.db, audit.System("cli"), id.New(), loginPhrase, nil)
	require.ErrorIs(t, err, auth.ErrUserNotFound)
}

// TestSetPasswordRevokesSessions は、パスワードを設定したら、ほかのセッションを失効させることを確かめる。
// 本人が変えたときは、今のセッションだけを残す。漏れた先のセッションを切れるようにするためである。
func TestSetPasswordRevokesSessions(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	passwords := auth.NewPasswords(f.clock, audit.NewRecorder(f.clock), newTestPolicy(t))
	login := func() (string, *auth.Session) {
		token, session, err := f.sessions.Create(t.Context(), f.db, f.user.ID, f.client)
		require.NoError(t, err)

		return token, session
	}
	usable := func(token string) bool {
		_, err := f.sessions.Authenticate(t.Context(), f.db, token)
		if errors.Is(err, auth.ErrSessionInvalid) {
			return false
		}
		require.NoError(t, err)

		return true
	}

	current, session := login()
	other, _ := login()
	// 前のパスワードで確かめ、第二要素の確認を待っているログイン。
	pending, _, err := f.logins.Password(t.Context(), f.db, loginName, loginPhrase, f.client)
	require.NoError(t, err)
	require.NoError(t, passwords.Set(t.Context(), f.db, audit.User(f.user.ID), f.user.ID, "neko-to-mikan-no-fuyu-2026", &session.ID))
	assert.True(t, usable(current), "the session that changed the password must stay")
	assert.False(t, usable(other), "the other sessions must be revoked")
	_, err = f.logins.BeginEnrollment(t.Context(), f.db, pending)
	require.ErrorIs(t, err, auth.ErrLoginChallengeInvalid, "the login waiting for the second factor must be discarded")

	// 管理者が設定し直したときと、CLI で設定したときは、すべて失効させる。
	require.NoError(t, passwords.Set(t.Context(), f.db, audit.System("cli"), f.user.ID, "yuki-no-yoru-ni-hanabi-2027", nil))
	assert.False(t, usable(current))
}
