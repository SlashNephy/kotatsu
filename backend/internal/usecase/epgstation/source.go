// Package epgstation は、EPGStation の DB を読み取り専用で読む (設計書 6.3)。
//
// EPGStation は TypeORM で MySQL、MariaDB、SQLite のどれかにデータを置く。
// テーブル名は snake_case、列名は camelCase で、どの DB でも識別子の大文字と小文字は区別されないので、
// クエリは識別子を引用符で囲まずに書き、方言ごとに書き分けない。
package epgstation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-sql-driver/mysql"
	// SQLite のドライバ。CGO を使わずにバイナリを配布できるよう、Pure Go の実装を使う。
	_ "modernc.org/sqlite"
)

// requiredMigration は、読み取りに使う列がそろっている、EPGStation の最後のマイグレーションの名前の接頭辞。
// 末尾のタイムスタンプは DB の種類ごとに違うので、接頭辞で比べる。
const requiredMigration = "AddEventRelay"

var (
	// ErrUnsupportedSource は、接続先の URL の形式に対応していないことを表す。
	ErrUnsupportedSource = errors.New("unsupported EPGStation database URL")
	// ErrUnsupportedSchema は、EPGStation の DB のスキーマが古いか、EPGStation の DB ではないことを表す。
	ErrUnsupportedSchema = errors.New("unsupported EPGStation database schema")

	// errMalformedURL は、接続先の URL を読めなかったことを表す。
	errMalformedURL = errors.New("the URL is malformed; percent-encode the symbols in the password")
)

// Source は、読み取り専用で開いた EPGStation の DB。
type Source struct {
	db *sql.DB
}

// Open は、EPGStation の DB を読み取り専用で開く。rawURL は次のどれか。
//
//   - mysql://user:password@host:3306/database (MariaDB も同じ。クエリは go-sql-driver/mysql の DSN の引数として渡す)
//   - sqlite:///path/to/database.db
func Open(ctx context.Context, rawURL string) (*Source, error) {
	driver, dsn, err := parseSourceURL(rawURL)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open the EPGStation database: %w", err)
	}
	// ファイルが無い、認証に失敗した、などの接続の失敗を、スキーマの問題と区別して返す。
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("failed to connect to the EPGStation database: %w", err)
	}
	source := &Source{db: db}
	if err := source.checkSchema(ctx); err != nil {
		_ = db.Close()

		return nil, err
	}

	return source, nil
}

// Close は DB を閉じる。
func (s *Source) Close() error {
	return s.db.Close()
}

// parseSourceURL は、接続先の URL を database/sql のドライバ名と DSN にする。
func parseSourceURL(rawURL string) (string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		// url.Parse のエラーは、URL の全文や、読めなかったポートのような一部を含む。URL はパスワードを含みうるので、理由を伏せる。
		return "", "", fmt.Errorf("%w: %w", ErrUnsupportedSource, errMalformedURL)
	}

	switch u.Scheme {
	case "mysql", "mariadb":
		return "mysql", mysqlDSN(u), nil
	case "sqlite":
		return sqliteDSN(u)
	default:
		return "", "", fmt.Errorf("%w: scheme must be mysql, mariadb, or sqlite: %q", ErrUnsupportedSource, u.Scheme)
	}
}

// mysqlDSN は、URL を go-sql-driver/mysql の DSN にする。
func mysqlDSN(u *url.URL) string {
	cfg := mysql.NewConfig()
	cfg.User = u.User.Username()
	cfg.Passwd, _ = u.User.Password()
	cfg.Net = "tcp"
	cfg.Addr = u.Host
	cfg.DBName = strings.TrimPrefix(u.Path, "/")
	dsn := cfg.FormatDSN()
	if u.RawQuery != "" {
		separator := "?"
		if strings.Contains(dsn, "?") {
			separator = "&"
		}
		dsn += separator + u.RawQuery
	}

	return dsn
}

// sqliteDSN は、URL を SQLite の DSN にする。
// 書き込みを防ぐため、ファイルを読み取り専用で開き、接続も query_only にする。
// mode=ro では、ファイルが無いときに空の DB を作らず、接続に失敗する。
func sqliteDSN(u *url.URL) (string, string, error) {
	path := u.Path
	if u.Opaque != "" {
		path = u.Opaque
	}
	if path == "" {
		return "", "", fmt.Errorf("%w: sqlite URL must have a file path", ErrUnsupportedSource)
	}
	query := url.Values{}
	query.Set("mode", "ro")
	query.Add("_pragma", "query_only(1)")
	dsn := &url.URL{Scheme: "file", Opaque: (&url.URL{Path: path}).EscapedPath(), RawQuery: query.Encode()}

	return "sqlite", dsn.String(), nil
}

// checkSchema は、EPGStation の DB で、読み取りに使う列がそろうマイグレーションまで適用されているかを確かめる。
func (s *Source) checkSchema(ctx context.Context) error {
	return s.readOnly(ctx, func(tx *sql.Tx) error {
		var n int
		err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM migrations WHERE name LIKE ?", requiredMigration+"%").Scan(&n)
		if err != nil {
			return fmt.Errorf("%w: failed to read the migrations table: %w", ErrUnsupportedSchema, err)
		}
		if n == 0 {
			return fmt.Errorf("%w: migration %s has not been applied; update EPGStation to the latest version and start it once to migrate the database", ErrUnsupportedSchema, requiredMigration)
		}

		return nil
	})
}

// readOnly は、読み取り専用のトランザクションで fn を実行する。トランザクションはコミットせずに捨てる。
// MySQL と MariaDB では START TRANSACTION READ ONLY になるので、書き込みは DB に拒まれる。
func (s *Source) readOnly(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelDefault, ReadOnly: true})
	if err != nil {
		return fmt.Errorf("failed to begin a read-only transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	return fn(tx)
}
