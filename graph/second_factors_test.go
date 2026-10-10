package graph_test

import (
	"encoding/base32"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/totp"
)

const (
	secondFactorsQuery = `{ secondFactors { totp remainingRecoveryCodes } }`
	beginEnrollment    = `mutation { beginTotpEnrollment { secret uri expiresAt } }`
	confirmEnrollment  = `mutation($code: String!) { confirmTotpEnrollment(code: $code) { codes } }`
	deleteTOTP         = `mutation { deleteTotp { totp remainingRecoveryCodes } }`
	regenerateCodes    = `mutation { regenerateRecoveryCodes { codes } }`
	codeReauthRequired = "REAUTHENTICATION_REQUIRED"
	varCode            = "code"
)

type secondFactorStatus struct {
	TOTP                   bool `json:"totp"`
	RemainingRecoveryCodes int  `json:"remainingRecoveryCodes"`
}

func (f *fixture) status(t *testing.T) secondFactorStatus {
	t.Helper()

	var got struct {
		SecondFactors secondFactorStatus `json:"secondFactors"`
	}
	require.Empty(t, f.query(t, secondFactorsQuery, nil, &got))

	return got.SecondFactors
}

// beginTOTP は、TOTP の登録を始め、秘密キーを返す。
func (f *fixture) beginTOTP(t *testing.T) []byte {
	t.Helper()

	var got struct {
		BeginTotpEnrollment struct {
			Secret string `json:"secret"`
			URI    string `json:"uri"`
		} `json:"beginTotpEnrollment"`
	}
	require.Empty(t, f.query(t, beginEnrollment, nil, &got))
	assert.Contains(t, got.BeginTotpEnrollment.URI, "otpauth://totp/Kotatsu:"+testUsername+"?")
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(got.BeginTotpEnrollment.Secret)
	require.NoError(t, err)

	return secret
}

func (f *fixture) totpCode(t *testing.T, secret []byte) string {
	t.Helper()

	code, err := totp.Code(secret, totp.Step(f.clock.Now()), totp.Digits)
	require.NoError(t, err)

	return code
}

// TestManageTOTP は、ログイン中のユーザーが、TOTP を登録し、リカバリーコードを作り直し、TOTP を消せることを確かめる。
func TestManageTOTP(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	assert.Equal(t, secondFactorStatus{TOTP: false, RemainingRecoveryCodes: 0}, f.status(t))

	secret := f.beginTOTP(t)
	assert.Equal(t, []string{codeBadUserInput}, f.query(t, confirmEnrollment, map[string]any{varCode: "000000"}, nil))

	var confirmed struct {
		ConfirmTotpEnrollment struct {
			Codes []string `json:"codes"`
		} `json:"confirmTotpEnrollment"`
	}
	require.Empty(t, f.query(t, confirmEnrollment, map[string]any{varCode: f.totpCode(t, secret)}, &confirmed))
	assert.Len(t, confirmed.ConfirmTotpEnrollment.Codes, auth.RecoveryCodeCount)
	assert.Equal(t, secondFactorStatus{TOTP: true, RemainingRecoveryCodes: auth.RecoveryCodeCount}, f.status(t))

	var regenerated struct {
		RegenerateRecoveryCodes struct {
			Codes []string `json:"codes"`
		} `json:"regenerateRecoveryCodes"`
	}
	require.Empty(t, f.query(t, regenerateCodes, nil, &regenerated))
	assert.NotEqual(t, confirmed.ConfirmTotpEnrollment.Codes, regenerated.RegenerateRecoveryCodes.Codes)

	var deleted struct {
		DeleteTotp secondFactorStatus `json:"deleteTotp"`
	}
	require.Empty(t, f.query(t, deleteTOTP, nil, &deleted))
	assert.Equal(t, secondFactorStatus{TOTP: false, RemainingRecoveryCodes: 0}, deleted.DeleteTotp)
	assert.Equal(t, []string{codeNotFound}, f.query(t, deleteTOTP, nil, nil))
	assert.Equal(t, []string{codeNotFound}, f.query(t, regenerateCodes, nil, nil))

	var audited int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_events
		WHERE action IN ('user.totp_enabled', 'user.recovery_codes_regenerated', 'user.totp_disabled') AND user_agent = $1`, testUserAgent).Scan(&audited))
	assert.Equal(t, 3, audited)
}

// TestManageTOTPRequiresRecentAuthentication は、第二要素を変えるには、直近 10 分以内の認証が要ることを確かめる (設計書 9.5)。
func TestManageTOTPRequiresRecentAuthentication(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	secret := f.beginTOTP(t)
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)

	for name, query := range map[string]string{
		"begin":      beginEnrollment,
		"confirm":    confirmEnrollment,
		"delete":     deleteTOTP,
		"regenerate": regenerateCodes,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []string{codeReauthRequired}, f.query(t, query, map[string]any{varCode: f.totpCode(t, secret)}, nil))
		})
	}
}

// TestViewerCanManageOwnTOTP は、視聴者も、自分の第二要素は扱えることを確かめる (設計書 9.7 の「自分の設定」)。
func TestViewerCanManageOwnTOTP(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil).as(t, "child", auth.RoleViewer)
	assert.Equal(t, secondFactorStatus{TOTP: false, RemainingRecoveryCodes: 0}, f.status(t))
	var got struct {
		BeginTotpEnrollment struct {
			Secret string `json:"secret"`
		} `json:"beginTotpEnrollment"`
	}
	require.Empty(t, f.query(t, beginEnrollment, nil, &got))
	assert.NotEmpty(t, got.BeginTotpEnrollment.Secret)
}
