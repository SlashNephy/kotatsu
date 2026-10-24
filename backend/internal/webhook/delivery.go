package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/notification"
)

const (
	// SignatureHeader は、ペイロードの署名を載せるヘッダー。
	// 値は「t=<UNIX 時刻>,v1=<HMAC-SHA256 の 16 進数>」で、HMAC は、署名用の秘密キーで「<UNIX 時刻>.<本文>」に対して計算する。
	SignatureHeader = "X-Kotatsu-Signature"
	// EventIDHeader と EventKindHeader は、イベントの ID と種類を載せるヘッダー。
	// 送り直しても同じ ID なので、受信側は ID で重複を除ける。
	EventIDHeader   = "X-Kotatsu-Event-Id"
	EventKindHeader = "X-Kotatsu-Event-Kind"

	// deliveryMaxAttempts は、1 回の配信を試す回数の上限。間隔は River の既定の指数バックオフに従う。
	deliveryMaxAttempts = 8
	// requestTimeout は、1 回の送信の時間の上限。
	requestTimeout = 10 * time.Second
	// maxResponseBytes は、応答の本文を読み捨てる量の上限。
	maxResponseBytes = 64 << 10
	// userAgent は、送信に付ける User-Agent。
	userAgent = "Kotatsu-Webhook/1"
)

// DeliverArgs は、1 つのイベントを 1 つの Webhook に送るジョブの引数。
type DeliverArgs struct {
	WebhookID uuid.UUID `json:"webhookId"`
	EventID   uuid.UUID `json:"eventId"`
}

var _ river.JobArgsWithInsertOpts = new(DeliverArgs)

func (DeliverArgs) Kind() string { return "webhook_deliver" }

func (DeliverArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: deliveryMaxAttempts}
}

// Inserter は、ジョブをトランザクションの中で投入する。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// Enqueuer は、配るイベントごとに、そのイベントの種類を送る有効な Webhook への配信のジョブを投入する。
type Enqueuer struct {
	inserter Inserter
}

var _ notification.Hook = new(Enqueuer)

// NewEnqueuer は Enqueuer を返す。
func NewEnqueuer(inserter Inserter) *Enqueuer {
	return &Enqueuer{inserter: inserter}
}

func (e *Enqueuer) OnEvent(ctx context.Context, tx pgx.Tx, event *notification.Event) error {
	webhookIDs, err := sqlc.New(tx).ListWebhooksForKind(ctx, string(event.Kind))
	if err != nil {
		return fmt.Errorf("failed to list webhooks: %w", err)
	}
	if len(webhookIDs) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, 0, len(webhookIDs))
	for _, webhookID := range webhookIDs {
		params = append(params, river.InsertManyParams{Args: DeliverArgs{WebhookID: webhookID, EventID: event.ID}, InsertOpts: nil})
	}
	if _, err := e.inserter.InsertManyTx(ctx, tx, params); err != nil {
		return fmt.Errorf("failed to enqueue webhook deliveries: %w", err)
	}

	return nil
}

// DeliverWorker は、イベントを Webhook に送る。送れなければエラーを返し、River の再試行に任せる。
// Webhook を消したか無効にしたなら、送らずに取りやめる。
type DeliverWorker struct {
	river.WorkerDefaults[DeliverArgs]

	db       sqlc.DBTX
	store    *Store
	clock    clock.Clock
	location *time.Location
	client   *http.Client
}

var _ river.Worker[DeliverArgs] = new(DeliverWorker)

// NewDeliverWorker は DeliverWorker を返す。location は、Discord や Slack に送る文の日時のタイムゾーン。
func NewDeliverWorker(db sqlc.DBTX, store *Store, clk clock.Clock, location *time.Location) *DeliverWorker {
	return &DeliverWorker{
		WorkerDefaults: river.WorkerDefaults[DeliverArgs]{},
		db:             db, store: store, clock: clk, location: location,
		client: &http.Client{
			Timeout: requestTimeout,
			// 署名した本文を別の送り先に渡さないよう、リダイレクトには従わない。3xx は失敗として扱う。
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (w *DeliverWorker) Timeout(*river.Job[DeliverArgs]) time.Duration {
	return 2 * requestTimeout
}

func (w *DeliverWorker) Work(ctx context.Context, job *river.Job[DeliverArgs]) error {
	q := sqlc.New(w.db)
	row, err := q.GetWebhook(ctx, job.Args.WebhookID)
	if errors.Is(err, pgx.ErrNoRows) {
		return river.JobCancel(ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("failed to get the webhook: %w", err)
	}
	if !row.Enabled {
		return river.JobCancel(errDisabled)
	}
	eventRow, err := q.GetNotificationEvent(ctx, job.Args.EventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return river.JobCancel(errEventNotFound)
	}
	if err != nil {
		return fmt.Errorf("failed to get the notification event: %w", err)
	}
	secret, err := w.store.open(row.SecretCiphertext)
	if err != nil {
		return err
	}
	body, err := Body(Format(row.Format), &eventRow, w.location)
	if err != nil {
		return err
	}

	return w.send(ctx, row.URL, secret, &eventRow, body)
}

func (w *DeliverWorker) send(ctx context.Context, url, secret string, event *sqlc.NotificationEvent, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build the webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set(EventIDHeader, event.ID.String())
	req.Header.Set(EventKindHeader, event.Kind)
	req.Header.Set(SignatureHeader, signature(secret, w.clock.Now(), body))

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send the webhook: %w", err)
	}
	defer resp.Body.Close()
	// 接続を使い回せるよう、応答の本文は上限まで読み捨てる。
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes)); err != nil {
		return fmt.Errorf("failed to read the webhook response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: %s", errUnexpectedStatus, resp.Status)
	}

	return nil
}

var (
	errDisabled         = errors.New("the webhook is disabled")
	errEventNotFound    = errors.New("the notification event no longer exists")
	errUnexpectedStatus = errors.New("the webhook endpoint returned an unexpected status")
)

// signature は、SignatureHeader の値を作る。
func signature(secret string, at time.Time, body []byte) string {
	timestamp := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)

	return "t=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// jsonBody は、汎用の JSON の形式の本文。
type jsonBody struct {
	ID         uuid.UUID       `json:"id"`
	Kind       string          `json:"kind"`
	OccurredAt time.Time       `json:"occurredAt"`
	Payload    json.RawMessage `json:"payload"`
}

// Body は、イベントを、形式 format の本文にする。Discord と Slack には、location の日時で書いた文を送る。
func Body(format Format, event *sqlc.NotificationEvent, location *time.Location) ([]byte, error) {
	var value any
	switch format {
	case FormatJSON:
		value = jsonBody{ID: event.ID, Kind: event.Kind, OccurredAt: event.OccurredAt.UTC(), Payload: event.Payload}
	case FormatDiscord, FormatSlack:
		payload := map[string]any{}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to decode the notification payload: %w", err)
		}
		text := notification.Message(notification.Kind(event.Kind), payload, location)
		if format == FormatDiscord {
			value = map[string]string{"content": text}
		} else {
			value = map[string]string{"text": text}
		}
	default:
		return nil, fmt.Errorf("%w: unknown format %q", ErrInvalidWebhook, format)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the webhook body: %w", err)
	}

	return body, nil
}
