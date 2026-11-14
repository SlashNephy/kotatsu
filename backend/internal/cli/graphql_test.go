package cli_test

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
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
				Username: "kotatsu", DisplayName: "こたつ", Role: domain.RoleViewer, PasswordHash: nil,
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

			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+<-addresses+"/internal/graphql",
				strings.NewReader(`{"query":"{ viewer { username } }"}`))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Cookie", auth.CookieName+"="+token)
			resp, err := httpclienttest.New(t).Do(req)
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

// graphQLPath は、画面が使う GraphQL の端点。
const graphQLPath = "/internal/graphql"

// jsonContentType は、GraphQL が受け付ける要求の Content-Type。
const jsonContentType = "application/json"

// TestTenbanRoutesGraphQL は、tenban が、GraphQL (/internal/graphql) を echo のルーターで受け (#1606)、
// mux のときと同じく、セッションの Cookie を読み、メソッドの扱いを GraphQL のハンドラーに任せることを確かめる。

func TestTenbanRoutesGraphQL(t *testing.T) {
	t.Parallel()

	base, pool := startTenban(t)
	user, err := auth.NewUsers(new(clock.Real)).Create(t.Context(), pool, &auth.NewUser{
		Username: "graphql", DisplayName: "GraphQL", Role: domain.RoleViewer, PasswordHash: nil,
	})
	require.NoError(t, err)
	session, _, err := auth.NewSessions(nil).Create(t.Context(), pool, user.ID, nil)
	require.NoError(t, err)
	const (
		query    = `{"query":"{ viewer { username } }"}`
		onlyPOST = `{"errors":[{"message":"only POST is allowed","extensions":{"code":"BAD_USER_INPUT"}}]}`
	)

	tests := []struct {
		name        string
		method      string
		path        string
		session     string
		contentType string
		wantStatus  int
		wantBody    string
	}{
		// GET でのクエリと POST 以外は、echo ではなく GraphQL のハンドラーが断る (設計書 9.5)。
		{name: "query", method: http.MethodPost, path: graphQLPath, session: session, contentType: jsonContentType, wantStatus: http.StatusOK, wantBody: `{"data":{"viewer":{"username":"graphql"}}}`},
		{name: "query without session", method: http.MethodPost, path: graphQLPath, session: "", contentType: jsonContentType, wantStatus: http.StatusUnauthorized, wantBody: `{"errors":[{"message":"login required","extensions":{"code":"UNAUTHENTICATED"}}]}`},
		{name: "query without JSON", method: http.MethodPost, path: graphQLPath, session: session, contentType: "text/plain", wantStatus: http.StatusUnsupportedMediaType, wantBody: `{"errors":[{"message":"Content-Type must be application/json","extensions":{"code":"BAD_USER_INPUT"}}]}`},
		{name: "GET", method: http.MethodGet, path: "/internal/graphql?query=%7Bviewer%7Busername%7D%7D", session: session, contentType: "", wantStatus: http.StatusMethodNotAllowed, wantBody: onlyPOST},
		{name: "PUT", method: http.MethodPut, path: graphQLPath, session: session, contentType: jsonContentType, wantStatus: http.StatusMethodNotAllowed, wantBody: onlyPOST},
		{name: "unknown path under GraphQL", method: http.MethodPost, path: "/internal/graphql/nope", session: session, contentType: jsonContentType, wantStatus: http.StatusNotFound, wantBody: "404 page not found"},
	}
	client := httpclienttest.New(t)
	// 購読は、同じ端点で SSE で届く。echo の応答を通しても、始めたときの今の数が、すぐに流れてくる。
	t.Run("subscription over SSE", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base+graphQLPath,
			strings.NewReader(`{"query":"subscription { unreadNotificationCount }"}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", jsonContentType)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cookie", auth.CookieName+"="+session)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() {
			_ = resp.Body.Close()
		}()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
				assert.JSONEq(t, `{"data":{"unreadNotificationCount":0}}`, data)

				return
			}
		}
		t.Fatalf("the subscription ended without an event: %v", scanner.Err())
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), tt.method, base+tt.path, strings.NewReader(query))
			require.NoError(t, err)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.session != "" {
				req.Header.Set("Cookie", auth.CookieName+"="+tt.session)
			}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer func() {
				_ = resp.Body.Close()
			}()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, resp.StatusCode, tt.name)
			assert.Equal(t, tt.wantBody, strings.TrimSpace(string(body)), tt.name)
		})
	}
}
