package auth_test

import (
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/webauthntest"
)

const (
	testOrigin  = "https://" + testHost
	deviceLabel = "ノートパソコン"
)

// passkeyFixture は、パスワードを設定したユーザーと、パスキーを扱う Passkeys。
type passkeyFixture struct {
	*loginFixture

	passkeys *auth.Passkeys
}

func newPasskeyFixture(t *testing.T) *passkeyFixture {
	t.Helper()

	f := newLoginFixture(t)

	return &passkeyFixture{loginFixture: f, passkeys: auth.NewPasskeys(&auth.PasskeysOptions{
		Clock: f.clock, Recorder: audit.NewRecorder(f.clock), Logger: slog.New(slog.DiscardHandler), Sessions: f.sessions,
	})}
}

func newAuthenticator(t *testing.T) *webauthntest.Authenticator {
	t.Helper()

	a, err := webauthntest.New(testOrigin)
	require.NoError(t, err)

	return a
}

// beginRegistration は、パスキーの登録を始め、認証器にパスキーを作らせた応答と、手続きのトークンを返す。
func (f *passkeyFixture) beginRegistration(t *testing.T, a *webauthntest.Authenticator) ([]byte, string) {
	t.Helper()

	creation, err := f.passkeys.BeginRegistration(t.Context(), f.db, f.user.ID, f.client)
	require.NoError(t, err)
	response, err := a.Create(creation.Options)
	require.NoError(t, err)

	return response, creation.Token
}

// register は、認証器 a のパスキーを登録する。
func (f *passkeyFixture) register(t *testing.T, a *webauthntest.Authenticator) *auth.Passkey {
	t.Helper()

	response, token := f.beginRegistration(t, a)
	passkey, err := f.passkeys.FinishRegistration(t.Context(), f.db, audit.User(f.user.ID), f.user.ID, token, deviceLabel, response, f.client)
	require.NoError(t, err)

	return passkey
}

// loginWithPasskey は、認証器 a のパスキーだけでログインする。
func (f *passkeyFixture) loginWithPasskey(t *testing.T, a *webauthntest.Authenticator) (*auth.LoginResult, error) {
	t.Helper()

	assertion, err := f.logins.BeginPasskeyLogin(t.Context(), f.db, f.client)
	require.NoError(t, err)
	assert.Empty(t, assertion.Options.AllowedCredentials, "the authenticator chooses the passkey")
	response, err := a.Get(assertion.Options)
	require.NoError(t, err)

	return f.logins.CompleteWithPasskey(t.Context(), f.db, assertion.Token, response, f.client)
}

// TestPasskeyRegistration は、認証器の応答を確かめてから、パスキーを登録することを確かめる。
func TestPasskeyRegistration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// authenticator は、登録を始める前に、認証器の振る舞いを変える。
		authenticator func(a *webauthntest.Authenticator)
		// after は、登録を始めた後に、送るトークンやユーザーを変える。
		after    func(t *testing.T, f *passkeyFixture, token *string, userID *uuid.UUID)
		passName string
		wantErr  error
	}{
		{name: "registered", passName: deviceLabel},
		{name: "name is trimmed", passName: "  " + deviceLabel + "  "},
		{name: "empty name", passName: "   ", wantErr: auth.ErrInvalidPasskeyName},
		{
			name: caseNoUV, passName: deviceLabel, wantErr: auth.ErrInvalidPasskey,
			authenticator: func(a *webauthntest.Authenticator) { a.UserVerified = false },
		},
		{
			name: "another origin", passName: deviceLabel, wantErr: auth.ErrInvalidPasskey,
			authenticator: func(a *webauthntest.Authenticator) { a.Origin = "https://evil.example" },
		},
		{
			name: "unknown ceremony", passName: deviceLabel, wantErr: auth.ErrWebAuthnCeremonyInvalid,
			after: func(_ *testing.T, _ *passkeyFixture, token *string, _ *uuid.UUID) { *token = "unknown" },
		},
		{
			name: "expired ceremony", passName: deviceLabel, wantErr: auth.ErrWebAuthnCeremonyInvalid,
			after: func(_ *testing.T, f *passkeyFixture, _ *string, _ *uuid.UUID) {
				f.clock.Advance(auth.PasskeyCeremonyTTL)
			},
		},
		{
			name: "ceremony of another user", passName: deviceLabel, wantErr: auth.ErrWebAuthnCeremonyInvalid,
			after: func(t *testing.T, f *passkeyFixture, _ *string, userID *uuid.UUID) {
				t.Helper()
				other, err := auth.NewUsers(f.clock).Create(t.Context(), f.db, &auth.NewUser{
					Username: otherName, DisplayName: otherName, Role: auth.RoleMember, PasswordHash: nil,
				})
				require.NoError(t, err)
				*userID = other.ID
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newPasskeyFixture(t)
			a := newAuthenticator(t)
			if tt.authenticator != nil {
				tt.authenticator(a)
			}
			response, token := f.beginRegistration(t, a)
			userID := f.user.ID
			if tt.after != nil {
				tt.after(t, f, &token, &userID)
			}

			passkey, err := f.passkeys.FinishRegistration(t.Context(), f.db, audit.User(userID), userID, token, tt.passName, response, f.client)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Zero(t, f.count(t, "SELECT count(*) FROM user_passkeys"))

				return
			}
			require.NoError(t, err)
			assert.Equal(t, deviceLabel, passkey.Name)
			assert.Equal(t, testHost, passkey.RPID)

			listed, err := f.passkeys.List(t.Context(), f.db, f.user.ID)
			require.NoError(t, err)
			require.Len(t, listed, 1)
			assert.Equal(t, passkey.ID, listed[0].ID)
			assert.Nil(t, listed[0].LastUsedAt)
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.passkey_registered' AND summary->>'name' = $1", deviceLabel))

			// 手続きは 1 回しか使えない。
			_, err = f.passkeys.FinishRegistration(t.Context(), f.db, audit.User(userID), userID, token, tt.passName, response, f.client)
			require.ErrorIs(t, err, auth.ErrWebAuthnCeremonyInvalid)
		})
	}
}

// TestPasskeyRegistrationConsumesTheCeremony は、確かめられなかった手続きも使用済みにすることを確かめる。
func TestPasskeyRegistrationConsumesTheCeremony(t *testing.T) {
	t.Parallel()

	f := newPasskeyFixture(t)
	a := newAuthenticator(t)
	a.UserVerified = false
	response, token := f.beginRegistration(t, a)
	_, err := f.passkeys.FinishRegistration(t.Context(), f.db, audit.User(f.user.ID), f.user.ID, token, deviceLabel, response, f.client)
	require.ErrorIs(t, err, auth.ErrInvalidPasskey)
	assert.Zero(t, f.count(t, "SELECT count(*) FROM webauthn_ceremonies"))
}

// TestPasskeyRegistrationLimit は、登録できるパスキーの数に上限があり、同じ認証器を重ねて登録させないことを確かめる。
func TestPasskeyRegistrationLimit(t *testing.T) {
	t.Parallel()

	f := newPasskeyFixture(t)
	first := newAuthenticator(t)
	f.register(t, first)

	// 登録済みの認証器は、作らせないよう、登録の開始で伝える。
	creation, err := f.passkeys.BeginRegistration(t.Context(), f.db, f.user.ID, f.client)
	require.NoError(t, err)
	require.Len(t, creation.Options.CredentialExcludeList, 1)
	assert.Equal(t, first.CredentialID(), []byte(creation.Options.CredentialExcludeList[0].CredentialID))

	for range auth.MaxPasskeys - 1 {
		f.register(t, newAuthenticator(t))
	}
	_, err = f.passkeys.BeginRegistration(t.Context(), f.db, f.user.ID, f.client)
	require.ErrorIs(t, err, auth.ErrTooManyPasskeys)
}

// TestPasskeyNeedsSecureContext は、パスキーを、ドメイン名の Secure Context でだけ使わせることを確かめる。
func TestPasskeyNeedsSecureContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		host   string
		secure bool
		want   error
	}{
		{name: "https domain", host: testHost, secure: true},
		{name: "https domain with port", host: testHost + ":8443", secure: true},
		{name: "http localhost", host: "localhost:8080", secure: false},
		{name: "http subdomain of localhost", host: "kotatsu.localhost:8080", secure: false},
		{name: "http domain", host: "kotatsu.lan:8080", secure: false, want: auth.ErrPasskeyUnavailable},
		{name: "https IPv4 address", host: "192.168.1.2:8443", secure: true, want: auth.ErrPasskeyUnavailable},
		{name: "http IPv6 loopback", host: "[::1]:8080", secure: false, want: auth.ErrPasskeyUnavailable},
	}
	f := newPasskeyFixture(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &auth.ClientInfo{UserAgent: browserUA, SourceIP: f.client.SourceIP, Host: tt.host, Secure: tt.secure}
			creation, err := f.passkeys.BeginRegistration(t.Context(), f.db, f.user.ID, client)
			_, loginErr := f.logins.BeginPasskeyLogin(t.Context(), f.db, client)
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
				require.ErrorIs(t, loginErr, tt.want)

				return
			}
			require.NoError(t, err)
			require.NoError(t, loginErr)
			assert.NotContains(t, creation.Options.RelyingParty.ID, ":", "the RP ID is a host name without a port")
		})
	}
}

// TestPasskeyLogin は、パスキーだけでログインでき、本人の確認や複製の疑いを確かめることを確かめる。
func TestPasskeyLogin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(t *testing.T, f *passkeyFixture, a *webauthntest.Authenticator)
		wantErr error
	}{
		{name: "logged in"},
		{
			name: "synced passkey keeps the counter zero",
			prepare: func(t *testing.T, f *passkeyFixture, a *webauthntest.Authenticator) {
				t.Helper()
				a.IncrementCounter = false
				_, err := f.loginWithPasskey(t, a)
				require.NoError(t, err)
			},
		},
		{
			name: caseNoUV, wantErr: auth.ErrInvalidPasskey,
			prepare: func(_ *testing.T, _ *passkeyFixture, a *webauthntest.Authenticator) {
				a.UserVerified = false
			},
		},
		{
			name: caseDisabledUser, wantErr: auth.ErrInvalidPasskey,
			prepare: func(t *testing.T, f *passkeyFixture, _ *webauthntest.Authenticator) {
				t.Helper()
				require.NoError(t, auth.NewUsers(f.clock).Disable(t.Context(), f.db, f.user.ID))
			},
		},
		{
			name: "cloned authenticator", wantErr: auth.ErrInvalidPasskey,
			prepare: func(t *testing.T, f *passkeyFixture, a *webauthntest.Authenticator) {
				t.Helper()
				_, err := f.loginWithPasskey(t, a)
				require.NoError(t, err)
				// 複製した認証器は、署名の回数が増えない。
				a.IncrementCounter = false
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newPasskeyFixture(t)
			a := newAuthenticator(t)
			f.register(t, a)
			if tt.prepare != nil {
				tt.prepare(t, f, a)
			}
			sessions := f.count(t, "SELECT count(*) FROM sessions")

			result, err := f.loginWithPasskey(t, a)
			assert.Zero(t, f.count(t, "SELECT count(*) FROM webauthn_ceremonies"), "the ceremony is single-use")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, sessions, f.count(t, "SELECT count(*) FROM sessions"))
				assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.login_failed' AND summary->>'method' = 'passkey'"))

				return
			}
			require.NoError(t, err)
			assert.Equal(t, f.user.ID, result.Session.UserID)
			assert.Equal(t, sessions+1, f.count(t, "SELECT count(*) FROM sessions"))
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM user_passkeys WHERE last_used_at = $1 AND COALESCE((credential->'authenticator'->>'signCount')::bigint, 0) = $2",
				testNow(), int64(a.Counter)))
		})
	}
}

// TestPasskeyLoginRejectsUnknownPasskey は、登録していないパスキーや、使用済みの手続きでは、ログインさせないことを確かめる。
func TestPasskeyLoginRejectsUnknownPasskey(t *testing.T) {
	t.Parallel()

	f := newPasskeyFixture(t)
	f.register(t, newAuthenticator(t))

	_, err := f.loginWithPasskey(t, newAuthenticator(t))
	require.ErrorIs(t, err, auth.ErrInvalidPasskey)
	_, err = f.logins.CompleteWithPasskey(t.Context(), f.db, "unknown", []byte(`{}`), f.client)
	require.ErrorIs(t, err, auth.ErrWebAuthnCeremonyInvalid)
	assert.Zero(t, f.count(t, "SELECT count(*) FROM sessions"))
}

// TestPasskeyAsSecondFactor は、パスワードの後の第二要素として、パスキーを使えることを確かめる。
func TestPasskeyAsSecondFactor(t *testing.T) {
	t.Parallel()

	f := newPasskeyFixture(t)
	a := newAuthenticator(t)
	f.register(t, a)

	token, challenge, err := f.logins.Password(t.Context(), f.db, loginName, loginPhrase, f.client)
	require.NoError(t, err)
	methods, err := f.logins.SecondFactorMethods(t.Context(), f.db, challenge)
	require.NoError(t, err)
	assert.Equal(t, []auth.SecondFactor{auth.SecondFactorPasskey}, methods)
	// パスキーがあれば、パスワードを知っているだけでは、TOTP を登録させない。
	_, err = f.logins.BeginEnrollment(t.Context(), f.db, token)
	require.ErrorIs(t, err, auth.ErrSecondFactorEnrolled)

	// 確認を始めずに応答を送っても、受け付けない。
	_, err = f.logins.CompleteWithPasskeySecondFactor(t.Context(), f.db, token, []byte(`{}`), f.client)
	require.ErrorIs(t, err, auth.ErrInvalidSecondFactor)

	// 本人の確認をしなかった応答は、失敗として数え、確認を始め直させる。
	assertion, err := f.logins.BeginPasskeySecondFactor(t.Context(), f.db, token, f.client)
	require.NoError(t, err)
	require.Len(t, assertion.Options.AllowedCredentials, 1)
	a.UserVerified = false
	response, err := a.Get(assertion.Options)
	require.NoError(t, err)
	_, err = f.logins.CompleteWithPasskeySecondFactor(t.Context(), f.db, token, response, f.client)
	require.ErrorIs(t, err, auth.ErrInvalidSecondFactor)
	a.UserVerified = true
	response, err = a.Get(assertion.Options)
	require.NoError(t, err)
	_, err = f.logins.CompleteWithPasskeySecondFactor(t.Context(), f.db, token, response, f.client)
	require.ErrorIs(t, err, auth.ErrInvalidSecondFactor, "the challenge is single-use")
	assert.Equal(t, 3, f.count(t, "SELECT failed_attempts FROM login_challenges"))

	assertion, err = f.logins.BeginPasskeySecondFactor(t.Context(), f.db, token, f.client)
	require.NoError(t, err)
	response, err = a.Get(assertion.Options)
	require.NoError(t, err)
	result, err := f.logins.CompleteWithPasskeySecondFactor(t.Context(), f.db, token, response, f.client)
	require.NoError(t, err)
	assert.Equal(t, f.user.ID, result.Session.UserID)
	assert.Zero(t, f.count(t, "SELECT count(*) FROM login_challenges"))
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.login' AND summary->>'method' = 'passkey'"))
}

// TestPasskeySecondFactorNeedsAPasskey は、パスキーのないユーザーには、パスキーでの確認を始めさせないことを確かめる。
func TestPasskeySecondFactorNeedsAPasskey(t *testing.T) {
	t.Parallel()

	f := newPasskeyFixture(t)
	_, err := f.logins.BeginPasskeySecondFactor(t.Context(), f.db, f.passwordStep(t), f.client)
	require.ErrorIs(t, err, auth.ErrNoPasskey)
}

// TestRenameAndDeletePasskey は、パスキーの名前を変え、消せること、パスワードのないユーザーの最後のパスキーは消せないことを確かめる。
func TestRenameAndDeletePasskey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// withoutPassword なら、パスワードを持たないユーザーにする。
		withoutPassword bool
		passkeys        int
		wantErr         error
	}{
		{name: "the only passkey of a user with a password", passkeys: 1},
		{name: "one of two passkeys of a user without a password", withoutPassword: true, passkeys: 2},
		{name: "the last passkey of a user without a password", withoutPassword: true, passkeys: 1, wantErr: auth.ErrLastPasskey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newPasskeyFixture(t)
			var target *auth.Passkey
			for range tt.passkeys {
				target = f.register(t, newAuthenticator(t))
			}
			if tt.withoutPassword {
				_, err := f.db.Exec(t.Context(), "UPDATE users SET password_hash = NULL WHERE id = $1", f.user.ID)
				require.NoError(t, err)
			}
			actor := audit.User(f.user.ID)

			err := pgx.BeginFunc(t.Context(), f.db, func(tx pgx.Tx) error {
				return f.passkeys.Rename(t.Context(), tx, actor, f.user.ID, target.ID, "スマートフォン")
			})
			require.NoError(t, err)
			err = pgx.BeginFunc(t.Context(), f.db, func(tx pgx.Tx) error {
				return f.passkeys.Delete(t.Context(), tx, actor, f.user.ID, target.ID)
			})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM user_passkeys WHERE name = 'スマートフォン'"))

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.passkeys-1, f.count(t, "SELECT count(*) FROM user_passkeys"))
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.passkey_deleted' AND summary->>'name' = 'スマートフォン'"))
		})
	}
}

// TestPasskeyOfAnotherUser は、ほかのユーザーのパスキーを、変えも消しもさせないことを確かめる。
func TestPasskeyOfAnotherUser(t *testing.T) {
	t.Parallel()

	f := newPasskeyFixture(t)
	passkey := f.register(t, newAuthenticator(t))
	other, err := auth.NewUsers(f.clock).Create(t.Context(), f.db, &auth.NewUser{
		Username: otherName, DisplayName: otherName, Role: auth.RoleMember, PasswordHash: nil,
	})
	require.NoError(t, err)
	actor := audit.User(other.ID)

	tests := []struct {
		name string
		op   func(tx pgx.Tx) error
	}{
		{name: "rename", op: func(tx pgx.Tx) error {
			return f.passkeys.Rename(t.Context(), tx, actor, other.ID, passkey.ID, "スマートフォン")
		}},
		{name: "delete", op: func(tx pgx.Tx) error {
			return f.passkeys.Delete(t.Context(), tx, actor, other.ID, passkey.ID)
		}},
	}
	for _, tt := range tests {
		require.ErrorIs(t, pgx.BeginFunc(t.Context(), f.db, tt.op), auth.ErrPasskeyNotFound, tt.name)
	}
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM user_passkeys WHERE name = $1", deviceLabel))
}

// TestPasskeyIsBoundToTheHost は、パスキーを、登録したときのホスト名でだけ使わせることを確かめる。
func TestPasskeyIsBoundToTheHost(t *testing.T) {
	t.Parallel()

	f := newPasskeyFixture(t)
	a := newAuthenticator(t)
	f.register(t, a)
	other := &auth.ClientInfo{UserAgent: browserUA, SourceIP: f.client.SourceIP, Host: "kotatsu.other.example", Secure: true}
	a.Origin = "https://" + other.Host

	// ほかのホスト名では、パスキーだけでログインできない。
	assertion, err := f.logins.BeginPasskeyLogin(t.Context(), f.db, other)
	require.NoError(t, err)
	response, err := a.Get(assertion.Options)
	require.NoError(t, err)
	_, err = f.logins.CompleteWithPasskey(t.Context(), f.db, assertion.Token, response, other)
	require.ErrorIs(t, err, auth.ErrInvalidPasskey)

	// 第二要素としても、ほかのホスト名で登録したパスキーは候補にしない。
	token, _, err := f.logins.Password(t.Context(), f.db, loginName, loginPhrase, other)
	require.NoError(t, err)
	_, err = f.logins.BeginPasskeySecondFactor(t.Context(), f.db, token, other)
	require.ErrorIs(t, err, auth.ErrNoPasskey)

	// ほかのホスト名でも、パスキーを登録できる。登録済みのパスキーは、除外の候補にしない。
	creation, err := f.passkeys.BeginRegistration(t.Context(), f.db, f.user.ID, other)
	require.NoError(t, err)
	assert.Empty(t, creation.Options.CredentialExcludeList)
}

// TestPasskeyWithMixedCaseHost は、Host ヘッダーに大文字が混じっていても、小文字の RP ID でパスキーを登録することを確かめる。
// ブラウザーは、オリジンのホスト名を小文字にして署名する。go-webauthn は、オリジンを大文字と小文字を区別せずに比べる。
func TestPasskeyWithMixedCaseHost(t *testing.T) {
	t.Parallel()

	f := newPasskeyFixture(t)
	f.client.Host = "Kotatsu.Example"
	passkey := f.register(t, newAuthenticator(t))
	assert.Equal(t, testHost, passkey.RPID)
}
