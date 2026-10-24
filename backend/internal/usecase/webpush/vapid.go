package webpush

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// tokenLifetime は、VAPID の JWT の有効期間。RFC 8292 は 24 時間以内を求める。
const tokenLifetime = 12 * time.Hour

// Keys は、Web Push の送り手を示す VAPID の鍵 (RFC 8292)。
type Keys struct {
	private *ecdsa.PrivateKey
	// public は、非圧縮の 65 バイトの公開鍵。
	public []byte
}

// PublicKey は、ブラウザが購読するときに applicationServerKey に渡す、非圧縮の公開鍵を返す。
func (k *Keys) PublicKey() []byte {
	return k.public
}

func generateKeys() (*Keys, error) {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate a VAPID key: %w", err)
	}

	return keysOf(private)
}

func parseKeys(raw []byte) (*Keys, error) {
	private, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the VAPID key: %w", err)
	}

	return keysOf(private)
}

func keysOf(private *ecdsa.PrivateKey) (*Keys, error) {
	public, err := private.PublicKey.Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to encode the VAPID public key: %w", err)
	}

	return &Keys{private: private, public: public}, nil
}

// authorization は、プッシュサービスの endpoint に送るリクエストの Authorization ヘッダーの値を作る (RFC 8292 3)。
// JWT の aud は endpoint のオリジン、sub は送り手の連絡先 subject にする。
func (k *Keys) authorization(endpoint, subject string, now time.Time) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("failed to parse the endpoint: %w", err)
	}
	claims, err := json.Marshal(map[string]any{
		"aud": u.Scheme + "://" + u.Host,
		"exp": now.Add(tokenLifetime).Unix(),
		"sub": subject,
	})
	if err != nil {
		return "", fmt.Errorf("failed to encode the VAPID claims: %w", err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	unsigned := encode([]byte(`{"typ":"JWT","alg":"ES256"}`)) + "." + encode(claims)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, k.private, digest[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign the VAPID token: %w", err)
	}
	// ES256 の署名は、r と s をそれぞれ 32 バイトにして並べる (RFC 7518 3.4)。
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])

	return "vapid t=" + unsigned + "." + encode(signature) + ", k=" + encode(k.public), nil
}
