package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
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

			result, err := backup.WriteFile(cmd.Context(), &backup.WriteOptions{
				DB: pool, Store: storeOf(cfg), SecretKey: cfg.Auth.SecretKey.Value(), Clock: opts.Clock, Logger: opts.Logger,
			}, output, recipients)
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

			keyOut, err := createSecretKeyFile(secretKeyOut)
			if err != nil {
				return err
			}
			defer keyOut.discard()
			f, err := os.Open(input)
			if err != nil {
				return fmt.Errorf("failed to open the backup: %w", err)
			}
			defer f.Close()
			manifest, err := backup.Restore(cmd.Context(), &backup.RestoreOptions{
				DB: pool, Store: storeOf(cfg), Logger: opts.Logger, CheckSecretKey: keyOut.check(cfg.Auth.SecretKey.Value()),
			}, f, identities)
			if err != nil {
				return err
			}
			if err := keyOut.commit(); err != nil {
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

// secretKeyFile は、kotatsu admin restore --secret-key-out で、バックアップのアプリの秘密キーを書くファイル。
// 書けないことが戻した後に分からないよう、戻す前に空で作っておき、戻し終えてから書く。
// 戻せなかったら消し、ファイルだけが残って同じコマンドでやり直せなくなるのを防ぐ。
type secretKeyFile struct {
	path string
	f    *os.File
	key  string
	done bool
}

// createSecretKeyFile は、path に、持ち主だけが読めるファイルを作る。既にあれば作らない。path が空なら、ファイルを作らない。
func createSecretKeyFile(path string) (*secretKeyFile, error) {
	out := &secretKeyFile{path: path, f: nil, key: "", done: false}
	if path == "" {
		return out, nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to create the secret key file: %w", err)
	}
	out.f = f

	return out, nil
}

// check は、バックアップの秘密キーを確かめる関数を返す。ファイルに書き出さないなら、設定の秘密キー configured と同じでなければ戻さない。
func (s *secretKeyFile) check(configured string) func(string) error {
	return func(key string) error {
		if s.f == nil && key != configured {
			return errSecretKeyMismatch
		}
		s.key = key

		return nil
	}
}

// commit は、戻し終えた後に、秘密キーをファイルに書く。
func (s *secretKeyFile) commit() error {
	s.done = true
	if s.f == nil {
		return nil
	}
	if _, err := s.f.WriteString(s.key); err != nil {
		return fmt.Errorf("failed to write the secret key file: %w", err)
	}
	if err := s.f.Close(); err != nil {
		return fmt.Errorf("failed to write the secret key file: %w", err)
	}

	return nil
}

// discard は、戻せなかったときに、作っておいたファイルを消す。
func (s *secretKeyFile) discard() {
	if s.f == nil || s.done {
		return
	}
	_ = s.f.Close()
	_ = os.Remove(s.path)
}
