package cli

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/SlashNephy/kotatsu/internal/config"
)

const (
	// displayNameAnnotation は、--help のコマンド一覧に Name の代わりに出す名前を持つ Annotation のキー。
	displayNameAnnotation = "kotatsu.displayName"
	// configPathEnv は、--config を省略したときに設定ファイルのパスを引く環境変数。
	configPathEnv = "KOTATSU_CONFIG"
)

// NewCommand は kotatsu のルートコマンドを組み立てる。lookupEnv は設定の読み込みに使う。
func NewCommand(logger *slog.Logger, roles []*Role, lookupEnv config.LookupEnv) *cobra.Command {
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

	start := func(ctx context.Context, targets []*Role) error {
		path := configPath
		if path == "" {
			path, _ = lookupEnv(configPathEnv)
		}

		cfg, err := config.Load(path, lookupEnv)
		if err != nil {
			return fmt.Errorf("failed to load the config: %w", err)
		}
		logger.InfoContext(ctx, "config loaded", slog.String("path", path), slog.Any("config", cfg))

		return runRoles(ctx, logger, cfg, targets)
	}

	for _, role := range roles {
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
			return start(cmd.Context(), roles)
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
func runRoles(ctx context.Context, logger *slog.Logger, cfg *config.Config, roles []*Role) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, role := range roles {
		eg.Go(func() error {
			return role.Run(ctx, logger.With(slog.String("role", role.DisplayName())), cfg)
		})
	}

	return eg.Wait()
}
