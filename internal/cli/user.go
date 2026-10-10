package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/config"
	"github.com/SlashNephy/kotatsu/internal/password"
)

// errPasswordRequired は、標準入力からパスワードを読めなかったことを表す。
var errPasswordRequired = errors.New("the password must be given on the standard input")

// hibpOf は、設定で有効にしたときだけ、Have I Been Pwned への照会を返す。
func hibpOf(cfg *config.Config) *password.HIBP {
	if !cfg.Auth.HIBP.Enabled {
		return nil
	}

	return password.NewHIBP(cfg.Auth.HIBP.URL)
}

// newPasswordPolicy は、設定に従って、パスワードの規定を作る。
func newPasswordPolicy(opts *Options, cfg *config.Config) (*password.Policy, error) {
	return password.NewPolicy(&password.PolicyOptions{HIBP: hibpOf(cfg), StrengthTimeout: opts.PasswordStrengthTimeout, Logger: opts.Logger})
}

// newUserCommand は kotatsu user を作る。初回セットアップ (#31) の画面ができるまで、管理者がパスワードを設定するのにも使う。
func newUserCommand(opts *Options, connect func(ctx context.Context) (*config.Config, *pgxpool.Pool, error)) *cobra.Command {
	userCmd := &cobra.Command{
		Use:   "user",
		Short: "Manage local accounts",
		Args:  cobra.NoArgs,
	}
	userCmd.AddCommand(&cobra.Command{
		Use:   "set-password <username>",
		Short: "Set the password of a local account; the password is read from the standard input",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// コマンドライン引数に渡すと、シェルの履歴やプロセスの一覧に残るので、標準入力から読む。
			line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
			secret := strings.TrimRight(line, "\r\n")
			if secret == "" {
				if err != nil {
					return fmt.Errorf("%w: %w", errPasswordRequired, err)
				}

				return errPasswordRequired
			}

			cfg, pool, err := connect(cmd.Context())
			if err != nil {
				return err
			}
			defer closePool(pool)
			policy, err := newPasswordPolicy(opts, cfg)
			if err != nil {
				return err
			}
			userID, err := auth.UserIDByUsername(cmd.Context(), pool, args[0])
			if err != nil {
				return err
			}

			err = auth.NewPasswords(opts.Clock, audit.NewRecorder(opts.Clock), policy).Set(cmd.Context(), pool, audit.System("cli"), userID, secret)
			if rejected, ok := errors.AsType[*password.RejectedError](err); ok {
				return fmt.Errorf("the password was rejected: %s", rejected.Reason)
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "set the password of %s\n", args[0])

			return err
		},
	})

	return userCmd
}

// newAdminCommand は kotatsu admin を作る。
func newAdminCommand(opts *Options, connect func(ctx context.Context) (*config.Config, *pgxpool.Pool, error)) *cobra.Command {
	adminCmd := &cobra.Command{
		Use:   "admin",
		Short: "Administer the Kotatsu instance",
		Args:  cobra.NoArgs,
	}
	adminCmd.AddCommand(&cobra.Command{
		Use:   "setup-token",
		Short: "Issue a token to create the first administrator at /setup",
		Long: "Issue a token to create the first administrator at /setup. " +
			"Use this when roles run in separate processes, for example with kubectl exec. " +
			"The token is valid until an administrator is created.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, pool, err := connect(cmd.Context())
			if err != nil {
				return err
			}
			defer closePool(pool)

			token, err := auth.NewSetup(opts.Clock, audit.NewRecorder(opts.Clock), nil, opts.Logger).IssueToken(cmd.Context(), pool)
			if errors.Is(err, auth.ErrSetupDone) {
				return errors.New("an administrator already exists; the setup is done")
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), token)

			return err
		},
	})

	return adminCmd
}

// logSetupToken は、管理者がいなければ、セットアップトークンを発行してログに出す (設計書 9.2)。
func logSetupToken(ctx context.Context, logger *slog.Logger, clk clock.Clock, pool *pgxpool.Pool) error {
	setup := auth.NewSetup(clk, audit.NewRecorder(clk), nil, logger)
	needed, err := setup.Needed(ctx, pool)
	if err != nil || !needed {
		return err
	}
	token, err := setup.IssueToken(ctx, pool)
	if errors.Is(err, auth.ErrSetupDone) {
		return nil
	}
	if err != nil {
		return err
	}
	logger.WarnContext(ctx, "no administrator exists yet; open /setup in a browser and enter this setup token to create one",
		slog.String("setup_token", token))

	return nil
}
