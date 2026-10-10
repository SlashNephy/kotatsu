// Package webpush は、PWA から購読した端末に、Web Push で通知を送る (設計書 6.2)。
//
// 本文は、購読したブラウザの鍵で暗号化し (RFC 8291)、送り手は VAPID の鍵で示す (RFC 8292)。
// VAPID の鍵は、初めて使うときに作り、秘密鍵はアプリの秘密キーで暗号化して保存する (設計書 8.4)。
package webpush

import (
	"context"
	"crypto/ecdh"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/secretbox"
)

const (
	// SealPurpose は、VAPID の秘密鍵を暗号化する鍵の用途。
	SealPurpose = "kotatsu/vapid/v1"
	// maxEndpointLength は、購読の送り先の URL の長さの上限。
	maxEndpointLength = 2048
	// maxUserAgentLength は、購読した端末の User-Agent を保存する文字数の上限。
	maxUserAgentLength = 512
)

// ErrInvalidSubscription は、購読の送り先か鍵が正しくないことを表す。
var ErrInvalidSubscription = errors.New("invalid web push subscription")

// Subscription は、ブラウザの PushSubscription。
type Subscription struct {
	ID       uuid.UUID
	Endpoint string
	// P256DH は、ブラウザの P-256 の公開鍵 (非圧縮の 65 バイト)。
	P256DH []byte
	// Auth は、認証用の秘密 (16 バイト)。
	Auth      []byte
	UserAgent string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Service は、VAPID の鍵と、ユーザーの購読を扱う。
type Service struct {
	clock clock.Clock
	box   *secretbox.Box

	mu   sync.Mutex
	keys *Keys
}

// NewService は Service を返す。appSecret は、VAPID の秘密鍵を暗号化するアプリの秘密キー。
func NewService(clk clock.Clock, appSecret string) (*Service, error) {
	box, err := secretbox.New(appSecret, SealPurpose)
	if err != nil {
		return nil, err
	}

	return &Service{clock: clk, box: box, mu: sync.Mutex{}, keys: nil}, nil
}

// Keys は、VAPID の鍵を返す。まだなければ作って保存する。
// 複数のインスタンスが同時に作っても、先に保存した鍵を皆が使う。鍵は変わらないので、一度読んだら覚えておく。
func (s *Service) Keys(ctx context.Context, db sqlc.DBTX) (*Keys, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keys != nil {
		return s.keys, nil
	}
	q := sqlc.New(db)
	row, err := q.GetVAPIDKeys(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := s.create(ctx, q); err != nil {
			return nil, err
		}
		row, err = q.GetVAPIDKeys(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the VAPID keys: %w", err)
	}
	raw, err := s.box.Open(row.PrivateKeyCiphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt the VAPID key: %w", err)
	}
	keys, err := parseKeys(raw)
	if err != nil {
		return nil, err
	}
	s.keys = keys

	return keys, nil
}

// Subscribe は、ユーザーの購読を保存する。同じ端末を別のユーザーが購読していたら、このユーザーの購読に移す。
// 送り先は、プッシュサービスの https の URL に限る。
func (s *Service) Subscribe(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, subscription *Subscription) error {
	if err := validate(subscription); err != nil {
		return err
	}
	// User-Agent は、端末を見分けられれば足りるので、文字の途中で切らないように、上限の文字数で切る。
	userAgent := strings.ToValidUTF8(subscription.UserAgent, "")
	if runes := []rune(userAgent); len(runes) > maxUserAgentLength {
		userAgent = string(runes[:maxUserAgentLength])
	}
	err := sqlc.New(db).UpsertWebPushSubscription(ctx, sqlc.UpsertWebPushSubscriptionParams{
		ID: id.New(), UserID: userID, Endpoint: subscription.Endpoint, P256DH: subscription.P256DH, Auth: subscription.Auth,
		UserAgent: userAgent, Now: s.clock.Now(),
	})
	if err != nil {
		return fmt.Errorf("failed to save the web push subscription: %w", err)
	}

	return nil
}

// Unsubscribe は、ユーザーの購読を消し、消したかどうかを返す。
func (s *Service) Unsubscribe(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, endpoint string) (bool, error) {
	n, err := sqlc.New(db).DeleteWebPushSubscription(ctx, sqlc.DeleteWebPushSubscriptionParams{UserID: userID, Endpoint: endpoint})
	if err != nil {
		return false, fmt.Errorf("failed to delete the web push subscription: %w", err)
	}

	return n > 0, nil
}

// List は、ユーザーの購読を、古い順に返す。
func (s *Service) List(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) ([]*Subscription, error) {
	rows, err := sqlc.New(db).ListWebPushSubscriptions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list web push subscriptions: %w", err)
	}
	subscriptions := make([]*Subscription, 0, len(rows))
	for i := range rows {
		subscriptions = append(subscriptions, subscriptionOf(&rows[i]))
	}

	return subscriptions, nil
}

func (s *Service) create(ctx context.Context, q *sqlc.Queries) error {
	keys, err := generateKeys()
	if err != nil {
		return err
	}
	raw, err := keys.private.Bytes()
	if err != nil {
		return fmt.Errorf("failed to encode the VAPID key: %w", err)
	}
	sealed, err := s.box.Seal(raw)
	if err != nil {
		return fmt.Errorf("failed to encrypt the VAPID key: %w", err)
	}
	err = q.InsertVAPIDKeys(ctx, sqlc.InsertVAPIDKeysParams{PublicKey: keys.public, PrivateKeyCiphertext: sealed, Now: s.clock.Now()})
	if err != nil {
		return fmt.Errorf("failed to save the VAPID keys: %w", err)
	}

	return nil
}

func subscriptionOf(row *sqlc.WebPushSubscription) *Subscription {
	return &Subscription{
		ID: row.ID, Endpoint: row.Endpoint, P256DH: row.P256DH, Auth: row.Auth, UserAgent: row.UserAgent,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func validate(subscription *Subscription) error {
	if len(subscription.Endpoint) > maxEndpointLength {
		return fmt.Errorf("%w: the endpoint is too long", ErrInvalidSubscription)
	}
	u, err := url.Parse(subscription.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("%w: the endpoint must be an https URL", ErrInvalidSubscription)
	}
	if _, err := ecdh.P256().NewPublicKey(subscription.P256DH); err != nil {
		return fmt.Errorf("%w: the p256dh key is not a P-256 public key", ErrInvalidSubscription)
	}
	if len(subscription.Auth) != authSecretLength {
		return fmt.Errorf("%w: the auth secret must be %d bytes", ErrInvalidSubscription, authSecretLength)
	}

	return nil
}
