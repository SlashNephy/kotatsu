package epgstation_test

import (
	"crypto/sha256"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/epgstationtest"
	"github.com/SlashNephy/kotatsu/internal/usecase/epgstation"
)

// mysqlName と sqliteName は、URL のスキームと database/sql のドライバ名に共通する名前。
const (
	mysqlName  = "mysql"
	sqliteName = "sqlite"
)

func TestParseSourceURL(t *testing.T) {
	t.Parallel()

	sqlitePath := filepath.Join(t.TempDir(), "データ ベース.db")
	require.NoError(t, os.WriteFile(sqlitePath, []byte{}, 0o600))

	tests := []struct {
		name       string
		rawURL     string
		wantDriver string
		wantDSN    string
		wantErr    error
	}{
		{
			name:       "mysql with an escaped password",
			rawURL:     (&url.URL{Scheme: mysqlName, User: url.UserPassword("epgstation", "p@ss"), Host: "db.example:3306", Path: "/epgstation"}).String(),
			wantDriver: mysqlName,
			wantDSN:    "epgstation:p@ss@tcp(db.example:3306)/epgstation",
			wantErr:    nil,
		},
		{
			name:       "mariadb with driver parameters",
			rawURL:     "mariadb://root@127.0.0.1/epgstation?tls=true&charset=utf8mb4",
			wantDriver: mysqlName,
			wantDSN:    "root@tcp(127.0.0.1)/epgstation?tls=true&charset=utf8mb4",
			wantErr:    nil,
		},
		{
			name:       "sqlite opens the file read-only",
			rawURL:     epgstationtest.SQLiteURL(sqlitePath),
			wantDriver: sqliteName,
			wantDSN:    "file:" + (&url.URL{Path: "/" + strings.TrimPrefix(filepath.ToSlash(sqlitePath), "/")}).EscapedPath() + "?_pragma=query_only%281%29&mode=ro",
			wantErr:    nil,
		},
		{
			name:       "unknown scheme",
			rawURL:     "postgres://localhost/epgstation",
			wantDriver: "",
			wantDSN:    "",
			wantErr:    epgstation.ErrUnsupportedSource,
		},
		{
			name:       "sqlite without a path",
			rawURL:     "sqlite://",
			wantDriver: "",
			wantDSN:    "",
			wantErr:    epgstation.ErrUnsupportedSource,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			driver, dsn, err := epgstation.ParseSourceURL(tt.rawURL)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantDriver, driver)
			assert.Equal(t, tt.wantDSN, dsn)
		})
	}
}

// TestParseSourceURLHidesMalformedURL は、URL を読めないとき、エラーにパスワードを含む URL を残さないことを確かめる (#1012)。
// エラーは、CLI の終わりにログへ書かれる。
func TestParseSourceURLHidesMalformedURL(t *testing.T) {
	t.Parallel()

	// パスワードに、エスケープしない「/」がある。url.Parse は、その前をポートとして読もうとして失敗する。
	const leaked = "Ab3/xY+9"
	_, _, err := epgstation.ParseSourceURL("mysql://epgstation:" + leaked + "@mariadb:3306/epgstation")

	require.ErrorIs(t, err, epgstation.ErrUnsupportedSource)
	assert.NotContains(t, err.Error(), "Ab3")
	assert.NotContains(t, err.Error(), "xY+9")
}

func TestOpenFailsWithoutSQLiteFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.db")
	_, err := epgstation.Open(t.Context(), (&url.URL{Scheme: sqliteName, Path: path}).String())
	require.Error(t, err)
	// 読み取り専用で開くので、空の DB のファイルを作らない。
	assert.NoFileExists(t, path)
}

func TestOpenRejectsUnsupportedSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup []string
	}{
		{
			name:  "the latest migration has not been applied",
			setup: []string{"DELETE FROM migrations WHERE name LIKE 'AddEventRelay%'"},
		},
		{
			name:  "not an EPGStation database",
			setup: []string{"DROP TABLE migrations"},
		},
	}
	for _, dialect := range epgstationtest.Dialects() {
		for _, tt := range tests {
			t.Run(string(dialect)+"/"+tt.name, func(t *testing.T) {
				t.Parallel()

				db := epgstationtest.New(t, dialect)
				epgstationtest.Exec(t, db.Writer, tt.setup...)

				_, err := epgstation.Open(t.Context(), db.URL)
				require.ErrorIs(t, err, epgstation.ErrUnsupportedSchema)
			})
		}
	}
}

// TestReadDoesNotWrite は、移行元の DB に書き込まないことを確かめる (設計書 6.3)。
func TestReadDoesNotWrite(t *testing.T) {
	t.Parallel()

	for _, dialect := range epgstationtest.Dialects() {
		t.Run(string(dialect), func(t *testing.T) {
			t.Parallel()

			db := epgstationtest.New(t, dialect)
			epgstationtest.Exec(t, db.Writer, seed()...)
			source, err := epgstation.Open(t.Context(), db.URL)
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = source.Close()
			})

			err = source.ExecInReadOnlyTx(t.Context(), "INSERT INTO recorded_tag (name, halfWidthName, color) VALUES ('x', 'x', '#000000')")
			require.Error(t, err)

			_, err = source.Read(t.Context())
			require.NoError(t, err)
			var tags int
			require.NoError(t, db.Writer.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM recorded_tag").Scan(&tags))
			assert.Equal(t, 2, tags)
		})
	}
}

// TestReadLeavesSQLiteFileUntouched は、読んだあとも、SQLite の DB のファイルが書き換わったり、
// ジャーナルのファイルが残ったりしていないことを確かめる。
func TestReadLeavesSQLiteFileUntouched(t *testing.T) {
	t.Parallel()

	db := epgstationtest.New(t, epgstationtest.SQLite)
	epgstationtest.Exec(t, db.Writer, seed()...)
	require.NoError(t, db.Writer.Close())
	dir := filepath.Dir(db.Path)
	before := hashFiles(t, dir)

	source, err := epgstation.Open(t.Context(), db.URL)
	require.NoError(t, err)
	_, err = source.Read(t.Context())
	require.NoError(t, err)
	require.NoError(t, source.Close())

	assert.Equal(t, before, hashFiles(t, dir))
}

// hashFiles は、dir にあるファイルの名前と内容のハッシュを返す。ジャーナルのファイルが増えても気付けるようにする。
func hashFiles(t *testing.T, dir string) map[string][sha256.Size]byte {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	hashes := map[string][sha256.Size]byte{}
	for _, entry := range entries {
		b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, err)
		hashes[entry.Name()] = sha256.Sum256(b)
	}

	return hashes
}
