package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"filippo.io/age"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/usecase/backup"
)

// errSecretKeyMismatch は、バックアップのアプリの秘密キーが、設定の秘密キーと違うことを表す。
var errSecretKeyMismatch = errors.New("the backup was made with a different app secret key (auth.secret_key) than the one configured; " +
	"pass --secret-key-out to save the key of the backup, and set it as auth.secret_key before starting kotatsu")

// newBackupCommand は kotatsu admin backup を作る (#1926、設計書 11.4)。
// DB とキャプチャの画像を、age の公開鍵で暗号化した 1 つのファイルに書き出す。
func newBackupCommand(opts *Options, connect func(ctx context.Context) (*config.Config, *pgxpool.Pool, error)) *cobra.Command {
	var output, recipientsFile string
	var recipientKeys []string
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Write an encrypted backup of the database and the captures",
		Long: "Write a backup of the database, the app secret key, and the capture images to a file, encrypted to age public keys. " +
			"Recordings, their generated files, and data fetched again from the tuner server (the EPG) are not included. " +
			"Run this where the data directory is mounted. Restore the backup with kotatsu admin restore.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			recipients, err := parseRecipients(recipientKeys, recipientsFile)
			if err != nil {
				return err
			}
			cfg, pool, err := connect(cmd.Context())
			if err != nil {
				return err
			}
			defer closePool(pool)

			result, err := writeBackupFile(cmd.Context(), output, func(w io.Writer) (*backup.WriteResult, error) {
				return backup.Write(cmd.Context(), &backup.WriteOptions{
					DB: pool, Store: storeOf(cfg), SecretKey: cfg.Auth.SecretKey.Value(), Clock: opts.Clock, Logger: opts.Logger,
				}, w, recipients)
			})
			if err != nil {
				return err
			}
			opts.Logger.InfoContext(cmd.Context(), "backup written",
				slog.String("path", output), slog.Int64("bytes", result.Bytes), slog.Int64("schema_version", result.Manifest.SchemaVersion),
				slog.Int("tables", len(result.Manifest.Tables)), slog.Int("captures", result.Manifest.Captures), slog.Int("missing_captures", result.MissingCaptures))

			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "path of the backup file to write; it must not exist")
	cmd.Flags().StringArrayVarP(&recipientKeys, "recipient", "r", nil, "age public key (age1...) to encrypt the backup to; repeat to add more")
	cmd.Flags().StringVarP(&recipientsFile, "recipients-file", "R", "", "file with age public keys, one per line")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

// newRestoreCommand は kotatsu admin restore を作る (#1926、設計書 11.4)。
// kotatsu admin backup のファイルを、age の秘密鍵で読み、空の DB に戻す。
func newRestoreCommand(opts *Options, connect func(ctx context.Context) (*config.Config, *pgxpool.Pool, error)) *cobra.Command {
	var input, identityFile, secretKeyOut string
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore a backup into an empty database",
		Long: "Restore a file written by kotatsu admin backup into an empty database, and write the capture images to the data directory. " +
			"Stop every role and create an empty database first. The schema is migrated to the version of the backup, " +
			"the rows are loaded, and then the schema is migrated to the latest version. " +
			"If the app secret key of the backup differs from auth.secret_key, pass --secret-key-out to save it, " +
			"and set it as auth.secret_key before starting kotatsu.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			identities, err := parseIdentities(identityFile)
			if err != nil {
				return err
			}
			cfg, pool, err := connect(cmd.Context())
			if err != nil {
				return err
			}
			defer closePool(pool)

			f, err := os.Open(input)
			if err != nil {
				return fmt.Errorf("failed to open the backup: %w", err)
			}
			defer f.Close()
			manifest, err := backup.Restore(cmd.Context(), &backup.RestoreOptions{
				DB: pool, Store: storeOf(cfg), Logger: opts.Logger,
				CheckSecretKey: func(secretKey string) error {
					if secretKeyOut != "" {
						return writeSecretKeyFile(secretKeyOut, secretKey)
					}
					if cfg.Auth.SecretKey.Value() != secretKey {
						return errSecretKeyMismatch
					}

					return nil
				},
			}, f, identities)
			if err != nil {
				return err
			}
			opts.Logger.InfoContext(cmd.Context(), "backup restored",
				slog.String("path", input), slog.Time("created_at", manifest.CreatedAt), slog.String("kotatsu_version", manifest.KotatsuVersion),
				slog.Int64("schema_version", manifest.SchemaVersion), slog.Int("tables", len(manifest.Tables)), slog.Int("captures", manifest.Captures))
			if secretKeyOut != "" {
				opts.Logger.WarnContext(cmd.Context(), "the app secret key of the backup was saved; set it as auth.secret_key before starting kotatsu",
					slog.String("path", secretKeyOut))
			}

			return nil
		},
	}
	cmd.Flags().StringVarP(&input, "input", "i", "", "path of the backup file to restore")
	cmd.Flags().StringVar(&identityFile, "identity", "", "file with the age private key (AGE-SECRET-KEY-1...) to decrypt the backup")
	cmd.Flags().StringVar(&secretKeyOut, "secret-key-out", "", "path to save the app secret key of the backup to (created with mode 0600; it must not exist)")
	_ = cmd.MarkFlagRequired("input")
	_ = cmd.MarkFlagRequired("identity")

	return cmd
}

// parseRecipients は、フラグとファイルの age の公開鍵を読む。
func parseRecipients(keys []string, file string) ([]age.Recipient, error) {
	lines := slices.Clone(keys)
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read the recipients file: %w", err)
		}
		lines = append(lines, string(data))
	}
	if len(lines) == 0 {
		return nil, errors.New("pass an age public key with --recipient or --recipients-file")
	}
	recipients, err := age.ParseRecipients(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		return nil, fmt.Errorf("failed to parse the age public keys: %w", err)
	}

	return recipients, nil
}

// parseIdentities は、age の秘密鍵のファイルを読む。
func parseIdentities(file string) ([]age.Identity, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open the identity file: %w", err)
	}
	defer f.Close()
	identities, err := age.ParseIdentities(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the age private keys: %w", err)
	}

	return identities, nil
}

// writeBackupFile は、path と同じディレクトリの一時ファイルに write で書き、書き終えたら path に名前を変える。
// 途中で失敗したら、一時ファイルを消す。バックアップには秘密情報が入るので、持ち主だけが読める許可で作る。
func writeBackupFile(ctx context.Context, path string, write func(w io.Writer) (*backup.WriteResult, error)) (*backup.WriteResult, error) {
	if _, err := os.Lstat(path); err == nil {
		return nil, fmt.Errorf("the backup file %s already exists", path)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return nil, fmt.Errorf("failed to create the backup file: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return nil, fmt.Errorf("failed to restrict the permission of the backup file: %w", err)
	}
	result, err := write(tmp)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		return nil, fmt.Errorf("failed to flush the backup file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("failed to close the backup file: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return nil, fmt.Errorf("failed to rename the backup file: %w", err)
	}
	ok = true

	return result, nil
}

// writeSecretKeyFile は、アプリの秘密キーを、持ち主だけが読める新しいファイルに書く。既にあれば上書きしない。
func writeSecretKeyFile(path, secretKey string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create the secret key file: %w", err)
	}
	if _, err := f.WriteString(secretKey); err != nil {
		_ = f.Close()

		return fmt.Errorf("failed to write the secret key file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to write the secret key file: %w", err)
	}

	return nil
}
