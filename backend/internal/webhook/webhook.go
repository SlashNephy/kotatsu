// Package webhook は、管理者が設定する Webhook を扱う (設計書 6.2)。
//
// Webhook は、通知するイベントを、外部のサービスに送る。形式は、汎用の JSON、Discord、Slack の 3 つで、
// どの形式でも、受信側が送信元を確かめられるよう、ペイロードに HMAC の署名を付ける。
// 署名用の秘密キーは、アプリの秘密キーで暗号化して保存する (設計書 8.4)。
package webhook

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/secretbox"
	"github.com/SlashNephy/kotatsu/internal/notification"
)

const (
	// SealPurpose は、署名用の秘密キーを暗号化する鍵の用途 (secretbox の purpose)。
	SealPurpose = "kotatsu/webhook/v1"
	// secretPrefix は、署名用の秘密キーの先頭に付ける文字列。ログや設定で見分けやすくする。
	secretPrefix = "whsec_"
	// secretBytes は、署名用の秘密キーの乱数のバイト数。
	secretBytes = 32
	// maxNameLength と maxURLLength は、名前と URL の長さの上限。
	maxNameLength = 100
	maxURLLength  = 2048
	// auditTarget は、Webhook の監査ログの対象の種類。
	auditTarget = "webhook"
)

var (
	// ErrInvalidWebhook は、Webhook の設定が正しくないことを表す。
	ErrInvalidWebhook = errors.New("invalid webhook")
	// ErrNotFound は、Webhook が見つからないことを表す。
	ErrNotFound = errors.New("webhook not found")
)

// Format は、Webhook で送る形式。
type Format string

const (
	// FormatJSON は、汎用の JSON。イベントの種類と、画面に出す文の材料をそのまま送る。
	FormatJSON Format = "json"
	// FormatDiscord は、Discord の Webhook の形式。
	FormatDiscord Format = "discord"
	// FormatSlack は、Slack の Incoming Webhook の形式。
	FormatSlack Format = "slack"
)

// Formats は、Webhook で送る形式を返す。
func Formats() []Format {
	return []Format{FormatJSON, FormatDiscord, FormatSlack}
}

// Webhook は、管理者が設定した Webhook。署名用の秘密キーは含めない。
type Webhook struct {
	ID        uuid.UUID
	Name      string
	URL       string
	Format    Format
	Kinds     []notification.Kind
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Input は、Webhook を作るときの設定。
type Input struct {
	Name    string
	URL     string
	Format  Format
	Kinds   []notification.Kind
	Enabled bool
}

// Changes は、Webhook の変える項目。nil の項目は変えない。
type Changes struct {
	Name    *string
	URL     *string
	Format  *Format
	Kinds   []notification.Kind
	Enabled *bool
}

// Database は、Webhook を書くのに使う DB。監査ログと一緒に書くため、トランザクションを始められる。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Store は、Webhook の設定を読み書きする。
type Store struct {
	clock    clock.Clock
	recorder audit.Recorder
	box      *secretbox.Box
}

// NewStore は Store を返す。appSecret は、署名用の秘密キーを暗号化するアプリの秘密キー。
func NewStore(clk clock.Clock, recorder audit.Recorder, appSecret string) (*Store, error) {
	box, err := secretbox.New(appSecret, SealPurpose)
	if err != nil {
		return nil, err
	}

	return &Store{clock: clk, recorder: recorder, box: box}, nil
}

// Create は、Webhook を作り、作った Webhook と、署名用の秘密キーを返す。
// 管理者に秘密キーを示すのは、このときだけにする。後から復号する Secret は、配信のジョブが署名に使う。
func (s *Store) Create(ctx context.Context, db Database, actor *audit.Actor, input *Input) (*Webhook, string, error) {
	normalized, err := normalize(input)
	if err != nil {
		return nil, "", err
	}
	secret, err := newSecret()
	if err != nil {
		return nil, "", err
	}
	sealed, err := s.box.Seal([]byte(secret))
	if err != nil {
		return nil, "", fmt.Errorf("failed to encrypt the webhook secret: %w", err)
	}

	var created *Webhook
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, err := sqlc.New(tx).InsertWebhook(ctx, sqlc.InsertWebhookParams{
			ID: id.New(), Name: normalized.Name, URL: normalized.URL, Format: string(normalized.Format),
			Kinds: kindStrings(normalized.Kinds), SecretCiphertext: sealed, Enabled: normalized.Enabled, Now: s.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to save the webhook: %w", err)
		}
		created = webhookOf(&row)

		return s.record(ctx, tx, actor, "webhook.created", created.ID, map[string]any{"to": summaryOf(created)})
	})
	if err != nil {
		return nil, "", err
	}

	return created, secret, nil
}

// List は、Webhook を作った順に返す。
func (s *Store) List(ctx context.Context, db sqlc.DBTX) ([]*Webhook, error) {
	rows, err := sqlc.New(db).ListWebhooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list webhooks: %w", err)
	}
	webhooks := make([]*Webhook, 0, len(rows))
	for i := range rows {
		webhooks = append(webhooks, webhookOf(&rows[i]))
	}

	return webhooks, nil
}

// Update は、Webhook の changes の項目を変え、変えた後の Webhook を返す。
func (s *Store) Update(ctx context.Context, db Database, actor *audit.Actor, webhookID uuid.UUID, changes *Changes) (*Webhook, error) {
	if changes == nil {
		return nil, fmt.Errorf("%w: changes is nil", ErrInvalidWebhook)
	}

	var updated *Webhook
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		row, err := q.LockWebhook(ctx, webhookID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to lock the webhook: %w", err)
		}
		current := webhookOf(&row)
		normalized, err := normalize(apply(current, changes))
		if err != nil {
			return err
		}
		saved, err := q.UpdateWebhook(ctx, sqlc.UpdateWebhookParams{
			ID: webhookID, Name: normalized.Name, URL: normalized.URL, Format: string(normalized.Format),
			Kinds: kindStrings(normalized.Kinds), Enabled: normalized.Enabled, Now: s.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to update the webhook: %w", err)
		}
		updated = webhookOf(&saved)

		return s.record(ctx, tx, actor, "webhook.updated", webhookID, map[string]any{"from": summaryOf(current), "to": summaryOf(updated)})
	})
	if err != nil {
		return nil, err
	}

	return updated, nil
}

// Delete は、Webhook を消す。送っている途中の配信は、Webhook が見つからなくなるので取りやめる。
func (s *Store) Delete(ctx context.Context, db Database, actor *audit.Actor, webhookID uuid.UUID) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		row, err := q.LockWebhook(ctx, webhookID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to lock the webhook: %w", err)
		}
		if _, err := q.DeleteWebhook(ctx, webhookID); err != nil {
			return fmt.Errorf("failed to delete the webhook: %w", err)
		}

		return s.record(ctx, tx, actor, "webhook.deleted", webhookID, map[string]any{"from": summaryOf(webhookOf(&row))})
	})
}

// Secret は、Webhook の署名用の秘密キーを復号して返す。署名に使うためのもので、管理者に示すためのものではない。
func (s *Store) Secret(ctx context.Context, db sqlc.DBTX, webhookID uuid.UUID) (string, error) {
	row, err := sqlc.New(db).GetWebhook(ctx, webhookID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to get the webhook: %w", err)
	}

	return s.open(row.SecretCiphertext)
}

// open は、暗号化した署名用の秘密キーを復号する。
func (s *Store) open(sealed []byte) (string, error) {
	secret, err := s.box.Open(sealed)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt the webhook secret: %w", err)
	}

	return string(secret), nil
}

func (s *Store) record(ctx context.Context, tx pgx.Tx, actor *audit.Actor, action string, webhookID uuid.UUID, summary map[string]any) error {
	return s.recorder.Record(ctx, tx, &audit.Event{
		Actor:      actor,
		Action:     action,
		TargetType: auditTarget,
		TargetID:   webhookID.String(),
		Summary:    summary,
		// 送信元は、操作者 (actor) に持たせる。
		SourceIP:  netip.Addr{},
		UserAgent: "",
	})
}

// summaryOf は、監査ログに残す Webhook の設定。Discord や Slack の URL は、それ自体が秘密のトークンを含むので、ホストだけを残す。
func summaryOf(webhook *Webhook) map[string]any {
	host := ""
	if parsed, err := url.Parse(webhook.URL); err == nil {
		host = parsed.Host
	}

	return map[string]any{
		"name": webhook.Name, "host": host, "format": webhook.Format, "kinds": webhook.Kinds, "enabled": webhook.Enabled,
	}
}

// apply は、current に changes の nil でない項目を重ねた設定を返す。
func apply(current *Webhook, changes *Changes) *Input {
	input := &Input{Name: current.Name, URL: current.URL, Format: current.Format, Kinds: current.Kinds, Enabled: current.Enabled}
	if changes.Name != nil {
		input.Name = *changes.Name
	}
	if changes.URL != nil {
		input.URL = *changes.URL
	}
	if changes.Format != nil {
		input.Format = *changes.Format
	}
	if changes.Kinds != nil {
		input.Kinds = changes.Kinds
	}
	if changes.Enabled != nil {
		input.Enabled = *changes.Enabled
	}

	return input
}

// normalize は、設定を確かめ、名前と URL の前後の空白を落とし、種類の重なりを除いて並べた設定を返す。
func normalize(input *Input) (*Input, error) {
	if input == nil {
		return nil, fmt.Errorf("%w: input is nil", ErrInvalidWebhook)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLength {
		return nil, fmt.Errorf("%w: name must be 1 to %d characters", ErrInvalidWebhook, maxNameLength)
	}
	rawURL := strings.TrimSpace(input.URL)
	if err := validateURL(rawURL); err != nil {
		return nil, err
	}
	if !slices.Contains(Formats(), input.Format) {
		return nil, fmt.Errorf("%w: unknown format %q", ErrInvalidWebhook, input.Format)
	}
	if len(input.Kinds) == 0 {
		return nil, fmt.Errorf("%w: at least one kind is required", ErrInvalidWebhook)
	}
	kinds := make([]notification.Kind, 0, len(input.Kinds))
	for _, kind := range input.Kinds {
		if _, err := notification.ParseKind(string(kind)); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidWebhook, err)
		}
		if !slices.Contains(notification.WebhookKinds(), kind) {
			return nil, fmt.Errorf("%w: %q cannot be sent to webhooks", ErrInvalidWebhook, kind)
		}
		if !slices.Contains(kinds, kind) {
			kinds = append(kinds, kind)
		}
	}
	// 画面に並べる順にそろえる。
	slices.SortFunc(kinds, func(a, b notification.Kind) int {
		return slices.Index(notification.WebhookKinds(), a) - slices.Index(notification.WebhookKinds(), b)
	})

	return &Input{Name: name, URL: rawURL, Format: input.Format, Kinds: kinds, Enabled: input.Enabled}, nil
}

// validateURL は、送り先の URL が、ホストを持つ http か https の URL であることを確かめる。
// 家庭内のサーバーに送ることもあるので、プライベートアドレスは拒まない。設定できるのは管理者だけである。
func validateURL(rawURL string) error {
	if rawURL == "" || len(rawURL) > maxURLLength {
		return fmt.Errorf("%w: url must be 1 to %d bytes", ErrInvalidWebhook, maxURLLength)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("%w: url must be an absolute http or https URL", ErrInvalidWebhook)
	}

	return nil
}

// newSecret は、署名用の秘密キーを作る。
func newSecret() (string, error) {
	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate a webhook secret: %w", err)
	}

	return secretPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func webhookOf(row *sqlc.Webhook) *Webhook {
	kinds := make([]notification.Kind, 0, len(row.Kinds))
	for _, kind := range row.Kinds {
		kinds = append(kinds, notification.Kind(kind))
	}

	return &Webhook{
		ID: row.ID, Name: row.Name, URL: row.URL, Format: Format(row.Format), Kinds: kinds,
		Enabled: row.Enabled, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func kindStrings(kinds []notification.Kind) []string {
	result := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		result = append(result, string(kind))
	}

	return result
}
