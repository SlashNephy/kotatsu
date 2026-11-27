package database

import (
	"context"
	"encoding/json"
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
	"github.com/SlashNephy/kotatsu/internal/domain/episode"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// CompatibleSince は、このバイナリの最新のスキーマで動かしてよいバイナリの下限。
// バイナリが知っている最新のマイグレーションのバージョンで比較する。
//
// マイグレーションは、カラムを追加し、新しいカラムを使うように切り替え、古いカラムを削除する、という段階を踏んで
// 古いバイナリとの互換性を保つ (設計書 11.4)。古いバイナリの読み書きを壊すマイグレーション
// (古いバイナリがまだ使うカラムの削除など) を足したときは、その値をそのマイグレーションのバージョンに上げる。
//
// 20261124120830: 番組名と概要の書き換えの記録 (program_rewrites) のテーブルと、system_settings.rule_unmatched_grace_minutes を消した (#2129)。
// 古いバイナリは、EPG の同期でこのテーブルに書き込み、その同期の書き込みがすべて失敗するので、動かさない。
const CompatibleSince int64 = 20261124120830

var (
	// ErrSchemaOutdated は、DB のスキーマがこのバイナリより古いことを表す。
	ErrSchemaOutdated = errors.New("the database schema is older than this binary requires; run kotatsu migrate")
	// ErrBinaryOutdated は、DB のスキーマがこのバイナリでは動かせないほど新しいことを表す。
	ErrBinaryOutdated = errors.New("the database schema is newer than this binary supports; upgrade kotatsu")
)

// Migrate は、埋め込んだマイグレーションのうち未適用のものを適用する。
// 適用済みなら何もしない。
func Migrate(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	// マイグレーションは、大きなテーブルの書き換えで長くかかることがあるので、問い合わせの時間の上限を外した接続で流す (#527)。
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

	if err := renormalizeDashVariants(ctx, migrationPool); err != nil {
		return err
	}
	if err := fillRecordingsNormalized(ctx, logger, migrationPool); err != nil {
		return err
	}
	if err := fillRecordingHistorySeries(ctx, logger, migrationPool); err != nil {
		return err
	}
	if err := fillCapturesNormalized(ctx, logger, migrationPool); err != nil {
		return err
	}
	if err := fillAutoCaptionSearchGrams(ctx, logger, migrationPool); err != nil {
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

// ErrUnknownVersion は、指定したスキーマのバージョンが、このバイナリに埋め込んだマイグレーションにないことを表す。
var ErrUnknownVersion = errors.New("the schema version is not one of the migrations embedded in this binary")

// MigrateTo は、埋め込んだマイグレーションのうち、version までの未適用のものを適用する (#1926)。
// バックアップを戻すときに、バックアップしたときのスキーマに合わせてから行を流し込むのに使う。
// Migrate と違い、正規化したカラムを埋める処理と、動かしてよいバイナリの下限の記録はしない。どちらも最新のスキーマのカラムを読み書きするからである。
// 戻し終えたら Migrate で最新まで上げ、その中で埋める。
func MigrateTo(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, version int64) error {
	if !slices.Contains(MigrationVersions(), version) {
		return fmt.Errorf("%w: %d", ErrUnknownVersion, version)
	}
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

	current, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get the schema version: %w", err)
	}
	if current > version {
		return fmt.Errorf("the database schema (version %d) is already newer than version %d; migrations cannot be undone", current, version)
	}
	results, err := provider.UpTo(ctx, version)
	if err != nil {
		return fmt.Errorf("failed to apply migrations up to version %d: %w", version, err)
	}
	for _, result := range results {
		logger.InfoContext(ctx, "migration applied", slog.Int64("version", result.Source.Version), slog.Duration("duration", result.Duration))
	}
	logger.InfoContext(ctx, "database schema migrated to the requested version", slog.Int64("version", version), slog.Int("applied", len(results)))

	return nil
}

// fillBatchSize は、正規化したカラムを埋めるときに、1 回に読み書きする行の数。
const fillBatchSize = 500

// fillRecordingsNormalized は、正規化した番組名と概要のカラムがない録画のカラムを埋める (#1044、設計書 3.5)。
// 正規化は囲み文字を括弧にしてから NFKC にするので (domain.Normalize)、SQL の normalize では書けず、Go で埋める。
// カラムを足す前の録画と、カラムを知らない古いバイナリがローリング更新の間に書いた録画を、次の kotatsu migrate で埋める。
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
		errs := []error{}
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

// fillCapturesNormalized は、正規化した字幕の文のカラムがないキャプチャのカラムを埋める (#2084、設計書 3.5)。
// 正規化は囲み文字を括弧にしてから NFKC にするので (domain.Normalize)、SQL の normalize では書けず、Go で埋める。
// カラムを足す前のキャプチャと、カラムを知らない古いバイナリがローリング更新の間に書いたキャプチャを、次の kotatsu migrate で埋める。
// 字幕が出ていなかったキャプチャには空を書き、次の migrate で読み直さないようにする。
func fillCapturesNormalized(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	q := sqlc.New(pool)
	filled := 0
	for {
		rows, err := q.ListCapturesWithoutNormalized(ctx, fillBatchSize)
		if err != nil {
			return fmt.Errorf("failed to list the captures to normalize: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		params := make([]sqlc.SetCaptureNormalizedParams, 0, len(rows))
		for _, row := range rows {
			caption := ""
			if row.Caption != nil {
				caption = *row.Caption
			}
			params = append(params, sqlc.SetCaptureNormalizedParams{
				ID:                row.ID,
				CaptionNormalized: new(domain.Normalize(caption)),
			})
		}
		errs := []error{}
		q.SetCaptureNormalized(ctx, params).Exec(func(i int, err error) {
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to normalize capture %s: %w", params[i].ID, err))
			}
		})
		if err := errors.Join(errs...); err != nil {
			return err
		}
		filled += len(rows)
	}
	if filled > 0 {
		logger.InfoContext(ctx, "captures normalized", slog.Int("count", filled))
	}

	return nil
}

// autoCaptionFillBatchSize は、自動生成の字幕の gram を埋めるときに、1 回に読み書きする字幕の数。
// 字幕は録画ごとに数千件あり、1 行が大きいので、正規化したカラムを埋めるとき (fillBatchSize) より少なくする。
const autoCaptionFillBatchSize = 50

// fillAutoCaptionSearchGrams は、字幕の文で探すための gram (#2214、設計書 3.5) が今の字幕のものでない、自動生成の字幕の gram を作り直す。
// gram は Go の正規化 (domain.Normalize) をした文から作るので、SQL では書けず、Go で埋める。
// カラムを足す前の字幕と、カラムを知らない古いバイナリがローリング更新の間に書き直した字幕を、次の kotatsu migrate で埋める。
// 埋めるまでの字幕の録画は検索の候補に残るので、検索の結果は変わらない。
func fillAutoCaptionSearchGrams(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	q := sqlc.New(pool)
	filled := 0
	for {
		rows, err := q.ListRecordingAutoCaptionsWithoutSearchGrams(ctx, autoCaptionFillBatchSize)
		if err != nil {
			return fmt.Errorf("failed to list the auto captions without search grams: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		params := make([]sqlc.SetRecordingAutoCaptionsSearchGramsParams, 0, len(rows))
		for _, row := range rows {
			// 字幕の形は autocaption の storedCue。このパッケージは autocaption から使われるので、文だけを読む。
			var cues []struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(row.Cues, &cues); err != nil {
				return fmt.Errorf("failed to decode the auto captions of recording %s: %w", row.RecordingID, err)
			}
			texts := make([]string, 0, len(cues))
			for _, c := range cues {
				texts = append(texts, domain.Normalize(c.Text))
			}
			params = append(params, sqlc.SetRecordingAutoCaptionsSearchGramsParams{
				SearchGrams: match.TextGrams(texts), RecordingID: row.RecordingID, GeneratedAt: row.GeneratedAt,
			})
		}
		errs := []error{}
		q.SetRecordingAutoCaptionsSearchGrams(ctx, params).Exec(func(i int, err error) {
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to set the search grams of the auto captions of recording %s: %w", params[i].RecordingID, err))
			}
		})
		if err := errors.Join(errs...); err != nil {
			return err
		}
		filled += len(rows)
	}
	if filled > 0 {
		logger.InfoContext(ctx, "auto caption search grams filled", slog.Int("count", filled))
	}

	return nil
}

// fillRecordingHistorySeries は、シリーズ名のカラムがない録画履歴のカラムを埋める (#1499、設計書 4.4)。
// シリーズ名は Go の回の読み方 (episode.Parse) で読むので、SQL では書けず、Go で埋める。
// カラムを足す前の履歴と、カラムを知らない古いバイナリがローリング更新の間に書いた履歴を、次の kotatsu migrate で埋める。
// シリーズ名を読めない番組名の履歴には空を書き、次の migrate で読み直さないようにする。
func fillRecordingHistorySeries(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	q := sqlc.New(pool)
	filled := 0
	for {
		rows, err := q.ListRecordingHistoryWithoutSeries(ctx, fillBatchSize)
		if err != nil {
			return fmt.Errorf("failed to list the recording history without series: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		params := make([]sqlc.SetRecordingHistorySeriesParams, 0, len(rows))
		for _, row := range rows {
			params = append(params, sqlc.SetRecordingHistorySeriesParams{ID: row.ID, Series: new(episode.SeriesOf(row.Name))})
		}
		errs := []error{}
		q.SetRecordingHistorySeries(ctx, params).Exec(func(i int, err error) {
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to set the series of recording history %s: %w", params[i].ID, err))
			}
		})
		if err := errors.Join(errs...); err != nil {
			return err
		}
		filled += len(rows)
	}
	if filled > 0 {
		logger.InfoContext(ctx, "recording history series filled", slog.Int("count", filled))
	}

	return nil
}

// migrationLockID は、マイグレーションのロックの ID。goose の既定の ID と同じものを使う。
const migrationLockID = lock.DefaultLockID

// WithMigrationLock は、マイグレーションのロックを取ってから fn を実行し、終わったら手放す (設計書 11.4)。
// kotatsu migrate を同時に実行しても、1 つずつ順に適用するので、どれも失敗しない。
//
// ロックは、PostgreSQL の advisory lock ではなく、テーブル (goose_lock) の行で取る。トランザクションプーリングを経由すると、
// セッションの advisory lock は、取った接続と手放す接続が違うことがあり、手放せずに残るからである。
// 行には期限があり、取った側が落ちても、期限が過ぎれば取得し直せる。
func WithMigrationLock(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, fn func(ctx context.Context) error) error {
	locker, err := lock.NewPostgresTableLocker(
		lock.WithTableLockID(migrationLockID),
		// ほかの migrate が終わるのを、1 秒ごとに確認し、5 分まで待つ。
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

// CheckSchema は、このバイナリが DB のスキーマで動けるかを確認する。
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
