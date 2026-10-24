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

	"github.com/SlashNephy/kotatsu/internal/assign"
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/lease"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/recording"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
	"github.com/SlashNephy/kotatsu/internal/settings"
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
	// Assign は、チューナーの割り当ての設定。nil なら既定値を使う。
	Assign *assign.RunnerOptions
	// Follow は、番組の変更への追従の設定。nil なら既定値を使う。
	Follow *reserve.FollowerOptions
	// Recording は、録画の割り振りの設定。nil なら既定値を使う。
	Recording *recording.SchedulerOptions
}

// Run は、ctx が終わるまでリースを取り合い、リーダーになっている間だけ neko の処理を動かす。
// リーダーの間は、EPG を同期し (設計書 3.3)、放送が終わった番組をアーカイブに移し (設計書 3.4)、
// チューナーの構成とチューナーサーバーの状態を把握し (設計書 3.6、3.7)、ルールから予約を作り (設計書 4.3)、
// 予約をチューナーに割り当て (設計書 4.5)、番組の変更に予約を追従させ (設計書 4.6)、録画を mikan に割り振る (設計書 1.5)。DB への書き込みは、その世代のリースを持っている間だけ通す。
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
			// 過去の番組の保持期間は、画面から変えられる (設計書 11.2) ので、消すたびに引く。
			archive := epg.ArchiverOptions{Grace: 0, Retention: 0, RetentionFunc: nil, Interval: 0, BatchSize: 0}
			if opts.Archive != nil {
				archive = *opts.Archive
			}
			if archive.RetentionFunc == nil {
				archive.RetentionFunc = func(ctx context.Context) (time.Duration, error) {
					system, err := settings.Load(ctx, opts.DB)
					if err != nil {
						return 0, err
					}

					return system.ArchiveRetention(), nil
				}
			}

			return epg.NewArchiver(opts.DB, opts.Clock, guard, epgLogger, &archive).Run(ctx)
		})
		eg.Go(func() error {
			reservations := reserve.NewReservations(opts.Clock, audit.NewRecorder(opts.Clock))
			engine := rule.NewEngine(opts.DB, opts.Clock, opts.Location, rule.Guard(guard), reservations)

			return rule.NewRunner(engine, opts.DB, logger.With(slog.String("component", "rules")), opts.Rules).Run(ctx)
		})
		eg.Go(func() error {
			reservations := reserve.NewReservations(opts.Clock, audit.NewRecorder(opts.Clock))

			return reserve.NewFollower(opts.DB, reserve.Guard(guard), logger.With(slog.String("component", "follow")), reservations, opts.Follow).Run(ctx)
		})
		eg.Go(func() error {
			// 録画設定の全体の既定値は、画面から変えられる (設計書 11.2) ので、割り当てのたびに引く。
			defaults := func(ctx context.Context, db sqlc.DBTX) (*reserve.Settings, error) {
				system, err := settings.Load(ctx, db)
				if err != nil {
					return nil, err
				}

				return system.Recording, nil
			}
			runner := assign.NewRunner(opts.DB, opts.Clock, assign.Guard(guard), logger.With(slog.String("component", "assign")), defaults, opts.Assign)

			return runner.Run(ctx)
		})
		eg.Go(func() error {
			scheduler := recording.NewScheduler(opts.DB, opts.Clock, recording.Guard(guard), logger.With(slog.String("component", "recording")), opts.Recording)

			return scheduler.Run(ctx)
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
