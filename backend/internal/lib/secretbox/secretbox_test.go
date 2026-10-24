package secretbox_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/secretbox"
)

const (
	appKey  = "0123456789abcdef0123456789abcdef"
	purpose = "kotatsu/test/v1"
)

func TestSealAndOpen(t *testing.T) {
	t.Parallel()

	box, err := secretbox.New(appKey, purpose)
	require.NoError(t, err)
	sealed, err := box.Seal([]byte("totp secret"))
	require.NoError(t, err)
	again, err := box.Seal([]byte("totp secret"))
	require.NoError(t, err)
	assert.NotEqual(t, sealed, again, "the nonce must differ")

	opened, err := box.Open(sealed)
	require.NoError(t, err)
	assert.Equal(t, []byte("totp secret"), opened)
}

func TestOpenRejects(t *testing.T) {
	t.Parallel()

	box, err := secretbox.New(appKey, purpose)
	require.NoError(t, err)
	sealed, err := box.Seal([]byte("totp secret"))
	require.NoError(t, err)

	otherKey, err := secretbox.New("fedcba9876543210fedcba9876543210", purpose)
	require.NoError(t, err)
	otherPurpose, err := secretbox.New(appKey, "kotatsu/other/v1")
	require.NoError(t, err)
	tampered := append([]byte{}, sealed...)
	tampered[len(tampered)-1] ^= 1
	unknownVersion := append([]byte{2}, sealed[1:]...)

	tests := []struct {
		name       string
		box        *secretbox.Box
		ciphertext []byte
	}{
		{name: "another application key", box: otherKey, ciphertext: sealed},
		{name: "another purpose", box: otherPurpose, ciphertext: sealed},
		{name: "tampered", box: box, ciphertext: tampered},
		{name: "unknown version", box: box, ciphertext: unknownVersion},
		{name: "too short", box: box, ciphertext: sealed[:5]},
		{name: "empty", box: box, ciphertext: []byte{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.box.Open(tt.ciphertext)
			require.ErrorIs(t, err, secretbox.ErrDecrypt)
		})
	}
}

func TestNewRequiresKey(t *testing.T) {
	t.Parallel()

	_, err := secretbox.New("", purpose)
	require.ErrorIs(t, err, secretbox.ErrNoKey)
}
