package database

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3/lock"

	"github.com/SlashNephy/kotatsu/db"
	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/episode"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// CompatibleSince は、このバイナリの最新のスキーマで動かしてよいバイナリの下限。
// バイナリが知っているスキーマの版 (SchemaVersion) で比べる。
//
// スキーマの変更は、列を追加し、新しい列を使うように切り替え、古い列を削除する、という段階を踏んで
// 古いバイナリとの互換性を保つ (設計書 11.4)。古いバイナリの読み書きを壊す変更
// (古いバイナリがまだ使う列の削除など) を足したときは、その値をその変更のスキーマの版に上げる。
//
// 20261123030402: 照合の正規化で、波ダッシュとダッシュの類をそろえた (#2041)。古いバイナリは、そろえる前の書き方のキーワードで、
// 書き直した列と照合して、一致しなくなったルール予約を消すので、動かさない。
const CompatibleSince int64 = 20261123030402

// GooseFloorVersion は、新しい仕組みへそのまま移せる、goose のマイグレーションの版の下限 (#2040)。
// v2026.11.5 に入れた最後のマイグレーションの版で、それより古い DB は、先に v2026.11.5 で kotatsu migrate を実行してもらう。
const GooseFloorVersion int64 = 20261120170214

var (
	// ErrSchemaOutdated は、DB のスキーマがこのバイナリより古いことを表す。
	ErrSchemaOutdated = errors.New("the database schema is older than this binary requires; run kotatsu migrate")
	// ErrBinaryOutdated は、DB のスキーマがこのバイナリでは動かせないほど新しいことを表す。
	ErrBinaryOutdated = errors.New("the database schema is newer than this binary supports; upgrade kotatsu")
	// ErrDatabaseTooOld は、DB が、新しい仕組みへそのまま移せないほど古い goose のマイグレーションで止まっていることを表す。
	ErrDatabaseTooOld = errors.New("the database was migrated by a Kotatsu older than v2026.11.5; run kotatsu migrate of v2026.11.5 first")
)

// SchemaVersion は、このバイナリに埋め込んだスキーマの版 (db/schema.sum) を返す。
func SchemaVersion() int64 {
	version, _ := db.SchemaSum()

	return version
}

// Migrate は、埋め込んだスキーマ (db/schema.sql) と DB の差を当て、まだ流していない 1 回だけのスクリプトを流す (#2040)。
// 差がなく、流すスクリプトもなければ、スキーマは変えない。表、列、型を消す差があれば、当てずに ErrDestructiveChange を返す。
func Migrate(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	return migrate(ctx, logger, pool, db.Scripts)
}

// migrate は、Migrate の本体。1 回だけのスクリプトを scriptsFS の scripts/ から読む。
func migrate(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, scriptsFS fs.FS) error {
	// スキーマの変更は、大きな表の書き換えで長くかかることがあるので、問い合わせの時間の上限を外した接続で流す (#527)。
	migrationPool, err := withoutStatementTimeout(ctx, pool)
	if err != nil {
		return err
	}
	if migrationPool != pool {
		defer migrationPool.Close()
	}

	state, err := readSchemaState(ctx, migrationPool)
	if err != nil {
		return err
	}
	scripts, err := loadScripts(scriptsFS)
	if err != nil {
		return err
	}
	plan, err := planScripts(ctx, migrationPool, state, scripts)
	if err != nil {
		return err
	}

	// 差を当てる前に流すスクリプト (列の改名や、意図して消すものなど)。流したことを同じトランザクションで記録するよう、
	// goose から移る DB でも、記録する表を先に作る。schema.sql と同じ定義なので、差を当てるときには変えない。
	if len(plan.before) > 0 && !state.hasScripts {
		if _, err := migrationPool.Exec(ctx, createScriptsTable); err != nil {
			return fmt.Errorf("failed to create the table of the one-time scripts: %w", err)
		}
	}
	if err := runScripts(ctx, logger, migrationPool, plan.before); err != nil {
		return err
	}

	if err := applySchema(ctx, logger, migrationPool); err != nil {
		return err
	}

	// 差を当てたので、スクリプトを記録する表がある。流したものとみなすものを記録する。
	if err := sqlc.New(migrationPool).MarkSchemaScriptsApplied(ctx, sqlc.MarkSchemaScriptsAppliedParams{
		Versions:  plan.assumed,
		AppliedAt: time.Now(),
	}); err != nil {
		return fmt.Errorf("failed to record the one-time scripts: %w", err)
	}

	// 差を当てた後に流すスクリプト (足した列を埋めるものなど)。
	if err := runScripts(ctx, logger, migrationPool, plan.after); err != nil {
		return err
	}
	if err := fill(ctx, logger, migrationPool); err != nil {
		return err
	}

	if err := sqlc.New(migrationPool).RecordSchemaVersion(ctx, sqlc.RecordSchemaVersionParams{
		MinBinaryVersion: CompatibleSince,
		SchemaVersion:    SchemaVersion(),
		UpdatedAt:        time.Now(),
	}); err != nil {
		return fmt.Errorf("failed to record the schema version: %w", err)
	}
	logger.InfoContext(ctx, "database schema is up to date", slog.Int64("version", SchemaVersion()))

	return nil
}

// scriptPlan は、Migrate が流す 1 回だけのスクリプトと、流さずに流したものとみなすスクリプトの版。
type scriptPlan struct {
	before  []*script
	after   []*script
	assumed []int64
}

// planScripts は、DB の状態から、流すスクリプトを決める。DB がこのバイナリより新しいか、移せないほど古ければ、エラーを返す。
func planScripts(ctx context.Context, pool *pgxpool.Pool, state *schemaState, scripts []*script) (*scriptPlan, error) {
	if state.schemaVersion > SchemaVersion() {
		return nil, fmt.Errorf("%w (database: %d, this binary: %d)", ErrBinaryOutdated, state.schemaVersion, SchemaVersion())
	}
	// goose のマイグレーションで上げていた DB を、初めて新しい仕組みで上げる。goose_db_version は、ローリング更新の間に
	// 古いバイナリが起動時に読むので、消さずに残す。
	transition := state.hasCompatibility && state.schemaVersion == 0 && state.gooseVersion > 0
	if transition && state.gooseVersion < GooseFloorVersion {
		return nil, fmt.Errorf("%w (database: %d, required: %d or newer)", ErrDatabaseTooOld, state.gooseVersion, GooseFloorVersion)
	}
	// 空の DB から作るときは、書き換えるデータがないので、どのスクリプトも流したものとみなす。
	fresh := !state.hasCompatibility

	applied, err := appliedScripts(ctx, pool, state)
	if err != nil {
		return nil, err
	}
	plan := &scriptPlan{before: []*script{}, after: []*script{}, assumed: []int64{}}
	for _, s := range scripts {
		switch {
		case applied[s.version]:
		// goose のマイグレーションで流した書き換えは、同じ版のスクリプトでは流さない。
		case fresh || (transition && s.version <= state.gooseVersion):
			plan.assumed = append(plan.assumed, s.version)
		case s.phase == phaseBefore:
			plan.before = append(plan.before, s)
		default:
			plan.after = append(plan.after, s)
		}
	}

	return plan, nil
}

// ForgetScriptsAfter は、版が version より新しい 1 回だけのスクリプトを、流していないことにする。
// バックアップを戻した後に、書き出したときの版より新しいスクリプトを、戻した行に流し直すのに使う (#2040)。
// 次の Migrate で流す。
func ForgetScriptsAfter(ctx context.Context, pool *pgxpool.Pool, version int64) error {
	if err := sqlc.New(pool).ForgetSchemaScriptsAfter(ctx, version); err != nil {
		return fmt.Errorf("failed to forget the one-time scripts after version %d: %w", version, err)
	}

	return nil
}

// fill は、新しい DB にも要る行を足し、Go で計算する列のうち、埋まっていないものを埋める。
func fill(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	if err := ensureRows(ctx, pool); err != nil {
		return err
	}
	if err := renormalizeDashVariants(ctx, pool); err != nil {
		return err
	}
	if err := fillRecordingsNormalized(ctx, logger, pool); err != nil {
		return err
	}
	if err := fillRecordingHistorySeries(ctx, logger, pool); err != nil {
		return err
	}

	return fillCapturesNormalized(ctx, logger, pool)
}

// ensureRows は、新しい DB にも要る行を足す。今ある行は変えない。
func ensureRows(ctx context.Context, pool *pgxpool.Pool) error {
	values := domain.RecordingFailureReasonValues()
	reasons := make([]string, 0, len(values))
	for _, reason := range values {
		reasons = append(reasons, reason.String())
	}
	if err := sqlc.New(pool).EnsureRecordingFailureCounts(ctx, reasons); err != nil {
		return fmt.Errorf("failed to add the recording failure counts: %w", err)
	}

	return nil
}

// fillBatchSize は、正規化した列を埋めるときに、1 回に読み書きする行の数。
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

// fillCapturesNormalized は、正規化した字幕の文の列がないキャプチャの列を埋める (#2084、設計書 3.5)。
// 正規化は囲み文字を括弧にしてから NFKC にするので (domain.Normalize)、SQL の normalize では書けず、Go で埋める。
// 列を足す前のキャプチャと、列を知らない古いバイナリがローリング更新の間に書いたキャプチャを、次の kotatsu migrate で埋める。
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

// fillRecordingHistorySeries は、シリーズ名の列がない録画履歴の列を埋める (#1499、設計書 4.4)。
// シリーズ名は Go の回の読み方 (episode.Parse) で読むので、SQL では書けず、Go で埋める。
// 列を足す前の履歴と、列を知らない古いバイナリがローリング更新の間に書いた履歴を、次の kotatsu migrate で埋める。
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
	state, err := readSchemaState(ctx, pool)
	if err != nil {
		return err
	}
	latest := SchemaVersion()
	if state.schemaVersion < latest {
		return fmt.Errorf("%w (database: %d, required: %d)", ErrSchemaOutdated, state.schemaVersion, latest)
	}
	if latest < state.minBinaryVersion {
		return fmt.Errorf("%w (database requires binaries that know version %d, this binary knows up to %d)", ErrBinaryOutdated, state.minBinaryVersion, latest)
	}

	return nil
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
