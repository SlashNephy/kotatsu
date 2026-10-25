package auth_test

import (
	"crypto/sha256"
	"encoding/base32"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/secretbox"
	"github.com/SlashNephy/kotatsu/internal/lib/totp"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// appKeyMaterial は、テストのアプリの秘密キー。
const appKeyMaterial = "kotatsu-test-application-secret-key-0123456789"

type secondFactorFixture struct {
	*loginFixture
}

func newSecondFactorFixture(t *testing.T) *secondFactorFixture {
	t.Helper()

	return &secondFactorFixture{loginFixture: newLoginFixture(t)}
}

// code は、登録で受け取った秘密キーの、今のコードを返す。
func (f *secondFactorFixture) code(t *testing.T, enrollment *auth.TOTPEnrollment) string {
	t.Helper()

	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	require.NoError(t, err)
	code, err := totp.Code(secret, totp.Step(f.clock.Now()), totp.Digits)
	require.NoError(t, err)

	return code
}

// enroll は、TOTP を登録し、秘密キーとリカバリーコードを返す。
func (f *secondFactorFixture) enroll(t *testing.T) (*auth.TOTPEnrollment, []string) {
	t.Helper()

	enrollment, err := f.factors.BeginTOTP(t.Context(), f.db, f.user.ID, f.user.Username)
	require.NoError(t, err)
	var codes []string
	require.NoError(t, pgx.BeginFunc(t.Context(), f.db, func(tx pgx.Tx) error {
		codes, err = f.factors.ConfirmTOTP(t.Context(), tx, audit.User(f.user.ID), f.user.ID, f.code(t, enrollment))

		return err
	}))

	return enrollment, codes
}

func (f *secondFactorFixture) verify(t *testing.T, code string) error {
	t.Helper()

	return pgx.BeginFunc(t.Context(), f.db, func(tx pgx.Tx) error {
		return f.factors.VerifyTOTP(t.Context(), tx, f.user.ID, code)
	})
}

func (f *secondFactorFixture) methods(t *testing.T) []auth.SecondFactor {
	t.Helper()

	methods, err := f.factors.Methods(t.Context(), f.db, f.user.ID)
	require.NoError(t, err)

	return methods
}

func TestTOTPEnrollment(t *testing.T) {
	t.Parallel()

	f := newSecondFactorFixture(t)
	assert.Empty(t, f.methods(t))

	enrollment, err := f.factors.BeginTOTP(t.Context(), f.db, f.user.ID, f.user.Username)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(enrollment.URI, "otpauth://totp/Kotatsu:Kotatsu?"))
	assert.Equal(t, testNow().Add(10*time.Minute), enrollment.ExpiresAt)
	// 確かめるまでは、有効にならない。
	assert.Empty(t, f.methods(t))

	// 違うコードでは確かめられない。
	err = pgx.BeginFunc(t.Context(), f.db, func(tx pgx.Tx) error {
		_, err := f.factors.ConfirmTOTP(t.Context(), tx, audit.User(f.user.ID), f.user.ID, "000000")

		return err
	})
	require.ErrorIs(t, err, auth.ErrInvalidSecondFactor)

	var codes []string
	require.NoError(t, pgx.BeginFunc(t.Context(), f.db, func(tx pgx.Tx) error {
		codes, err = f.factors.ConfirmTOTP(t.Context(), tx, audit.User(f.user.ID), f.user.ID, f.code(t, enrollment))

		return err
	}))
	assert.Len(t, codes, auth.RecoveryCodeCount)
	for _, code := range codes {
		assert.Regexp(t, `^[a-z2-7]{5}-[a-z2-7]{5}$`, code)
	}
	assert.Equal(t, []auth.SecondFactor{auth.SecondFactorTOTP, auth.SecondFactorRecoveryCode}, f.methods(t))

	// 秘密キーは、アプリの秘密キーで暗号化して保存する。
	var sealed []byte
	require.NoError(t, f.db.QueryRow(t.Context(), "SELECT secret_encrypted FROM user_totps WHERE user_id = $1", f.user.ID).Scan(&sealed))
	assert.NotContains(t, string(sealed), enrollment.Secret)
	box, err := secretbox.New(appKeyMaterial, auth.TOTPPurpose)
	require.NoError(t, err)
	opened, err := box.Open(sealed)
	require.NoError(t, err)
	assert.Equal(t, enrollment.Secret, totp.Encode(opened))
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.totp_enabled'"))
}

func TestTOTPEnrollmentExpires(t *testing.T) {
	t.Parallel()

	f := newSecondFactorFixture(t)
	enrollment, err := f.factors.BeginTOTP(t.Context(), f.db, f.user.ID, f.user.Username)
	require.NoError(t, err)
	f.clock.Advance(10*time.Minute + time.Second)

	err = pgx.BeginFunc(t.Context(), f.db, func(tx pgx.Tx) error {
		_, err := f.factors.ConfirmTOTP(t.Context(), tx, audit.User(f.user.ID), f.user.ID, f.code(t, enrollment))

		return err
	})
	require.ErrorIs(t, err, auth.ErrNoTOTPEnrollment)
}

// TestVerifyTOTPRejectsReplay は、一度受け付けたコードを、二度と受け付けないことを確かめる。
func TestVerifyTOTPRejectsReplay(t *testing.T) {
	t.Parallel()

	f := newSecondFactorFixture(t)
	enrollment, _ := f.enroll(t)
	// 登録で使ったコードは、ログインには使えない。
	require.ErrorIs(t, f.verify(t, f.code(t, enrollment)), auth.ErrInvalidSecondFactor)

	f.clock.Advance(totp.Period)
	code := f.code(t, enrollment)
	require.NoError(t, f.verify(t, code))
	require.ErrorIs(t, f.verify(t, code), auth.ErrInvalidSecondFactor)
	require.ErrorIs(t, f.verify(t, "000000"), auth.ErrInvalidSecondFactor)
}

// TestRecoveryCodesAreSingleUse は、リカバリーコードが 1 回だけ使え、作り直すと古いコードが使えなくなることを確かめる。
func TestRecoveryCodesAreSingleUse(t *testing.T) {
	t.Parallel()

	f := newSecondFactorFixture(t)
	_, codes := f.enroll(t)

	// 大文字と区切りの有無は区別しない。
	require.NoError(t, f.factors.UseRecoveryCode(t.Context(), f.db, f.user.ID, strings.ToUpper(strings.ReplaceAll(codes[0], "-", ""))))
	require.ErrorIs(t, f.factors.UseRecoveryCode(t.Context(), f.db, f.user.ID, codes[0]), auth.ErrInvalidSecondFactor)
	require.ErrorIs(t, f.factors.UseRecoveryCode(t.Context(), f.db, f.user.ID, "aaaaa-aaaaa"), auth.ErrInvalidSecondFactor)

	var regenerated []string
	require.NoError(t, pgx.BeginFunc(t.Context(), f.db, func(tx pgx.Tx) error {
		var err error
		regenerated, err = f.factors.RegenerateRecoveryCodes(t.Context(), tx, audit.User(f.user.ID), f.user.ID)

		return err
	}))
	require.ErrorIs(t, f.factors.UseRecoveryCode(t.Context(), f.db, f.user.ID, codes[1]), auth.ErrInvalidSecondFactor)
	require.NoError(t, f.factors.UseRecoveryCode(t.Context(), f.db, f.user.ID, regenerated[1]))
}

// TestRecoveryCodesResistDatabaseLeak は、リカバリーコードを、DB だけでは総当たりで戻せない形で保存することを確かめる (#1011)。
// 50 ビットのコードは、鍵のないハッシュなら総当たりで戻せる。DB の外のアプリの秘密キーがなければ、照合できない。
func TestRecoveryCodesResistDatabaseLeak(t *testing.T) {
	t.Parallel()

	f := newSecondFactorFixture(t)
	_, codes := f.enroll(t)

	rows, err := f.db.Query(t.Context(), "SELECT code_hash FROM user_recovery_codes WHERE user_id = $1", f.user.ID)
	require.NoError(t, err)
	stored, err := pgx.CollectRows(rows, pgx.RowTo[[]byte])
	require.NoError(t, err)
	require.Len(t, stored, len(codes))
	for _, code := range codes {
		sum := sha256.Sum256([]byte(strings.ReplaceAll(code, "-", "")))
		assert.NotContains(t, stored, sum[:], "the stored hash must not be a plain SHA-256 of the code")
	}

	// アプリの秘密キーが違えば、同じコードでも照合できない。
	otherKey, err := auth.NewSecondFactors(f.clock, audit.NewRecorder(f.clock), appKeyMaterial+"-other")
	require.NoError(t, err)
	require.ErrorIs(t, otherKey.UseRecoveryCode(t.Context(), f.db, f.user.ID, codes[0]), auth.ErrInvalidSecondFactor)
	require.NoError(t, f.factors.UseRecoveryCode(t.Context(), f.db, f.user.ID, codes[0]))
}

// TestLegacyRecoveryCodes は、#1011 より前に作ったリカバリーコード (鍵のない SHA-256) も、作り直すまで 1 回だけ使えることを確かめる。
func TestLegacyRecoveryCodes(t *testing.T) {
	t.Parallel()

	f := newSecondFactorFixture(t)
	f.enroll(t)
	const legacy = "abcde-fghij"
	sum := sha256.Sum256([]byte("abcdefghij"))
	_, err := f.db.Exec(t.Context(), "INSERT INTO user_recovery_codes (id, user_id, code_hash, created_at) VALUES (gen_random_uuid(), $1, $2, now())", f.user.ID, sum[:])
	require.NoError(t, err)

	require.NoError(t, f.factors.UseRecoveryCode(t.Context(), f.db, f.user.ID, strings.ToUpper(legacy)))
	require.ErrorIs(t, f.factors.UseRecoveryCode(t.Context(), f.db, f.user.ID, legacy), auth.ErrInvalidSecondFactor)
	// ほかのユーザーとしては使えない。
	other, err := auth.NewUsers(f.clock).Create(t.Context(), f.db, &auth.NewUser{Username: nekoName, DisplayName: nekoName, Role: auth.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	_, err = f.db.Exec(t.Context(), "UPDATE user_recovery_codes SET used_at = NULL WHERE user_id = $1", f.user.ID)
	require.NoError(t, err)
	require.ErrorIs(t, f.factors.UseRecoveryCode(t.Context(), f.db, other.ID, legacy), auth.ErrInvalidSecondFactor)
}

// TestDeleteTOTP は、TOTP を消すと、予備のリカバリーコードも消え、第二要素がなくなることを確かめる。
func TestDeleteTOTP(t *testing.T) {
	t.Parallel()

	f := newSecondFactorFixture(t)
	_, codes := f.enroll(t)
	deleteTOTP := func() error {
		return pgx.BeginFunc(t.Context(), f.db, func(tx pgx.Tx) error {
			return f.factors.DeleteTOTP(t.Context(), tx, audit.User(f.user.ID), f.user.ID)
		})
	}

	version := func() int64 {
		v, err := query.New(f.db).UserSettingsVersion(t.Context(), f.user.ID)
		require.NoError(t, err)

		return v
	}
	// 登録と削除のたびに、ほかの端末に知らせるため、設定の版数を上げる。
	assert.Equal(t, int64(1), version())
	require.NoError(t, deleteTOTP())
	assert.Equal(t, int64(2), version())
	assert.Empty(t, f.methods(t))
	require.ErrorIs(t, f.factors.UseRecoveryCode(t.Context(), f.db, f.user.ID, codes[0]), auth.ErrInvalidSecondFactor)
	require.ErrorIs(t, deleteTOTP(), auth.ErrNoTOTP)
	err := pgx.BeginFunc(t.Context(), f.db, func(tx pgx.Tx) error {
		_, err := f.factors.RegenerateRecoveryCodes(t.Context(), tx, audit.User(f.user.ID), f.user.ID)

		return err
	})
	require.ErrorIs(t, err, auth.ErrNoTOTP)
	assert.Equal(t, int64(2), version(), "failed changes do not bump the version")
}

// TestTOTPNeedsTheSameApplicationSecret は、アプリの秘密キーが変わると、保存した秘密キーを読めないことを確かめる。
func TestTOTPNeedsTheSameApplicationSecret(t *testing.T) {
	t.Parallel()

	f := newSecondFactorFixture(t)
	enrollment, _ := f.enroll(t)
	other, err := auth.NewSecondFactors(f.clock, audit.NewRecorder(f.clock), "another-application-secret-key-0123456789")
	require.NoError(t, err)

	f.clock.Advance(totp.Period)
	err = pgx.BeginFunc(t.Context(), f.db, func(tx pgx.Tx) error {
		return other.VerifyTOTP(t.Context(), tx, f.user.ID, f.code(t, enrollment))
	})
	require.ErrorIs(t, err, secretbox.ErrDecrypt)

	_, err = auth.NewSecondFactors(f.clock, audit.NewRecorder(f.clock), "")
	require.ErrorIs(t, err, secretbox.ErrNoKey)
}
