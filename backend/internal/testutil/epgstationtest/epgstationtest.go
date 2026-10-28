// Package epgstationtest は、EPGStation の DB を使うテストのために、EPGStation のスキーマを適用した空の DB を用意する。
//
// SQLite は一時ディレクトリにファイルを作る。MySQL と MariaDB は、環境変数 KOTATSU_TEST_EPGSTATION_MYSQL_URL と
// KOTATSU_TEST_EPGSTATION_MARIADB_URL で接続先 (mysql://user:password@host:port/) を与える。データベースを作れる権限が要る。
// 未設定なら、ローカルではテストを飛ばし、CI (環境変数 CI がある) では失敗させる。
package epgstationtest

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	// SQLite のドライバ。
	_ "modernc.org/sqlite"
)

// Dialect は、EPGStation が対応する DB の種類。
type Dialect string

const (
	SQLite  Dialect = "sqlite"
	MySQL   Dialect = "mysql"
	MariaDB Dialect = "mariadb"
)

// EnvMySQLURL と EnvMariaDBURL は、テストに使う MySQL と MariaDB の接続先を与える環境変数。
const (
	EnvMySQLURL   = "KOTATSU_TEST_EPGSTATION_MYSQL_URL"
	EnvMariaDBURL = "KOTATSU_TEST_EPGSTATION_MARIADB_URL"
)

//go:embed schema/*.sql
var schemas embed.FS

// Dialects は、テストで確かめる DB の種類を返す。
func Dialects() []Dialect {
	return []Dialect{SQLite, MySQL, MariaDB}
}

// DB は、EPGStation のスキーマを適用したテスト用の DB。
type DB struct {
	// URL は epgstation.Open に渡す接続先。
	URL string
	// Writer は、テストのデータを入れるための、書き込みのできる接続。
	Writer *sql.DB
	// Path は、SQLite の DB のファイルのパス。SQLite のときだけ持つ。
	Path string
}

// New は、EPGStation のスキーマを適用した空の DB を作る。DB はテストの終了時に消す。
func New(t *testing.T, dialect Dialect) *DB {
	t.Helper()

	var db *DB
	switch dialect {
	case SQLite:
		db = newSQLite(t)
	case MySQL:
		db = newMySQL(t, EnvMySQLURL)
	case MariaDB:
		db = newMySQL(t, EnvMariaDBURL)
	default:
		t.Fatalf("unknown dialect %q", dialect)
	}

	schemaName := "sqlite"
	if dialect != SQLite {
		schemaName = "mysql"
	}
	Exec(t, db.Writer, statements(t, schemaName)...)

	return db
}

// Exec は、queries を順に実行する。
func Exec(t *testing.T, db *sql.DB, queries ...string) {
	t.Helper()

	for _, query := range queries {
		_, err := db.ExecContext(t.Context(), query)
		require.NoError(t, err, query)
	}
}

func newSQLite(t *testing.T) *DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "database.db")
	writer, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = writer.Close()
	})

	return &DB{URL: SQLiteURL(path), Writer: writer, Path: path}
}

// SQLiteURL は、SQLite のファイル path を指す sqlite: の URL を返す。
// Windows のパス (C:\...) は、利用者が書くのと同じく sqlite:///C:/... の形にする。
func SQLiteURL(path string) string {
	return (&url.URL{Scheme: "sqlite", Path: "/" + strings.TrimPrefix(filepath.ToSlash(path), "/")}).String()
}

func newMySQL(t *testing.T, env string) *DB {
	t.Helper()

	adminURL, ok := os.LookupEnv(env)
	if !ok || adminURL == "" {
		if _, inCI := os.LookupEnv("CI"); inCI {
			t.Fatalf("%s must be set in CI", env)
		}
		t.Skipf("%s is not set; skipping a test that needs it", env)
	}
	u, err := url.Parse(adminURL)
	require.NoError(t, err)

	admin, err := sql.Open("mysql", dsn(u, ""))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = admin.Close()
	})

	name := "epgstation_test_" + randomSuffix(t)
	Exec(t, admin, "CREATE DATABASE "+name+" CHARACTER SET utf8mb4")
	t.Cleanup(func() {
		// t.Context() はクリーンアップの時点で終わっているので、別の context を使う。
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP DATABASE "+name); err != nil {
			t.Errorf("failed to drop the test database %s: %v", name, err)
		}
	})

	writer, err := sql.Open("mysql", dsn(u, name))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = writer.Close()
	})
	u.Path = "/" + name

	return &DB{URL: u.String(), Writer: writer, Path: ""}
}

func dsn(u *url.URL, database string) string {
	cfg := mysql.NewConfig()
	cfg.User = u.User.Username()
	cfg.Passwd, _ = u.User.Password()
	cfg.Net = "tcp"
	cfg.Addr = u.Host
	cfg.DBName = database

	return cfg.FormatDSN()
}

// statements は、スキーマのファイルの文を返す。ファイルは 1 行に 1 文で書いてある。
func statements(t *testing.T, name string) []string {
	t.Helper()

	f, err := schemas.Open("schema/" + name + ".sql")
	require.NoError(t, err)
	defer func() {
		_ = f.Close()
	}()

	queries := []string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		queries = append(queries, strings.TrimSuffix(line, ";"))
	}
	require.NoError(t, scanner.Err())

	return queries
}

func randomSuffix(t *testing.T) string {
	t.Helper()

	b := make([]byte, 6)
	_, err := rand.Read(b)
	require.NoError(t, err)

	return hex.EncodeToString(b)
}
