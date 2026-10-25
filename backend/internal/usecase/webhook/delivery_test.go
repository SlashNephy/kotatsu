package webhook_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/webhook"
)

// received は、受信側が受け取ったリクエスト。
type received struct {
	header http.Header
	body   []byte
}

// receiver は、受け取ったリクエストを残し、status を返す受信側を立てる。
func receiver(t *testing.T, status int) (*httptest.Server, <-chan received) {
	t.Helper()

	requests := make(chan received, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		requests <- received{header: r.Header.Clone(), body: body}
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)

	return server, requests
}

func (f *fixture) createWebhook(t *testing.T, url string, format webhook.Format, kinds []domain.NotificationKind, enabled bool) (uuid.UUID, string) {
	t.Helper()

	created, secret, err := f.store.Create(t.Context(), f.pool, audit.System("test"), &webhook.Input{
		Name: string(format), URL: url, Format: format, Kinds: kinds, Enabled: enabled,
	})
	require.NoError(t, err)

	return created.ID, secret
}

// emit は、イベントを記録し、その ID を返す。
func (f *fixture) emit(t *testing.T, kind domain.NotificationKind, payload map[string]any) uuid.UUID {
	t.Helper()

	require.NoError(t, notification.Emit(t.Context(), f.pool, f.clock, kind, payload))
	var eventID uuid.UUID
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT id FROM notification_events ORDER BY occurred_at DESC, id DESC LIMIT 1").Scan(&eventID))

	return eventID
}

func (f *fixture) deliver(t *testing.T, webhookID, eventID uuid.UUID) error {
	t.Helper()

	worker := webhook.NewDeliverWorker(f.pool, f.store, f.clock, tokyo(t))
	job := &river.Job[webhook.DeliverArgs]{JobRow: new(rivertype.JobRow), Args: webhook.DeliverArgs{WebhookID: webhookID, EventID: eventID}}

	return worker.Work(t.Context(), job)
}

func tokyo(t *testing.T) *time.Location {
	t.Helper()

	location, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	return location
}

// verify は、受信側の手順で署名を確かめる。HMAC-SHA256 を、秘密キーで「<t>.<本文>」に対して計算し、v1 と比べる。
func verify(t *testing.T, secret string, request received) time.Time {
	t.Helper()

	parts := map[string]string{}
	for part := range strings.SplitSeq(request.header.Get("X-Kotatsu-Signature"), ",") {
		key, value, ok := strings.Cut(part, "=")
		require.True(t, ok, part)
		parts[key] = value
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts["t"] + "." + string(request.body)))
	want := hex.EncodeToString(mac.Sum(nil))
	assert.True(t, hmac.Equal([]byte(want), []byte(parts["v1"])), "the signature must match the body")
	unix, err := strconv.ParseInt(parts["t"], 10, 64)
	require.NoError(t, err)

	return time.Unix(unix, 0).UTC()
}

// TestEnqueuerQueuesDeliveries は、イベントを配るとき、その種類を送る有効な Webhook ごとに、配信のジョブを投入することを確かめる。
func TestEnqueuerQueuesDeliveries(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	failed := []domain.NotificationKind{domain.NotificationKindRecordingFailed}
	target, _ := f.createWebhook(t, "https://a.example/hook", webhook.FormatJSON, failed, true)
	f.createWebhook(t, "https://b.example/hook", webhook.FormatJSON, failed, false)
	f.createWebhook(t, "https://c.example/hook", webhook.FormatJSON, []domain.NotificationKind{domain.NotificationKindStorageLow}, true)
	eventID := f.emit(t, domain.NotificationKindRecordingFailed, nil)

	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
	inserter, err := jobs.NewClient(f.pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	// 画面内の通知を受け取るユーザーがいなくても、Webhook には送る。
	n, err := notification.NewDispatcher(f.pool, f.clock, webhook.NewEnqueuer(inserter)).DispatchOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	rows, err := f.pool.Query(t.Context(), "SELECT (args->>'webhookId')::uuid, (args->>'eventId')::uuid, max_attempts FROM river_job WHERE kind = 'webhook_deliver'")
	require.NoError(t, err)
	type queued struct {
		WebhookID   uuid.UUID
		EventID     uuid.UUID
		MaxAttempts int
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[queued])
	require.NoError(t, err)
	assert.Equal(t, []queued{{WebhookID: target, EventID: eventID, MaxAttempts: 8}}, got)
}

// TestDeliverWorkerSendsSignedBody は、3 つの形式の本文を送り、受信側が署名を確かめられることを確かめる (設計書 6.2)。
func TestDeliverWorkerSendsSignedBody(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"reservationId": "0199d000-0000-7000-8000-000000000001", "name": "こたつ特集", "serviceName": "NHK総合",
		"startAt": "2026-10-10T12:30:00Z", "endAt": "2026-10-10T13:00:00Z",
	}
	tests := []struct {
		name   string
		format webhook.Format
		want   func(eventID uuid.UUID) string
	}{
		{name: "汎用の JSON", format: webhook.FormatJSON, want: func(eventID uuid.UUID) string {
			return `{"id": "` + eventID.String() + `", "kind": "reservation_conflict", "occurredAt": "2026-10-10T12:00:00Z", "payload": {
				"reservationId": "0199d000-0000-7000-8000-000000000001", "name": "こたつ特集", "serviceName": "NHK総合",
				"startAt": "2026-10-10T12:30:00Z", "endAt": "2026-10-10T13:00:00Z"}}`
		}},
		{name: "Discord", format: webhook.FormatDiscord, want: func(uuid.UUID) string {
			return `{"content": "チューナーが足りず、録画できない見込みの予約があります。10/10（土）21:30 NHK総合「こたつ特集」", "allowed_mentions": {"parse": []}}`
		}},
		{name: "Slack", format: webhook.FormatSlack, want: func(uuid.UUID) string {
			return `{"text": "チューナーが足りず、録画できない見込みの予約があります。10/10（土）21:30 NHK総合「こたつ特集」"}`
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			server, requests := receiver(t, http.StatusNoContent)
			webhookID, secret := f.createWebhook(t, server.URL, tt.format, []domain.NotificationKind{domain.NotificationKindReservationConflict}, true)
			eventID := f.emit(t, domain.NotificationKindReservationConflict, payload)
			f.clock.Advance(time.Minute)

			require.NoError(t, f.deliver(t, webhookID, eventID))
			request := <-requests
			assert.JSONEq(t, tt.want(eventID), string(request.body))
			assert.Equal(t, "application/json", request.header.Get("Content-Type"))
			assert.Equal(t, eventID.String(), request.header.Get("X-Kotatsu-Event-Id"))
			assert.Equal(t, "reservation_conflict", request.header.Get("X-Kotatsu-Event-Kind"))
			// 署名の時刻は、送った時刻にする。受信側は、古すぎる時刻の署名を拒める。
			assert.Equal(t, f.clock.Now(), verify(t, secret, request))
		})
	}
}

// TestBodyEscapesNames は、Discord と Slack の本文で、名前に書いた記法が効かないことを確かめる (#1010)。
func TestBodyEscapesNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		format  webhook.Format
		program string
		want    string
	}{
		{
			name: "Discord のメンションと偽装リンク", format: webhook.FormatDiscord,
			program: "@everyone <@&123> [確認](https://example.com/login)",
			want:    `{"content": "「@everyone \\<@&123> \\[確認\\](https://example.com/login)」の録画を始めました", "allowed_mentions": {"parse": []}}`,
		},
		{
			name: "Discord の強調とコードと改行", format: webhook.FormatDiscord,
			program: "**太字** `code` ||spoiler||\n# 見出し",
			want:    `{"content": "「\\*\\*太字\\*\\* \\` + "`" + `code\\` + "`" + ` \\|\\|spoiler\\|\\| # 見出し」の録画を始めました", "allowed_mentions": {"parse": []}}`,
		},
		{
			name: "Discord で記法を始めない括弧は打ち消さない", format: webhook.FormatDiscord,
			program: "こたつ特集(再)",
			want:    `{"content": "「こたつ特集(再)」の録画を始めました", "allowed_mentions": {"parse": []}}`,
		},
		{
			name: "Slack のメンションとリンク", format: webhook.FormatSlack,
			program: "<!channel> <https://example.com/login|確認> & more",
			want:    `{"text": "「&lt;!channel&gt; &lt;https://example.com/login|確認&gt; &amp; more」の録画を始めました"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload, err := json.Marshal(map[string]any{"name": tt.program})
			require.NoError(t, err)
			event := &sqlc.NotificationEvent{ID: uuid.New(), Kind: string(domain.NotificationKindRecordingStarted), Payload: payload}

			body, err := webhook.Body(tt.format, event, time.UTC)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(body))
		})
	}
}

// TestDeliverWorkerRedactsURL は、送れなかったときのエラーに、URL のパスのトークンを残さないことを確かめる (#1016)。
// エラーは、River がログとジョブの記録に残す。
func TestDeliverWorkerRedactsURL(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	server, _ := receiver(t, http.StatusNoContent)
	server.Close()
	kinds := []domain.NotificationKind{domain.NotificationKindStorageLow}
	webhookID, _ := f.createWebhook(t, server.URL+"/api/webhooks/123/secret-token", webhook.FormatDiscord, kinds, true)
	eventID := f.emit(t, domain.NotificationKindStorageLow, nil)

	err := f.deliver(t, webhookID, eventID)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-token")
	assert.Contains(t, err.Error(), server.URL, "the host is kept for debugging")
}

// TestDeliverWorkerFailures は、送れなければ再試行に任せ、送る理由がなくなれば取りやめることを確かめる。
func TestDeliverWorkerFailures(t *testing.T) {
	t.Parallel()

	kinds := []domain.NotificationKind{domain.NotificationKindStorageLow}
	tests := []struct {
		name       string
		status     int
		enabled    bool
		deleted    bool
		wantCancel bool
	}{
		{name: "受信側のエラーは再試行する", status: http.StatusInternalServerError, enabled: true, deleted: false, wantCancel: false},
		{name: "リダイレクトには従わず再試行する", status: http.StatusFound, enabled: true, deleted: false, wantCancel: false},
		{name: "無効にした Webhook には送らない", status: http.StatusOK, enabled: false, deleted: false, wantCancel: true},
		{name: "消した Webhook には送らない", status: http.StatusOK, enabled: true, deleted: true, wantCancel: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			server, requests := receiver(t, tt.status)
			webhookID, _ := f.createWebhook(t, server.URL, webhook.FormatJSON, kinds, tt.enabled)
			if tt.deleted {
				require.NoError(t, f.store.Delete(t.Context(), f.pool, audit.System("test"), webhookID))
			}
			eventID := f.emit(t, domain.NotificationKindStorageLow, nil)

			err := f.deliver(t, webhookID, eventID)
			require.Error(t, err)
			var cancel *rivertype.JobCancelError
			assert.Equal(t, tt.wantCancel, errors.As(err, &cancel), "%v", err)
			if tt.wantCancel {
				assert.Empty(t, requests, "nothing is sent")
			} else {
				assert.Len(t, requests, 1)
			}
		})
	}
}
