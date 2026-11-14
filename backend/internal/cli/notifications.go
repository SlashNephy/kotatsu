package cli

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/segjoin"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
	"github.com/SlashNephy/kotatsu/internal/usecase/webhook"
	"github.com/SlashNephy/kotatsu/internal/usecase/webpush"
)

// notificationParts は、nekonote で、通知するイベントを配り、Webhook と Web Push で送る部品 (設計書 6.2)。
type notificationParts struct {
	dispatcher *notification.Dispatcher
	webhooks   *webhook.Store
	webPush    *webpush.Service
}

// newDispatcher は、通知するイベントを、ユーザーごとの画面内の通知に配る部品を作る。
// 配るときに、Webhook と Web Push の配信、録画のファイルをつなぐジョブ、CM の区間を見分けるジョブ、変換のジョブを inserter で入れる。
func newDispatcher(pool *pgxpool.Pool, clk clock.Clock, inserter *jobs.Client) *notification.Dispatcher {
	return notification.NewDispatcher(pool, clk,
		webhook.NewEnqueuer(inserter), webpush.NewEnqueuer(inserter), segjoin.NewEnqueuer(inserter),
		cmdetect.NewEnqueuer(inserter, clk), transcode.NewEnqueuer(inserter))
}
