package webpush_test

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/secretbox"
	"github.com/SlashNephy/kotatsu/internal/webpush"
)

const appKeyMaterial = "kotatsu-web-push-test-application-key-0123456789"

type fixture struct {
	pool    *pgxpool.Pool
	clock   *clocktest.Fake
	service *webpush.Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	service, err := webpush.NewService(clk, appKeyMaterial)
	require.NoError(t, err)

	return &fixture{pool: pool, clock: clk, service: service}
}

func (f *fixture) user(t *testing.T, username string) uuid.UUID {
	t.Helper()

	userID := id.New()
	require.NoError(t, sqlc.New(f.pool).InsertUser(t.Context(), sqlc.InsertUserParams{
		ID: userID, Username: username, DisplayName: username, Role: "member", PasswordHash: nil, CreatedAt: f.clock.Now(),
	}))

	return userID
}

// browser は、購読するブラウザ。通知を復号する鍵を持つ。
type browser struct {
	private *ecdh.PrivateKey
	auth    []byte
}

func newBrowser(t *testing.T) *browser {
	t.Helper()

	private, err := ecdh.P256().GenerateKey(rand.Reader)
	require.NoError(t, err)
	auth := make([]byte, 16)
	_, err = rand.Read(auth)
	require.NoError(t, err)

	return &browser{private: private, auth: auth}
}

func (b *browser) subscription(endpoint string) *webpush.Subscription {
	return &webpush.Subscription{
		ID: uuid.Nil, Endpoint: endpoint, P256DH: b.private.PublicKey().Bytes(), Auth: b.auth, UserAgent: "Mozilla/5.0",
		CreatedAt: time.Time{}, UpdatedAt: time.Time{},
	}
}

// decrypt は、ブラウザの手順 (RFC 8291) で、受け取った本文を復号する。
func (b *browser) decrypt(t *testing.T, body []byte) []byte {
	t.Helper()

	require.Greater(t, len(body), 21)
	salt, recordSize, idLength := body[:16], binary.BigEndian.Uint32(body[16:20]), int(body[20])
	assert.Equal(t, uint32(4096), recordSize)
	asPublicBytes, ciphertext := body[21:21+idLength], body[21+idLength:]
	asPublic, err := ecdh.P256().NewPublicKey(asPublicBytes)
	require.NoError(t, err)
	ecdhSecret, err := b.private.ECDH(asPublic)
	require.NoError(t, err)
	keyInfo := "WebPush: info\x00" + string(b.private.PublicKey().Bytes()) + string(asPublicBytes)
	ikm, err := hkdf.Key(sha256.New, ecdhSecret, b.auth, keyInfo, 32)
	require.NoError(t, err)
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	require.NoError(t, err)
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	require.NoError(t, err)
	block, err := aes.NewCipher(cek)
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	record, err := gcm.Open(nil, nonce, ciphertext, nil)
	require.NoError(t, err)
	// 最後のレコードは、平文の後ろに区切りの 0x02 を付けている。
	require.NotEmpty(t, record)
	require.Equal(t, byte(0x02), record[len(record)-1])

	return record[:len(record)-1]
}

// vapidClaims は、プッシュサービスの手順 (RFC 8292) で、Authorization ヘッダーの JWT の署名を、k の公開鍵で確かめ、
// JWT の claim と、k の公開鍵を返す。
func vapidClaims(t *testing.T, authorization string) (map[string]any, []byte) {
	t.Helper()

	rest, ok := strings.CutPrefix(authorization, "vapid ")
	require.True(t, ok, authorization)
	params := map[string]string{}
	for part := range strings.SplitSeq(rest, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		require.True(t, ok, part)
		params[key] = value
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(params["k"])
	require.NoError(t, err)
	public, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), publicKey)
	require.NoError(t, err)

	parts := strings.Split(params["t"], ".")
	require.Len(t, parts, 3)
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	assert.JSONEq(t, `{"typ":"JWT","alg":"ES256"}`, string(header))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	require.NoError(t, err)
	require.Len(t, signature, 64)
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])
	assert.True(t, ecdsa.Verify(public, digest[:], r, s), "the VAPID signature must verify with k")

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	claims := map[string]any{}
	require.NoError(t, json.Unmarshal(payload, &claims))

	return claims, publicKey
}

// TestKeys は、VAPID の鍵を初めて使うときに作り、どのインスタンスでも同じ鍵を使い、秘密鍵をアプリの秘密キーで暗号化して保存することを確かめる。
func TestKeys(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	// 2 つのインスタンスが同時に作っても、同じ鍵になる。
	other, err := webpush.NewService(f.clock, appKeyMaterial)
	require.NoError(t, err)
	results := make(chan []byte, 2)
	for _, service := range []*webpush.Service{f.service, other} {
		go func() {
			keys, err := service.Keys(t.Context(), f.pool)
			assert.NoError(t, err)
			if keys == nil {
				results <- nil

				return
			}
			results <- keys.PublicKey()
		}()
	}
	first, second := <-results, <-results
	require.Len(t, first, 65)
	assert.Equal(t, byte(0x04), first[0], "the public key is an uncompressed point")
	assert.Equal(t, first, second)

	var count int
	var publicKey, sealed []byte
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) OVER (), public_key, private_key_ciphertext FROM vapid_keys").
		Scan(&count, &publicKey, &sealed))
	assert.Equal(t, 1, count)
	assert.Equal(t, first, publicKey)
	// 秘密鍵は、アプリの秘密キーから導出した VAPID 用の鍵で暗号化している。別の用途の鍵では開けない。
	box, err := secretbox.New(appKeyMaterial, webpush.SealPurpose)
	require.NoError(t, err)
	raw, err := box.Open(sealed)
	require.NoError(t, err)
	assert.Len(t, raw, 32)
	otherBox, err := secretbox.New(appKeyMaterial, "kotatsu/webhook/v1")
	require.NoError(t, err)
	_, err = otherBox.Open(sealed)
	require.ErrorIs(t, err, secretbox.ErrDecrypt)
}

// TestSubscribe は、購読を保存し、正しくない購読を拒み、同じ端末を別のユーザーが購読し直したら移すことを確かめる。
func TestSubscribe(t *testing.T) {
	t.Parallel()

	const endpoint = "https://push.example/send/abc"
	valid := newBrowser(t).subscription(endpoint)
	tests := []struct {
		name   string
		modify func(s *webpush.Subscription)
	}{
		{name: "http の送り先", modify: func(s *webpush.Subscription) { s.Endpoint = "http://push.example/send/abc" }},
		{name: "資格情報を含む送り先", modify: func(s *webpush.Subscription) { s.Endpoint = "https://user:pass@push.example/send" }},
		{name: "長すぎる送り先", modify: func(s *webpush.Subscription) { s.Endpoint = "https://push.example/" + strings.Repeat("a", 2048) }},
		{name: "曲線の上にない公開鍵", modify: func(s *webpush.Subscription) { s.P256DH = make([]byte, 65) }},
		{name: "短い認証用の秘密", modify: func(s *webpush.Subscription) { s.Auth = s.Auth[:8] }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			subscription := *valid
			subscription.P256DH = append([]byte{}, valid.P256DH...)
			tt.modify(&subscription)
			require.ErrorIs(t, f.service.Subscribe(t.Context(), f.pool, f.user(t, "mikan"), &subscription), webpush.ErrInvalidSubscription)
		})
	}

	t.Run("保存し、移し、消す", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		mikan, yuzu := f.user(t, "mikan"), f.user(t, "yuzu")
		require.NoError(t, f.service.Subscribe(t.Context(), f.pool, mikan, valid))
		got, err := f.service.List(t.Context(), f.pool, mikan)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, valid.P256DH, got[0].P256DH)
		assert.Equal(t, "Mozilla/5.0", got[0].UserAgent)

		// 長い User-Agent は、文字の途中で切らずに、512 文字にする。
		long := newBrowser(t).subscription("https://push.example/send/long")
		long.UserAgent = strings.Repeat("あ", 600)
		require.NoError(t, f.service.Subscribe(t.Context(), f.pool, mikan, long))
		got, err = f.service.List(t.Context(), f.pool, mikan)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, strings.Repeat("あ", 512), got[1].UserAgent)
		_, err = f.service.Unsubscribe(t.Context(), f.pool, mikan, long.Endpoint)
		require.NoError(t, err)

		// 同じ端末を別のユーザーが購読し直したら、そのユーザーの購読に移る。
		require.NoError(t, f.service.Subscribe(t.Context(), f.pool, yuzu, valid))
		for user, want := range map[uuid.UUID]int{mikan: 0, yuzu: 1} {
			got, err := f.service.List(t.Context(), f.pool, user)
			require.NoError(t, err)
			assert.Len(t, got, want)
		}
		// ほかのユーザーの購読は消せない。
		deleted, err := f.service.Unsubscribe(t.Context(), f.pool, mikan, endpoint)
		require.NoError(t, err)
		assert.False(t, deleted)
		deleted, err = f.service.Unsubscribe(t.Context(), f.pool, yuzu, endpoint)
		require.NoError(t, err)
		assert.True(t, deleted)
	})
}
