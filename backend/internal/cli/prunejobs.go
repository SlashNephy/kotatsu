package cli

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/library"
	"github.com/SlashNephy/kotatsu/internal/usecase/networkactivity"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

// pruneJobParts は、保持期間を過ぎたものを消すジョブの Worker (#1607)。
type pruneJobParts struct {
	audit           *audit.PruneWorker
	notifications   *notification.PruneWorker
	watchProgress   *library.PruneProgressWorker
	networkActivity *networkactivity.PruneWorker
}

// newAuditPruneWorker は、監査ログを、設定の保持期間を過ぎたら消す Worker を作る。接続元の情報は、別の短い保持期間で消す。
func newAuditPruneWorker(deps *Deps, pool *pgxpool.Pool, clk clock.Clock) *audit.PruneWorker {
	retention := time.Duration(deps.Config.Audit.RetentionDays) * 24 * time.Hour
	clientInfoRetention := time.Duration(deps.Config.Audit.ClientInfoRetentionDays) * 24 * time.Hour

	return audit.NewPruneWorker(pool, clk, retention, clientInfoRetention)
}

// newNotificationPruneWorker は、配り終えた通知を、画面から変えられる保持期間を過ぎたら消す Worker を作る (#551)。
func newNotificationPruneWorker(pool *pgxpool.Pool, clk clock.Clock) *notification.PruneWorker {
	return notification.NewPruneWorker(pool, clk, func(ctx context.Context) (time.Duration, error) {
		system, err := settings.Load(ctx, pool)
		if err != nil {
			return 0, err
		}

		return system.NotificationRetention(), nil
	})
}

// newWatchProgressPruneWorker は、録画の再生位置を、画面から変えられる保持期間を過ぎたら消す Worker を作る (#1366)。
func newWatchProgressPruneWorker(pool *pgxpool.Pool, clk clock.Clock) *library.PruneProgressWorker {
	return library.NewPruneProgressWorker(pool, clk, func(ctx context.Context) (time.Duration, error) {
		system, err := settings.Load(ctx, pool)
		if err != nil {
			return 0, err
		}

		return system.WatchProgressRetention(), nil
	})
}

// newNetworkActivityPruneWorker は、外部との通信の記録を、画面から変えられる保持期間を過ぎたら消す Worker を作る (#1369)。
func newNetworkActivityPruneWorker(pool *pgxpool.Pool, clk clock.Clock) *networkactivity.PruneWorker {
	return networkactivity.NewPruneWorker(pool, clk, func(ctx context.Context) (time.Duration, error) {
		system, err := settings.Load(ctx, pool)
		if err != nil {
			return 0, err
		}

		return time.Duration(system.NetworkActivityRetentionDays) * 24 * time.Hour, nil
	})
}
