package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"
	"uuid"

	"filippo.io/age"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// WriteOptions は、Write に渡す依存。
type WriteOptions struct {
	DB *pgxpool.Pool
	// Store は、キャプチャの画像を読むデータディレクトリを持つ Store。キャプチャがなければ、データディレクトリがなくてもよい。
	Store *kago.Store
	// SecretKey は、アプリの秘密キー (auth.secret_key)。バックアップに入れる。
	SecretKey string
	Clock     clock.Clock
	Logger    *slog.Logger
}

// WriteResult は、書き出した結果。
type WriteResult struct {
	Manifest *Manifest
	// Bytes は、書いた大きさ (暗号化した後)。
	Bytes int64
	// MissingCaptures は、行はあるのに、データディレクトリに画像がなかったキャプチャの数。
	MissingCaptures int
}

// Write は、DB とキャプチャの画像を、recipients の公開鍵で暗号化して w に書く。
// DB は 1 つの読み取り専用のトランザクション (REPEATABLE READ) で読むので、書き出す間に変わっても、表どうしは同じ時点のものになる。
func Write(ctx context.Context, opts *WriteOptions, w io.Writer, recipients []age.Recipient) (*WriteResult, error) {
	if len(recipients) == 0 {
		return nil, ErrNoRecipients
	}
	if opts.SecretKey == "" {
		return nil, ErrNoSecretKey
	}

	tx, err := opts.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("failed to begin the backup transaction: %w", err)
	}
	// 読むだけなので、終えたら取り消す。
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := liftTimeouts(ctx, tx); err != nil {
		return nil, err
	}

	manifest, err := readManifest(ctx, tx, opts.Clock.Now())
	if err != nil {
		return nil, err
	}
	captures, err := listCaptures(ctx, tx)
	if err != nil {
		return nil, err
	}
	manifest.Captures = len(captures)
	if len(captures) > 0 && !opts.Store.HasDataDir() {
		return nil, ErrNoDataDir
	}

	counter := &countingWriter{w: w, n: 0}
	encrypted, err := age.Encrypt(counter, recipients...)
	if err != nil {
		return nil, fmt.Errorf("failed to start encrypting the backup: %w", err)
	}
	compressed := gzip.NewWriter(encrypted)
	archive := tar.NewWriter(compressed)
	missing, err := writeArchive(ctx, tx, opts, archive, manifest, captures)
	if err != nil {
		return nil, err
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("failed to finish the backup archive: %w", err)
	}
	if err := compressed.Close(); err != nil {
		return nil, fmt.Errorf("failed to finish compressing the backup: %w", err)
	}
	if err := encrypted.Close(); err != nil {
		return nil, fmt.Errorf("failed to finish encrypting the backup: %w", err)
	}

	return &WriteResult{Manifest: manifest, Bytes: counter.n, MissingCaptures: missing}, nil
}

// writeArchive は、tar のエントリーを順に書く。データディレクトリに画像がなかったキャプチャの数を返す。
func writeArchive(ctx context.Context, tx pgx.Tx, opts *WriteOptions, archive *tar.Writer, manifest *Manifest, captures []capture) (int, error) {
	modTime := manifest.CreatedAt
	if err := writeJSON(archive, manifestName, modTime, manifest); err != nil {
		return 0, err
	}
	if err := writeEntry(archive, secretKeyName, modTime, []byte(opts.SecretKey)); err != nil {
		return 0, err
	}
	trailer := &end{TableBytes: make(map[string]int64, len(manifest.Tables)), Captures: 0}
	for _, table := range manifest.Tables {
		n, err := copyTable(ctx, tx, archive, table, modTime)
		if err != nil {
			return 0, err
		}
		trailer.TableBytes[table.Name] = n
	}
	missing := 0
	for _, c := range captures {
		written, err := writeCapture(archive, opts.Store, c, modTime)
		if err != nil {
			return 0, err
		}
		if !written {
			missing++
			opts.Logger.WarnContext(ctx, "capture image not found; backing up the capture without its image",
				slog.String("user_id", c.userID.String()), slog.String("capture_id", c.id.String()))

			continue
		}
		trailer.Captures++
	}

	return missing, writeJSON(archive, endName, modTime, trailer)
}

// liftTimeouts は、トランザクションの中だけ、問い合わせとトランザクションの中で何もしない時間の上限を外す (設計書 1.4)。
// 大きな表の COPY は、設定の上限より長くかかることがある。SET LOCAL はトランザクションの中だけに効くので、プーラーを経由しても残らない。
func liftTimeouts(ctx context.Context, tx pgx.Tx) error {
	for _, sql := range []string{"SET LOCAL statement_timeout = 0", "SET LOCAL idle_in_transaction_session_timeout = 0"} {
		if _, err := tx.Exec(ctx, sql); err != nil {
			return fmt.Errorf("failed to lift the timeouts for the backup: %w", err)
		}
	}

	return nil
}

// readManifest は、スキーマの版と、書き出す表と列を、トランザクションの時点で読む。表は、外部キーで指される表が先になるよう並べる。
func readManifest(ctx context.Context, tx pgx.Tx, now time.Time) (*Manifest, error) {
	var version int64
	if err := tx.QueryRow(ctx, "SELECT COALESCE(max(schema_version), 0) FROM schema_compatibility").Scan(&version); err != nil {
		return nil, fmt.Errorf("failed to read the schema version: %w", err)
	}
	if latest := database.SchemaVersion(); version > latest {
		return nil, fmt.Errorf("%w (database: %d, this binary: %d)", ErrSchemaNewer, version, latest)
	}

	columns, err := readColumns(ctx, tx)
	if err != nil {
		return nil, err
	}
	included := map[string]bool{}
	for _, name := range includedTables() {
		included[name] = true
	}
	names := []string{}
	for _, name := range slices.Sorted(maps.Keys(columns)) {
		switch {
		case isExcluded(name):
		case included[name]:
			names = append(names, name)
		default:
			return nil, fmt.Errorf("%w: %s", ErrUnclassifiedTable, name)
		}
	}

	keys, err := readForeignKeys(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := checkExcludedReferences(keys, included); err != nil {
		return nil, err
	}
	ordered, err := sortByReferences(names, keys)
	if err != nil {
		return nil, err
	}

	tables := make([]*Table, 0, len(ordered))
	for _, name := range ordered {
		tables = append(tables, &Table{Name: name, Columns: columns[name]})
	}

	return &Manifest{
		Format:         formatVersion,
		KotatsuVersion: buildinfo.Get().String(),
		SchemaVersion:  version,
		CreatedAt:      now.UTC(),
		Tables:         tables,
		Captures:       0,
	}, nil
}

// readColumns は、今のスキーマの表ごとの列を、表の中の順に返す。生成列 (GENERATED ... STORED) は COPY FROM で書けないので除く。
func readColumns(ctx context.Context, tx pgx.Tx) (map[string][]string, error) {
	rows, err := tx.Query(ctx, `
SELECT c.relname, a.attname
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = current_schema() AND c.relkind IN ('r', 'p') AND a.attnum > 0 AND NOT a.attisdropped AND a.attgenerated = ''
ORDER BY c.relname, a.attnum`)
	if err != nil {
		return nil, fmt.Errorf("failed to list the columns: %w", err)
	}
	columns := map[string][]string{}
	var table, column string
	if _, err := pgx.ForEachRow(rows, []any{&table, &column}, func() error {
		columns[table] = append(columns[table], column)

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to list the columns: %w", err)
	}

	return columns, nil
}

// foreignKey は、表 child の列 columns が、表 parent を指す外部キー。
type foreignKey struct {
	child   string
	parent  string
	columns []string
}

func readForeignKeys(ctx context.Context, tx pgx.Tx) ([]*foreignKey, error) {
	rows, err := tx.Query(ctx, `
SELECT c.relname, p.relname, array_agg(a.attname::text ORDER BY a.attnum)
FROM pg_constraint k
JOIN pg_class c ON c.oid = k.conrelid
JOIN pg_class p ON p.oid = k.confrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = k.conrelid AND a.attnum = ANY (k.conkey)
WHERE k.contype = 'f' AND n.nspname = current_schema()
GROUP BY k.oid, c.relname, p.relname
ORDER BY c.relname, p.relname`)
	if err != nil {
		return nil, fmt.Errorf("failed to list the foreign keys: %w", err)
	}
	keys := []*foreignKey{}
	var child, parent string
	var columns []string
	if _, err := pgx.ForEachRow(rows, []any{&child, &parent, &columns}, func() error {
		keys = append(keys, &foreignKey{child: child, parent: parent, columns: slices.Clone(columns)})

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to list the foreign keys: %w", err)
	}

	return keys, nil
}

// checkExcludedReferences は、入れる表が、外す表を指していないかを確かめる。
// 指していても、その列を NULL にして書き出す (nullColumns) なら許す。そうでなければ、戻すときに外部キーの制約で失敗する。
func checkExcludedReferences(keys []*foreignKey, included map[string]bool) error {
	for _, key := range keys {
		if !included[key.child] || included[key.parent] {
			continue
		}
		nulled := nullColumns()[key.child]
		if !slices.ContainsFunc(key.columns, func(column string) bool { return !slices.Contains(nulled, column) }) {
			continue
		}

		return fmt.Errorf("%w: %s refers to %s, which is not backed up", ErrUnclassifiedTable, key.child, key.parent)
	}

	return nil
}

// sortByReferences は、names を、外部キーで指される表が先になるよう並べる。順の決まらない表どうしは、名前の順にする。
// 自分を指す外部キーは、1 つの COPY の中で確かめられるので、順に関わらない。表どうしで指し合っていれば、流し込めないので誤りにする。
func sortByReferences(names []string, keys []*foreignKey) ([]string, error) {
	parents := map[string]map[string]bool{}
	for _, name := range names {
		parents[name] = map[string]bool{}
	}
	for _, key := range keys {
		_, child := parents[key.child]
		_, parent := parents[key.parent]
		if child && parent && key.child != key.parent {
			parents[key.child][key.parent] = true
		}
	}

	ordered := make([]string, 0, len(names))
	done := map[string]bool{}
	for len(ordered) < len(names) {
		progressed := false
		for _, name := range names {
			if done[name] {
				continue
			}
			ready := true
			for parent := range parents[name] {
				if !done[parent] {
					ready = false

					break
				}
			}
			if ready {
				ordered = append(ordered, name)
				done[name] = true
				progressed = true
			}
		}
		if !progressed {
			rest := slices.DeleteFunc(slices.Clone(names), func(name string) bool { return done[name] })

			return nil, fmt.Errorf("the foreign keys of these tables refer to each other in a cycle: %s", strings.Join(rest, ", "))
		}
	}

	return ordered, nil
}

// capture は、キャプチャの行の、画像の場所を決める ID の組。
type capture struct {
	userID uuid.UUID
	id     uuid.UUID
}

func listCaptures(ctx context.Context, tx pgx.Tx) ([]capture, error) {
	rows, err := tx.Query(ctx, "SELECT user_id, id FROM captures ORDER BY user_id, id")
	if err != nil {
		return nil, fmt.Errorf("failed to list the captures: %w", err)
	}
	captures := []capture{}
	var c capture
	if _, err := pgx.ForEachRow(rows, []any{&c.userID, &c.id}, func() error {
		captures = append(captures, c)

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to list the captures: %w", err)
	}

	return captures, nil
}

// copyTable は、表 table の行を COPY TO で読み、chunkSize ごとのエントリーにして書く。書いた大きさを返す。
func copyTable(ctx context.Context, tx pgx.Tx, archive *tar.Writer, table *Table, modTime time.Time) (int64, error) {
	nulled := nullColumns()[table.Name]
	exprs := make([]string, 0, len(table.Columns))
	for _, column := range table.Columns {
		if slices.Contains(nulled, column) {
			exprs = append(exprs, "NULL AS "+pgx.Identifier{column}.Sanitize())

			continue
		}
		exprs = append(exprs, pgx.Identifier{column}.Sanitize())
	}
	sql := fmt.Sprintf("COPY (SELECT %s FROM %s) TO STDOUT", strings.Join(exprs, ", "), pgx.Identifier{table.Name}.Sanitize())

	chunks := &chunkWriter{archive: archive, prefix: tablesPrefix + table.Name + "/", modTime: modTime, buf: make([]byte, 0, chunkSize), seq: 0, total: 0}
	if _, err := tx.Conn().PgConn().CopyTo(ctx, chunks, sql); err != nil {
		return 0, fmt.Errorf("failed to copy table %s: %w", table.Name, err)
	}
	if err := chunks.flush(); err != nil {
		return 0, err
	}

	return chunks.total, nil
}

// chunkWriter は、書かれたものを chunkSize ごとに区切り、連番の名前のエントリーにして書く。
type chunkWriter struct {
	archive *tar.Writer
	prefix  string
	modTime time.Time
	buf     []byte
	seq     int
	total   int64
}

func (c *chunkWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		n := min(len(p), chunkSize-len(c.buf))
		c.buf = append(c.buf, p[:n]...)
		p = p[n:]
		written += n
		if len(c.buf) == chunkSize {
			if err := c.flush(); err != nil {
				return written, err
			}
		}
	}

	return written, nil
}

// flush は、ためたものを 1 つのエントリーにして書く。空なら何もしない。行のない表は、エントリーを持たない。
func (c *chunkWriter) flush() error {
	if len(c.buf) == 0 {
		return nil
	}
	if err := writeEntry(c.archive, fmt.Sprintf("%s%06d", c.prefix, c.seq), c.modTime, c.buf); err != nil {
		return err
	}
	c.total += int64(len(c.buf))
	c.seq++
	c.buf = c.buf[:0]

	return nil
}

// writeCapture は、キャプチャ c の画像をエントリーにして書く。画像がデータディレクトリになければ、書かずに false を返す。
func writeCapture(archive *tar.Writer, store *kago.Store, c capture, modTime time.Time) (bool, error) {
	location := kago.CaptureLocation(c.userID, c.id)
	f, err := store.Open(location.PoolID, location.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to open the capture image %s: %w", location.Path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("failed to stat the capture image %s: %w", location.Path, err)
	}
	if info.Size() > maxCaptureSize {
		return false, fmt.Errorf("the capture image %s is larger than %d bytes", location.Path, maxCaptureSize)
	}
	name := capturesPrefix + c.userID.String() + "/" + c.id.String() + captureExt
	if err := archive.WriteHeader(entryHeader(name, modTime, info.Size())); err != nil {
		return false, fmt.Errorf("failed to write the backup entry %s: %w", name, err)
	}
	// 読む間に画像が縮んでも、ヘッダーに書いた大きさのとおりに書けなければ、誤りにする。
	if _, err := io.CopyN(archive, f, info.Size()); err != nil {
		return false, fmt.Errorf("failed to write the backup entry %s: %w", name, err)
	}

	return true, nil
}

func writeJSON(archive *tar.Writer, name string, modTime time.Time, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to encode %s: %w", name, err)
	}

	return writeEntry(archive, name, modTime, data)
}

func writeEntry(archive *tar.Writer, name string, modTime time.Time, data []byte) error {
	if err := archive.WriteHeader(entryHeader(name, modTime, int64(len(data)))); err != nil {
		return fmt.Errorf("failed to write the backup entry %s: %w", name, err)
	}
	if _, err := archive.Write(data); err != nil {
		return fmt.Errorf("failed to write the backup entry %s: %w", name, err)
	}

	return nil
}

func entryHeader(name string, modTime time.Time, size int64) *tar.Header {
	return &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Size:     size,
		Mode:     fileMode,
		ModTime:  modTime,
		Format:   tar.FormatPAX,
	}
}

// countingWriter は、書いた大きさを数える。
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)

	return n, err
}
