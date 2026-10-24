package cli_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
)

// TestTenbanServesGraphQL は、tenban が /graphql を公開し、他のロールは公開しないことを確かめる。
func TestTenbanServesGraphQL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role string
		// wantStatus は、ログインして viewer を引いたときの状態コード。
		wantStatus int
		wantBody   string
	}{
		{role: "tenban", wantStatus: http.StatusOK, wantBody: `{"data":{"viewer":{"username":"kotatsu"}}}`},
		{role: roleKyusu, wantStatus: http.StatusNotFound, wantBody: "404 page not found"},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			t.Parallel()

			dbConfig := databasetest.New(t)
			env := testEnv()
			env["KOTATSU_DATABASE_URL"] = dbConfig.URL.Value()
			opts := cli.DefaultOptions(slog.New(slog.DiscardHandler))
			opts.LookupEnv = func(key string) (string, bool) {
				v, ok := env[key]

				return v, ok
			}
			opts.Roles = filterRoles(opts.Roles, tt.role)
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
			user, err := auth.NewUsers(new(clock.Real)).Create(t.Context(), pool, &auth.NewUser{
				Username: "kotatsu", DisplayName: "こたつ", Role: auth.RoleViewer, PasswordHash: nil,
			})
			require.NoError(t, err)
			token, _, err := auth.NewSessions(nil).Create(t.Context(), pool, user.ID, nil)
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(t.Context())
			cmd := cli.NewCommand(opts)
			cmd.SetArgs([]string{tt.role})
			done := make(chan error, 1)
			go func() {
				done <- cmd.ExecuteContext(ctx)
			}()
			t.Cleanup(func() {
				cancel()
				assert.NoError(t, <-done)
			})

			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+<-addresses+"/graphql",
				strings.NewReader(`{"query":"{ viewer { username } }"}`))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Cookie", auth.CookieName+"="+token)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() {
				_ = resp.Body.Close()
			}()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			assert.Equal(t, tt.wantBody, strings.TrimSpace(string(body)))
		})
	}
}
