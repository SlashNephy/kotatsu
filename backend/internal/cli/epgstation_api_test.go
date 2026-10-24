package cli_test

import (
	"context"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
)

// TestTenbanServesEPGStationAPI は、tenban が /api/* を、API トークンでだけ受け付けることを確かめる (#431、設計書 9.8)。
// セッションの Cookie では受け付けない。チューナーサーバーを設定していないので、ロゴとライブ視聴は 503 になる。
func TestTenbanServesEPGStationAPI(t *testing.T) {
	t.Parallel()

	dbConfig := databasetest.New(t)
	env := testEnv()
	env["KOTATSU_DATABASE_URL"] = dbConfig.URL.Value()
	opts := cli.DefaultOptions(slog.New(slog.DiscardHandler))
	opts.LookupEnv = func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	}
	opts.Roles = filterRoles(opts.Roles, roleTenban)
	addresses := make(chan string, 1)
	opts.RunRegistry = func(ctx context.Context, _ *slog.Logger, _ *cli.Deps, _ []string, listenAddr string) error {
		addresses <- listenAddr
		<-ctx.Done()

		return nil
	}
	migrateCmd := cli.NewCommand(opts)
	migrateCmd.SetArgs([]string{subcommandMigrate})
	require.NoError(t, migrateCmd.ExecuteContext(t.Context()))

	pool, err := database.Open(t.Context(), dbConfig)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	clk := new(clock.Real)
	user, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{
		Username: adminUsername, DisplayName: "こたつ", Role: auth.RoleViewer, PasswordHash: nil,
	})
	require.NoError(t, err)
	session, _, err := auth.NewSessions(nil).Create(t.Context(), pool, user.ID, nil)
	require.NoError(t, err)
	token, _, err := auth.NewAPITokens(pool, clk, audit.NewRecorder(clk)).Create(t.Context(), audit.User(user.ID), user.ID, "kiririn")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cmd := cli.NewCommand(opts)
	cmd.SetArgs([]string{roleTenban})
	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})
	base := "http://" + <-addresses

	tests := []struct {
		name   string
		path   string
		cookie bool
		bearer bool
		want   int
	}{
		{name: "トークン", path: "/api/version", cookie: false, bearer: true, want: http.StatusOK},
		{name: "セッションの Cookie だけ", path: "/api/version", cookie: true, bearer: false, want: http.StatusUnauthorized},
		{name: "どちらもない", path: "/api/channels", cookie: false, bearer: false, want: http.StatusUnauthorized},
		{name: "ロゴ", path: "/api/channels/3273601024/logo", cookie: false, bearer: true, want: http.StatusServiceUnavailable},
		{name: "ライブ視聴", path: "/api/streams/live/3273601024/m2ts?mode=0", cookie: false, bearer: true, want: http.StatusServiceUnavailable},
		{name: "ない録画の動画", path: "/api/videos/999999", cookie: false, bearer: true, want: http.StatusNotFound},
		{name: "ない録画のサムネイル", path: "/api/thumbnails/999999", cookie: false, bearer: true, want: http.StatusNotFound},
		{name: "用意していない API", path: "/api/reserves", cookie: false, bearer: true, want: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+tt.path, http.NoBody)
			require.NoError(t, err)
			if tt.cookie {
				req.Header.Set("Cookie", auth.CookieName+"="+session)
			}
			if tt.bearer {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, tt.want, resp.StatusCode)
		})
	}
}
