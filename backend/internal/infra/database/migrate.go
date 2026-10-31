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
	"github.com/pressly/goose/v3/lock"

	"github.com/SlashNephy/kotatsu/db"
	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
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

	if err := fillRecordingsNormalized(ctx, logger, migrationPool); err != nil {
		return err
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

// fillBatchSize は、正規化した列を埋めるときに、1 回に読み書きする録画の数。
const fillBatchSize = 500

// fillRecordingsNormalized は、正規化した番組名と概要の列がない録画の列を埋める (#1044、設計書 3.5)。
// 正規化は囲み文字を括弧にしてから NFKC にするので (domain.Normalize)、SQL の normalize では書けず、Go で埋める。
// 列を足す前の録画と、列を知らない古いバイナリがローリング更新の間に書いた録画を、次の kotatsu migrate で埋める。
func fillRecordingsNormalized(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	q := sqlc.New(pool)
	filled := 0
	for {
		rows, err := q.ListRecordingsWithoutNormalized(ctx, fillBatchSize)
		if err != nil {
			return fmt.Errorf("failed to list the recordings to normalize: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		params := make([]sqlc.SetRecordingNormalizedParams, 0, len(rows))
		for _, row := range rows {
			params = append(params, sqlc.SetRecordingNormalizedParams{
				ID:                    row.ID,
				NameNormalized:        new(domain.Normalize(row.Name)),
				DescriptionNormalized: new(domain.Normalize(row.Description)),
			})
		}
		var errs []error
		q.SetRecordingNormalized(ctx, params).Exec(func(i int, err error) {
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to normalize recording %s: %w", params[i].ID, err))
			}
		})
		if err := errors.Join(errs...); err != nil {
			return err
		}
		filled += len(rows)
	}
	if filled > 0 {
		logger.InfoContext(ctx, "recordings normalized", slog.Int("count", filled))
	}

	return nil
}

// migrationLockID は、マイグレーションのロックの ID。goose の既定の ID と同じものを使う。
const migrationLockID = lock.DefaultLockID

// WithMigrationLock は、マイグレーションのロックを取ってから fn を実行し、終わったら手放す (設計書 11.4)。
// kotatsu migrate を同時に実行しても、1 つずつ順に適用するので、どれも失敗しない。
//
// ロックは、PostgreSQL の advisory lock ではなく、表 (goose_lock) の行で取る。トランザクションプーリングを経由すると、
// セッションの advisory lock は、取った接続と手放す接続が違うことがあり、手放せずに残るからである。
// 行には期限があり、取った側が落ちても、期限が過ぎれば取り直せる。
func WithMigrationLock(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, fn func(ctx context.Context) error) error {
	locker, err := lock.NewPostgresTableLocker(
		lock.WithTableLockID(migrationLockID),
		// ほかの migrate が終わるのを、1 秒ごとに確かめ、5 分まで待つ。
		lock.WithTableLockTimeout(time.Second, 300),
		lock.WithTableLogger(logger),
	)
	if err != nil {
		return fmt.Errorf("failed to create the migration lock: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	if err := locker.Lock(ctx, db); err != nil {
		return fmt.Errorf("failed to acquire the migration lock: %w", err)
	}
	fnErr := fn(ctx)
	// fn が失敗しても、ロックは手放す。
	if err := locker.Unlock(context.WithoutCancel(ctx), db); err != nil {
		return errors.Join(fnErr, fmt.Errorf("failed to release the migration lock: %w", err))
	}

	return fnErr
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
