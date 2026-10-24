// Package secretbox は、アプリの秘密キーで、DB に保存する秘密情報を暗号化する (設計書 8.4)。
//
// 用途ごとに、アプリの秘密キーから HKDF-SHA256 で別の鍵を導出し、AES-256-GCM で暗号化する。
// 暗号文の先頭には形式の版を 1 バイト付け、後から方式を変えられるようにする。
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

// version1 は、AES-256-GCM で、版、nonce、暗号文の順に並べる形式。
const version1 byte = 1

// keyLength は AES-256 の鍵の長さ。
const keyLength = 32

var (
	// ErrDecrypt は、暗号文が壊れている、別の鍵で暗号化された、別の用途のもの、のどれかを表す。
	ErrDecrypt = errors.New("failed to decrypt the secret")
	// ErrNoKey は、アプリの秘密キーがないことを表す。
	ErrNoKey = errors.New("the application secret key is not configured")
)

// Box は、1 つの用途の秘密情報を暗号化する。
type Box struct {
	aead cipher.AEAD
	// purpose は、暗号文を別の用途で復号できないよう、追加の認証データに入れる。
	purpose []byte
}

// New は、アプリの秘密キー secret から purpose 用の鍵を導出して Box を返す。
// purpose には "kotatsu/totp/v1" のように、用途と版を表す文字列を渡す。
func New(secret, purpose string) (*Box, error) {
	if secret == "" {
		return nil, ErrNoKey
	}
	key, err := hkdf.Key(sha256.New, []byte(secret), nil, purpose, keyLength)
	if err != nil {
		return nil, fmt.Errorf("failed to derive a key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create a cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create a cipher: %w", err)
	}

	return &Box{aead: aead, purpose: []byte(purpose)}, nil
}

// Seal は plaintext を暗号化する。同じ平文でも、nonce が毎回変わるので、別の暗号文になる。
func (b *Box) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate a nonce: %w", err)
	}
	out := make([]byte, 0, 1+len(nonce)+len(plaintext)+b.aead.Overhead())
	out = append(out, version1)
	out = append(out, nonce...)

	return b.aead.Seal(out, nonce, plaintext, b.purpose), nil
}

// Open は Seal で暗号化したものを復号する。
func (b *Box) Open(ciphertext []byte) ([]byte, error) {
	nonceSize := b.aead.NonceSize()
	if len(ciphertext) < 1+nonceSize || ciphertext[0] != version1 {
		return nil, ErrDecrypt
	}
	plaintext, err := b.aead.Open(nil, ciphertext[1:1+nonceSize], ciphertext[1+nonceSize:], b.purpose)
	if err != nil {
		return nil, ErrDecrypt
	}

	return plaintext, nil
}
