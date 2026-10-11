package cli

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/assign"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/config"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/httpserver"
	"github.com/SlashNephy/kotatsu/internal/jobs"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/metrics"
	"github.com/SlashNephy/kotatsu/internal/neko"
	"github.com/SlashNephy/kotatsu/internal/recording"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
	"github.com/SlashNephy/kotatsu/internal/settings"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

// Deps は、ロールを起動する前に用意し、全ロールで共有するもの。
type Deps struct {
	Config *config.Config
	// DB は、スキーマのバージョンを確かめた後の接続。
	DB *pgxpool.Pool
	// Metrics は /metrics で公開するレジストリ。各ロールのメトリクスはここに登録する。
	Metrics *prometheus.Registry
	Clock   clock.Clock
	// InstanceID は、このプロセスのインスタンスの ID。リースの持ち主や、インスタンスの登録簿に使う。
	InstanceID uuid.UUID
	// Jobs は River のクライアント。ジョブはドメインの書き込みと同じトランザクションで InsertTx で投入する。
	Jobs *jobs.Client
	// JobWorkers は、Jobs で実行するジョブの Worker。nekonote を動かさないプロセスや、Worker がないときは nil。
	JobWorkers *river.Workers
}

// defaultTimeZone は、表示のタイムゾーンの既定値 (設計書 8.1)。
const defaultTimeZone = "Asia/Tokyo"

// tenbanRoleName は、API を受け持つロールの名前。
const tenbanRoleName = "tenban"

// nekonoteRoleName は、ジョブを実行するロールの名前。
const nekonoteRoleName = "nekonote"

// kyusuRoleName は、サーバー側でのトランスコードを受け持つロールの名前。
const kyusuRoleName = "kyusu"

// Runner はロールの本体。ctx が終わるまで動き続ける。
type Runner func(ctx context.Context, logger *slog.Logger, deps *Deps) error

// Role は kotatsu のサブコマンドで起動できるロール。
type Role struct {
	// Name はコードネーム (neko など)。
	Name string
	// Alias は役割名 (scheduler など)。サブコマンドとしても受け付ける。
	Alias string
	// Short は --help に出す説明。
	Short string
	Run   Runner
}

// DisplayName は --help とログに出す名前。役割名を必ず併記する。
func (r *Role) DisplayName() string {
	return r.Name + " (" + r.Alias + ")"
}

// DefaultRoles は全ロールを返す。呼び出しのたびに新しい値を返すので、呼び出し側で書き換えてよい。
// neko 以外のロールの本体は未実装で、起動ログを出して終了を待つだけである。
func DefaultRoles() []*Role {
	return []*Role{
		{
			Name:  tenbanRoleName,
			Alias: "server",
			Short: "Serve the GraphQL API, the web UI, and media streams",
			Run:   waitUntilDone,
		},
		{
			Name:  "neko",
			Alias: "scheduler",
			Short: "Sync the EPG, match rules, and assign recordings (one active instance)",
			Run:   runNeko,
		},
		{
			Name:  "mikan",
			Alias: "recorder",
			Short: "Record broadcast streams to files",
			Run:   runMikan,
		},
		{
			Name:  nekonoteRoleName,
			Alias: "worker",
			Short: "Run background jobs such as thumbnails and notifications",
			Run:   runNekonote,
		},
		{
			Name:  kyusuRoleName,
			Alias: "transcoder",
			Short: "Transcode streams on demand",
			Run:   waitUntilDone,
		},
	}
}

func runNeko(ctx context.Context, logger *slog.Logger, deps *Deps) error {
	logger.InfoContext(ctx, "role started")
	defer logger.InfoContext(ctx, "role stopped")

	var tuner *mirakurun.Client
	if deps.Config.Tuner.URL.Value() != "" {
		var err error
		if tuner, err = newTunerClient(deps); err != nil {
			return err
		}
	}

	// 表示のタイムゾーンは設定で決める (設計書 8.1) が、設定の項目はまだないので、既定の Asia/Tokyo を使う。
	location, err := time.LoadLocation(defaultTimeZone)
	if err != nil {
		return fmt.Errorf("failed to load the time zone %s: %w", defaultTimeZone, err)
	}

	// EPG の同期の遅れは、最後に全量を突き合わせた時刻から求める (設計書 11.3)。リーダーでない間は 0 のままにする。
	lastFullSync := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metrics.Namespace, Name: "epg_last_full_sync_timestamp_seconds",
		Help: "Unix time when this instance last synced the whole EPG as the neko leader.",
	})
	deps.Metrics.MustRegister(lastFullSync)

	return neko.Run(ctx, logger, &neko.Options{
		DB:         deps.DB,
		Clock:      deps.Clock,
		InstanceID: deps.InstanceID,
		Tuner:      tuner,
		EPG: &epg.SyncerOptions{
			FullSyncInterval: 0, MinFullSyncInterval: 0, BatchWindow: 0, BackoffInitial: 0, BackoffMax: 0, StableAfter: 0, OnTunerEvent: nil,
			OnFullSync: func(at time.Time) { lastFullSync.Set(float64(at.Unix())) },
		},
		Archive:  nil,
		Tuners:   nil,
		Location: location,
		Rules: &rule.RunnerOptions{
			Debounce:     0,
			Interval:     0,
			PollOnly:     deps.Config.Database.Pooler,
			PollInterval: 0,
		},
		Assign: &assign.RunnerOptions{
			Debounce:     0,
			Interval:     0,
			PollOnly:     deps.Config.Database.Pooler,
			PollInterval: 0,
		},
		Follow: &reserve.FollowerOptions{
			Debounce:     0,
			Interval:     0,
			PollOnly:     deps.Config.Database.Pooler,
			PollInterval: 0,
		},
		Recording: &recording.SchedulerOptions{
			Lead:         0,
			Interval:     0,
			Debounce:     0,
			PollOnly:     deps.Config.Database.Pooler,
			PollInterval: 0,
		},
	})
}

// runNekonote は、ctx が終わるまで River のジョブを実行する。止まるときは、実行中のジョブが終わるのを待つ。
func runNekonote(ctx context.Context, logger *slog.Logger, deps *Deps) error {
	logger.InfoContext(ctx, "role started")
	defer logger.InfoContext(ctx, "role stopped")

	if deps.JobWorkers == nil {
		logger.InfoContext(ctx, "no job workers are registered yet")
		<-ctx.Done()

		return nil
	}

	if err := deps.Jobs.Start(ctx); err != nil {
		return fmt.Errorf("failed to start the River client: %w", err)
	}
	<-ctx.Done()

	// 終了のシグナルで ctx は終わっているので、実行中のジョブを待つための context を別に作る。
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), httpserver.ShutdownTimeout)
	defer cancel()
	if err := deps.Jobs.Stop(stopCtx); err != nil {
		return fmt.Errorf("failed to stop the River client gracefully: %w", err)
	}

	return nil
}

func waitUntilDone(ctx context.Context, logger *slog.Logger, _ *Deps) error {
	logger.InfoContext(ctx, "role started")
	<-ctx.Done()
	logger.InfoContext(ctx, "role stopped")

	return nil
}

// newTunerClient は、設定したチューナーサーバーのクライアントを返す。
func newTunerClient(deps *Deps) (*mirakurun.Client, error) {
	return mirakurun.New(&mirakurun.Options{
		URL:            deps.Config.Tuner.URL.Value(),
		Username:       "",
		Password:       "",
		UserAgent:      "",
		HTTPClient:     nil,
		RequestTimeout: 0,
		DetectInterval: 0,
		Clock:          deps.Clock,
	})
}

// runMikan は、ctx が終わるまで、割り振りを取って録画する (設計書 1.5、5 章)。
// 止めるときは、録画中の割り振りを残し、ほかの mikan か、再起動した mikan に続きを任せる。
func runMikan(ctx context.Context, logger *slog.Logger, deps *Deps) error {
	logger.InfoContext(ctx, "role started")
	defer logger.InfoContext(ctx, "role stopped")

	if deps.Config.Tuner.URL.Value() == "" {
		// neko と同じく、チューナーサーバーを設定するまでは、何もせずに待つ。
		logger.WarnContext(ctx, "tuner server is not configured; recording is disabled")
		<-ctx.Done()

		return nil
	}
	tuner, err := newTunerClient(deps)
	if err != nil {
		return err
	}
	// 表示のタイムゾーンは設定で決める (設計書 8.1) が、設定の項目はまだないので、既定の Asia/Tokyo を使う。
	location, err := time.LoadLocation(defaultTimeZone)
	if err != nil {
		return fmt.Errorf("failed to load the time zone %s: %w", defaultTimeZone, err)
	}
	// 録画設定の全体の既定値は、画面から変えられる (設計書 11.2) ので、録画のたびに引く。
	defaults := func(ctx context.Context, db sqlc.DBTX) (*reserve.Settings, error) {
		system, err := settings.Load(ctx, db)
		if err != nil {
			return nil, err
		}

		return system.Recording, nil
	}
	recorder := recording.NewRecorder(deps.DB, deps.Clock, tuner, kago.New(deps.Config.Storage.Pools, nil), defaults, location,
		logger.With(slog.String("component", "recorder")), nil)
	worker := recording.NewWorker(deps.DB, deps.Clock, deps.InstanceID, logger, recorder.Session(), &recording.WorkerOptions{
		LeaseTTL: 0, RenewInterval: 0, AcquireInterval: 0, HandoverTimeout: 0, PollOnly: deps.Config.Database.Pooler, PollInterval: 0,
	})

	return worker.Run(ctx)
}
