package webpush

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	// recordSize は、暗号化した本文のレコードの大きさ (RFC 8188)。本文は 1 つのレコードに収める。
	recordSize = 4096
	// saltLength は、本文ごとに選ぶソルトの長さ。
	saltLength = 16
	// authSecretLength は、ブラウザが購読ごとに作る、認証用の秘密の長さ (RFC 8291)。
	authSecretLength = 16
	// publicKeyLength は、P-256 の非圧縮の公開鍵の長さ。
	publicKeyLength = 65
	// headerLength は、暗号化した本文の先頭に付ける、ソルト、レコードの大きさ、送り手の公開鍵の長さ。
	headerLength = saltLength + 4 + 1 + publicKeyLength
	// paddingDelimiter は、最後のレコードの平文の終わりに付ける区切り (RFC 8188)。
	paddingDelimiter = 0x02
	// tagLength は、AES-GCM の認証タグの長さ。
	tagLength = 16
	// MaxPayloadLength は、暗号化する平文の長さの上限。プッシュサービスは 4096 バイトまでの本文を受け付ける。
	MaxPayloadLength = recordSize - headerLength - 1 - tagLength
)

// ErrPayloadTooLarge は、平文が長すぎて、1 つのレコードに収まらないことを表す。
var ErrPayloadTooLarge = errors.New("the web push payload is too large")

// Encrypt は、平文 plaintext を、購読したブラウザの公開鍵 uaPublic と認証用の秘密 authSecret で、
// RFC 8291 の aes128gcm の形式に暗号化する。送り手の鍵とソルトは、本文ごとに新しく作る。
func Encrypt(plaintext, uaPublic, authSecret []byte) ([]byte, error) {
	asPrivate, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate an ephemeral key: %w", err)
	}
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate a salt: %w", err)
	}

	return encrypt(plaintext, uaPublic, authSecret, asPrivate, salt)
}

// encrypt は、送り手の鍵 asPrivate とソルト salt を受け取って暗号化する。RFC 8291 の付録 A の値で確かめられるよう、分けている。
func encrypt(plaintext, uaPublic, authSecret []byte, asPrivate *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	if len(plaintext) > MaxPayloadLength {
		return nil, fmt.Errorf("%w: %d bytes", ErrPayloadTooLarge, len(plaintext))
	}
	if len(authSecret) != authSecretLength {
		return nil, fmt.Errorf("%w: the auth secret must be %d bytes", ErrInvalidSubscription, authSecretLength)
	}
	ua, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSubscription, err)
	}
	ecdhSecret, err := asPrivate.ECDH(ua)
	if err != nil {
		return nil, fmt.Errorf("failed to compute the shared secret: %w", err)
	}
	asPublic := asPrivate.PublicKey().Bytes()

	// 共有の秘密と認証用の秘密を組み合わせて、鍵の材料 (IKM) を作る (RFC 8291 3.4)。
	keyInfo := make([]byte, 0, len("WebPush: info\x00")+2*publicKeyLength)
	keyInfo = append(keyInfo, "WebPush: info\x00"...)
	keyInfo = append(keyInfo, uaPublic...)
	keyInfo = append(keyInfo, asPublic...)
	ikm, err := hkdf.Key(sha256.New, ecdhSecret, authSecret, string(keyInfo), sha256.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to derive the input keying material: %w", err)
	}
	// 本文の鍵と nonce を作る (RFC 8188 2.2、2.3)。
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, fmt.Errorf("failed to derive the content encryption key: %w", err)
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, fmt.Errorf("failed to derive the nonce: %w", err)
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, fmt.Errorf("failed to create a cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create a cipher: %w", err)
	}

	body := make([]byte, 0, headerLength+len(plaintext)+1+tagLength)
	body = append(body, salt...)
	body = binary.BigEndian.AppendUint32(body, recordSize)
	body = append(body, publicKeyLength)
	body = append(body, asPublic...)
	record := make([]byte, 0, len(plaintext)+1)
	record = append(record, plaintext...)
	record = append(record, paddingDelimiter)

	return gcm.Seal(body, nonce, record, nil), nil
}
