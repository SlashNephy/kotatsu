package totp_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/totp"
)

// rfcSecret は、RFC 6238 の付録 B の SHA-1 の秘密キー。
const rfcSecret = "12345678901234567890"

// TestCodeMatchesRFC6238 は、RFC 6238 の付録 B の SHA-1 のテストベクター (8 桁) と一致することを確かめる。
func TestCodeMatchesRFC6238(t *testing.T) {
	t.Parallel()

	tests := []struct {
		unix int64
		want string
	}{
		{unix: 59, want: "94287082"},
		{unix: 1111111109, want: "07081804"},
		{unix: 1111111111, want: "14050471"},
		{unix: 1234567890, want: "89005924"},
		{unix: 2000000000, want: "69279037"},
		{unix: 20000000000, want: "65353130"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			got, err := totp.Code([]byte(rfcSecret), totp.Step(time.Unix(tt.unix, 0)), 8)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVerify(t *testing.T) {
	t.Parallel()

	now := time.Unix(1111111111, 0)
	step := totp.Step(now)
	code := func(s int64) string {
		c, err := totp.Code([]byte(rfcSecret), s, totp.Digits)
		require.NoError(t, err)

		return c
	}

	tests := []struct {
		name         string
		code         string
		lastUsedStep int64
		wantStep     int64
		wantOK       bool
	}{
		{name: "current step", code: code(step), wantStep: step, wantOK: true},
		{name: "previous step within the skew", code: code(step - 1), wantStep: step - 1, wantOK: true},
		{name: "next step within the skew", code: code(step + 1), wantStep: step + 1, wantOK: true},
		{name: "spaces are ignored", code: code(step)[:3] + " " + code(step)[3:], wantStep: step, wantOK: true},
		{name: "too old", code: code(step - 2)},
		{name: "already used", code: code(step), lastUsedStep: step},
		{name: "wrong code", code: "000000"},
		{name: "wrong length", code: "12345"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok, err := totp.Verify([]byte(rfcSecret), tt.code, now, tt.lastUsedStep)
			require.NoError(t, err)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantStep, got)
			}
		})
	}
}

func TestURI(t *testing.T) {
	t.Parallel()

	secret, err := totp.NewSecret()
	require.NoError(t, err)
	require.Len(t, secret, 20)

	u, err := url.Parse(totp.URI("Kotatsu", "こたつ", secret))
	require.NoError(t, err)
	assert.Equal(t, "otpauth", u.Scheme)
	assert.Equal(t, "totp", u.Host)
	assert.Equal(t, "/Kotatsu:こたつ", u.Path)
	assert.Equal(t, url.Values{
		"secret": {totp.Encode(secret)}, "issuer": {"Kotatsu"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"},
	}, u.Query())
}
