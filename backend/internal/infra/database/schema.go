package database

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"ariga.io/atlas/sql/postgres"
	"ariga.io/atlas/sql/schema"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/SlashNephy/kotatsu/db"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// ErrDestructiveChange は、スキーマの差に、表、列、型を消す変更があることを表す (#2040)。
// 消すときは、差を当てる前に流す 1 回だけのスクリプト (db/scripts/*.before.sql) で消す。
var ErrDestructiveChange = errors.New("the schema change drops tables, columns or types; drop them in a one-time script")

// desiredSchemaName は、埋め込んだ schema.sql を流して読み取る、一時のスキーマの名前。
// 読み取ったらトランザクションごと取り消すので、DB には残らない。
const desiredSchemaName = "kotatsu_desired_schema"

// ignoredObjects は、schema.sql の外で作る表と型の名前 (glob) を返す。比べる対象から外し、消す差にしない。
// River の表と型は rivermigrate が、goose の表は、新しい仕組みに移る前の kotatsu migrate が作る (#2040)。
func ignoredObjects() []string {
	return []string{"river_*", "goose_db_version", "goose_lock"}
}

// schemaState は、kotatsu migrate が記録した、DB のスキーマの状態。
type schemaState struct {
	// hasCompatibility は、スキーマの版を記録する表 (schema_compatibility) があるか。なければ空の DB である。
	hasCompatibility bool
	// hasScripts は、流したスクリプトを記録する表 (schema_scripts) があるか。
	hasScripts bool
	// schemaVersion は、当てたスキーマの版。goose のマイグレーションで上げていた DB では 0。
	schemaVersion int64
	// minBinaryVersion は、動かしてよいバイナリの下限。
	minBinaryVersion int64
	// gooseVersion は、goose のマイグレーションで当てた最新の版。goose で上げていない DB では 0。
	gooseVersion int64
}

// readSchemaState は、DB のスキーマの状態を読む。表や列がなければ、ないものとして読む。
func readSchemaState(ctx context.Context, pool *pgxpool.Pool) (*schemaState, error) {
	state := &schemaState{hasCompatibility: false, hasScripts: false, schemaVersion: 0, minBinaryVersion: 0, gooseVersion: 0}
	var hasGoose, hasSchemaVersion bool
	if err := pool.QueryRow(ctx, `SELECT
		to_regclass('public.schema_compatibility') IS NOT NULL,
		to_regclass('public.schema_scripts') IS NOT NULL,
		to_regclass('public.goose_db_version') IS NOT NULL,
		EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'schema_compatibility' AND column_name = 'schema_version'
		)`).Scan(&state.hasCompatibility, &state.hasScripts, &hasGoose, &hasSchemaVersion); err != nil {
		return nil, fmt.Errorf("failed to read the schema state: %w", err)
	}
	if state.hasCompatibility {
		query := "SELECT COALESCE(max(min_binary_version), 0), 0 FROM schema_compatibility"
		if hasSchemaVersion {
			query = "SELECT COALESCE(max(min_binary_version), 0), COALESCE(max(schema_version), 0) FROM schema_compatibility"
		}
		if err := pool.QueryRow(ctx, query).Scan(&state.minBinaryVersion, &state.schemaVersion); err != nil {
			return nil, fmt.Errorf("failed to read the schema version: %w", err)
		}
	}
	if hasGoose {
		if err := pool.QueryRow(ctx, "SELECT COALESCE(max(version_id), 0) FROM goose_db_version WHERE is_applied").
			Scan(&state.gooseVersion); err != nil {
			return nil, fmt.Errorf("failed to read the goose version: %w", err)
		}
	}

	return state, nil
}

// applySchema は、埋め込んだ schema.sql と DB の差を、1 つのトランザクションで当てる。
func applySchema(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	conn := stdlib.OpenDBFromPool(pool)
	defer conn.Close()

	desired, err := desiredSchema(ctx, conn)
	if err != nil {
		return err
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin the schema change: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	driver, err := postgres.Open(tx)
	if err != nil {
		return fmt.Errorf("failed to open the schema driver: %w", err)
	}
	current, err := driver.InspectSchema(ctx, "public", &schema.InspectOptions{Exclude: ignoredObjects()})
	if err != nil {
		return fmt.Errorf("failed to inspect the database schema: %w", err)
	}
	changes, err := driver.SchemaDiff(current, desired)
	if err != nil {
		return fmt.Errorf("failed to diff the database schema: %w", err)
	}
	if drops := destructiveChanges(changes); len(drops) > 0 {
		return fmt.Errorf("%w: %s", ErrDestructiveChange, strings.Join(drops, ", "))
	}
	if len(changes) == 0 {
		return nil
	}
	plan, err := driver.PlanChanges(ctx, "kotatsu", changes)
	if err != nil {
		return fmt.Errorf("failed to plan the schema change: %w", err)
	}
	for _, change := range plan.Changes {
		if _, err := tx.ExecContext(ctx, change.Cmd, change.Args...); err != nil {
			return fmt.Errorf("failed to apply the schema change %q: %w", change.Cmd, err)
		}
		logger.InfoContext(ctx, "schema change applied", slog.String("statement", change.Cmd))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit the schema change: %w", err)
	}

	return nil
}

// desiredSchema は、埋め込んだ schema.sql を一時のスキーマに流して読み取る。
// 流したものは、トランザクションごと取り消す。SET LOCAL を使うので、トランザクションプーリングを経由しても、ほかの接続に残らない。
func desiredSchema(ctx context.Context, conn *sql.DB) (*schema.Schema, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin reading schema.sql: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "CREATE SCHEMA "+desiredSchemaName); err != nil {
		return nil, fmt.Errorf("failed to create the schema to read schema.sql: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "SET LOCAL search_path TO "+desiredSchemaName); err != nil {
		return nil, fmt.Errorf("failed to set the search path to read schema.sql: %w", err)
	}
	if _, err := tx.ExecContext(ctx, db.Schema); err != nil {
		return nil, fmt.Errorf("failed to run schema.sql: %w", err)
	}
	driver, err := postgres.Open(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to open the schema driver: %w", err)
	}
	desired, err := driver.InspectSchema(ctx, desiredSchemaName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect schema.sql: %w", err)
	}
	// DB の public と比べるので、同じ名前にする。表は、このスキーマを指しているので、表の修飾も public になる。
	desired.Name = "public"

	return desired, nil
}

// destructiveChanges は、changes のうち、表、列、型を消す変更を、消すものの名前で返す。
func destructiveChanges(changes []schema.Change) []string {
	drops := []string{}
	for _, change := range changes {
		switch c := change.(type) {
		case *schema.DropTable:
			drops = append(drops, "table "+c.T.Name)
		case *schema.DropObject:
			drops = append(drops, fmt.Sprintf("object %v", objectName(c.O)))
		case *schema.ModifyTable:
			for _, tc := range c.Changes {
				if dc, ok := tc.(*schema.DropColumn); ok {
					drops = append(drops, "column "+c.T.Name+"."+dc.C.Name)
				}
			}
		}
	}

	return drops
}

// objectName は、表のほかのスキーマのもの (列挙型など) の名前を返す。
func objectName(o schema.Object) string {
	if e, ok := o.(*schema.EnumType); ok {
		return e.T
	}

	return fmt.Sprintf("%T", o)
}

// scriptPhase は、1 回だけのスクリプトを流すときが、差を当てる前か後か。
type scriptPhase string

const (
	phaseBefore scriptPhase = "before"
	phaseAfter  scriptPhase = "after"
)

// script は、データを書き換える 1 回だけのスクリプト (db/scripts)。
// SQL で書けない書き換え (Go で正規化し直すものなど) は、body の代わりに run を持つ (goScripts)。
type script struct {
	version int64
	name    string
	phase   scriptPhase
	body    string
	run     func(ctx context.Context, tx pgx.Tx) error
}

// goScripts は、Go で書いた 1 回だけのスクリプト。版は、足したときに上げたスキーマの版 (db/schema.sum) にする。
// 足した版より古いスキーマで書き出したバックアップを戻すと、また流す。
func goScripts() []*script {
	return []*script{
		{version: renormalizeDashVariantsVersion, name: "renormalize_dash_variants", phase: phaseAfter, body: "", run: renormalizeDashVariants},
	}
}

// createScriptsTable は、流したスクリプトを記録する表を作る。schema.sql の schema_scripts と同じ定義にする。
// goose から移る DB で、差を当てる前に流すスクリプトを記録するのに使う。
const createScriptsTable = "CREATE TABLE IF NOT EXISTS schema_scripts (version bigint PRIMARY KEY, applied_at timestamptz NOT NULL)"

// loadScripts は、fsys の scripts/ にある 1 回だけのスクリプトを、版の古い順に読む。
// ファイルの名前は「版_名前.before.sql」か「版_名前.after.sql」にする。
func loadScripts(fsys fs.FS) ([]*script, error) {
	names, err := fs.Glob(fsys, "scripts/*.sql")
	if err != nil {
		return nil, fmt.Errorf("failed to list the one-time scripts: %w", err)
	}
	scripts := make([]*script, 0, len(names))
	seen := map[int64]string{}
	for _, path := range names {
		name := strings.TrimPrefix(path, "scripts/")
		prefix, rest, ok := strings.Cut(name, "_")
		version, err := strconv.ParseInt(prefix, 10, 64)
		if !ok || err != nil {
			return nil, fmt.Errorf("invalid one-time script name %s: the name must start with the version", name)
		}
		var phase scriptPhase
		switch {
		case strings.HasSuffix(rest, ".before.sql"):
			phase = phaseBefore
		case strings.HasSuffix(rest, ".after.sql"):
			phase = phaseAfter
		default:
			return nil, fmt.Errorf("invalid one-time script name %s: the name must end with .before.sql or .after.sql", name)
		}
		if other, ok := seen[version]; ok {
			return nil, fmt.Errorf("one-time scripts %s and %s have the same version", other, name)
		}
		seen[version] = name
		body, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("failed to read the one-time script %s: %w", name, err)
		}
		scripts = append(scripts, &script{version: version, name: name, phase: phase, body: string(body), run: nil})
	}
	for _, s := range goScripts() {
		if other, ok := seen[s.version]; ok {
			return nil, fmt.Errorf("one-time scripts %s and %s have the same version", other, s.name)
		}
		seen[s.version] = s.name
		scripts = append(scripts, s)
	}
	slices.SortFunc(scripts, func(a, b *script) int { return cmp.Compare(a.version, b.version) })

	return scripts, nil
}

// appliedScripts は、流したスクリプトの版を返す。記録する表がまだなければ、空を返す。
func appliedScripts(ctx context.Context, pool *pgxpool.Pool, state *schemaState) (map[int64]bool, error) {
	applied := map[int64]bool{}
	if !state.hasScripts {
		return applied, nil
	}
	versions, err := sqlc.New(pool).ListAppliedSchemaScripts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the applied one-time scripts: %w", err)
	}
	for _, version := range versions {
		applied[version] = true
	}

	return applied, nil
}

// runScripts は、scripts を古い順に流す。
func runScripts(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, scripts []*script) error {
	for _, s := range scripts {
		if err := runScript(ctx, logger, pool, s); err != nil {
			return err
		}
	}

	return nil
}

// runScript は、1 回だけのスクリプトを 1 つのトランザクションで流し、同じトランザクションで流したことを記録する。
func runScript(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, s *script) error {
	start := time.Now()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin the one-time script %s: %w", s.name, err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if s.run != nil {
		err = s.run(ctx, tx)
	} else {
		_, err = tx.Exec(ctx, s.body)
	}
	if err != nil {
		return fmt.Errorf("failed to run the one-time script %s: %w", s.name, err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO schema_scripts (version, applied_at) VALUES ($1, $2)", s.version, time.Now()); err != nil {
		return fmt.Errorf("failed to record the one-time script %s: %w", s.name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit the one-time script %s: %w", s.name, err)
	}
	logger.InfoContext(ctx, "one-time script applied", slog.String("script", s.name), slog.Duration("duration", time.Since(start)))

	return nil
}
