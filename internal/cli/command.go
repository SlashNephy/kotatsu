package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/errgroup"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/config"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/httpserver"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/registry"
	"github.com/SlashNephy/kotatsu/internal/telemetry"
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

	OpenDatabase func(ctx context.Context, cfg *config.Database) (*pgxpool.Pool, error)
	// CheckSchema は、ロールを起動する前に、DB のスキーマでこのバイナリが動けるかを確かめる。
	CheckSchema func(ctx context.Context, pool *pgxpool.Pool) error
	// Migrate は kotatsu migrate の本体。
	Migrate func(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error
	// RunRegistry は、ctx が終わるまで roles のインスタンスを登録簿に載せておく。
	// listenAddr は API の HTTP サーバーが実際に待ち受けているアドレス。
	RunRegistry func(ctx context.Context, logger *slog.Logger, deps *Deps, roles []string, listenAddr string) error
}

// DefaultOptions は、本番で使う依存を返す。
func DefaultOptions(logger *slog.Logger) *Options {
	return &Options{
		Logger:       logger,
		Roles:        DefaultRoles(),
		LookupEnv:    os.LookupEnv,
		OpenDatabase: database.Open,
		CheckSchema:  database.CheckSchema,
		Migrate:      database.Migrate,
		RunRegistry:  runRegistry,
	}
}

func runRegistry(ctx context.Context, logger *slog.Logger, deps *Deps, roles []string, listenAddr string) error {
	address, err := advertiseAddress(listenAddr)
	if err != nil {
		return err
	}

	return registry.New(deps.DB, deps.Clock, deps.InstanceID).Run(ctx, logger, roles, address)
}

// advertiseAddress は、他のインスタンスからこのインスタンスに届くアドレスを、待ち受けのアドレスから決める。
// 待ち受けのホストが空か 0.0.0.0 や :: などの全アドレスなら、ホスト名を使う。
// ポートに 0 を指定したときも実際に割り当てられたポートを載せるよう、設定値ではなく待ち受けたアドレスを渡す。
func advertiseAddress(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("invalid listen address %s: %w", listen, err)
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
		Use:   "kotatsu",
		Short: "Kotatsu is a web application to reserve, record, and watch Japanese TV broadcasts",
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

	// connect は設定を読み込み、DB に接続する。返した pool は呼び出し側で閉じる。
	connect := func(ctx context.Context) (*config.Config, *pgxpool.Pool, error) {
		path := configPath
		if path == "" {
			path, _ = opts.LookupEnv(configPathEnv)
		}

		cfg, err := config.Load(path, opts.LookupEnv)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load the config: %w", err)
		}
		opts.Logger.InfoContext(ctx, "config loaded", slog.String("path", path), slog.Any("config", cfg))

		pool, err := opts.OpenDatabase(ctx, &cfg.Database)
		if err != nil {
			return nil, nil, err
		}

		return cfg, pool, nil
	}

	start := func(ctx context.Context, targets []*Role) error {
		cfg, pool, err := connect(ctx)
		if err != nil {
			return err
		}
		defer closePool(pool)

		if err := opts.CheckSchema(ctx, pool); err != nil {
			return fmt.Errorf("refusing to start: %w", err)
		}

		return serve(ctx, opts, &Deps{
			Config:     cfg,
			DB:         pool,
			Metrics:    telemetry.NewRegistry(),
			Clock:      new(clock.Real),
			InstanceID: id.New(),
		}, targets)
	}

	for _, role := range opts.Roles {
		root.AddCommand(&cobra.Command{
			Use:         role.Name,
			Aliases:     []string{role.Alias},
			Short:       role.Short,
			Args:        cobra.NoArgs,
			Annotations: map[string]string{displayNameAnnotation: role.DisplayName()},
			RunE: func(cmd *cobra.Command, _ []string) error {
				return start(cmd.Context(), []*Role{role})
			},
		})
	}

	root.AddCommand(&cobra.Command{
		Use:   "all",
		Short: "Run all roles in a single process",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return start(cmd.Context(), opts.Roles)
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
	eg.Go(func() error {
		names := make([]string, 0, len(roles))
		for _, role := range roles {
			names = append(names, role.Name)
		}

		return opts.RunRegistry(ctx, logger, deps, names, apiListener.Addr().String())
	})
	eg.Go(func() error {
		return httpserver.Serve(ctx, logger.With(slog.String("server", "metrics")), metricsListener, metricsHandler)
	})

	return eg.Wait()
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
