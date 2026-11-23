package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"uuid"

	"filippo.io/age"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/usecase/keyrebuild"
)

// RestoreOptions は、Restore に渡す依存。
type RestoreOptions struct {
	DB *pgxpool.Pool
	// Store は、キャプチャの画像を書くデータディレクトリを持つ Store。バックアップにキャプチャがなければ、データディレクトリがなくてもよい。
	Store  *kago.Store
	Logger *slog.Logger
	// CheckSecretKey は、バックアップに入っているアプリの秘密キーを受け取る。DB を変える前に呼ぶ。誤りを返せば、戻さない。
	CheckSecretKey func(secretKey string) error
}

// Restore は、Write で書き出したバックアップを、identities の秘密鍵で読み、空の DB に戻す。
//
// 今のスキーマを当て、1 つのトランザクションの中で表を流し込み、キャプチャの画像をデータディレクトリに書く。
// 流し込み終えたら、書き出したときの版より新しい 1 回だけのスクリプトを、戻した行に流す (#2040)。戻す間は、マイグレーションのロックを持つ。
func Restore(ctx context.Context, opts *RestoreOptions, r io.Reader, identities []age.Identity) (*Manifest, error) {
	decrypted, err := age.Decrypt(r, identities...)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt the backup: %w", err)
	}
	decompressed, err := gzip.NewReader(decrypted)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidBackup, err)
	}
	defer decompressed.Close()
	archive := tar.NewReader(decompressed)

	manifest, err := readManifestEntry(archive)
	if err != nil {
		return nil, err
	}
	secretKey, err := readEntry(archive, secretKeyName, maxSecretKeySize)
	if err != nil {
		return nil, err
	}
	if manifest.Captures > 0 && !opts.Store.HasDataDir() {
		return nil, ErrNoDataDir
	}
	if err := opts.CheckSecretKey(string(secretKey)); err != nil {
		return nil, err
	}

	err = database.WithMigrationLock(ctx, opts.Logger, opts.DB, func(ctx context.Context) error {
		if err := checkEmpty(ctx, opts.DB); err != nil {
			return err
		}
		// 今のスキーマを当ててから流し込む。書き出したときより後に足した列は、既定の値で埋まる (#2040)。
		if err := database.Migrate(ctx, opts.Logger, opts.DB); err != nil {
			return err
		}
		if err := load(ctx, opts, archive, manifest); err != nil {
			return err
		}
		// 書き出したときの版より新しい 1 回だけのスクリプトを、戻した行に流す。正規化した列を埋める処理も、ここで戻した行に効く。
		if err := database.ForgetScriptsAfter(ctx, opts.DB, manifest.SchemaVersion); err != nil {
			return err
		}
		if err := database.Migrate(ctx, opts.Logger, opts.DB); err != nil {
			return err
		}
		// 戻した行のキーも、kotatsu migrate と同じく、今のバイナリの計算に合わせる。
		if err := keyrebuild.Run(ctx, opts.Logger, opts.DB); err != nil {
			return err
		}

		return jobs.Migrate(ctx, opts.Logger, opts.DB)
	})
	if err != nil {
		return nil, err
	}

	return manifest, nil
}

func readManifestEntry(archive *tar.Reader) (*Manifest, error) {
	data, err := readEntry(archive, manifestName, maxManifestSize)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("%w: failed to decode %s: %w", ErrInvalidBackup, manifestName, err)
	}
	if manifest.Format != formatVersion {
		return nil, fmt.Errorf("%w: format %d (this binary reads format %d)", ErrUnsupportedFormat, manifest.Format, formatVersion)
	}
	if latest := database.SchemaVersion(); manifest.SchemaVersion > latest {
		return nil, fmt.Errorf("%w (backup: %d, this binary: %d)", ErrBackupNewer, manifest.SchemaVersion, latest)
	}
	// それより古いファイルには、今のスキーマで消えた列が残っていることがある (#2040)。
	if manifest.SchemaVersion < database.GooseFloorVersion {
		return nil, fmt.Errorf("%w (backup: %d, required: %d or newer)", ErrBackupTooOld, manifest.SchemaVersion, database.GooseFloorVersion)
	}
	seen := map[string]bool{}
	for _, table := range manifest.Tables {
		if table == nil || table.Name == "" || len(table.Columns) == 0 || seen[table.Name] {
			return nil, fmt.Errorf("%w: invalid table list in %s", ErrInvalidBackup, manifestName)
		}
		seen[table.Name] = true
	}

	return &manifest, nil
}

// readEntry は、次のエントリーが name であることを確かめ、limit バイトまで読む。
func readEntry(archive *tar.Reader, name string, limit int64) ([]byte, error) {
	header, err := archive.Next()
	if err != nil {
		return nil, fmt.Errorf("%w: missing %s: %w", ErrInvalidBackup, name, err)
	}
	if header.Name != name {
		return nil, fmt.Errorf("%w: expected %s, found %s", ErrInvalidBackup, name, header.Name)
	}

	return readBounded(archive, header, limit)
}

// readBounded は、エントリーの中身を、limit バイトまで読む。大きすぎるエントリーは、読まずに誤りにする。
func readBounded(archive *tar.Reader, header *tar.Header, limit int64) ([]byte, error) {
	if header.Size < 0 || header.Size > limit {
		return nil, fmt.Errorf("%w: %s is too large (%d bytes)", ErrInvalidBackup, header.Name, header.Size)
	}
	data, err := io.ReadAll(io.LimitReader(archive, header.Size))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read %s: %w", ErrInvalidBackup, header.Name, err)
	}

	return data, nil
}

// checkEmpty は、戻す先の DB (今のスキーマ) に表がないことを確かめる。kotatsu migrate のロックの表 (goose_lock) は除く。
func checkEmpty(ctx context.Context, db *pgxpool.Pool) error {
	var tables []string
	if err := db.QueryRow(ctx, `
SELECT COALESCE(array_agg(c.relname::text ORDER BY c.relname), '{}')
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = current_schema() AND c.relkind IN ('r', 'p') AND c.relname <> 'goose_lock'`).Scan(&tables); err != nil {
		return fmt.Errorf("failed to list the tables of the database: %w", err)
	}
	if len(tables) > 0 {
		shown := tables[:min(len(tables), 5)]

		return fmt.Errorf("%w; it has %d tables (%s)", ErrDatabaseNotEmpty, len(tables), strings.Join(shown, ", "))
	}

	return nil
}

// load は、1 つのトランザクションの中で、表を流し込み、キャプチャの画像を書く。終わりのエントリーまで読めたときだけ確定する。
func load(ctx context.Context, opts *RestoreOptions, archive *tar.Reader, manifest *Manifest) error {
	tx, err := opts.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin the restore transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := liftTimeouts(ctx, tx); err != nil {
		return err
	}
	if err := truncate(ctx, tx, manifest.Tables); err != nil {
		return err
	}

	loader := &loader{tx: tx, store: opts.Store, manifest: manifest, next: 0, tableBytes: map[string]int64{}, captures: nil, written: 0}
	if err := loader.run(ctx, archive); err != nil {
		return err
	}
	if err := resetIdentities(ctx, tx, manifest.Tables); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit the restore: %w", err)
	}
	opts.Logger.InfoContext(ctx, "backup loaded", slog.Int("tables", len(manifest.Tables)), slog.Int("captures", loader.written))

	return nil
}

// truncate は、流し込む表を空にする。マイグレーションが入れた行 (recording_failure_counts など) と、主キーがぶつからないようにする。
func truncate(ctx context.Context, tx pgx.Tx, tables []*Table) error {
	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, pgx.Identifier{table.Name}.Sanitize())
	}
	if len(names) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, "TRUNCATE "+strings.Join(names, ", ")+" CASCADE"); err != nil {
		return fmt.Errorf("failed to empty the tables to restore: %w", err)
	}

	return nil
}

// loader は、tar のエントリーを順に読んで、DB とデータディレクトリに戻す。
type loader struct {
	tx       pgx.Tx
	store    *kago.Store
	manifest *Manifest
	// next は、次に流し込める表の manifest.Tables の位置。表は manifest の順にしか現れない。
	next       int
	tableBytes map[string]int64
	// captures は、戻したキャプチャの行。画像は、行のあるものだけを書く。最初の画像を読むときに引く。
	captures map[capture]bool
	written  int
}

func (l *loader) run(ctx context.Context, archive *tar.Reader) error {
	header, err := archive.Next()
	ended := false
	for !errors.Is(err, io.EOF) {
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidBackup, err)
		}
		if ended {
			return fmt.Errorf("%w: unexpected entry %s after %s", ErrInvalidBackup, header.Name, endName)
		}
		switch {
		case strings.HasPrefix(header.Name, tablesPrefix):
			header, err = l.copyTable(ctx, archive, header)

			continue
		case strings.HasPrefix(header.Name, capturesPrefix):
			if err := l.writeCapture(ctx, archive, header); err != nil {
				return err
			}
		case header.Name == endName:
			if err := l.checkEnd(archive, header); err != nil {
				return err
			}
			ended = true
		default:
			return fmt.Errorf("%w: unexpected entry %s", ErrInvalidBackup, header.Name)
		}
		header, err = archive.Next()
	}
	if !ended {
		return fmt.Errorf("%w: the file ends before %s; it may be truncated", ErrInvalidBackup, endName)
	}

	return nil
}

// copyTable は、header から始まる 1 つの表のエントリーを、COPY FROM で流し込む。表の後の次のエントリーを返す。
func (l *loader) copyTable(ctx context.Context, archive *tar.Reader, header *tar.Header) (*tar.Header, error) {
	name, seq, err := parseChunkName(header.Name)
	if err != nil {
		return nil, err
	}
	index := slices.IndexFunc(l.manifest.Tables, func(table *Table) bool { return table.Name == name })
	if index < l.next || seq != 0 {
		return nil, fmt.Errorf("%w: unexpected entry %s", ErrInvalidBackup, header.Name)
	}
	l.next = index + 1
	table := l.manifest.Tables[index]

	columns := make([]string, 0, len(table.Columns))
	for _, column := range table.Columns {
		columns = append(columns, pgx.Identifier{column}.Sanitize())
	}
	sql := fmt.Sprintf("COPY %s (%s) FROM STDIN", pgx.Identifier{table.Name}.Sanitize(), strings.Join(columns, ", "))
	src := &chunkReader{archive: archive, table: name, header: header, seq: 0, next: nil, nextErr: nil, total: 0}
	if _, err := l.tx.Conn().PgConn().CopyFrom(ctx, src, sql); err != nil {
		return nil, fmt.Errorf("failed to restore table %s: %w", name, err)
	}
	if src.nextErr != nil && !errors.Is(src.nextErr, io.EOF) {
		return nil, fmt.Errorf("%w: %w", ErrInvalidBackup, src.nextErr)
	}
	l.tableBytes[name] = src.total

	return src.next, src.nextErr
}

// parseChunkName は、tables/<表>/<連番> の名前を読む。
func parseChunkName(name string) (string, int, error) {
	table, seq, ok := strings.Cut(strings.TrimPrefix(name, tablesPrefix), "/")
	n, err := strconv.Atoi(seq)
	if !ok || table == "" || err != nil || n < 0 {
		return "", 0, fmt.Errorf("%w: invalid entry %s", ErrInvalidBackup, name)
	}

	return table, n, nil
}

// chunkReader は、1 つの表の連番のエントリーを、つないで読む。表の後のエントリーに来たら、そのヘッダーを next に残して終える。
type chunkReader struct {
	archive *tar.Reader
	table   string
	header  *tar.Header
	seq     int
	// next と nextErr は、表の後の次のエントリーと、それを読んだときの誤り (終わりなら io.EOF)。
	next    *tar.Header
	nextErr error
	total   int64
}

func (c *chunkReader) Read(p []byte) (int, error) {
	for {
		if c.header == nil {
			return 0, io.EOF
		}
		n, err := c.archive.Read(p)
		c.total += int64(n)
		if n > 0 || !errors.Is(err, io.EOF) {
			if errors.Is(err, io.EOF) {
				err = nil
			}

			return n, err
		}
		// このエントリーを読み終えたので、次のエントリーを見る。同じ表の続きなら読み続ける。
		next, err := c.archive.Next()
		if err != nil {
			c.header, c.next, c.nextErr = nil, nil, err

			continue
		}
		name, seq, parseErr := parseChunkName(next.Name)
		if !strings.HasPrefix(next.Name, tablesPrefix) || parseErr != nil || name != c.table {
			c.header, c.next, c.nextErr = nil, next, nil

			continue
		}
		if seq != c.seq+1 {
			return 0, fmt.Errorf("%w: unexpected entry %s", ErrInvalidBackup, next.Name)
		}
		c.header, c.seq = next, seq
	}
}

// writeCapture は、キャプチャの画像を、データディレクトリに書く。名前は ID の組に限り、戻した行のあるものだけを書く。
func (l *loader) writeCapture(ctx context.Context, archive *tar.Reader, header *tar.Header) error {
	c, err := parseCaptureName(header.Name)
	if err != nil {
		return err
	}
	if l.captures == nil {
		rows, err := listCaptures(ctx, l.tx)
		if err != nil {
			return err
		}
		l.captures = make(map[capture]bool, len(rows))
		for _, row := range rows {
			l.captures[row] = true
		}
	}
	if !l.captures[c] {
		return fmt.Errorf("%w: %s has no capture row", ErrInvalidBackup, header.Name)
	}
	data, err := readBounded(archive, header, maxCaptureSize)
	if err != nil {
		return err
	}
	location := kago.CaptureLocation(c.userID, c.id)
	if err := l.store.WriteFile(location.PoolID, location.Path, data); err != nil {
		return fmt.Errorf("failed to write the capture image %s: %w", location.Path, err)
	}
	l.written++

	return nil
}

// parseCaptureName は、captures/<ユーザーの ID>/<キャプチャの ID>.png の名前を読む。ID は正規の形 (小文字のハイフン区切り) に限る。
func parseCaptureName(name string) (capture, error) {
	invalid := fmt.Errorf("%w: invalid entry %s", ErrInvalidBackup, name)
	user, file, ok := strings.Cut(strings.TrimPrefix(name, capturesPrefix), "/")
	id, png := strings.CutSuffix(file, captureExt)
	if !ok || !png {
		return capture{}, invalid
	}
	userID, err := uuid.Parse(user)
	if err != nil || userID.String() != user {
		return capture{}, invalid
	}
	captureID, err := uuid.Parse(id)
	if err != nil || captureID.String() != id {
		return capture{}, invalid
	}

	return capture{userID: userID, id: captureID}, nil
}

// checkEnd は、終わりのエントリーの大きさと、読んだ大きさが合うかを確かめる。
func (l *loader) checkEnd(archive *tar.Reader, header *tar.Header) error {
	data, err := readBounded(archive, header, maxManifestSize)
	if err != nil {
		return err
	}
	var trailer end
	if err := json.Unmarshal(data, &trailer); err != nil {
		return fmt.Errorf("%w: failed to decode %s: %w", ErrInvalidBackup, endName, err)
	}
	for _, table := range l.manifest.Tables {
		if trailer.TableBytes[table.Name] != l.tableBytes[table.Name] {
			return fmt.Errorf("%w: table %s has %d bytes, but %d bytes were written", ErrInvalidBackup,
				table.Name, l.tableBytes[table.Name], trailer.TableBytes[table.Name])
		}
	}
	if trailer.Captures != l.written {
		return fmt.Errorf("%w: %d capture images were restored, but %d were written", ErrInvalidBackup, l.written, trailer.Captures)
	}

	return nil
}

// resetIdentities は、戻した表の IDENTITY の列の連番を、戻した値の次から振るよう進める (recordings.api_id など)。
// COPY FROM は列の値をそのまま書き、連番を進めないので、進めないと、次に足す行の値が戻した行とぶつかる。
func resetIdentities(ctx context.Context, tx pgx.Tx, tables []*Table) error {
	rows, err := tx.Query(ctx, `
SELECT c.relname, a.attname
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = current_schema() AND a.attidentity <> '' AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY c.relname, a.attnum`)
	if err != nil {
		return fmt.Errorf("failed to list the identity columns: %w", err)
	}
	type identity struct{ table, column string }
	identities := []identity{}
	var table, column string
	if _, err := pgx.ForEachRow(rows, []any{&table, &column}, func() error {
		identities = append(identities, identity{table: table, column: column})

		return nil
	}); err != nil {
		return fmt.Errorf("failed to list the identity columns: %w", err)
	}

	for _, id := range identities {
		if !slices.ContainsFunc(tables, func(t *Table) bool { return t.Name == id.table }) {
			continue
		}
		quotedTable, quotedColumn := pgx.Identifier{id.table}.Sanitize(), pgx.Identifier{id.column}.Sanitize()
		sql := fmt.Sprintf("SELECT setval(pg_get_serial_sequence($1, $2), COALESCE((SELECT max(%s) FROM %s), 0) + 1, false)", quotedColumn, quotedTable)
		if _, err := tx.Exec(ctx, sql, quotedTable, id.column); err != nil {
			return fmt.Errorf("failed to reset the sequence of %s.%s: %w", id.table, id.column, err)
		}
	}

	return nil
}
