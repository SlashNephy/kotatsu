// Package totp は、RFC 6238 の TOTP を扱う (設計書 9.3)。
//
// 認証アプリが既定で使う、HMAC-SHA1、30 秒、6 桁にする。
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 と認証アプリが HMAC-SHA1 を定めている。
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// Period は、コードが変わる間隔。
	Period = 30 * time.Second
	// Digits はコードの桁数。
	Digits = 6
	// secretLength は、秘密キーの長さ (RFC 4226 が勧める 160 ビット)。
	secretLength = 20
	// skew は、時計のずれを許すステップの数。前後 1 ステップ (30 秒) まで受け付ける。
	skew = 1
)

// NewSecret は、ランダムな秘密キーを作る。
func NewSecret() ([]byte, error) {
	secret := make([]byte, secretLength)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("failed to generate a TOTP secret: %w", err)
	}

	return secret, nil
}

// Encode は、認証アプリに手で入力する形 (パディングのない Base32) にする。
func Encode(secret []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
}

// URI は、認証アプリが QR コードから読む otpauth URI を返す。
func URI(issuer, account string, secret []byte) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	query := url.Values{}
	query.Set("secret", Encode(secret))
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", strconv.Itoa(Digits))
	query.Set("period", strconv.Itoa(int(Period.Seconds())))

	return "otpauth://totp/" + label + "?" + query.Encode()
}

// Step は、時刻 t のステップ (Unix 時間を Period で割ったもの) を返す。
func Step(t time.Time) int64 {
	return t.Unix() / int64(Period.Seconds())
}

// Code は、ステップ step のコードを digits 桁で返す (RFC 4226 の HOTP)。
func Code(secret []byte, step int64, digits int) (string, error) {
	// RFC 6238 と認証アプリは HMAC-SHA1 を使う。
	if step < 0 {
		return "", fmt.Errorf("the step must not be negative: %d", step)
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, secret)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	modulo := uint32(1)
	for range digits {
		modulo *= 10
	}

	return fmt.Sprintf("%0*d", digits, value%modulo), nil
}

// Verify は、code が now のコードか、前後 skew ステップのコードかを確かめ、一致したステップを返す。
// 一度使ったコードを使い回せないよう、lastUsedStep 以下のステップのコードは受け付けない。
// 一致しなければ、ok を false にする。
func Verify(secret []byte, code string, now time.Time, lastUsedStep int64) (int64, bool, error) {
	code = strings.ReplaceAll(code, " ", "")
	if len(code) != Digits {
		return 0, false, nil
	}
	current := Step(now)
	for step := current - skew; step <= current+skew; step++ {
		if step <= lastUsedStep {
			continue
		}
		want, err := Code(secret, step, Digits)
		if err != nil {
			return 0, false, err
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true, nil
		}
	}

	return 0, false, nil
}
