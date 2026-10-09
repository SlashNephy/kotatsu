// Package neko は neko (scheduler) のロールを実装する。
//
// neko は全体を見て結果を一つに決める処理を担うので、アクティブにするのは 1 台だけにする (設計書 1.3)。
// 複数のインスタンスを起動しても、名前 neko のリースを取ったインスタンスだけがリーダーとして動く。
package neko

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/lease"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/tunerstate"
)

const (
	// LeaseName は neko のリーダー選出に使うリースの名前。
	LeaseName = "neko"
	// LeaseTTL と RenewInterval は、設計書の付録 A の初期値。
	LeaseTTL      = 15 * time.Second
	RenewInterval = 5 * time.Second
)

// Options は neko の依存。
type Options struct {
	DB         *pgxpool.Pool
	Clock      clock.Clock
	InstanceID uuid.UUID
	// Tuner はチューナーサーバーのクライアント。nil なら EPG を同期しない。
	Tuner *mirakurun.Client
	// EPG は EPG の同期の設定。nil なら既定値を使う。
	EPG *epg.SyncerOptions
	// Archive は、放送が終わった番組のアーカイブの設定。nil なら既定値を使う。
	Archive *epg.ArchiverOptions
	// Tuners は、チューナーの状態の把握の設定。nil なら既定値を使う。
	Tuners *tunerstate.Options
	// Location は、ルールの曜日と時間帯を判定する、表示のタイムゾーン (設計書 8.1)。
	Location *time.Location
	// Rules は、ルールの照合の設定。nil なら既定値を使う。
	Rules *rule.RunnerOptions
}

// Run は、ctx が終わるまでリースを取り合い、リーダーになっている間だけ neko の処理を動かす。
// リーダーの間は、EPG を同期し (設計書 3.3)、放送が終わった番組をアーカイブに移し (設計書 3.4)、
// チューナーの構成とチューナーサーバーの状態を把握し (設計書 3.6、3.7)、ルールから予約を作る (設計書 4.3)。DB への書き込みは、その世代のリースを持っている間だけ通す。
func Run(ctx context.Context, logger *slog.Logger, opts *Options) error {
	manager := lease.NewManager(opts.DB, opts.Clock, LeaseName, opts.InstanceID, LeaseTTL)
	elector, err := lease.NewElector(manager, logger, RenewInterval)
	if err != nil {
		return err
	}

	return elector.Run(ctx, func(ctx context.Context, generation int64) error {
		guard := func(ctx context.Context, tx pgx.Tx) error {
			return manager.Guard(ctx, tx, generation)
		}
		epgLogger := logger.With(slog.String("component", "epg"))

		eg, ctx := errgroup.WithContext(ctx)
		eg.Go(func() error {
			return epg.NewArchiver(opts.DB, opts.Clock, guard, epgLogger, opts.Archive).Run(ctx)
		})
		eg.Go(func() error {
			reservations := reserve.NewReservations(opts.Clock, audit.NewRecorder(opts.Clock))
			engine := rule.NewEngine(opts.DB, opts.Clock, opts.Location, rule.Guard(guard), reservations)

			return rule.NewRunner(engine, opts.DB, logger.With(slog.String("component", "rules")), opts.Rules).Run(ctx)
		})
		if opts.Tuner == nil {
			logger.WarnContext(ctx, "tuner server is not configured; EPG sync and tuner monitoring are disabled")

			return eg.Wait()
		}
		monitor := tunerstate.NewMonitor(opts.DB, opts.Tuner, opts.Clock, tunerstate.Guard(guard), logger.With(slog.String("component", "tuners")), opts.Tuners)
		eg.Go(func() error {
			return monitor.Run(ctx)
		})
		eg.Go(func() error {
			var syncerOptions epg.SyncerOptions
			if opts.EPG != nil {
				syncerOptions = *opts.EPG
			}
			syncerOptions.OnTunerEvent = monitor.Trigger
			reconciler := epg.NewReconciler(opts.DB, opts.Tuner, opts.Clock, guard)

			return epg.NewSyncer(reconciler, opts.Tuner, opts.Clock, epgLogger, &syncerOptions).Run(ctx)
		})

		return eg.Wait()
	})
}
