// Package stream は、tenban が配信する映像 (/internal/streams/*、署名付き URL は /api/streams/*) を扱う (設計書 7.2、7.6、9.6)。
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
	"uuid"
)

const (
	// signPurpose は、アプリの秘密キーから、URL の署名のキーを導くときの用途。用途ごとに別のキーを使う。
	// v2 から、URL ごとの ID (#1757) も署名する。v1 の URL は、署名が合わなくなるので使えない。
	signPurpose = "kotatsu/stream-url/v2"
	// LiveURLLifetime は、ライブ視聴の署名付き URL の有効期限 (設計書 9.6、#1756)。
	// ライブは長さが決まらないので、漏れたときに使える時間を短くするため、発行から一律で 1 時間にする。
	// 期限は開くときにだけ確かめるので、期限の前に開いたストリームは、期限を過ぎても流れ続ける。
	LiveURLLifetime = time.Hour
	// RecordingURLMargin は、録画の署名付き URL の有効期限に、録画の長さへ足す時間 (設計書 9.6、#1756)。
	// 見ている途中で一時停止やシークをして開き直しても切れないよう、録画の長さより少し長くする。
	RecordingURLMargin = time.Hour

	// 署名付き URL のクエリのキー。
	userParam    = "user"
	idParam      = "id"
	expiresParam = "expires"
	sigParam     = "sig"
)

var (
	// ErrNoKey は、アプリの秘密キーがないことを表す。
	ErrNoKey = errors.New("the secret key is not configured")
	// ErrInvalidSignature は、署名がないか、正しくないか、期限が切れていることを表す。
	ErrInvalidSignature = errors.New("invalid or expired signature")
)

// SignedPathPrefix は、署名付き URL で配信するパスの頭。前段の認証のプロキシから外す /api の下に置く (設計書 9.1、#1605)。
const SignedPathPrefix = "/api/streams/"

// RecordingURLLifetime は、長さ length の録画の署名付き URL の有効期限を返す (#1756)。
// 長さが負なら (番組の時刻が壊れているときなど)、0 とみなす。
func RecordingURLLifetime(length time.Duration) time.Duration {
	return max(length, 0) + RecordingURLMargin
}

// ServicePath は、サービスのライブ視聴の、署名付き URL のパスを返す。
func ServicePath(serviceID int64) string {
	return SignedPathPrefix + "services/" + strconv.FormatInt(serviceID, 10)
}

// RecordingPath は、録画の再生の、署名付き URL のパスを返す。
func RecordingPath(recordingID uuid.UUID) string {
	return SignedPathPrefix + "recordings/" + recordingID.String()
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

// Sign は、resource (例: /api/streams/services/3273601024) を、userID が expiresAt まで使える URL にする。
// urlID は、発行した URL の記録の ID (#1757)。署名に含めるので、記録を消せば、その URL だけを取り消せる。
// 録画の再生の開始位置 (start) のようなほかのクエリは署名の対象にしないので、同じ URL のまま位置を変えられる。
func (s *Signer) Sign(resource string, userID, urlID uuid.UUID, expiresAt time.Time) *SignedURL {
	expires := expiresAt.Unix()
	query := url.Values{}
	query.Set(userParam, userID.String())
	query.Set(idParam, urlID.String())
	query.Set(expiresParam, strconv.FormatInt(expires, 10))
	query.Set(sigParam, s.signature(resource, userID, urlID, expires))

	return &SignedURL{URL: resource + "?" + query.Encode(), ExpiresAt: time.Unix(expires, 0).UTC()}
}

// Verified は、署名を確かめた URL の、発行したユーザーと、URL の記録の ID。
type Verified struct {
	UserID uuid.UUID
	URLID  uuid.UUID
}

// Verify は、resource へのリクエストのクエリ query の署名を確かめ、URL を発行したユーザーと、URL の記録の ID を返す。
// 署名がないか、正しくないか、now の時点で期限が切れていれば、ErrInvalidSignature を返す。
// 記録が残っているか (取り消していないか) は確かめないので、呼び出し側が確かめる。
func (s *Signer) Verify(resource string, query url.Values, now time.Time) (*Verified, error) {
	userID, err := uuid.Parse(query.Get(userParam))
	if err != nil {
		return nil, ErrInvalidSignature
	}
	urlID, err := uuid.Parse(query.Get(idParam))
	if err != nil {
		return nil, ErrInvalidSignature
	}
	expires, err := strconv.ParseInt(query.Get(expiresParam), 10, 64)
	if err != nil {
		return nil, ErrInvalidSignature
	}
	want := s.signature(resource, userID, urlID, expires)
	if !hmac.Equal([]byte(want), []byte(query.Get(sigParam))) {
		return nil, ErrInvalidSignature
	}
	if !now.Before(time.Unix(expires, 0)) {
		return nil, ErrInvalidSignature
	}

	return &Verified{UserID: userID, URLID: urlID}, nil
}

// signature は、リソース、ユーザー、URL の記録の ID、有効期限を改行でつないだ文字列の HMAC-SHA256 を返す。
// リソースのパスには改行を含められないので、区切りを取り違えることはない。
func (s *Signer) signature(resource string, userID, urlID uuid.UUID, expires int64) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(resource + "\n" + userID.String() + "\n" + urlID.String() + "\n" + strconv.FormatInt(expires, 10)))

	return hex.EncodeToString(mac.Sum(nil))
}
