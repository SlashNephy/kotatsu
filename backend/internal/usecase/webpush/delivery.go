package webpush

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/publicnet"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
)

const (
	// deliveryMaxAttempts は、1 回の配信を試す回数の上限。間隔は River の既定の指数バックオフに従う。
	deliveryMaxAttempts = 5
	// requestTimeout は、1 回の送信の時間の上限。
	requestTimeout = 10 * time.Second
	// messageTTL は、端末がオフラインのとき、プッシュサービスが通知を預かる時間。
	messageTTL = 24 * time.Hour
	// maxResponseBytes は、応答の本文を読み捨てる量の上限。
	maxResponseBytes = 64 << 10
	// title は、通知の見出し。
	title = "Kotatsu"
)

// DeliverArgs は、1 つのイベントを 1 つの購読に送るジョブの引数。購読ごとに分けて、再試行も購読ごとに行う。
type DeliverArgs struct {
	SubscriptionID uuid.UUID `json:"subscriptionId"`
	EventID        uuid.UUID `json:"eventId"`
}

var _ river.JobArgsWithInsertOpts = new(DeliverArgs)

func (DeliverArgs) Kind() string { return "web_push_deliver" }

func (DeliverArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: deliveryMaxAttempts}
}

// Inserter は、ジョブをトランザクションの中で投入する。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// Enqueuer は、配るイベントごとに、そのイベントの種類を Web Push で受け取る購読への配信のジョブを投入する。
type Enqueuer struct {
	inserter Inserter
}

var _ notification.Hook = new(Enqueuer)

// NewEnqueuer は Enqueuer を返す。
func NewEnqueuer(inserter Inserter) *Enqueuer {
	return &Enqueuer{inserter: inserter}
}

func (e *Enqueuer) OnEvent(ctx context.Context, tx pgx.Tx, event *notification.Event) error {
	subscriptionIDs, err := sqlc.New(tx).ListWebPushSubscriptionsForKind(ctx, sqlc.ListWebPushSubscriptionsForKindParams{
		Kind: string(event.Kind), Audience: event.Audience.UserIDs, ByRule: event.Audience.ByRule,
	})
	if err != nil {
		return fmt.Errorf("failed to list web push subscriptions: %w", err)
	}
	if len(subscriptionIDs) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, 0, len(subscriptionIDs))
	for _, subscriptionID := range subscriptionIDs {
		params = append(params, river.InsertManyParams{Args: DeliverArgs{SubscriptionID: subscriptionID, EventID: event.ID}, InsertOpts: nil})
	}
	if _, err := e.inserter.InsertManyTx(ctx, tx, params); err != nil {
		return fmt.Errorf("failed to enqueue web push deliveries: %w", err)
	}

	return nil
}

// Payload は、Service Worker が受け取る、通知の本文。
type Payload struct {
	Title      string            `json:"title"`
	Body       string            `json:"body"`
	EventID    uuid.UUID         `json:"eventId"`
	Kind       notification.Kind `json:"kind"`
	OccurredAt time.Time         `json:"occurredAt"`
	// URL は、通知を押したときに開く画面のパス。なければ省き、Service Worker は通知の一覧を開く。
	URL string `json:"url,omitempty"`
}

// DeliverWorker は、イベントを購読した端末に送る。
// プッシュサービスが購読の失効 (404 か 410) を返したら、購読を消して取りやめる。混んでいるか落ちているときは、River の再試行に任せる。
type DeliverWorker struct {
	river.WorkerDefaults[DeliverArgs]

	db       sqlc.DBTX
	service  *Service
	clock    clock.Clock
	subject  string
	location *time.Location
	client   *http.Client
}

var _ river.Worker[DeliverArgs] = new(DeliverWorker)

// NewDeliverWorker は DeliverWorker を返す。subject は、プッシュサービスに示す送り手の連絡先 (mailto: か https: の URI)。
// location は、通知の文の日時のタイムゾーン。client は、NewHTTPClient で作ったものを渡す。
func NewDeliverWorker(
	db sqlc.DBTX, service *Service, clk clock.Clock, subject string, location *time.Location, client *http.Client,
) *DeliverWorker {
	return &DeliverWorker{
		WorkerDefaults: river.WorkerDefaults[DeliverArgs]{},
		db:             db, service: service, clock: clk, subject: subject, location: location, client: client,
	}
}

func (w *DeliverWorker) Timeout(*river.Job[DeliverArgs]) time.Duration {
	return 2 * requestTimeout
}

func (w *DeliverWorker) Work(ctx context.Context, job *river.Job[DeliverArgs]) error {
	q := sqlc.New(w.db)
	subscription, err := q.GetWebPushSubscription(ctx, job.Args.SubscriptionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return river.JobCancel(errSubscriptionNotFound)
	}
	if err != nil {
		return fmt.Errorf("failed to get the web push subscription: %w", err)
	}
	event, err := q.GetNotificationEvent(ctx, job.Args.EventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return river.JobCancel(errEventNotFound)
	}
	if err != nil {
		return fmt.Errorf("failed to get the notification event: %w", err)
	}
	plaintext, err := w.payload(&event)
	if err != nil {
		return err
	}
	body, err := Encrypt(plaintext, subscription.P256DH, subscription.Auth)
	if err != nil {
		return river.JobCancel(err)
	}
	keys, err := w.service.Keys(ctx, w.db)
	if err != nil {
		return err
	}

	return w.send(ctx, &subscription, keys, notification.Kind(event.Kind), body)
}

func (w *DeliverWorker) payload(event *sqlc.NotificationEvent) ([]byte, error) {
	payload := map[string]any{}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to decode the notification payload: %w", err)
	}
	kind := notification.Kind(event.Kind)
	plaintext, err := json.Marshal(&Payload{
		Title: title, Body: notification.Message(kind, payload, w.location), EventID: event.ID, Kind: kind, OccurredAt: event.OccurredAt,
		URL: notification.URL(payload),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode the web push payload: %w", err)
	}

	return plaintext, nil
}

func (w *DeliverWorker) send(ctx context.Context, subscription *sqlc.WebPushSubscription, keys *Keys, kind notification.Kind, body []byte) error {
	authorization, err := keys.authorization(subscription.Endpoint, w.subject, w.clock.Now())
	if err != nil {
		return river.JobCancel(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, subscription.Endpoint, bytes.NewReader(body))
	if err != nil {
		return river.JobCancel(fmt.Errorf("failed to build the web push request: %w", err))
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", strconv.Itoa(int(messageTTL.Seconds())))
	req.Header.Set("Urgency", urgency(kind))
	req.Header.Set("Authorization", authorization)

	resp, err := w.client.Do(req)
	if errors.Is(err, errNonPublicAddress) {
		// LAN の中の送り先は、送り直しても送らないので、取りやめる。
		return river.JobCancel(fmt.Errorf("failed to send the web push: %w", err))
	}
	if err != nil {
		return fmt.Errorf("failed to send the web push: %w", err)
	}
	defer resp.Body.Close()
	// 接続を使い回せるよう、応答の本文は上限まで読み捨てる。
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes)); err != nil {
		return fmt.Errorf("failed to read the web push response: %w", err)
	}

	return w.handle(ctx, subscription, resp)
}

// handle は、プッシュサービスの応答に応じて、配信を終えるか、購読を消すか、送り直す。
func (w *DeliverWorker) handle(ctx context.Context, subscription *sqlc.WebPushSubscription, resp *http.Response) error {
	switch {
	case resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices:
		return nil
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		// 購読が失効した。送った後に同じ端末が購読し直していたら、新しい鍵の購読は残す。
		err := sqlc.New(w.db).DeleteExpiredWebPushSubscription(ctx, sqlc.DeleteExpiredWebPushSubscriptionParams{
			ID: subscription.ID, P256DH: subscription.P256DH, Auth: subscription.Auth,
		})
		if err != nil {
			return fmt.Errorf("failed to delete the expired web push subscription: %w", err)
		}

		return river.JobCancel(fmt.Errorf("%w: %s", errExpired, resp.Status))
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError:
		return fmt.Errorf("%w: %s", errUnexpectedStatus, resp.Status)
	default:
		// 本文が大きすぎるか、VAPID の署名が通らないなど、送り直しても通らない。
		return river.JobCancel(fmt.Errorf("%w: %s", errUnexpectedStatus, resp.Status))
	}
}

// urgency は、イベントの種類ごとの、プッシュサービスへの緊急度 (RFC 8030 5.3)。録画の失敗のように、すぐ知らせたいものは high にする。
func urgency(kind notification.Kind) string {
	switch kind {
	case notification.KindRecordingFailed, notification.KindStorageLow, notification.KindTunerServerUnreachable:
		return "high"
	case notification.KindRecordingStarted, notification.KindRecordingCompleted, notification.KindReservationConflict,
		notification.KindWatchReminder:
		return "normal"
	default:
		return "normal"
	}
}

var (
	errSubscriptionNotFound = errors.New("the web push subscription no longer exists")
	errEventNotFound        = errors.New("the notification event no longer exists")
	errExpired              = errors.New("the web push subscription has expired")
	errUnexpectedStatus     = errors.New("the push service returned an unexpected status")
	errNonPublicAddress     = publicnet.ErrNonPublicAddress
)

// NewHTTPClient は、プッシュサービスに送る HTTP クライアントを返す。
// 購読の送り先はユーザーが渡すので、LAN の中のサーバーに送らせないよう、接続するときに、グローバルなアドレスかを確かめる。
// 名前を引いた後のアドレスで確かめるので、DNS の応答を後から変えられても、LAN の中には送らない。
// 同じ理由で、環境変数のプロキシは使わず、リダイレクトにも従わない。
func NewHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: requestTimeout, Control: publicnet.Control}

	return &http.Client{
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         dialer.DialContext,
			ForceAttemptHTTP2:   true,
			TLSHandshakeTimeout: requestTimeout,
			MaxIdleConns:        10,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout:       requestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
