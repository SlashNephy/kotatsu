// Package innertoken は、ロールの間のリクエストを認可するトークンを、アプリの秘密キーから導く。
//
// ロールの間の HTTP (tenban から kyusu への中継、ほかのロールから nabe へのストリームの要求) は、
// インスタンスのサーバーに載り、tenban の接続元の制限やセッションを通らない。そこで、アプリの秘密キーを知る
// インスタンスからのリクエストだけを受け付ける。用途ごとに別のトークンを導き、1 つの用途のトークンを、ほかの用途に使えないようにする。
package innertoken

import (
	"crypto/hkdf"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// bearerPrefix は、Authorization ヘッダーのトークンの前置き。
const bearerPrefix = "Bearer "

// ErrNoKey は、アプリの秘密キーがないことを表す。
var ErrNoKey = errors.New("the secret key is not configured")

// Token は、ロールの間のリクエストを認可するトークン。
type Token struct {
	value string
}

// New は、アプリの秘密キー secret から、用途 purpose のトークンを導く。
func New(secret, purpose string) (*Token, error) {
	if secret == "" {
		return nil, ErrNoKey
	}
	key, err := hkdf.Key(sha256.New, []byte(secret), nil, purpose, sha256.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to derive the token for %s: %w", purpose, err)
	}

	return &Token{value: hex.EncodeToString(key)}, nil
}

// Authorize は、リクエストにトークンを付ける。
func (t *Token) Authorize(r *http.Request) {
	r.Header.Set("Authorization", bearerPrefix+t.value)
}

// Middleware は、トークンを付けたリクエストだけ next に渡し、それ以外には 401 を返すミドルウェアを返す。
func (t *Token) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), bearerPrefix)
		if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(t.value)) != 1 {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

			return
		}
		next.ServeHTTP(w, r)
	})
}
