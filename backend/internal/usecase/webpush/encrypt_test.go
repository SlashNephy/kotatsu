package webpush_test

import (
	"crypto/ecdh"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/webpush"
)

// decodeRFC は、RFC の本文の base64url の値を、空白を除いて読む。
func decodeRFC(t *testing.T, value string) []byte {
	t.Helper()

	decoded, err := base64.RawURLEncoding.DecodeString(strings.Join(strings.Fields(value), ""))
	require.NoError(t, err)

	return decoded
}

// TestEncryptRFC8291 は、RFC 8291 の付録 A の入力から、付録 A と同じ暗号文を作ることを確かめる。
func TestEncryptRFC8291(t *testing.T) {
	t.Parallel()

	asPrivate, err := ecdh.P256().NewPrivateKey(decodeRFC(t, "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	require.NoError(t, err)
	got, err := webpush.EncryptWith(
		decodeRFC(t, "V2hlbiBJIGdyb3cgdXAsIEkgd2FudCB0byBiZSBhIHdhdGVybWVsb24"),
		decodeRFC(t, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV- JvLexhqUzORcx aOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"),
		decodeRFC(t, "BTBZMqHH6r4Tts7J_aSIgg"),
		asPrivate,
		decodeRFC(t, "DGv6ra1nlYgDCS1FRnbzlw"),
	)
	require.NoError(t, err)

	header := decodeRFC(t, `DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z 9KsN6nGRTbVYI_c7VJSPQTBtkgcy27ml
		mlMoZIIgDll6e3vCYLocInmYWAmS6Tlz AC8wEqKK6PBru3jl7A8`)
	ciphertext := decodeRFC(t, `8pfeW0KbunFT06SuDKoJH9Ql87S1QUrd irN6GcG7sFz1y1sqLgVi1VhjVkHsUoEs
		bI_0LpXMuGvnzQ`)
	assert.Equal(t, append(header, ciphertext...), got)
}

// TestEncryptRejects は、長すぎる平文と、正しくない購読の鍵を拒むことを確かめる。
func TestEncryptRejects(t *testing.T) {
	t.Parallel()

	ua, err := ecdh.P256().GenerateKey(nil)
	require.NoError(t, err)
	auth := make([]byte, 16)
	tests := []struct {
		name       string
		plaintext  []byte
		uaPublic   []byte
		authSecret []byte
		wantErr    error
	}{
		{name: "長すぎる平文", plaintext: make([]byte, webpush.MaxPayloadLength+1), uaPublic: ua.PublicKey().Bytes(), authSecret: auth, wantErr: webpush.ErrPayloadTooLarge},
		{name: "曲線の上にない公開鍵", plaintext: []byte("x"), uaPublic: make([]byte, 65), authSecret: auth, wantErr: webpush.ErrInvalidSubscription},
		{name: "短い認証用の秘密", plaintext: []byte("x"), uaPublic: ua.PublicKey().Bytes(), authSecret: auth[:8], wantErr: webpush.ErrInvalidSubscription},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := webpush.Encrypt(tt.plaintext, tt.uaPublic, tt.authSecret)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}

	// 上限ちょうどの平文は、1 つのレコードに収まり、プッシュサービスが受け付ける 4096 バイトになる。
	body, err := webpush.Encrypt(make([]byte, webpush.MaxPayloadLength), ua.PublicKey().Bytes(), auth)
	require.NoError(t, err)
	assert.Len(t, body, 4096)
}
