package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/totp"
	"github.com/SlashNephy/kotatsu/internal/testutil/webauthntest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// reauthFixture は、TOTP を登録してログインしたセッションを持つ loginFixture。
type reauthFixture struct {
	*loginFixture

	secret        []byte
	recoveryCodes []string
	principal     *auth.Principal
}

func newReauthFixture(t *testing.T) *reauthFixture {
	t.Helper()

	f := newLoginFixture(t)
	secret, result := f.enrollAtLogin(t)
	// 再認証が要るよう、ログインから時間を進める。TOTP のコードも、ログインのときと違うものになる。
	f.clock.Advance(auth.RecentAuthenticationWindow + totp.Period)

	return &reauthFixture{
		loginFixture:  f,
		secret:        secret,
		recoveryCodes: result.RecoveryCodes,
		principal: &auth.Principal{
			UserID: f.user.ID, Role: f.user.Role, SessionID: result.Session.ID, AuthenticatedAt: result.Session.CreatedAt,
		},
	}
}

// authenticatedAt は、セッションの認証の日時を、セッションのトークンを使わずに DB から読む。
func (f *reauthFixture) authenticatedAt(t *testing.T) time.Time {
	t.Helper()

	var at time.Time
	require.NoError(t, f.db.QueryRow(t.Context(), "SELECT authenticated_at FROM sessions WHERE id = $1", f.principal.SessionID).Scan(&at))

	return at.UTC()
}

// TestReauthenticate は、ログイン中のセッションのまま、パスワードと第二要素で再認証できることを確かめる (設計書 9.5)。
func TestReauthenticate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		phrase  string
		method  auth.SecondFactor
		code    func(t *testing.T, f *reauthFixture) string
		prepare func(t *testing.T, f *reauthFixture)
		wantErr error
		// wantFailedMethod は、失敗したときに、監査ログに残す方式。
		wantFailedMethod string
	}{
		{name: "password and TOTP", phrase: loginPhrase, method: auth.SecondFactorTOTP},
		{
			name: "password and recovery code", phrase: loginPhrase, method: auth.SecondFactorRecoveryCode,
			code: func(_ *testing.T, f *reauthFixture) string { return f.recoveryCodes[0] },
		},
		{
			name: caseWrongPassword, phrase: "mikan-kotatsu-neko-2025!", method: auth.SecondFactorTOTP,
			wantErr: auth.ErrInvalidCredentials, wantFailedMethod: passwordText,
		},
		{
			name: "wrong TOTP code", phrase: loginPhrase, method: auth.SecondFactorTOTP,
			code:    func(*testing.T, *reauthFixture) string { return "000000" },
			wantErr: auth.ErrInvalidSecondFactor, wantFailedMethod: "totp",
		},
		{
			name: "used recovery code", phrase: loginPhrase, method: auth.SecondFactorRecoveryCode,
			code: func(t *testing.T, f *reauthFixture) string {
				t.Helper()
				require.NoError(t, f.factors.UseRecoveryCode(t.Context(), f.db, f.user.ID, f.recoveryCodes[0]))

				return f.recoveryCodes[0]
			},
			wantErr: auth.ErrInvalidSecondFactor, wantFailedMethod: "recovery_code",
		},
		{
			// セッションが残っていても、無効にしたユーザーは再認証できない。
			name: caseDisabledUser, phrase: loginPhrase, method: auth.SecondFactorTOTP,
			wantErr: auth.ErrInvalidCredentials, wantFailedMethod: passwordText,
			prepare: func(t *testing.T, f *reauthFixture) {
				t.Helper()
				require.NoError(t, auth.NewUsers(f.clock).Disable(t.Context(), f.db, f.user.ID))
			},
		},
		{name: "passkey with a password", phrase: loginPhrase, method: auth.SecondFactorPasskey, wantErr: auth.ErrUnsupportedSecondFactor},
		{
			name: "revoked session", phrase: loginPhrase, method: auth.SecondFactorTOTP, wantErr: auth.ErrUnauthenticated,
			prepare: func(t *testing.T, f *reauthFixture) {
				t.Helper()
				require.NoError(t, f.sessions.Revoke(t.Context(), f.db, f.user.ID, f.principal.SessionID))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newReauthFixture(t)
			if tt.prepare != nil {
				tt.prepare(t, f)
			}
			code := f.totpCode(t, f.secret)
			if tt.code != nil {
				code = tt.code(t, f)
			}
			before := f.authenticatedAt(t)

			at, err := f.logins.Reauthenticate(t.Context(), f.db, f.principal, tt.phrase, tt.method, code, f.client)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, before, f.authenticatedAt(t))
				assert.Equal(t, boolToInt(tt.wantFailedMethod != ""), f.count(t,
					"SELECT count(*) FROM audit_events WHERE action = 'user.reauthentication_failed' AND summary->>'method' = $1 AND source_ip = '192.168.1.10'",
					tt.wantFailedMethod))

				return
			}
			require.NoError(t, err)
			assert.Equal(t, f.clock.Now(), at)
			assert.Equal(t, f.clock.Now().UTC(), f.authenticatedAt(t))
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.reauthenticated' AND summary->>'method' = $1", string(tt.method)))
		})
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}

	return 0
}

// TestReauthenticateIsThrottled は、再認証の失敗も、ログインと同じく数えて待たせることを確かめる。
// パスワードを知っている人に、第二要素を総当たりで当てさせないためである。
func TestReauthenticateIsThrottled(t *testing.T) {
	t.Parallel()

	f := newReauthFixture(t)
	for range 6 {
		_, err := f.logins.Reauthenticate(t.Context(), f.db, f.principal, loginPhrase, auth.SecondFactorTOTP, "000000", f.client)
		require.ErrorIs(t, err, auth.ErrInvalidSecondFactor)
	}
	_, err := f.logins.Reauthenticate(t.Context(), f.db, f.principal, loginPhrase, auth.SecondFactorTOTP, f.totpCode(t, f.secret), f.client)
	require.ErrorIs(t, err, auth.ErrThrottled)

	// 待ち時間を過ぎれば、再認証できる。成功すると、アカウント単位の失敗の回数を消す。
	f.clock.Advance(time.Minute)
	_, err = f.logins.Reauthenticate(t.Context(), f.db, f.principal, loginPhrase, auth.SecondFactorTOTP, f.totpCode(t, f.secret), f.client)
	require.NoError(t, err)
	assert.Zero(t, f.count(t, "SELECT count(*) FROM login_throttles WHERE key LIKE 'account:%'"))
}

// TestPasskeyReauthentication は、パスキーだけで再認証できることを確かめる。
func TestPasskeyReauthentication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(t *testing.T, f *passkeyFixture, a *webauthntest.Authenticator, principal *auth.Principal)
		wantErr error
	}{
		{name: "reauthenticated"},
		{
			name: caseNoUV, wantErr: auth.ErrInvalidPasskey,
			prepare: func(_ *testing.T, _ *passkeyFixture, a *webauthntest.Authenticator, _ *auth.Principal) {
				a.UserVerified = false
			},
		},
		{
			name: "ceremony of another user", wantErr: auth.ErrWebAuthnCeremonyInvalid,
			prepare: func(t *testing.T, f *passkeyFixture, _ *webauthntest.Authenticator, principal *auth.Principal) {
				t.Helper()
				other, err := auth.NewUsers(f.clock).Create(t.Context(), f.db, &auth.NewUser{
					Username: otherName, DisplayName: otherName, Role: auth.RoleMember, PasswordHash: nil,
				})
				require.NoError(t, err)
				principal.UserID = other.ID
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newPasskeyFixture(t)
			a := newAuthenticator(t)
			f.register(t, a)
			_, session, err := f.sessions.Create(t.Context(), f.db, f.user.ID, f.client)
			require.NoError(t, err)
			principal := &auth.Principal{UserID: f.user.ID, Role: f.user.Role, SessionID: session.ID, AuthenticatedAt: session.CreatedAt}
			f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)

			assertion, err := f.logins.BeginPasskeyReauthentication(t.Context(), f.db, principal, f.client)
			require.NoError(t, err)
			require.Len(t, assertion.Options.AllowedCredentials, 1)
			if tt.prepare != nil {
				tt.prepare(t, f, a, principal)
			}
			response, err := a.Get(assertion.Options)
			require.NoError(t, err)

			at, err := f.logins.CompletePasskeyReauthentication(t.Context(), f.db, principal, assertion.Token, response, f.client)
			var stored time.Time
			require.NoError(t, f.db.QueryRow(t.Context(), "SELECT authenticated_at FROM sessions WHERE id = $1", session.ID).Scan(&stored))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.True(t, stored.Equal(session.CreatedAt), "the session must not be reauthenticated")

				return
			}
			require.NoError(t, err)
			assert.Equal(t, f.clock.Now(), at)
			assert.True(t, stored.Equal(f.clock.Now()))
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.reauthenticated' AND summary->>'method' = 'passkey'"))
		})
	}
}

// TestPasskeyReauthenticationNeedsAPasskey は、パスキーのないユーザーには、パスキーでの再認証を始めさせないことを確かめる。
func TestPasskeyReauthenticationNeedsAPasskey(t *testing.T) {
	t.Parallel()

	f := newPasskeyFixture(t)
	_, session, err := f.sessions.Create(t.Context(), f.db, f.user.ID, f.client)
	require.NoError(t, err)
	_, err = f.logins.BeginPasskeyReauthentication(t.Context(), f.db, &auth.Principal{
		UserID: f.user.ID, Role: f.user.Role, SessionID: session.ID, AuthenticatedAt: session.CreatedAt,
	}, f.client)
	require.ErrorIs(t, err, auth.ErrNoPasskey)
}

// TestReauthenticateHandler は、再認証の HTTP の応答を確かめる。
func TestReauthenticateHandler(t *testing.T) {
	t.Parallel()

	f := newReauthFixture(t)
	client := func(*http.Request) *auth.ClientInfo { return f.client }
	handler := f.logins.ReauthenticateHandler(f.db, client)
	serve := func(principal *auth.Principal, body string) (int, string) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/reauth", strings.NewReader(body))
		req.Header.Set("Content-Type", jsonType)
		if principal != nil {
			req = req.WithContext(auth.WithPrincipal(req.Context(), principal))
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		return rec.Code, rec.Body.String()
	}
	body := func(phrase, method, code string) string {
		return `{"password":"` + phrase + `","method":"` + method + `","code":"` + code + `"}`
	}

	tests := []struct {
		name       string
		principal  *auth.Principal
		body       string
		wantStatus int
		want       string
	}{
		{name: "not logged in", body: body(loginPhrase, "totp", "000000"), wantStatus: http.StatusUnauthorized, want: `"code":"UNAUTHENTICATED"`},
		{name: "missing code", principal: f.principal, body: body(loginPhrase, "totp", ""), wantStatus: http.StatusBadRequest, want: `"code":"BAD_REQUEST"`},
		{name: "passkey with a password", principal: f.principal, body: body(loginPhrase, "passkey", "x"), wantStatus: http.StatusBadRequest, want: `"code":"BAD_REQUEST"`},
		{name: "wrong code", principal: f.principal, body: body(loginPhrase, "totp", "000000"), wantStatus: http.StatusUnauthorized, want: `"code":"INVALID_SECOND_FACTOR"`},
		{name: caseWrongPassword, principal: f.principal, body: body("wrong-password-2026", "totp", "000000"), wantStatus: http.StatusUnauthorized, want: `"code":"INVALID_CREDENTIALS"`},
		{
			name: "reauthenticated", principal: f.principal, body: body(loginPhrase, "totp", f.totpCode(t, f.secret)),
			wantStatus: http.StatusOK, want: `"authenticatedAt":"` + f.clock.Now().UTC().Format(time.RFC3339Nano) + `"`,
		},
	}
	for _, tt := range tests {
		code, got := serve(tt.principal, tt.body)
		assert.Equal(t, tt.wantStatus, code, tt.name)
		assert.Contains(t, got, tt.want, tt.name)
	}
}
