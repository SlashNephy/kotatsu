package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/SlashNephy/kotatsu/internal/config"
	"github.com/SlashNephy/kotatsu/internal/database"
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
	}
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

		return runRoles(ctx, opts.Logger, &Deps{Config: cfg, DB: pool}, targets)
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
