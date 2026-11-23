package cli

import (
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/capacity"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/usecase/timeshift"
)

// storageJobParts は、保存先プールの容量と、タイムシフトのバッファを扱うジョブの Worker (#1607)。
type storageJobParts struct {
	capacity       *capacity.CheckWorker
	timeshiftPrune *timeshift.PruneWorker
	promote        *recording.PromoteWorker
	reapPromotions *recording.ReapPromotionsWorker
}

// newCapacityCheckWorker は、このインスタンスにマウントされた保存先プールの容量を確認する Worker を作る (設計書 6.1)。
func newCapacityCheckWorker(pool *pgxpool.Pool, clk clock.Clock, store *kago.Store, logger *slog.Logger) *capacity.CheckWorker {
	return capacity.NewCheckWorker(capacity.NewManager(pool, clk, store, audit.NewRecorder(clk), logger.With(slog.String("component", "capacity"))))
}

// newTimeshiftPruneWorker は、タイムシフトのバッファから、保持時間を過ぎたチャンクを破棄する Worker を作る (設計書 5.7)。
// タイムシフトのディレクトリがなければ、何もしない。
func newTimeshiftPruneWorker(pool *pgxpool.Pool, clk clock.Clock, store *kago.Store, logger *slog.Logger) *timeshift.PruneWorker {
	return timeshift.NewPruneWorker(pool, clk, store, logger.With(slog.String("component", "timeshift")))
}

// newPromoteWorker は、タイムシフトのバッファにある、放送の終わった番組を、録画に出力する Worker を作る (設計書 5.7)。
func newPromoteWorker(pool *pgxpool.Pool, clk clock.Clock, store *kago.Store, location *time.Location, logger *slog.Logger) *recording.PromoteWorker {
	return recording.NewPromoteWorker(pool, clk, store, settings.RecordingDefaults, location, logger.With(slog.String("component", "timeshift_promote")))
}

// newReapPromotionsWorker は、出力の途中で止まったものを片付ける Worker を作る (設計書 5.7)。
func newReapPromotionsWorker(pool *pgxpool.Pool, clk clock.Clock, logger *slog.Logger) *recording.ReapPromotionsWorker {
	return recording.NewReapPromotionsWorker(pool, clk, logger.With(slog.String("component", "timeshift_promote")))
}
