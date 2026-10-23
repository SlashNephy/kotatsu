// Package stream は、tenban が配信する映像 (/streams/*) を扱う (設計書 7.2、7.6、9.6)。
//
// ブラウザ内の再生は、同じオリジンへのリクエストなので、セッションの Cookie で認証する。
// VLC や mpv のような外部プレイヤーには、リソース、ユーザー、有効期限に HMAC-SHA256 で署名した短命の URL を渡す。
package stream

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	// signPurpose は、アプリの秘密キーから、URL の署名のキーを導くときの用途。用途ごとに別のキーを使う。
	signPurpose = "kotatsu/stream-url/v1"
	// DefaultURLLifetime は、署名付き URL の有効期限の既定値 (設計書 9.6)。
	DefaultURLLifetime = 6 * time.Hour

	// 署名付き URL のクエリのキー。
	userParam    = "user"
	expiresParam = "expires"
	sigParam     = "sig"
)

var (
	// ErrNoKey は、アプリの秘密キーがないことを表す。
	ErrNoKey = errors.New("the secret key is not configured")
	// ErrInvalidSignature は、署名がないか、正しくないか、期限が切れていることを表す。
	ErrInvalidSignature = errors.New("invalid or expired signature")
)

// ServicePath は、サービスのライブ視聴のパスを返す。
func ServicePath(serviceID int64) string {
	return "/streams/services/" + strconv.FormatInt(serviceID, 10)
}

// RecordingPath は、録画の再生のパスを返す。
func RecordingPath(recordingID uuid.UUID) string {
	return "/streams/recordings/" + recordingID.String()
}

// Signer は、映像の URL に署名し、署名を確かめる。
type Signer struct {
	key []byte
}

// NewSigner は、アプリの秘密キー secret から署名のキーを導き、Signer を返す。
func NewSigner(secret string) (*Signer, error) {
	if secret == "" {
		return nil, ErrNoKey
	}
	key, err := hkdf.Key(sha256.New, []byte(secret), nil, signPurpose, sha256.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to derive the signing key: %w", err)
	}

	return &Signer{key: key}, nil
}

// SignedURL は、署名付き URL と、その有効期限。
type SignedURL struct {
	// URL は、パスとクエリ。オリジンは含めないので、外部プレイヤーに渡すときは、呼び出し側が補う。
	URL       string
	ExpiresAt time.Time
}

// Sign は、resource (例: /streams/services/3273601024) を、userID が expiresAt まで使える URL にする。
// 録画の再生の開始位置 (start) のようなほかのクエリは署名の対象にしないので、同じ URL のまま位置を変えられる。
func (s *Signer) Sign(resource string, userID uuid.UUID, expiresAt time.Time) *SignedURL {
	expires := expiresAt.Unix()
	query := url.Values{}
	query.Set(userParam, userID.String())
	query.Set(expiresParam, strconv.FormatInt(expires, 10))
	query.Set(sigParam, s.signature(resource, userID, expires))

	return &SignedURL{URL: resource + "?" + query.Encode(), ExpiresAt: time.Unix(expires, 0).UTC()}
}

// Verify は、resource へのリクエストのクエリ query の署名を確かめ、URL を発行したユーザーを返す。
// 署名がないか、正しくないか、now の時点で期限が切れていれば、ErrInvalidSignature を返す。
func (s *Signer) Verify(resource string, query url.Values, now time.Time) (uuid.UUID, error) {
	userID, err := uuid.Parse(query.Get(userParam))
	if err != nil {
		return uuid.Nil, ErrInvalidSignature
	}
	expires, err := strconv.ParseInt(query.Get(expiresParam), 10, 64)
	if err != nil {
		return uuid.Nil, ErrInvalidSignature
	}
	want := s.signature(resource, userID, expires)
	if !hmac.Equal([]byte(want), []byte(query.Get(sigParam))) {
		return uuid.Nil, ErrInvalidSignature
	}
	if !now.Before(time.Unix(expires, 0)) {
		return uuid.Nil, ErrInvalidSignature
	}

	return userID, nil
}

// signature は、リソース、ユーザー、有効期限を改行でつないだ文字列の HMAC-SHA256 を返す。
// リソースのパスには改行を含められないので、区切りを取り違えることはない。
func (s *Signer) signature(resource string, userID uuid.UUID, expires int64) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(resource + "\n" + userID.String() + "\n" + strconv.FormatInt(expires, 10)))

	return hex.EncodeToString(mac.Sum(nil))
}
