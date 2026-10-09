package cli_test

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
)

// TestUnreachableTunerServerDoesNotAffectReadiness は、チューナーサーバーに接続できなくても、
// neko は縮退中として動き続け、/healthz と /readyz は成功することを確かめる (設計書 3.7)。
func TestUnreachableTunerServerDoesNotAffectReadiness(t *testing.T) {
	t.Parallel()

	dbConfig := databasetest.New(t)
	// 待ち受けていないポートを、接続できないチューナーサーバーとして使う。
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tunerURL := "http://" + listener.Addr().String() + "/"
	require.NoError(t, listener.Close())

	env := testEnv()
	env["KOTATSU_DATABASE_URL"] = dbConfig.URL.Value()
	env["KOTATSU_TUNER_URL"] = tunerURL
	opts := cli.DefaultOptions(slog.New(slog.DiscardHandler))
	opts.LookupEnv = func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	}
	opts.Roles = filterRoles(opts.Roles, "neko")
	addresses := make(chan string, 1)
	opts.RunRegistry = func(ctx context.Context, _ *slog.Logger, _ *cli.Deps, _ []string, listenAddr string) error {
		addresses <- listenAddr
		<-ctx.Done()

		return nil
	}

	migrateCmd := cli.NewCommand(opts)
	migrateCmd.SetArgs([]string{subcommandMigrate})
	require.NoError(t, migrateCmd.ExecuteContext(t.Context()))

	ctx, cancel := context.WithCancel(t.Context())
	cmd := cli.NewCommand(opts)
	cmd.SetArgs([]string{"neko"})
	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})

	pool, err := database.Open(t.Context(), dbConfig)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.Eventually(t, func() bool { return degraded(t, pool) }, 10*time.Second, 50*time.Millisecond)

	address := <-addresses
	for _, path := range []string{"/healthz", "/readyz"} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+path, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, path)
	}
}

func degraded(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()

	var n int
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM tuner_server_status WHERE degraded_since IS NOT NULL").Scan(&n))

	return n == 1
}
