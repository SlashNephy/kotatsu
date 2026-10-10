package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/config"
	"github.com/SlashNephy/kotatsu/internal/password"
)

// errPasswordRequired は、標準入力からパスワードを読めなかったことを表す。
var errPasswordRequired = errors.New("the password must be given on the standard input")

// newPasswordPolicy は、設定に従って、パスワードの規定を作る。
func newPasswordPolicy(opts *Options, cfg *config.Config) (*password.Policy, error) {
	var hibp *password.HIBP
	if cfg.Auth.HIBP.Enabled {
		hibp = password.NewHIBP(cfg.Auth.HIBP.URL)
	}

	return password.NewPolicy(&password.PolicyOptions{HIBP: hibp, StrengthTimeout: opts.PasswordStrengthTimeout, Logger: opts.Logger})
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
