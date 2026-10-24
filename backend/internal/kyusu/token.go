package kyusu

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

const (
	// derivePurpose は、アプリの秘密キーから、tenban が kyusu に送るトークンを導くときの用途。
	derivePurpose = "kotatsu/kyusu/v1"
	// bearerPrefix は、Authorization ヘッダーのトークンの前置き。
	bearerPrefix = "Bearer "
)

// ErrNoKey は、アプリの秘密キーがないことを表す。
var ErrNoKey = errors.New("the secret key is not configured")

// Token は、tenban が kyusu に送るリクエストを認可するトークン。
// kyusu の HTTP はインスタンスのサーバーに載り、tenban の接続元の制限やセッションを通らないので、
// アプリの秘密キーを知るインスタンスからのリクエストだけを受け付ける。
type Token struct {
	value string
}

// NewToken は、アプリの秘密キー secret からトークンを導く。
func NewToken(secret string) (*Token, error) {
	if secret == "" {
		return nil, ErrNoKey
	}
	key, err := hkdf.Key(sha256.New, []byte(secret), nil, derivePurpose, sha256.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to derive the kyusu token: %w", err)
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
