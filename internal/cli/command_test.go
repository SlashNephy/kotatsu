package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/config"
)

func TestHelpListsRolesWithAlias(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	cmd := cli.NewCommand(testOptions(slog.New(slog.DiscardHandler), cli.DefaultRoles(), validEnv))
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	require.NoError(t, cmd.Execute())

	tests := []string{
		"tenban (server)",
		"neko (scheduler)",
		"mikan (recorder)",
		"nekonote (worker)",
		"kyusu (transcoder)",
		"all ",
	}
	for _, want := range tests {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, out.String(), "\n  "+want)
		})
	}
}

func TestSubcommandAcceptsCodenameAndAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		codename string
		alias    string
	}{
		{codename: "tenban", alias: "server"},
		{codename: "neko", alias: "scheduler"},
		{codename: "mikan", alias: "recorder"},
		{codename: roleNekonote, alias: "worker"},
		{codename: "kyusu", alias: "transcoder"},
	}
	for _, tt := range tests {
		for _, arg := range []string{tt.codename, tt.alias} {
			t.Run(arg, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, []string{tt.codename}, startedRoles(t, arg))
			})
		}
	}
}

func TestAllStartsEveryRole(t *testing.T) {
	t.Parallel()

	roles := cli.DefaultRoles()
	want := make([]string, 0, len(roles))
	for _, role := range roles {
		want = append(want, role.Name)
	}

	assert.ElementsMatch(t, want, startedRoles(t, "all"))
}

// startedRoles は arg をサブコマンドとして実行し、起動したロールのコードネームを返す。
func startedRoles(t *testing.T, arg string) []string {
	t.Helper()

	var (
		mu      sync.Mutex
		started = []string{}
	)
	roles := cli.DefaultRoles()
	for _, role := range roles {
		role.Run = func(context.Context, *slog.Logger, *cli.Deps) error {
			mu.Lock()
			defer mu.Unlock()
			started = append(started, role.Name)

			return nil
		}
	}

	cmd := cli.NewCommand(testOptions(slog.New(slog.DiscardHandler), roles, validEnv))
	cmd.SetArgs([]string{arg})
	require.NoError(t, cmd.ExecuteContext(t.Context()))

	return started
}

func TestRoleFailureStopsOtherRoles(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	roles := cli.DefaultRoles()
	for _, role := range roles[1:] {
		role.Run = func(ctx context.Context, _ *slog.Logger, _ *cli.Deps) error {
			<-ctx.Done()

			return nil
		}
	}
	roles[0].Run = func(context.Context, *slog.Logger, *cli.Deps) error {
		return errBoom
	}

	cmd := cli.NewCommand(testOptions(slog.New(slog.DiscardHandler), roles, validEnv))
	cmd.SetArgs([]string{"all"})

	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(t.Context())
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, errBoom)
	case <-time.After(5 * time.Second):
		t.Fatal("other roles did not stop after a role failed")
	}
}

func TestRoleLogsDisplayNameUntilCanceled(t *testing.T) {
	t.Parallel()

	var (
		mu  sync.Mutex
		buf bytes.Buffer
	)
	logger := slog.New(slog.NewJSONHandler(&lockedWriter{mu: &mu, w: &buf}, nil))

	ctx, cancel := context.WithCancel(t.Context())
	cmd := cli.NewCommand(testOptions(logger, cli.DefaultRoles(), validEnv))
	// DB を使わないロールで確かめる。
	cmd.SetArgs([]string{"recorder"})

	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(ctx)
	}()

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()

		return strings.Contains(buf.String(), `"msg":"role started"`)
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("role did not stop after the context was canceled")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, buf.String(), `"role":"mikan (recorder)"`)
	assert.Contains(t, buf.String(), `"msg":"role stopped"`)
	assert.Contains(t, buf.String(), `"msg":"config loaded"`)
	assert.NotContains(t, buf.String(), "hunter2")
}

func TestUnknownSubcommandFails(t *testing.T) {
	t.Parallel()

	cmd := cli.NewCommand(testOptions(slog.New(slog.DiscardHandler), cli.DefaultRoles(), validEnv))
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"kago"})
	require.Error(t, cmd.Execute())
}

func TestRoleDoesNotStartWithInvalidConfig(t *testing.T) {
	t.Parallel()

	var started atomic.Bool
	roles := cli.DefaultRoles()
	for _, role := range roles {
		role.Run = func(context.Context, *slog.Logger, *cli.Deps) error {
			started.Store(true)

			return nil
		}
	}

	cmd := cli.NewCommand(testOptions(slog.New(slog.DiscardHandler), roles, func(string) (string, bool) { return "", false }))
	cmd.SetArgs([]string{"all"})
	require.ErrorContains(t, cmd.ExecuteContext(t.Context()), "database.url (KOTATSU_DATABASE_URL): is required")
	assert.False(t, started.Load())
}

// TestTenbanRequiresSecretKey は、アプリの秘密キーがなければ、tenban を起動しないことを確かめる (設計書 8.4)。
func TestTenbanRequiresSecretKey(t *testing.T) {
	t.Parallel()

	var started atomic.Bool
	roles := cli.DefaultRoles()
	for _, role := range roles {
		role.Run = func(context.Context, *slog.Logger, *cli.Deps) error {
			started.Store(true)

			return nil
		}
	}
	env := testEnv()
	delete(env, "KOTATSU_AUTH_SECRET_KEY")

	cmd := cli.NewCommand(testOptions(slog.New(slog.DiscardHandler), roles, func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	}))
	cmd.SetArgs([]string{roleTenban})
	require.ErrorContains(t, cmd.ExecuteContext(t.Context()), "auth.secret_key (KOTATSU_AUTH_SECRET_KEY) is required to run tenban")
	assert.False(t, started.Load())
}

func TestConfigFlagAndEnv(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	flagPath := filepath.Join(dir, "flag.yaml")
	require.NoError(t, os.WriteFile(flagPath, []byte("server:\n  listen: "+anyLocalPort+"\n"), 0o600))
	envPath := filepath.Join(dir, "env.yaml")
	require.NoError(t, os.WriteFile(envPath, []byte("server:\n  listen: localhost:0\n"), 0o600))

	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{name: "flag", args: []string{"--config", flagPath}, want: anyLocalPort},
		{name: "env", env: map[string]string{"KOTATSU_CONFIG": envPath}, want: "localhost:0"},
		{name: "flag wins over env", args: []string{"--config", flagPath}, env: map[string]string{"KOTATSU_CONFIG": envPath}, want: anyLocalPort},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got string
			roles := cli.DefaultRoles()
			roles[0].Run = func(_ context.Context, _ *slog.Logger, deps *cli.Deps) error {
				got = deps.Config.Server.Listen

				return nil
			}

			// server.listen は設定ファイルから与えるので、環境変数からは外す。
			env := testEnv()
			delete(env, "KOTATSU_SERVER_LISTEN")
			maps.Copy(env, tt.env)
			cmd := cli.NewCommand(testOptions(slog.New(slog.DiscardHandler), roles, func(key string) (string, bool) {
				v, ok := env[key]

				return v, ok
			}))
			cmd.SetArgs(append(slices.Clone(tt.args), roles[0].Name))
			require.NoError(t, cmd.ExecuteContext(t.Context()))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRoleDoesNotStartWhenSchemaIsIncompatible(t *testing.T) {
	t.Parallel()

	var started atomic.Bool
	roles := cli.DefaultRoles()
	for _, role := range roles {
		role.Run = func(context.Context, *slog.Logger, *cli.Deps) error {
			started.Store(true)

			return nil
		}
	}

	errIncompatible := errors.New("incompatible schema")
	opts := testOptions(slog.New(slog.DiscardHandler), roles, validEnv)
	opts.CheckSchema = func(context.Context, *pgxpool.Pool) error {
		return errIncompatible
	}

	cmd := cli.NewCommand(opts)
	cmd.SetArgs([]string{roles[0].Name})
	err := cmd.ExecuteContext(t.Context())
	require.ErrorIs(t, err, errIncompatible)
	require.ErrorContains(t, err, "refusing to start")
	assert.False(t, started.Load())
}

func TestMigrateRunsMigrationWithoutStartingRoles(t *testing.T) {
	t.Parallel()

	var started, migrated, checked atomic.Bool
	roles := cli.DefaultRoles()
	for _, role := range roles {
		role.Run = func(context.Context, *slog.Logger, *cli.Deps) error {
			started.Store(true)

			return nil
		}
	}

	opts := testOptions(slog.New(slog.DiscardHandler), roles, validEnv)
	opts.Migrate = func(context.Context, *slog.Logger, *pgxpool.Pool) error {
		migrated.Store(true)

		return nil
	}
	// migrate はスキーマが古い DB に対して実行するものなので、スキーマを確かめてはならない。
	opts.CheckSchema = func(context.Context, *pgxpool.Pool) error {
		checked.Store(true)

		return errors.New("must not be called")
	}

	cmd := cli.NewCommand(opts)
	cmd.SetArgs([]string{subcommandMigrate})
	require.NoError(t, cmd.ExecuteContext(t.Context()))
	assert.True(t, migrated.Load())
	assert.False(t, checked.Load())
	assert.False(t, started.Load())
}

// testOptions は、DB に接続しない Options を返す。
func testOptions(logger *slog.Logger, roles []*cli.Role, lookupEnv config.LookupEnv) *cli.Options {
	return &cli.Options{
		Logger:    logger,
		Roles:     roles,
		LookupEnv: lookupEnv,
		Clock:     new(clock.Real),
		OpenDatabase: func(context.Context, *config.Database) (*pgxpool.Pool, error) {
			return nil, nil
		},
		CheckSchema: func(context.Context, *pgxpool.Pool) error { return nil },
		Migrate:     func(context.Context, *slog.Logger, *pgxpool.Pool) error { return nil },
		RunRegistry: func(ctx context.Context, _ *slog.Logger, _ *cli.Deps, _ []string, _ string) error {
			<-ctx.Done()

			return nil
		},
		JobWorkers: func(*cli.Deps) (*river.Workers, []*river.PeriodicJob) { return nil, []*river.PeriodicJob{} },
	}
}

// validEnv は、起動に必要な設定を与える。HTTP サーバーはテストどうしで衝突しないよう空いているポートで待ち受ける。
func validEnv(key string) (string, bool) {
	v, ok := testEnv()[key]

	return v, ok
}

func testEnv() map[string]string {
	databaseURL := url.URL{Scheme: "postgres", User: url.UserPassword("user", "hunter2"), Host: "db", Path: "/kotatsu"}

	return map[string]string{
		"KOTATSU_DATABASE_URL":    databaseURL.String(),
		"KOTATSU_SERVER_LISTEN":   anyLocalPort,
		"KOTATSU_METRICS_LISTEN":  anyLocalPort,
		"KOTATSU_AUTH_SECRET_KEY": testKeyMaterial,
	}
}

// testKeyMaterial は、テストのアプリの秘密キー。tenban は、これがないと起動しない。
const testKeyMaterial = "kotatsu-cli-test-application-key-0123456789"

// anyLocalPort は、テストどうしで衝突しないよう空いているポートで待ち受けるアドレス。
const anyLocalPort = "127.0.0.1:0"

var _ io.Writer = new(lockedWriter)

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.w.Write(p)
}
