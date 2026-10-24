package webpush_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/lib/publicnet"
	"github.com/SlashNephy/kotatsu/internal/notification"
	"github.com/SlashNephy/kotatsu/internal/webpush"
)

const (
	subject = "mailto:admin@kotatsu.example"
	// nameKey は、通知の文の材料のうち、番組名のキー。
	nameKey = "name"
)

// received は、プッシュサービスが受け取ったリクエスト。
type received struct {
	header http.Header
	body   []byte
}

// pushService は、受け取ったリクエストを残し、respond の返す状態を返すプッシュサービスを立てる。
func pushService(t *testing.T, respond func() int) (*httptest.Server, <-chan received) {
	t.Helper()

	requests := make(chan received, 10)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		requests <- received{header: r.Header.Clone(), body: body}
		w.WriteHeader(respond())
	}))
	t.Cleanup(server.Close)

	return server, requests
}

// subscribe は、ユーザーを作り、ブラウザの購読を保存して、その ID を返す。
func (f *fixture) subscribe(t *testing.T, username, endpoint string, b *browser) uuid.UUID {
	t.Helper()

	userID := f.user(t, username)
	require.NoError(t, f.service.Subscribe(t.Context(), f.pool, userID, b.subscription(endpoint)))
	got, err := f.service.List(t.Context(), f.pool, userID)
	require.NoError(t, err)
	require.Len(t, got, 1)

	return got[0].ID
}

func (f *fixture) emit(t *testing.T, kind notification.Kind, payload map[string]any) uuid.UUID {
	t.Helper()

	require.NoError(t, notification.Emit(t.Context(), f.pool, f.clock, kind, payload))
	var eventID uuid.UUID
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT id FROM notification_events ORDER BY occurred_at DESC, id DESC LIMIT 1").Scan(&eventID))

	return eventID
}

func (f *fixture) deliver(t *testing.T, client *http.Client, subscriptionID, eventID uuid.UUID) error {
	t.Helper()

	location, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	worker := webpush.NewDeliverWorker(f.pool, f.service, f.clock, subject, location, client)
	job := &river.Job[webpush.DeliverArgs]{JobRow: new(rivertype.JobRow), Args: webpush.DeliverArgs{SubscriptionID: subscriptionID, EventID: eventID}}

	return worker.Work(t.Context(), job)
}

func (f *fixture) subscriptionExists(t *testing.T, subscriptionID uuid.UUID) bool {
	t.Helper()

	_, err := sqlc.New(f.pool).GetWebPushSubscription(t.Context(), subscriptionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	require.NoError(t, err)

	return true
}

// TestEnqueuerQueuesDeliveries は、イベントを配るとき、その種類を Web Push で受け取る、有効なユーザーの購読ごとに、配信のジョブを投入することを確かめる。
func TestEnqueuerQueuesDeliveries(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	target := f.subscribe(t, "mikan", "https://push.example/mikan", newBrowser(t))
	// 録画の失敗を Web Push で受け取らないユーザー。
	f.subscribe(t, "yuzu", "https://push.example/yuzu", newBrowser(t))
	_, err := f.pool.Exec(t.Context(), `INSERT INTO notification_preferences (user_id, kind, in_app, web_push, updated_at)
		SELECT id, 'recording_failed', true, false, now() FROM users WHERE username = 'yuzu'`)
	require.NoError(t, err)
	// 無効にしたユーザー。
	f.subscribe(t, "sudachi", "https://push.example/sudachi", newBrowser(t))
	_, err = f.pool.Exec(t.Context(), "UPDATE users SET disabled_at = now() WHERE username = 'sudachi'")
	require.NoError(t, err)
	eventID := f.emit(t, notification.KindRecordingFailed, map[string]any{nameKey: "こたつ特集"})

	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
	inserter, err := jobs.NewClient(f.pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	n, err := notification.NewDispatcher(f.pool, f.clock, webpush.NewEnqueuer(inserter)).DispatchOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	rows, err := f.pool.Query(t.Context(),
		"SELECT (args->>'subscriptionId')::uuid, (args->>'eventId')::uuid, max_attempts FROM river_job WHERE kind = 'web_push_deliver'")
	require.NoError(t, err)
	type queued struct {
		SubscriptionID uuid.UUID
		EventID        uuid.UUID
		MaxAttempts    int
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[queued])
	require.NoError(t, err)
	assert.Equal(t, []queued{{SubscriptionID: target, EventID: eventID, MaxAttempts: 5}}, got)
}

// TestDeliverWorkerSendsEncryptedNotification は、通知の文を購読の鍵で暗号化し、VAPID で署名して送り、
// ブラウザが復号できることを確かめる (設計書 6.2)。
func TestDeliverWorkerSendsEncryptedNotification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		kind        notification.Kind
		payload     map[string]any
		wantBody    string
		wantUrgency string
		// wantURL は、通知を押したときに開く画面のパス。空なら payload に入れない。
		wantURL string
	}{
		{
			name: "録画の失敗は、すぐに届ける", kind: notification.KindRecordingFailed, payload: map[string]any{nameKey: "こたつ特集"},
			wantBody: "「こたつ特集」の録画に失敗しました", wantUrgency: "high",
		},
		{
			name: "録画の完了", kind: notification.KindRecordingCompleted, payload: map[string]any{nameKey: "みかん紀行"},
			wantBody: "「みかん紀行」の録画が終わりました", wantUrgency: "normal",
		},
		{
			name: "視聴予約の通知は、押すとライブ視聴を開く", kind: notification.KindWatchReminder,
			payload: map[string]any{
				nameKey: "こたつの時間", "serviceName": "NHK総合", "startAt": "2026-10-10T12:05:00Z", "url": "/live/U2VydmljZToxMDI0",
			},
			wantBody: "21:05 から NHK総合で「こたつの時間」が始まります", wantUrgency: "normal", wantURL: "/live/U2VydmljZToxMDI0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			server, requests := pushService(t, func() int { return http.StatusCreated })
			b := newBrowser(t)
			subscriptionID := f.subscribe(t, "mikan", server.URL+"/send/mikan", b)
			eventID := f.emit(t, tt.kind, tt.payload)

			require.NoError(t, f.deliver(t, server.Client(), subscriptionID, eventID))
			request := <-requests
			assert.Equal(t, "aes128gcm", request.header.Get("Content-Encoding"))
			assert.Equal(t, "86400", request.header.Get("TTL"))
			assert.Equal(t, tt.wantUrgency, request.header.Get("Urgency"))

			claims, publicKey := vapidClaims(t, request.header.Get("Authorization"))
			keys, err := f.service.Keys(t.Context(), f.pool)
			require.NoError(t, err)
			assert.Equal(t, keys.PublicKey(), publicKey, "k is the VAPID public key the browser subscribed with")
			assert.Equal(t, server.URL, claims["aud"])
			assert.Equal(t, subject, claims["sub"])
			assert.InDelta(t, float64(f.clock.Now().Add(12*time.Hour).Unix()), claims["exp"], 0)

			var payload map[string]any
			require.NoError(t, json.Unmarshal(b.decrypt(t, request.body), &payload))
			want := map[string]any{
				"title": "Kotatsu", "body": tt.wantBody, "eventId": eventID.String(), "kind": string(tt.kind), "occurredAt": "2026-10-10T12:00:00Z",
			}
			if tt.wantURL != "" {
				want["url"] = tt.wantURL
			}
			assert.Equal(t, want, payload)
		})
	}
}

// TestDeliverWorkerResponses は、プッシュサービスの応答に応じて、配信を終えるか、購読を消すか、送り直すかを確かめる。
func TestDeliverWorkerResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		wantErr    bool
		wantCancel bool
		wantKept   bool
	}{
		{name: "受け付けた", status: http.StatusCreated, wantErr: false, wantCancel: false, wantKept: true},
		{name: "購読が失効した", status: http.StatusGone, wantErr: true, wantCancel: true, wantKept: false},
		{name: "購読が見つからない", status: http.StatusNotFound, wantErr: true, wantCancel: true, wantKept: false},
		{name: "混んでいるので送り直す", status: http.StatusTooManyRequests, wantErr: true, wantCancel: false, wantKept: true},
		{name: "落ちているので送り直す", status: http.StatusServiceUnavailable, wantErr: true, wantCancel: false, wantKept: true},
		{name: "送り直しても通らない", status: http.StatusForbidden, wantErr: true, wantCancel: true, wantKept: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			server, _ := pushService(t, func() int { return tt.status })
			subscriptionID := f.subscribe(t, "mikan", server.URL+"/send/mikan", newBrowser(t))
			eventID := f.emit(t, notification.KindRecordingStarted, nil)

			err := f.deliver(t, server.Client(), subscriptionID, eventID)
			assert.Equal(t, tt.wantErr, err != nil, "%v", err)
			var cancel *rivertype.JobCancelError
			assert.Equal(t, tt.wantCancel, errors.As(err, &cancel), "%v", err)
			assert.Equal(t, tt.wantKept, f.subscriptionExists(t, subscriptionID))
		})
	}
}

// TestDeliverWorkerKeepsResubscription は、送っている間に、同じ端末が新しい鍵で購読し直したら、失効が返っても、新しい購読を消さないことを確かめる。
func TestDeliverWorkerKeepsResubscription(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	var resubscribe func()
	server, _ := pushService(t, func() int {
		resubscribe()

		return http.StatusGone
	})
	endpoint := server.URL + "/send/mikan"
	subscriptionID := f.subscribe(t, "mikan", endpoint, newBrowser(t))
	var userID uuid.UUID
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT user_id FROM web_push_subscriptions WHERE id = $1", subscriptionID).Scan(&userID))
	resubscribe = func() {
		assert.NoError(t, f.service.Subscribe(t.Context(), f.pool, userID, newBrowser(t).subscription(endpoint)))
	}
	eventID := f.emit(t, notification.KindRecordingStarted, nil)

	var cancel *rivertype.JobCancelError
	require.ErrorAs(t, f.deliver(t, server.Client(), subscriptionID, eventID), &cancel)
	assert.True(t, f.subscriptionExists(t, subscriptionID))
}

// TestDeliverWorkerCancelsWithoutTarget は、購読かイベントが消えていたら、送らずに取りやめることを確かめる。
func TestDeliverWorkerCancelsWithoutTarget(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	server, requests := pushService(t, func() int { return http.StatusCreated })
	subscriptionID := f.subscribe(t, "mikan", server.URL+"/send/mikan", newBrowser(t))
	eventID := f.emit(t, notification.KindRecordingStarted, nil)

	var cancel *rivertype.JobCancelError
	require.ErrorAs(t, f.deliver(t, server.Client(), uuid.New(), eventID), &cancel)
	require.ErrorAs(t, f.deliver(t, server.Client(), subscriptionID, uuid.New()), &cancel)
	assert.Empty(t, requests)
}

// TestDeliverWorkerCancelsNonPublicEndpoint は、LAN の中の送り先には送らず、送り直さずに取りやめることを確かめる。
func TestDeliverWorkerCancelsNonPublicEndpoint(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	server, requests := pushService(t, func() int { return http.StatusCreated })
	subscriptionID := f.subscribe(t, "mikan", server.URL+"/send/mikan", newBrowser(t))
	eventID := f.emit(t, notification.KindRecordingStarted, nil)

	var cancel *rivertype.JobCancelError
	require.ErrorAs(t, f.deliver(t, webpush.NewHTTPClient(), subscriptionID, eventID), &cancel)
	assert.Empty(t, requests)
}

// TestNewHTTPClientRefusesNonPublicAddresses は、プッシュサービスに送るクライアントが、LAN の中のアドレスには接続しないことを確かめる。
func TestNewHTTPClientRefusesNonPublicAddresses(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(server.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL, nil)
	require.NoError(t, err)
	resp, err := webpush.NewHTTPClient().Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	require.ErrorIs(t, err, publicnet.ErrNonPublicAddress)
}
