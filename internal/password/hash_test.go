package password_test

import (
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/argon2"

	"github.com/SlashNephy/kotatsu/internal/password"
)

const strongPhrase = "mikan-kotatsu-neko-2026!"

func TestHashAndVerify(t *testing.T) {
	t.Parallel()

	encoded, err := password.Hash(strongPhrase)
	require.NoError(t, err)
	assert.Regexp(t, `^\$argon2id\$v=19\$m=19456,t=2,p=1\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`, encoded)

	// 同じパスワードでも、塩が違うので別のハッシュになる。
	again, err := password.Hash(strongPhrase)
	require.NoError(t, err)
	assert.NotEqual(t, encoded, again)

	got, err := password.Verify(strongPhrase, encoded)
	require.NoError(t, err)
	assert.Equal(t, &password.Verification{OK: true, NeedsRehash: false}, got)

	got, err = password.Verify("mikan-kotatsu-neko-2025!", encoded)
	require.NoError(t, err)
	assert.False(t, got.OK)

	require.NoError(t, password.VerifyDummy(strongPhrase))
}

// TestVerifyOlderParameters は、古いパラメーターのハッシュも検証でき、作り直しを求めることを確かめる。
func TestVerifyOlderParameters(t *testing.T) {
	t.Parallel()

	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte(strongPhrase), salt, 1, 8*1024, 1, 32)
	encoded := fmt.Sprintf("$argon2id$v=19$m=8192,t=1,p=1$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))

	got, err := password.Verify(strongPhrase, encoded)
	require.NoError(t, err)
	assert.Equal(t, &password.Verification{OK: true, NeedsRehash: true}, got)
}

func TestVerifyRejectsInvalidHashes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		encoded string
	}{
		{name: "empty", encoded: ""},
		{name: "bcrypt", encoded: "$2a$10$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyz012345"},
		{name: "other version", encoded: "$argon2id$v=16$m=19456,t=2,p=1$c2FsdA$a2V5"},
		{name: "zero memory", encoded: "$argon2id$v=19$m=0,t=2,p=1$c2FsdA$a2V5"},
		{name: "broken salt", encoded: "$argon2id$v=19$m=19456,t=2,p=1$!!!$a2V5"},
		{name: "empty key", encoded: "$argon2id$v=19$m=19456,t=2,p=1$MDEyMzQ1Njc4OWFiY2RlZg$"},
		{name: "short salt", encoded: "$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$a2V5"},
		{name: "prefix before the identifier", encoded: "x$argon2id$v=19$m=19456,t=2,p=1$MDEyMzQ1Njc4OWFiY2RlZg$a2V5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := password.Verify(strongPhrase, tt.encoded)
			require.ErrorIs(t, err, password.ErrInvalidHash)
		})
	}
}
