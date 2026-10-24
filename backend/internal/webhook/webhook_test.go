package webhook_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/notification"
	"github.com/SlashNephy/kotatsu/internal/secretbox"
	"github.com/SlashNephy/kotatsu/internal/webhook"
)

const appKeyMaterial = "kotatsu-webhook-test-application-key-0123456789"

type fixture struct {
	pool  *pgxpool.Pool
	clock *clocktest.Fake
	store *webhook.Store
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	store, err := webhook.NewStore(clk, audit.NewRecorder(clk), appKeyMaterial)
	require.NoError(t, err)

	return &fixture{pool: pool, clock: clk, store: store}
}

func validInput() *webhook.Input {
	return &webhook.Input{
		Name: "家族の Discord", URL: "https://discord.example/api/webhooks/1/token", Format: webhook.FormatDiscord,
		Kinds: []notification.Kind{notification.KindRecordingFailed, notification.KindRecordingStarted}, Enabled: true,
	}
}

func (f *fixture) create(t *testing.T) (*webhook.Webhook, string) {
	t.Helper()

	created, secret, err := f.store.Create(t.Context(), f.pool, audit.System("test"), validInput())
	require.NoError(t, err)

	return created, secret
}

func (f *fixture) auditActions(t *testing.T, webhookID uuid.UUID) []string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT action FROM audit_events WHERE target_type = 'webhook' AND target_id = $1 ORDER BY occurred_at, id", webhookID.String())
	require.NoError(t, err)
	defer rows.Close()
	actions := []string{}
	for rows.Next() {
		var action string
		require.NoError(t, rows.Scan(&action))
		actions = append(actions, action)
	}
	require.NoError(t, rows.Err())

	return actions
}

// TestCreate は、Webhook を作ると、署名用の秘密キーを一度だけ返し、暗号化して保存することを確かめる (設計書 8.4)。
func TestCreate(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	created, secret := f.create(t)
	assert.Equal(t, &webhook.Webhook{
		ID: created.ID, Name: "家族の Discord", URL: "https://discord.example/api/webhooks/1/token", Format: webhook.FormatDiscord,
		// 種類は、画面に並べる順にそろえる。
		Kinds:   []notification.Kind{notification.KindRecordingStarted, notification.KindRecordingFailed},
		Enabled: true, CreatedAt: f.clock.Now(), UpdatedAt: f.clock.Now(),
	}, normalizeTimes(created))
	assert.True(t, strings.HasPrefix(secret, "whsec_"))

	// DB には暗号文だけを置き、アプリの秘密キーで復号すると、返した秘密キーに戻る。
	var sealed []byte
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT secret_ciphertext FROM webhooks WHERE id = $1", created.ID).Scan(&sealed))
	assert.NotContains(t, string(sealed), secret)
	box, err := secretbox.New(appKeyMaterial, webhook.SealPurpose)
	require.NoError(t, err)
	opened, err := box.Open(sealed)
	require.NoError(t, err)
	assert.Equal(t, secret, string(opened))
	got, err := f.store.Secret(t.Context(), f.pool, created.ID)
	require.NoError(t, err)
	assert.Equal(t, secret, got)

	// 作るたびに、別の秘密キーにする。
	_, another := f.create(t)
	assert.NotEqual(t, secret, another)

	// 監査ログには、秘密のトークンを含みうる URL のパスや、秘密キーを残さない。
	var summary string
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT summary::text FROM audit_events WHERE target_id = $1", created.ID.String()).Scan(&summary))
	assert.Contains(t, summary, "discord.example")
	assert.NotContains(t, summary, "token")
	assert.NotContains(t, summary, secret)
}

func normalizeTimes(w *webhook.Webhook) *webhook.Webhook {
	copied := *w
	copied.CreatedAt, copied.UpdatedAt = w.CreatedAt.UTC(), w.UpdatedAt.UTC()

	return &copied
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	tests := []struct {
		name   string
		modify func(*webhook.Input)
	}{
		{name: "空の名前", modify: func(in *webhook.Input) { in.Name = " " }},
		{name: "長すぎる名前", modify: func(in *webhook.Input) { in.Name = strings.Repeat("あ", 101) }},
		{name: "空の URL", modify: func(in *webhook.Input) { in.URL = "" }},
		{name: "相対 URL", modify: func(in *webhook.Input) { in.URL = "/hooks" }},
		{name: "http でも https でもない URL", modify: func(in *webhook.Input) { in.URL = "ftp://example.com/hooks" }},
		{name: "長すぎる URL", modify: func(in *webhook.Input) { in.URL = "https://example.com/" + strings.Repeat("a", 2048) }},
		{name: "知らない形式", modify: func(in *webhook.Input) { in.Format = "teams" }},
		{name: "種類がない", modify: func(in *webhook.Input) { in.Kinds = []notification.Kind{} }},
		{name: "知らない種類", modify: func(in *webhook.Input) { in.Kinds = []notification.Kind{"unknown"} }},
		// 視聴予約の通知は個人に届けるものなので、全体向けの Webhook では送らない (設計書 4.8)。
		{name: "視聴予約の通知", modify: func(in *webhook.Input) { in.Kinds = []notification.Kind{notification.KindWatchReminder} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := validInput()
			tt.modify(input)
			_, _, err := f.store.Create(t.Context(), f.pool, audit.System("test"), input)
			require.ErrorIs(t, err, webhook.ErrInvalidWebhook)
		})
	}
	webhooks, err := f.store.List(t.Context(), f.pool)
	require.NoError(t, err)
	assert.Empty(t, webhooks)
}

// TestUpdateAndDelete は、変えた項目だけを変え、消した Webhook は見つからなくなることを確かめる。
func TestUpdateAndDelete(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	created, secret := f.create(t)
	f.clock.Advance(time.Minute)

	updated, err := f.store.Update(t.Context(), f.pool, audit.System("test"), created.ID, &webhook.Changes{
		Name: nil, URL: new(" https://hooks.slack.example/services/T/B/X "), Format: new(webhook.FormatSlack),
		Kinds: []notification.Kind{notification.KindStorageLow, notification.KindStorageLow}, Enabled: new(false),
	})
	require.NoError(t, err)
	assert.Equal(t, &webhook.Webhook{
		ID: created.ID, Name: created.Name, URL: "https://hooks.slack.example/services/T/B/X", Format: webhook.FormatSlack,
		Kinds: []notification.Kind{notification.KindStorageLow}, Enabled: false, CreatedAt: created.CreatedAt.UTC(), UpdatedAt: f.clock.Now(),
	}, normalizeTimes(updated))
	// 設定を変えても、秘密キーは変えない。
	got, err := f.store.Secret(t.Context(), f.pool, created.ID)
	require.NoError(t, err)
	assert.Equal(t, secret, got)

	_, err = f.store.Update(t.Context(), f.pool, audit.System("test"), created.ID, &webhook.Changes{
		Name: nil, URL: new("not a url"), Format: nil, Kinds: nil, Enabled: nil,
	})
	require.ErrorIs(t, err, webhook.ErrInvalidWebhook)

	require.NoError(t, f.store.Delete(t.Context(), f.pool, audit.System("test"), created.ID))
	webhooks, err := f.store.List(t.Context(), f.pool)
	require.NoError(t, err)
	assert.Empty(t, webhooks)
	assert.Equal(t, []string{"webhook.created", "webhook.updated", "webhook.deleted"}, f.auditActions(t, created.ID))

	tests := []struct {
		name string
		call func() error
	}{
		{name: "変える", call: func() error {
			_, err := f.store.Update(t.Context(), f.pool, audit.System("test"), created.ID, &webhook.Changes{Name: new("x"), URL: nil, Format: nil, Kinds: nil, Enabled: nil})

			return err
		}},
		{name: "消す", call: func() error { return f.store.Delete(t.Context(), f.pool, audit.System("test"), created.ID) }},
		{name: "秘密キーを読む", call: func() error {
			_, err := f.store.Secret(t.Context(), f.pool, created.ID)

			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name+"と見つからない", func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, tt.call(), webhook.ErrNotFound)
		})
	}
}

func TestNewStoreRequiresAppSecret(t *testing.T) {
	t.Parallel()

	_, err := webhook.NewStore(clocktest.NewFake(time.Time{}), audit.NewRecorder(clocktest.NewFake(time.Time{})), "")
	require.ErrorIs(t, err, secretbox.ErrNoKey)
}
