package database

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/SlashNephy/kotatsu/db"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// CompatibleSince は、このバイナリの最新のスキーマで動かしてよいバイナリの下限。
// バイナリが知っている最新のマイグレーションのバージョンで比べる。
//
// マイグレーションは、列を追加し、新しい列を使うように切り替え、古い列を削除する、という段階を踏んで
// 古いバイナリとの互換性を保つ (設計書 11.4)。古いバイナリの読み書きを壊すマイグレーション
// (古いバイナリがまだ使う列の削除など) を足したときは、その値をそのマイグレーションのバージョンに上げる。
const CompatibleSince int64 = 20261009102215

var (
	// ErrSchemaOutdated は、DB のスキーマがこのバイナリより古いことを表す。
	ErrSchemaOutdated = errors.New("the database schema is older than this binary requires; run kotatsu migrate")
	// ErrBinaryOutdated は、DB のスキーマがこのバイナリでは動かせないほど新しいことを表す。
	ErrBinaryOutdated = errors.New("the database schema is newer than this binary supports; upgrade kotatsu")
)

// Migrate は、埋め込んだマイグレーションのうち未適用のものを適用する。
// 適用済みなら何もしない。
func Migrate(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	// マイグレーションは、大きな表の書き換えで長くかかることがあるので、問い合わせの時間の上限を外した接続で流す (#527)。
	migrationPool, err := withoutStatementTimeout(ctx, pool)
	if err != nil {
		return err
	}
	if migrationPool != pool {
		defer migrationPool.Close()
	}
	provider, err := newProvider(migrationPool)
	if err != nil {
		return err
	}
	defer provider.Close()

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	for _, result := range results {
		logger.InfoContext(
			ctx,
			"migration applied",
			slog.Int64("version", result.Source.Version),
			slog.Duration("duration", result.Duration),
		)
	}

	if err := sqlc.New(pool).RaiseMinBinaryVersion(ctx, sqlc.RaiseMinBinaryVersionParams{
		MinBinaryVersion: CompatibleSince,
		UpdatedAt:        time.Now(),
	}); err != nil {
		return fmt.Errorf("failed to record the compatible binary version: %w", err)
	}

	current, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get the schema version: %w", err)
	}
	logger.InfoContext(ctx, "database schema is up to date", slog.Int64("version", current), slog.Int("applied", len(results)))

	return nil
}

// CheckSchema は、このバイナリが DB のスキーマで動けるかを確かめる。
//
// DB のスキーマがこのバイナリより古ければ ErrSchemaOutdated を返す。DB のスキーマのほうが新しいことは許す。
// ローリング更新では、新しいバイナリが kotatsu migrate を実行したあとも、古いバイナリが動き続けるからである。
// ただし、kotatsu migrate が記録した下限よりこのバイナリが古ければ ErrBinaryOutdated を返す。
func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	provider, err := newProvider(pool)
	if err != nil {
		return err
	}
	defer provider.Close()

	current, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get the schema version: %w", err)
	}
	latest := LatestVersion()
	if current < latest {
		return fmt.Errorf("%w (database: %d, required: %d)", ErrSchemaOutdated, current, latest)
	}

	minBinaryVersion, err := sqlc.New(pool).GetMinBinaryVersion(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// マイグレーションは適用されているのに下限が記録されていない。migrate の途中で止まった場合に起こる。
			return fmt.Errorf("%w (the compatible binary version is not recorded)", ErrSchemaOutdated)
		}

		return fmt.Errorf("failed to get the compatible binary version: %w", err)
	}
	if latest < minBinaryVersion {
		return fmt.Errorf("%w (database requires binaries that know version %d, this binary knows up to %d)", ErrBinaryOutdated, minBinaryVersion, latest)
	}

	return nil
}

// LatestVersion は、このバイナリに埋め込んだ最新のマイグレーションのバージョンを返す。
func LatestVersion() int64 {
	versions := MigrationVersions()

	return versions[len(versions)-1]
}

// MigrationVersions は、このバイナリに埋め込んだマイグレーションのバージョンを古い順に返す。
// goose と同じく、ファイル名の先頭の _ までをバージョンとして読む。
func MigrationVersions() []int64 {
	// goose.Provider は DB の接続がないと作れないので、ファイル名から直接読む。
	names, err := fs.Glob(migrationsFS(), "*.sql")
	if err != nil || len(names) == 0 {
		// 埋め込んだファイルが壊れているのはビルドの誤りで、実行時に回復できない。
		panic(fmt.Sprintf("no embedded migrations: %v", err))
	}

	versions := make([]int64, 0, len(names))
	for _, name := range names {
		prefix, _, _ := strings.Cut(name, "_")
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			panic(fmt.Sprintf("invalid migration file name %s: %v", name, err))
		}
		versions = append(versions, version)
	}
	slices.Sort(versions)

	return versions
}

func newProvider(pool *pgxpool.Pool) (*goose.Provider, error) {
	provider, err := goose.NewProvider(goose.DialectPostgres, stdlib.OpenDBFromPool(pool), migrationsFS())
	if err != nil {
		return nil, fmt.Errorf("failed to load migrations: %w", err)
	}

	return provider, nil
}

func migrationsFS() fs.FS {
	fsys, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		panic(fmt.Sprintf("invalid embedded migrations: %v", err))
	}

	return fsys
}

// withoutStatementTimeout は、pool に問い合わせの時間の上限があれば、上限を外した別のプールを返す。なければ pool をそのまま返す。
func withoutStatementTimeout(ctx context.Context, pool *pgxpool.Pool) (*pgxpool.Pool, error) {
	cfg := pool.Config()
	if _, ok := cfg.ConnConfig.RuntimeParams["statement_timeout"]; !ok {
		return pool, nil
	}
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "0"
	migrationPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create the database pool for migrations: %w", err)
	}

	return migrationPool, nil
}
