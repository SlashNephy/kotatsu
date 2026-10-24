package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/errgroup"

	"github.com/SlashNephy/kotatsu/internal/analysis"
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/avatar"
	"github.com/SlashNephy/kotatsu/internal/capacity"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/infra/registry"
	"github.com/SlashNephy/kotatsu/internal/infra/telemetry"
	"github.com/SlashNephy/kotatsu/internal/infra/update"
	"github.com/SlashNephy/kotatsu/internal/jikkyo"
	"github.com/SlashNephy/kotatsu/internal/kyusu"
	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/password"
	"github.com/SlashNephy/kotatsu/internal/metrics"
	"github.com/SlashNephy/kotatsu/internal/notification"
	"github.com/SlashNephy/kotatsu/internal/oauth"
	"github.com/SlashNephy/kotatsu/internal/player"
	"github.com/SlashNephy/kotatsu/internal/segjoin"
	"github.com/SlashNephy/kotatsu/internal/settings"
	"github.com/SlashNephy/kotatsu/internal/stream"
	"github.com/SlashNephy/kotatsu/internal/thumbnail"
	"github.com/SlashNephy/kotatsu/internal/watch"
	"github.com/SlashNephy/kotatsu/internal/web/epgstationapi"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
	"github.com/SlashNephy/kotatsu/internal/web/mcpserver"
	"github.com/SlashNephy/kotatsu/internal/web/spa"
	"github.com/SlashNephy/kotatsu/internal/web/webui"
	"github.com/SlashNephy/kotatsu/internal/webhook"
	"github.com/SlashNephy/kotatsu/internal/webpush"
)

const (
	// displayNameAnnotation は、--help のコマンド一覧に Name の代わりに出す名前を持つ Annotation のキー。
	displayNameAnnotation = "kotatsu.displayName"
	// configPathEnv は、--config を省略したときに設定ファイルのパスを引く環境変数。
	configPathEnv = "KOTATSU_CONFIG"
)

// Options は NewCommand に渡す依存。テストでは DB の操作を差し替える。
type Options struct {
	Logger    *slog.Logger
	Roles     []*Role
	LookupEnv config.LookupEnv
	Clock     clock.Clock

	OpenDatabase func(ctx context.Context, cfg *config.Database) (*pgxpool.Pool, error)
	// CheckSchema は、ロールを起動する前に、DB のスキーマでこのバイナリが動けるかを確かめる。
	CheckSchema func(ctx context.Context, pool *pgxpool.Pool) error
	// Migrate は kotatsu migrate の本体。
	Migrate func(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error
	// RunRegistry は、ctx が終わるまで roles のインスタンスを登録簿に載せておく。
	// listenAddr は API の HTTP サーバーが実際に待ち受けているアドレス。
	RunRegistry func(ctx context.Context, logger *slog.Logger, deps *Deps, roles []string, listenAddr string) error
	// JobWorkers は、nekonote で実行するジョブの Worker と定期ジョブを返す。
	JobWorkers func(deps *Deps, logger *slog.Logger) (*river.Workers, []*river.PeriodicJob, error)
	// WatchChanges は、tenban が subscription に知らせる変更の通知を見張るものを作る。
	WatchChanges func(ctx context.Context, logger *slog.Logger, deps *Deps) (ChangeFeed, error)
	// LogSetupToken は、all の起動時に、管理者がいなければセットアップトークンを発行してログに出す (設計書 9.2)。
	LogSetupToken func(ctx context.Context, logger *slog.Logger, clk clock.Clock, pool *pgxpool.Pool) error
	// PasswordStrengthTimeout は、パスワードの強度の判定の時間の上限。0 なら既定値にする。
	// テストは、-race で遅くなるので長くする。
	PasswordStrengthTimeout time.Duration
}

// DefaultOptions は、本番で使う依存を返す。
func DefaultOptions(logger *slog.Logger) *Options {
	return &Options{
		Logger:       logger,
		Roles:        DefaultRoles(),
		LookupEnv:    os.LookupEnv,
		Clock:        new(clock.Real),
		OpenDatabase: database.Open,
		CheckSchema:  checkSchema,
		Migrate:      migrate,
		RunRegistry:  runRegistry,
		JobWorkers:   jobWorkers,
		WatchChanges: watchChanges,

		LogSetupToken:           logSetupToken,
		PasswordStrengthTimeout: 0,
	}
}

// jobWorkers は、nekonote で実行するジョブの Worker と定期ジョブを返す。ジョブを足したら、ここに登録する。
func jobWorkers(deps *Deps, logger *slog.Logger) (*river.Workers, []*river.PeriodicJob, error) {
	workers := river.NewWorkers()
	retention := time.Duration(deps.Config.Audit.RetentionDays) * 24 * time.Hour
	river.AddWorker(workers, audit.NewPruneWorker(deps.DB, deps.Clock, retention))
	river.AddWorker(workers, auth.NewPruneSessionsWorker(deps.DB, deps.Clock))
	// MCP のクライアントに発行した、期限を過ぎた認可コードとトークンを消す (#963)。
	river.AddWorker(workers, oauth.NewPruneWorker(deps.DB, deps.Clock))
	// 配り終えた通知は、画面から変えられる保持期間を過ぎたら消す (#551)。
	river.AddWorker(workers, notification.NewPruneWorker(deps.DB, deps.Clock, func(ctx context.Context) (time.Duration, error) {
		system, err := settings.Load(ctx, deps.DB)
		if err != nil {
			return 0, err
		}

		return system.NotificationRetention(), nil
	}))

	// Webhook の配信のジョブは、イベントを配るトランザクションの中で投入する。
	// 投入に使う River のクライアントは、ここで作る Worker を必要とするので、投入だけを行うクライアントを別に作る。
	inserter, err := jobs.NewClient(deps.DB, logger, &jobs.Options{PollOnly: deps.Config.Database.Pooler, Workers: nil, PeriodicJobs: nil})
	if err != nil {
		return nil, nil, err
	}
	webhooks, err := webhook.NewStore(deps.Clock, audit.NewRecorder(deps.Clock), deps.Config.Auth.SecretKey.Value())
	if err != nil {
		return nil, nil, err
	}
	// 表示のタイムゾーンは設定で決める (設計書 8.1)。
	location, err := deps.Config.Location()
	if err != nil {
		return nil, nil, err
	}
	webPush, err := webpush.NewService(deps.Clock, deps.Config.Auth.SecretKey.Value())
	if err != nil {
		return nil, nil, err
	}
	dispatcher := notification.NewDispatcher(deps.DB, deps.Clock,
		webhook.NewEnqueuer(inserter), webpush.NewEnqueuer(inserter), segjoin.NewEnqueuer(inserter))
	river.AddWorker(workers, notification.NewDispatchWorker(dispatcher))
	river.AddWorker(workers, webhook.NewDeliverWorker(deps.DB, webhooks, deps.Clock, location))
	river.AddWorker(workers, webpush.NewDeliverWorker(deps.DB, webPush, deps.Clock, webPushSubject(deps.Config), location, webpush.NewHTTPClient()))

	// 保存先プールの容量は、このインスタンスにマウントされたプールについて調べる (設計書 6.1)。
	manager := capacity.NewManager(deps.DB, deps.Clock, storeOf(deps.Config), audit.NewRecorder(deps.Clock),
		logger.With(slog.String("component", "capacity")))
	river.AddWorker(workers, capacity.NewCheckWorker(manager))
	// 録画を終えたら、代表画像とプレビュー画像を作る (設計書 6.1)。FFmpeg がなければ、ジョブが理由を残して取りやめる。
	river.AddWorker(workers, thumbnail.NewGenerateWorker(deps.DB, storeOf(deps.Config), deps.Clock,
		!deps.Config.Thumbnail.DisablePreview))
	// 取り込んだ録画は、録画ファイルを読み直して索引と集計を作ってから、画像を作るジョブに渡す (設計書 6.3)。
	river.AddWorker(workers, analysis.NewAnalyzeWorker(deps.DB, storeOf(deps.Config), deps.Clock, inserter))
	// 引き継ぎや再起動で分かれた録画のファイルは、録画が終わったら 1 つにつなぐ (設計書 5.4)。
	// つないだあとは、索引と集計を作り直すために、解析のジョブに渡す。
	river.AddWorker(workers, segjoin.NewJoinWorker(deps.DB, storeOf(deps.Config), inserter,
		logger.With(slog.String("component", "segjoin")), 0))
	river.AddWorker(workers, segjoin.NewFinalizeWorker(deps.DB, storeOf(deps.Config), inserter))
	// 視聴予約は、放送の少し前に、予約したユーザーに通知する。通知を押すと、そのサービスのライブ視聴を開く (設計書 4.8)。
	river.AddWorker(workers, watch.NewNotifyWorker(watch.NewNotifier(deps.DB, deps.Clock, graph.ServiceLiveURL)))

	return workers, []*river.PeriodicJob{
		audit.PrunePeriodicJob(), auth.PruneSessionsPeriodicJob(), notification.DispatchPeriodicJob(), notification.PrunePeriodicJob(), capacity.CheckPeriodicJob(),
		watch.NotifyPeriodicJob(), oauth.PrunePeriodicJob(),
	}, nil
}

// defaultWebPushSubject は、Web Push の送り手の連絡先を設定していないときに、プッシュサービスに示す URI。
const defaultWebPushSubject = "https://github.com/SlashNephy/kotatsu"

// webPushSubject は、プッシュサービスに示す送り手の連絡先を返す。
func webPushSubject(cfg *config.Config) string {
	if cfg.WebPush.Subject == "" {
		return defaultWebPushSubject
	}

	return cfg.WebPush.Subject
}

// migrate は、Kotatsu のスキーマと River のテーブルのマイグレーションを適用する。
// 同時に実行されても安全なよう、両方をマイグレーションのロックの中で行う。
func migrate(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	return database.WithMigrationLock(ctx, logger, pool, func(ctx context.Context) error {
		if err := database.Migrate(ctx, logger, pool); err != nil {
			return err
		}

		return jobs.Migrate(ctx, logger, pool)
	})
}

// checkSchema は、Kotatsu のスキーマと River のテーブルで、このバイナリが動けるかを確かめる。
func checkSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if err := database.CheckSchema(ctx, pool); err != nil {
		return err
	}

	return jobs.CheckSchema(ctx, pool)
}

func runRegistry(ctx context.Context, logger *slog.Logger, deps *Deps, roles []string, listenAddr string) error {
	address, err := advertiseAddress(listenAddr, deps.Config.Server.AdvertiseHost)
	if err != nil {
		return err
	}

	return registry.New(deps.DB, deps.Clock, deps.InstanceID).Run(ctx, logger, roles, address)
}

// advertiseAddress は、他のインスタンスからこのインスタンスに届くアドレスを、待ち受けのアドレスから決める。
// advertiseHost があれば、そのホストを使う。なければ、待ち受けのホストを使い、それが空か 0.0.0.0 や :: などの全アドレスなら、
// ホスト名を使う。ポートに 0 を指定したときも実際に割り当てられたポートを載せるよう、設定値ではなく待ち受けたアドレスを渡す。
func advertiseAddress(listen, advertiseHost string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("invalid listen address %s: %w", listen, err)
	}
	if advertiseHost != "" {
		return net.JoinHostPort(advertiseHost, port), nil
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host, err = os.Hostname()
		if err != nil {
			return "", fmt.Errorf("failed to get the hostname: %w", err)
		}
	}

	return net.JoinHostPort(host, port), nil
}

// NewCommand は kotatsu のルートコマンドを組み立てる。
func NewCommand(opts *Options) *cobra.Command {
	root := &cobra.Command{
		Use:     "kotatsu",
		Short:   "Kotatsu is a web application to reserve, record, and watch Japanese TV broadcasts",
		Version: buildinfo.Get().String(),
		// エラーは main でログに出す。cobra にも出させると二重になる。
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	var configPath string
	root.PersistentFlags().StringVar(
		&configPath,
		"config",
		"",
		"path to the YAML config file (defaults to $"+configPathEnv+"; optional when every required setting is given by environment variables)",
	)

	// loadConfig は、--config か環境変数の設定ファイルと、環境変数から設定を読み込む。
	loadConfig := func(ctx context.Context) (*config.Config, error) {
		path := configPath
		if path == "" {
			path, _ = opts.LookupEnv(configPathEnv)
		}

		cfg, err := config.Load(path, opts.LookupEnv)
		if err != nil {
			return nil, fmt.Errorf("failed to load the config: %w", err)
		}
		build := buildinfo.Get()
		opts.Logger.InfoContext(ctx, "kotatsu version",
			slog.String("version", build.Version), slog.String("revision", build.Revision), slog.Bool("modified", build.Modified))
		opts.Logger.InfoContext(ctx, "config loaded", slog.String("path", path), slog.Any("config", cfg))

		return cfg, nil
	}

	// connect は設定を読み込み、DB に接続する。返した pool は呼び出し側で閉じる。
	connect := func(ctx context.Context) (*config.Config, *pgxpool.Pool, error) {
		cfg, err := loadConfig(ctx)
		if err != nil {
			return nil, nil, err
		}

		pool, err := opts.OpenDatabase(ctx, &cfg.Database)
		if err != nil {
			return nil, nil, err
		}

		return cfg, pool, nil
	}

	// start は targets のロールを起動する。issueSetupToken なら、管理者がいないときに、セットアップトークンを発行してログに出す。
	start := func(ctx context.Context, targets []*Role, issueSetupToken bool) error {
		cfg, pool, err := connect(ctx)
		if err != nil {
			return err
		}
		defer closePool(pool)

		if err := opts.CheckSchema(ctx, pool); err != nil {
			return fmt.Errorf("refusing to start: %w", err)
		}
		if issueSetupToken {
			if err := opts.LogSetupToken(ctx, opts.Logger, opts.Clock, pool); err != nil {
				return err
			}
		}

		deps := &Deps{
			Config:     cfg,
			DB:         pool,
			Metrics:    telemetry.NewRegistry(),
			Clock:      opts.Clock,
			InstanceID: id.New(),
			Jobs:       nil,
			JobWorkers: nil,
		}
		// 録画、チューナー、ジョブのキューの状態は、DB から読む (設計書 11.3)。
		deps.Metrics.MustRegister(metrics.NewDBCollector(pool, opts.Clock, deps.InstanceID, opts.Logger.With(slog.String("component", "metrics"))))

		// ジョブを実行するのは nekonote だけで、他のロールは投入だけを行う。
		periodicJobs := []*river.PeriodicJob{}
		if slices.ContainsFunc(targets, func(role *Role) bool { return role.Name == nekonoteRoleName }) {
			// Webhook の署名用の秘密キーは、アプリの秘密キーで暗号化して保存する (設計書 8.4)。秘密キーがなければ、署名できない。
			if cfg.Auth.SecretKey.Value() == "" {
				return errSecretKeyRequired
			}
			deps.JobWorkers, periodicJobs, err = opts.JobWorkers(deps, opts.Logger)
			if err != nil {
				return err
			}
		}
		deps.Jobs, err = jobs.NewClient(pool, opts.Logger, &jobs.Options{
			PollOnly:     cfg.Database.Pooler,
			Workers:      deps.JobWorkers,
			PeriodicJobs: periodicJobs,
		})
		if err != nil {
			return err
		}

		return serve(ctx, opts, deps, targets)
	}

	for _, role := range opts.Roles {
		root.AddCommand(&cobra.Command{
			Use:         role.Name,
			Aliases:     []string{role.Alias},
			Short:       role.Short,
			Args:        cobra.NoArgs,
			Annotations: map[string]string{displayNameAnnotation: role.DisplayName()},
			RunE: func(cmd *cobra.Command, _ []string) error {
				return start(cmd.Context(), []*Role{role}, false)
			},
		})
	}

	root.AddCommand(&cobra.Command{
		Use:   "all",
		Short: "Run all roles in a single process",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// all の構成では、初回セットアップのトークンを起動時にログへ出す (設計書 9.2)。
			return start(cmd.Context(), opts.Roles, true)
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "migrate",
		Short: "Apply database migrations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, pool, err := connect(cmd.Context())
			if err != nil {
				return err
			}
			defer closePool(pool)

			return opts.Migrate(cmd.Context(), opts.Logger, pool)
		},
	})

	root.AddCommand(newImportCommand(opts, loadConfig, connect))
	root.AddCommand(newUserCommand(opts, connect))
	root.AddCommand(newAdminCommand(opts, connect))

	// cobra の既定のテンプレートはコマンド一覧に Name だけを出すので、役割名を併記した名前に差し替える。
	// NamePadding は Name の長さから計算されるため、併記した名前に合わせた固定幅にする。
	root.SetUsageTemplate(strings.Replace(
		root.UsageTemplate(),
		"{{rpad .Name .NamePadding }}",
		`{{rpad (or (index .Annotations "`+displayNameAnnotation+`") .Name) 24}}`,
		1,
	))

	return root
}

// serve は、roles と HTTP サーバーを並行に動かす。
//
// HTTP サーバーは 2 つある。server.listen では /healthz と /readyz を、metrics.listen では /metrics を公開する。
// どれかが失敗すると、残りも止める。ロールがすべて終わったときも、HTTP サーバーを止めて終わる。
func serve(ctx context.Context, opts *Options, deps *Deps, roles []*Role) error {
	logger := opts.Logger
	tracerProvider, shutdownTracing, err := telemetry.NewTracerProvider(ctx, &deps.Config.Telemetry)
	if err != nil {
		return err
	}
	defer func() {
		// 終了のシグナルで ctx は終わっているので、送り残したトレースを送るための context を別に作る。
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), httpserver.ShutdownTimeout)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			logger.WarnContext(shutdownCtx, "failed to flush traces", slog.Any("error", err))
		}
	}()

	apiListener, err := httpserver.Listen(ctx, deps.Config.Server.Listen)
	if err != nil {
		return err
	}
	metricsListener, err := httpserver.Listen(ctx, deps.Config.Metrics.Listen)
	if err != nil {
		_ = apiListener.Close()

		return err
	}

	mux := http.NewServeMux()
	httpserver.RegisterHealth(mux, logger, deps.DB)
	var api *apiHandler
	// API は tenban が受け持つ (設計書 1.2、9.1)。
	if slices.ContainsFunc(roles, func(role *Role) bool { return role.Name == tenbanRoleName }) {
		api, err = newAPI(ctx, logger, deps, opts.WatchChanges)
		if err != nil {
			_ = apiListener.Close()
			_ = metricsListener.Close()

			return err
		}
		// ヘルスチェックは、Kubernetes などがプライベートでないアドレスから叩くこともあるので、接続元の制限に含めない。
		mux.Handle("/", api.handler)
	}
	var live *kyusu.Live
	var vod *kyusu.VOD
	// サーバー側でのトランスコードは kyusu が受け持つ (設計書 7.3)。tenban が、インスタンスの登録簿で引いて中継する。
	if slices.ContainsFunc(roles, func(role *Role) bool { return role.Name == kyusuRoleName }) {
		var token *kyusu.Token
		live, vod, token, err = newKyusu(ctx, logger, deps)
		if err != nil {
			_ = apiListener.Close()
			_ = metricsListener.Close()

			return err
		}
		mux.Handle(kyusu.LivePathPrefix, live.Handler(token))
		mux.Handle(kyusu.RecordingPathPrefix, vod.Handler(token))
	}
	handler := otelhttp.NewHandler(mux, "kotatsu", otelhttp.WithTracerProvider(tracerProvider))
	metricsHandler := telemetry.NewMetricsHandler(deps.Metrics)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		defer cancel()

		return runRoles(ctx, logger, deps, roles)
	})
	eg.Go(func() error {
		return httpserver.Serve(ctx, logger.With(slog.String("server", "api")), apiListener, handler)
	})
	if api != nil {
		eg.Go(func() error {
			return api.changes.Run(ctx)
		})
		if api.updates != nil {
			eg.Go(func() error {
				return api.updates.Run(ctx)
			})
		}
	}
	if live != nil {
		eg.Go(func() error {
			return live.Run(ctx)
		})
		eg.Go(func() error {
			return vod.Run(ctx)
		})
	}
	eg.Go(func() error {
		names := make([]string, 0, len(roles))
		for _, role := range roles {
			// トランスコードできない kyusu は、登録簿に載せない。画面は、登録簿に kyusu がいるかで、
			// サーバー側でのトランスコードを使えるかを判断する (設計書 7.1)。
			if role.Name == kyusuRoleName && (live == nil || !live.CanTranscode()) {
				continue
			}
			names = append(names, role.Name)
		}

		return opts.RunRegistry(ctx, logger, deps, names, apiListener.Addr().String())
	})
	eg.Go(func() error {
		return httpserver.Serve(ctx, logger.With(slog.String("server", "metrics")), metricsListener, metricsHandler)
	})

	return eg.Wait()
}

// newAPI は、tenban の API と SPA のハンドラーを返す。
//
// どのリクエストも、まず接続元を確かめる (設計書 9.5)。
// API は、ログイン中のユーザーをサーバー側のセッションから引く。/auth/* の状態を変えるリクエストは、Origin が同じでなければ拒否する。
// SPA は、ログイン画面も含むので、セッションを引かずに配信する。
// ChangeFeed は、変更の通知を見張り、subscription の購読者に知らせる。API を配信する間、Run で動かし続ける。
type ChangeFeed interface {
	graph.Changes
	Run(ctx context.Context) error
}

var _ ChangeFeed = new(notify.Broadcaster)

// apiHandler は、tenban の API のハンドラーと、subscription に変更を知らせる ChangeFeed。
type apiHandler struct {
	handler http.Handler
	changes ChangeFeed
	// updates は、新しいリリースを確かめる。確かめない設定なら nil。
	updates *update.Checker
}

var _ graph.Updates = new(update.Checker)

// newUpdates は、新しいリリースを確かめる Checker と、GraphQL に渡すそれを返す。
// 外部への通信になるので、設定で入れたときだけ確かめる (設計書 11.1、#506)。入れていなければ、どちらも nil。
// GraphQL には、nil の *update.Checker でなく nil のインターフェースを渡す。
func newUpdates(logger *slog.Logger, deps *Deps) (*update.Checker, graph.Updates) {
	if !deps.Config.UpdateCheck.Enabled {
		return nil, nil
	}
	checker := update.NewChecker(&update.Options{
		Current: buildinfo.Get().Version, URL: "", Interval: 0, HTTPClient: nil, Logger: logger.With(slog.String("component", "update")),
	})

	return checker, checker
}

// watchChanges は、subscription で知らせるデータの変更の通知を見張る。
func watchChanges(ctx context.Context, logger *slog.Logger, deps *Deps) (ChangeFeed, error) {
	watcher, err := notify.NewWatcher(ctx, deps.DB, logger.With(slog.String("component", "changes")), graph.WatchedChannels(), notify.WatcherOptions{
		UseListen: !deps.Config.Database.Pooler, PollInterval: notify.DefaultPollInterval,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to watch changes: %w", err)
	}

	return notify.NewBroadcaster(watcher), nil
}

func newAPI(ctx context.Context, logger *slog.Logger, deps *Deps, watch func(context.Context, *slog.Logger, *Deps) (ChangeFeed, error)) (*apiHandler, error) {
	policy, err := httpserver.NewAccessPolicy(&deps.Config.Server, logger.With(slog.String("component", "access")))
	if err != nil {
		return nil, err
	}
	sessions := auth.NewSessions(&auth.SessionsOptions{
		IdleTimeout:     0,
		AbsoluteTimeout: 0,
		// リバースプロキシが HTTPS を終端するときも、信頼済みのプロキシの X-Forwarded-Proto で Secure を付ける。
		IsSecure: func(r *http.Request) bool {
			client := httpserver.ClientFrom(r.Context())

			return client != nil && client.Secure
		},
		Clock: deps.Clock,
	})

	// 表示のタイムゾーンは設定で決める (設計書 8.1)。
	location, err := deps.Config.Location()
	if err != nil {
		return nil, err
	}

	// TOTP の秘密キーは、アプリの秘密キーで暗号化して保存する (設計書 8.4)。秘密キーがなければ、ログインを受け付けられない。
	if deps.Config.Auth.SecretKey.Value() == "" {
		return nil, errSecretKeyRequired
	}
	factors, err := auth.NewSecondFactors(deps.Clock, audit.NewRecorder(deps.Clock), deps.Config.Auth.SecretKey.Value())
	if err != nil {
		return nil, err
	}
	passkeys := auth.NewPasskeys(&auth.PasskeysOptions{
		Clock:    deps.Clock,
		Recorder: audit.NewRecorder(deps.Clock),
		Logger:   logger.With(slog.String("component", "passkey")),
		Sessions: sessions,
	})
	passwordPolicy, err := password.NewPolicy(&password.PolicyOptions{HIBP: hibpOf(deps.Config), StrengthTimeout: 0, Estimator: nil, Logger: logger})
	if err != nil {
		return nil, err
	}
	webhooks, err := webhook.NewStore(deps.Clock, audit.NewRecorder(deps.Clock), deps.Config.Auth.SecretKey.Value())
	if err != nil {
		return nil, err
	}
	// VAPID の鍵は、ブラウザが購読するために公開鍵を求めたときか、初めて通知を送るときに作る。
	webPush, err := webpush.NewService(deps.Clock, deps.Config.Auth.SecretKey.Value())
	if err != nil {
		return nil, err
	}
	// subscription は、どの tenban で起きた変更も届けるため、変更の通知のチャンネルを見張る (設計書 1.4、9.1)。
	changes, err := watch(ctx, logger, deps)
	if err != nil {
		return nil, err
	}
	// 外部プレイヤー用の映像の URL には、アプリの秘密キーから導いたキーで署名する (設計書 9.6)。
	signer, err := stream.NewSigner(deps.Config.Auth.SecretKey.Value())
	if err != nil {
		return nil, err
	}
	// 実況のチャンネルの対応表と、NX-Jikkyo と過去ログ API への問い合わせ (設計書 7.4)。
	jikkyoTable, err := jikkyo.NewTable(deps.Config.Jikkyo.Channels)
	if err != nil {
		return nil, err
	}
	jikkyoClient := jikkyo.NewClient(&jikkyo.ClientOptions{NXJikkyoURL: "", KakologURL: "", HTTPClient: nil, Clock: deps.Clock})
	updates, graphUpdates := newUpdates(logger, deps)
	apiTokens := auth.NewAPITokens(deps.DB, deps.Clock, audit.NewRecorder(deps.Clock))
	authorizationServer := oauth.NewServer(deps.DB, deps.Clock, oauth.NewFetcher(deps.Clock), audit.NewRecorder(deps.Clock))
	mux := http.NewServeMux()
	// 変更の購読 (SSE) は終わらないので、停止を始めたら打ち切る。
	graphQL := graph.NewHandler(&graph.Options{
		DB:                deps.DB,
		Recorder:          audit.NewRecorder(deps.Clock),
		SecondFactors:     factors,
		Passkeys:          passkeys,
		APITokens:         apiTokens,
		OAuth:             authorizationServer,
		Players:           player.NewService(deps.DB, deps.Clock),
		UserAdmin:         auth.NewUserAdmin(deps.Clock, audit.NewRecorder(deps.Clock), passwordPolicy),
		Passwords:         auth.NewPasswords(deps.Clock, audit.NewRecorder(deps.Clock), passwordPolicy),
		Sessions:          sessions,
		Settings:          settings.NewStore(deps.Clock, audit.NewRecorder(deps.Clock)),
		Webhooks:          webhooks,
		WebPush:           webPush,
		StreamSigner:      signer,
		JikkyoTable:       jikkyoTable,
		JikkyoClient:      jikkyoClient,
		Changes:           changes,
		Pools:             deps.Config.Storage.Pools,
		DataDir:           deps.Config.Storage.DataDir,
		FreeSpace:         nil,
		Jobs:              deps.Jobs,
		TotalSpace:        nil,
		Clock:             deps.Clock,
		Location:          location,
		Logger:            logger.With(slog.String("component", "graphql")),
		MaxDepth:          0,
		MaxComplexity:     0,
		HeartbeatInterval: 0,
		Updates:           graphUpdates,
	})
	mux.Handle("/graphql", httpserver.StopEventStreamsOnShutdown(graphQL))
	mux.Handle("POST /auth/logout", httpserver.RequireSameOrigin(sessions.LogoutHandler(deps.DB)))
	logins := auth.NewLogins(&auth.LoginsOptions{
		Clock:         deps.Clock,
		Recorder:      audit.NewRecorder(deps.Clock),
		Logger:        logger.With(slog.String("component", "login")),
		Sessions:      sessions,
		SecondFactors: factors,
		ChallengeTTL:  0,
		// パスワードを同時に確かめる数は、CPU の数までにする (#542)。
		MaxConcurrentVerifications: 0,
		VerificationWait:           0,
	})
	setup := auth.NewSetup(deps.Clock, audit.NewRecorder(deps.Clock), passwordPolicy, logger.With(slog.String("component", "setup")))
	mux.Handle("GET /auth/setup", setup.StatusHandler(deps.DB))
	// 重要な操作ができるか。画面が、重要な操作の入力を始める前に確かめる (設計書 9.5)。
	mux.Handle("GET /auth/reauth", auth.ReauthenticationStatusHandler(deps.Clock))
	// OIDC でのログイン (設計書 9.4)。IdP からのリダイレクトは GET なので、Origin を確かめずに受け付ける。
	oidc := newOIDC(deps.Clock, logger, deps.Config, sessions)
	mux.Handle("GET /auth/oidc", oidc.StatusHandler(deps.DB))
	mux.Handle("GET "+auth.OIDCCallbackPath, oidc.CallbackHandler(deps.DB, clientInfo))
	for pattern, handler := range map[string]http.Handler{
		"POST /auth/setup":                         setup.CreateHandler(deps.DB, clientInfo),
		"POST /auth/login":                         logins.PasswordHandler(deps.DB, clientInfo),
		"POST /auth/login/totp":                    logins.TOTPHandler(deps.DB, clientInfo),
		"POST /auth/login/recovery-code":           logins.RecoveryCodeHandler(deps.DB, clientInfo),
		"POST /auth/login/totp/enrollment":         logins.EnrollmentHandler(deps.DB),
		"POST /auth/login/totp/enrollment/confirm": logins.EnrollmentConfirmHandler(deps.DB, clientInfo),
		"POST /auth/login/passkey":                 logins.PasskeySecondFactorHandler(deps.DB, clientInfo),
		"POST /auth/login/passkey/confirm":         logins.PasskeySecondFactorConfirmHandler(deps.DB, clientInfo),
		"POST /auth/passkey/login":                 logins.PasskeyLoginHandler(deps.DB, clientInfo),
		"POST /auth/passkey/login/confirm":         logins.PasskeyLoginConfirmHandler(deps.DB, clientInfo),
		"POST /auth/passkey/registration":          passkeys.RegistrationHandler(deps.DB, clientInfo),
		"POST /auth/passkey/registration/confirm":  passkeys.RegistrationConfirmHandler(deps.DB, clientInfo),
		"POST /auth/oidc":                          oidc.BeginHandler(deps.DB, clientInfo),
		"POST /auth/reauth":                        logins.ReauthenticateHandler(deps.DB, clientInfo),
		"POST /auth/reauth/passkey":                logins.PasskeyReauthenticationHandler(deps.DB, clientInfo),
		"POST /auth/reauth/passkey/confirm":        logins.PasskeyReauthenticationConfirmHandler(deps.DB, clientInfo),
		"POST /auth/reauth/oidc":                   oidc.ReauthenticationHandler(deps.DB, clientInfo),
		"POST /auth/oidc/link":                     oidc.LinkHandler(deps.DB, clientInfo),
	} {
		mux.Handle(pattern, httpserver.RequireSameOrigin(handler))
	}
	// 映像の配信 (設計書 7.2、7.6、9.6)。セッションか署名付き URL で認可する。
	var opener stream.ServiceOpener
	var logos stream.LogoFetcher
	if deps.Config.Tuner.URL.Value() != "" {
		client, err := newTunerClient(deps)
		if err != nil {
			return nil, err
		}
		opener, logos = client, client
	}
	authorizer := stream.NewAuthorizer(signer, deps.DB, deps.Clock)
	streamLogger := logger.With(slog.String("component", "stream"))
	// ライブ視聴と録画の配信は終わらないので、停止を始めたら打ち切り、待たない。
	mux.Handle("GET /streams/services/{id}",
		httpserver.StopOnShutdown(authorizer.Middleware(stream.NewLiveHandler(opener, streamLogger))))
	mux.Handle("GET /streams/services/{id}/data-broadcast",
		httpserver.StopOnShutdown(authorizer.Middleware(stream.NewDataBroadcastHandler(opener, streamLogger))))
	// サーバー側でのトランスコードは、インスタンスの登録簿で引いた kyusu に中継する (設計書 7.3)。
	kyusuToken, err := kyusu.NewToken(deps.Config.Auth.SecretKey.Value())
	if err != nil {
		return nil, err
	}
	instances := registry.New(deps.DB, deps.Clock, deps.InstanceID)
	mux.Handle("GET /streams/services/{id}/hls/{preset}/{audio}/{file}",
		authorizer.Middleware(kyusu.NewRelay(instances, kyusuToken, streamLogger)))
	mux.Handle("GET /streams/recordings/{id}/hls/{preset}/{audio}/{file}",
		authorizer.Middleware(kyusu.NewRecordingRelay(instances, kyusuToken, streamLogger)))
	// 録画は、このインスタンスにマウントしたプールのものだけ流せる。
	mux.Handle("GET /streams/recordings/{id}",
		httpserver.StopOnShutdown(authorizer.Middleware(stream.NewRecordingHandler(deps.DB, storeOf(deps.Config), streamLogger, 0))))
	// 取り込んだ録画の、TS 以外のファイルも、このインスタンスにマウントしたプールのものだけ流せる (設計書 6.3)。
	mux.Handle("GET /streams/recordings/{id}/files/{file}",
		httpserver.StopOnShutdown(authorizer.Middleware(stream.NewOtherFileHandler(deps.DB, storeOf(deps.Config), streamLogger))))
	handleMedia(mux, deps, logger, authorizer, logos, jikkyoTable, jikkyoClient)
	api := sessions.Middleware(deps.DB)(mux)
	compat := newEPGStationAPI(deps, apiTokens, opener, logos, streamLogger)

	app, err := spa.NewHandler(webui.App(), logger.With(slog.String("component", "spa")))
	if err != nil {
		return nil, err
	}
	root := http.NewServeMux()
	root.Handle("/graphql", api)
	root.Handle("/auth/", api)
	root.Handle("/streams/", api)
	root.Handle("/media/", api)
	root.Handle("/api/", compat)
	handleOAuth(root, authorizationServer, graphQL, logger)
	root.Handle("/", app)

	return &apiHandler{handler: policy.Middleware(root), changes: changes, updates: updates}, nil
}

// handleMedia は、画像と実況の過去ログ (/media/*) を mux に登録する。どれも、セッションか署名付き URL で認可する。
func handleMedia(
	mux *http.ServeMux, deps *Deps, logger *slog.Logger, authorizer *stream.Authorizer, logos stream.LogoFetcher,
	jikkyoTable *jikkyo.Table, jikkyoClient *jikkyo.Client,
) {
	streamLogger := logger.With(slog.String("component", "stream"))
	// サービスのロゴは、チューナーサーバーから取って中継する。
	mux.Handle("GET /media/services/{id}/logo", authorizer.Middleware(stream.NewLogoHandler(logos, streamLogger)))
	// ユーザーのアイコンは、IdP から取って中継する (#778)。
	mux.Handle("GET /media/users/{id}/avatar",
		authorizer.Middleware(avatar.NewHandler(deps.DB, deps.Config.Auth.OIDC.Issuer, logger.With(slog.String("component", "avatar")))))
	// 録画の代表画像とプレビュー画像も、このインスタンスにマウントしたプールのものだけ配信できる。
	mux.Handle("GET /media/recordings/{id}/{file}",
		authorizer.Middleware(thumbnail.NewImageHandler(deps.DB, storeOf(deps.Config), streamLogger)))
	// 録画の時間帯の実況の過去ログは、過去ログ API から取って中継する (設計書 7.4)。
	mux.Handle("GET /media/recordings/{id}/"+jikkyo.CommentsFileName, authorizer.Middleware(
		jikkyo.NewCommentsHandler(deps.DB, jikkyoTable, jikkyoClient, deps.Clock, logger.With(slog.String("component", "jikkyo")))))
}

// handleOAuth は、MCP のクライアントにトークンを発行する認可サーバーのエンドポイント (#963、設計書 9.9) と、
// MCP サーバー (/mcp、#964、設計書 9.10) を root に登録する。どれもセッションの Cookie を使わない。
// 同意は、ログインした後の GraphQL で行う。MCP のツールは、ログイン中のユーザーとして、同じ GraphQL のハンドラーで操作する。
func handleOAuth(root *http.ServeMux, server *oauth.Server, graphQL http.Handler, logger *slog.Logger) {
	oauthLogger := logger.With(slog.String("component", "oauth"))
	root.Handle("GET "+oauth.MetadataPath, server.MetadataHandler(oauthLogger))
	root.Handle("GET "+oauth.ProtectedResourceMetadataPath, server.ProtectedResourceMetadataHandler(oauthLogger))
	root.Handle("GET "+oauth.AuthorizePath, server.AuthorizeHandler(oauthLogger))
	root.Handle("POST "+oauth.ExchangePath, server.TokenHandler(oauthLogger))
	root.Handle("POST "+oauth.RevokePath, server.RevokeHandler(oauthLogger))
	mcpHandler, err := mcpserver.NewHandler(server, graphQL, graph.Schema(), logger.With(slog.String("component", "mcp")))
	if err != nil {
		// ツールの GraphQL の操作は、ソースに書いた固定のものなので、ここで失敗するのはプログラムの誤り。テストで防ぐ。
		panic(err)
	}
	root.Handle(oauth.ResourcePath, mcpHandler)
}

// newEPGStationAPI は、EPGStation 互換の読み取り専用 API (/api/*) を返す (#431、設計書 9.1)。
// セッションの Cookie は使わず、API トークン (Bearer) だけで認証する (設計書 9.8)。トークンがなければ 401 を返す。
func newEPGStationAPI(deps *Deps, tokens *auth.APITokens, opener stream.ServiceOpener, logos stream.LogoFetcher, streamLogger *slog.Logger) http.Handler {
	h := epgstationapi.New(deps.DB, storeOf(deps.Config), streamLogger)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/version", h.Version)
	mux.HandleFunc("GET /api/config", h.Config)
	mux.HandleFunc("GET /api/channels", h.Channels)
	mux.Handle("GET /api/channels/{id}/logo", stream.NewLogoHandler(logos, streamLogger))
	mux.HandleFunc("GET /api/schedules", h.Schedules)
	// mode は、EPGStation のエンコードの設定の番号。Kotatsu は無変換の TS だけを返すので、読まない。
	mux.Handle("GET /api/streams/live/{id}/m2ts", httpserver.StopOnShutdown(stream.NewLiveHandler(opener, streamLogger)))
	mux.HandleFunc("GET /api/recorded", h.Recorded)
	mux.HandleFunc("GET /api/recorded/{id}", h.RecordedItem)
	// 動画とサムネイルは、録画の ID を読み替えて、録画の配信と代表画像の配信につなぐ。動画は Range に答える。
	mux.Handle("GET /api/videos/{id}", httpserver.StopOnShutdown(
		h.WithRecording(stream.NewRecordingHandler(deps.DB, storeOf(deps.Config), streamLogger, 0), "")))
	mux.Handle("GET /api/thumbnails/{id}",
		h.WithRecording(thumbnail.NewImageHandler(deps.DB, storeOf(deps.Config), streamLogger), thumbnail.ThumbnailFileName))

	return tokens.BearerMiddleware(auth.RequirePermission(auth.PermissionWatch)(mux))
}

// newKyusu は、kyusu のライブ視聴と録画の仮想 HLS のセッションと、tenban からのリクエストを確かめるトークンを返す。
// FFmpeg か tsreadex がなければ、起動はするが、トランスコードには理由を添えて 503 を返す。
// 録画は、このインスタンスにマウントした保存先プールから読む。
func newKyusu(ctx context.Context, logger *slog.Logger, deps *Deps) (*kyusu.Live, *kyusu.VOD, *kyusu.Token, error) {
	// tenban だけが、アプリの秘密キーから導いたトークンを付けて、kyusu に中継する。
	token, err := kyusu.NewToken(deps.Config.Auth.SecretKey.Value())
	if errors.Is(err, kyusu.ErrNoKey) {
		return nil, nil, nil, errSecretKeyRequired
	}
	if err != nil {
		return nil, nil, nil, err
	}
	var opener kyusu.ServiceOpener
	if deps.Config.Tuner.URL.Value() != "" {
		client, err := newTunerClient(deps)
		if err != nil {
			return nil, nil, nil, err
		}
		opener = client
	}
	kyusuLogger := logger.With(slog.String("component", "kyusu"))
	var liveEncoder kyusu.Encoder
	var vodEncoder kyusu.VODEncoder
	transcoder, err := kyusu.NewTranscoder(&deps.Config.Kyusu)
	switch {
	case errors.Is(err, kyusu.ErrToolNotFound):
		logger.WarnContext(ctx, "FFmpeg or tsreadex is not found; server-side transcoding is disabled", slog.Any("error", err))
	case err != nil:
		return nil, nil, nil, err
	default:
		liveEncoder, vodEncoder = transcoder, transcoder
	}
	live, err := kyusu.NewLive(&kyusu.LiveOptions{
		Opener: opener, Encoder: liveEncoder, WorkDir: deps.Config.Kyusu.WorkDir, Clock: deps.Clock, Logger: kyusuLogger, IdleTimeout: 0,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	vod, err := kyusu.NewVOD(&kyusu.VODOptions{
		Opener:  stream.NewRecordingSource(deps.DB, storeOf(deps.Config), 0),
		Encoder: vodEncoder, WorkDir: deps.Config.Kyusu.WorkDir, Clock: deps.Clock, Logger: kyusuLogger, IdleTimeout: 0, MaxSessions: 0,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	for kind, sessions := range map[string]func() int{"live": live.Sessions, "vod": vod.Sessions} {
		deps.Metrics.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: metrics.Namespace, Name: "kyusu_sessions", Help: "Number of kyusu transcoding sessions on this instance.",
			ConstLabels: prometheus.Labels{"kind": kind},
		}, func() float64 { return float64(sessions()) }))
	}

	return live, vod, token, nil
}

// errSecretKeyRequired は、tenban、nekonote か kyusu を、アプリの秘密キーなしで起動しようとしたことを表す。
var errSecretKeyRequired = errors.New("auth.secret_key (KOTATSU_AUTH_SECRET_KEY) is required to run tenban, nekonote, and kyusu; " +
	"set a random string of at least 32 bytes, e.g. the output of `openssl rand -base64 48`")

// clientInfo は、リクエストの送信元を返す。リバースプロキシを経由したときは、信頼済みのプロキシが伝えた値にする。
func clientInfo(r *http.Request) *auth.ClientInfo {
	info := &auth.ClientInfo{UserAgent: r.UserAgent(), SourceIP: netip.Addr{}, Host: r.Host, Secure: r.TLS != nil}
	if client := httpserver.ClientFrom(r.Context()); client != nil {
		info.SourceIP, info.Host, info.Secure = client.IP, client.Host, client.Secure
	}

	return info
}

// runRoles は roles を並行に動かす。どれかが失敗すると、残りも止める。
func runRoles(ctx context.Context, logger *slog.Logger, deps *Deps, roles []*Role) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, role := range roles {
		eg.Go(func() error {
			return role.Run(ctx, logger.With(slog.String("role", role.DisplayName())), deps)
		})
	}

	return eg.Wait()
}

// closePool は pool を閉じる。テストで差し替えた OpenDatabase は nil を返すことがある。
func closePool(pool *pgxpool.Pool) {
	if pool != nil {
		pool.Close()
	}
}

// storeOf は、設定のプールと、データディレクトリを使う kago.Store を返す。
func storeOf(cfg *config.Config) *kago.Store {
	return kago.NewOrdered(cfg.Storage.Pools, cfg.Storage.PoolOrder, nil).WithDataDir(cfg.Storage.DataDir)
}
