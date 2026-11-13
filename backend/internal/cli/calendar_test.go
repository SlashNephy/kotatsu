package cli_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// TestTenbanServesCalendar は、tenban が、echo のルーターで予約の iCalendar を返し (#1606)、
// 崩れたパスは、これまでどおり、整えた先へリダイレクトすることを確かめる。
func TestTenbanServesCalendar(t *testing.T) {
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
		Username: "calendar", DisplayName: "カレンダー", Role: domain.RoleViewer, PasswordHash: nil,
	})
	require.NoError(t, err)
	token, _, err := auth.NewCalendarFeeds(pool, clk, audit.NewRecorder(clk)).Create(t.Context(), audit.User(user.ID), user.ID)
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
		name         string
		path         string
		wantStatus   int
		wantType     string
		wantLocation string
	}{
		{name: "valid token", path: "/api/calendar/" + token + "/reservations.ics", wantStatus: http.StatusOK, wantType: "text/calendar; charset=utf-8"},
		// 秘密の値が違えば、見つからないことと区別させない。
		{name: "wrong token", path: "/api/calendar/kotatsu_cal_wrong/reservations.ics", wantStatus: http.StatusNotFound, wantType: "text/plain; charset=utf-8"},
		// echo のルートに当たらない崩れたパスは、これまでどおり、http.ServeMux が整えた先へリダイレクトする。
		{
			name: "double slash", path: "//api//calendar/" + token + "/reservations.ics", wantStatus: http.StatusTemporaryRedirect,
			wantLocation: "/api/calendar/" + token + "/reservations.ics",
		},
		// 移していない /api の下のほかのパスは、これまでどおり 404 になる。
		{name: "other api", path: "/api/unknown", wantStatus: http.StatusNotFound, wantType: "text/plain; charset=utf-8"},
	}
	client := httpclienttest.New(t)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+tt.path, nil)
			require.NoError(t, err)
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer func() {
				_ = resp.Body.Close()
			}()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, resp.StatusCode, string(body))
			if tt.wantType != "" {
				assert.Equal(t, tt.wantType, resp.Header.Get("Content-Type"))
			}
			if tt.wantLocation != "" {
				assert.Equal(t, tt.wantLocation, resp.Header.Get("Location"))
			}
		})
	}
}
