package cli

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/segjoin"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
	"github.com/SlashNephy/kotatsu/internal/usecase/watch"
	"github.com/SlashNephy/kotatsu/internal/usecase/webhook"
	"github.com/SlashNephy/kotatsu/internal/usecase/webpush"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

// notificationParts は、nekonote で、通知するイベントを配り、Webhook と Web Push で送るジョブの Worker (設計書 6.2)。
// 視聴予約を、放送の少し前に通知するジョブの Worker も持つ (設計書 4.8)。
type notificationParts struct {
	dispatch        *notification.DispatchWorker
	webhookDelivery *webhook.DeliverWorker
	webPushDelivery *webpush.DeliverWorker
	watchNotify     *watch.NotifyWorker
}

// locationOf は、表示のタイムゾーンを返す (設計書 8.1)。通知の本文の日時に使う。
func locationOf(deps *Deps) (*time.Location, error) {
	return deps.Config.Location()
}

// newWebhookDeliverWorker は、Webhook を配信する Worker を作る。
func newWebhookDeliverWorker(deps *Deps, pool *pgxpool.Pool, store *webhook.Store, clk clock.Clock, location *time.Location) *webhook.DeliverWorker {
	return webhook.NewDeliverWorker(pool, store, clk, location, deps.Network)
}

// newWebPushDeliverWorker は、Web Push を配信する Worker を作る。
func newWebPushDeliverWorker(
	deps *Deps, pool *pgxpool.Pool, service *webpush.Service, clk clock.Clock, location *time.Location,
) *webpush.DeliverWorker {
	return webpush.NewDeliverWorker(pool, service, clk, webPushSubject(deps.Config), location, webpush.NewHTTPClient(deps.Network))
}

// newWatchNotifyWorker は、視聴予約を、放送の少し前に、予約したユーザーに通知する Worker を作る (設計書 4.8)。
// 通知を押すと、そのサービスのライブ視聴を開く。
func newWatchNotifyWorker(pool *pgxpool.Pool, clk clock.Clock) *watch.NotifyWorker {
	return watch.NewNotifyWorker(watch.NewNotifier(pool, clk, graph.ServiceLiveURL))
}

// newDispatcher は、通知するイベントを、ユーザーごとの画面内の通知に配る部品を作る。
// 配るときに、Webhook と Web Push の配信、録画のファイルをつなぐジョブ、CM の区間を見分けるジョブ、字幕を自動生成するジョブ、変換のジョブを inserter で入れる。
func newDispatcher(pool *pgxpool.Pool, clk clock.Clock, inserter *jobs.Client) *notification.Dispatcher {
	return notification.NewDispatcher(pool, clk,
		webhook.NewEnqueuer(inserter), webpush.NewEnqueuer(inserter), segjoin.NewEnqueuer(inserter),
		cmdetect.NewEnqueuer(inserter, clk), autocaption.NewEnqueuer(inserter, clk), transcode.NewEnqueuer(inserter))
}
