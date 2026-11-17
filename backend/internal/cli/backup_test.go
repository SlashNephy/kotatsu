package cli_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
)

// newEnvOptions は、env を環境変数として読む Options を返す。
func newEnvOptions(env map[string]string) *cli.Options {
	opts := cli.DefaultOptions(slog.New(slog.DiscardHandler))
	opts.LookupEnv = func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	}

	return opts
}

func runCommand(t *testing.T, opts *cli.Options, args ...string) error {
	t.Helper()

	cmd := cli.NewCommand(opts)
	cmd.SetArgs(args)

	return cmd.ExecuteContext(t.Context())
}

// TestAdminBackupRestore は、kotatsu admin backup が持ち主だけが読めるファイルを書き、kotatsu admin restore が空の DB に戻すことと、
// アプリの秘密キーが設定と違えば、--secret-key-out を渡さない限り戻さないことを確かめる (#1926)。
func TestAdminBackupRestore(t *testing.T) {
	t.Parallel()

	sourceDB := databasetest.New(t)
	sourceEnv := testEnv()
	sourceEnv["KOTATSU_DATABASE_URL"] = sourceDB.URL.Value()
	source := newEnvOptions(sourceEnv)
	require.NoError(t, runCommand(t, source, subcommandMigrate))
	pool, err := database.Open(t.Context(), sourceDB)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	now := time.Now()
	_, err = pool.Exec(t.Context(), "INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES ($1, 'admin', 'Admin', 'admin', $2, $2)", id.New(), now)
	require.NoError(t, err)

	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	dir := t.TempDir()
	identityFile := filepath.Join(dir, "key.txt")
	require.NoError(t, os.WriteFile(identityFile, []byte("# test key\n"+identity.String()+"\n"), 0o600))
	output := filepath.Join(dir, "kotatsu.backup")
	require.NoError(t, runCommand(t, source, "admin", "backup", "--output", output, "--recipient", identity.Recipient().String()))
	info, err := os.Stat(output)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	// 書いたファイルを上書きしない。
	require.Error(t, runCommand(t, source, "admin", "backup", "--output", output, "--recipient", identity.Recipient().String()))

	targetDB := databasetest.New(t)
	targetEnv := testEnv()
	targetEnv["KOTATSU_DATABASE_URL"] = targetDB.URL.Value()
	targetEnv["KOTATSU_AUTH_SECRET_KEY"] = "another-application-key-for-the-restore-test"
	target := newEnvOptions(targetEnv)
	require.ErrorContains(t, runCommand(t, target, "admin", "restore", "--input", output, "--identity", identityFile), "--secret-key-out")

	secretKeyOut := filepath.Join(dir, "secret-key")
	require.NoError(t, runCommand(t, target, "admin", "restore", "--input", output, "--identity", identityFile, "--secret-key-out", secretKeyOut))
	secretKey, err := os.ReadFile(secretKeyOut)
	require.NoError(t, err)
	assert.Equal(t, testKeyMaterial, string(secretKey))
	info, err = os.Stat(secretKeyOut)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	restored, err := database.Open(t.Context(), targetDB)
	require.NoError(t, err)
	t.Cleanup(restored.Close)
	require.NoError(t, database.CheckSchema(t.Context(), restored))
	var users int
	require.NoError(t, restored.QueryRow(t.Context(), "SELECT count(*) FROM users WHERE username = 'admin'").Scan(&users))
	assert.Equal(t, 1, users)
}

// TestMigrateTo は、kotatsu migrate --to が、指定したバージョンまでで止めることを確かめる (#1926)。
func TestMigrateTo(t *testing.T) {
	t.Parallel()

	dbConfig := databasetest.New(t)
	env := testEnv()
	env["KOTATSU_DATABASE_URL"] = dbConfig.URL.Value()
	opts := newEnvOptions(env)
	versions := database.MigrationVersions()
	version := versions[len(versions)-2]
	require.NoError(t, runCommand(t, opts, subcommandMigrate, "--to", strconv.FormatInt(version, 10)))

	pool, err := database.Open(t.Context(), dbConfig)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	var current int64
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT max(version_id) FROM goose_db_version").Scan(&current))
	assert.Equal(t, version, current)
}
