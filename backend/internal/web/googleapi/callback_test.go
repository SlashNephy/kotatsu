package googleapi_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/googletest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/google"
	"github.com/SlashNephy/kotatsu/internal/web/googleapi"
)

// TestCallbackHandler は、Google から戻ってきたリクエストを、連携の画面へ戻し、接続したことか、失敗の理由だけを伝えることを確認する (#1902)。
func TestCallbackHandler(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC))
	fake := googletest.New("client", "secret")
	fake.RedirectURIs = []string{"http://kotatsu.test" + google.CallbackPath}
	server := httptest.NewServer(fake.Handler())
	t.Cleanup(server.Close)
	service, err := google.NewService(&google.Options{
		Clock: clk, Recorder: audit.NewRecorder(clk), AppSecret: strings.Repeat("k", 32), HTTPClient: server.Client(),
		Logger: slog.New(slog.DiscardHandler), URL: server.URL,
	})
	require.NoError(t, err)
	info := &auth.ClientInfo{UserAgent: "test", SourceIP: netip.MustParseAddr("127.0.0.1"), Host: "kotatsu.test", Secure: false}
	user, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{Username: "admin", DisplayName: "admin", Role: domain.RoleAdmin, PasswordHash: nil})
	require.NoError(t, err)
	_, session, err := auth.NewSessions(&auth.SessionsOptions{Clock: clk}).Create(t.Context(), pool, user.ID, info)
	require.NoError(t, err)
	principal := &auth.Principal{UserID: user.ID, Role: domain.RoleAdmin, SessionID: session.ID, AuthenticatedAt: session.CreatedAt, OAuthClientID: ""}
	require.NoError(t, service.UpdateApp(t.Context(), pool, audit.User(user.ID), &google.AppChanges{ClientID: "client", ClientSecret: new("secret")}))
	handler := googleapi.CallbackHandler(service, pool, func(*http.Request) *auth.ClientInfo { return info }, slog.New(slog.DiscardHandler))

	// authorize は、認可を始め、偽の Google で許して戻ってきたときのクエリを返す。
	authorize := func(t *testing.T) string {
		t.Helper()
		authURL, err := service.BeginDrive(t.Context(), pool, principal, "http://kotatsu.test")
		require.NoError(t, err)
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, authURL, http.NoBody)
		require.NoError(t, err)
		res, err := client.Do(req)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		location, err := url.Parse(res.Header.Get("Location"))
		require.NoError(t, err)
		assert.Equal(t, "/internal/auth/google/callback", location.Path)

		return location.RawQuery
	}

	tests := []struct {
		name      string
		query     func(t *testing.T) string
		principal *auth.Principal
		want      string
	}{
		{name: "connected", query: authorize, principal: principal, want: "/admin/integrations?connected=google-drive"},
		{name: "no state", query: func(*testing.T) string { return "code=abc" }, principal: principal, want: "/admin/integrations?error=google_expired"},
		{name: "an unknown state", query: func(*testing.T) string { return "state=forged&code=abc" }, principal: principal, want: "/admin/integrations?error=google_expired"},
		{name: "not logged in", query: authorize, principal: nil, want: "/admin/integrations?error=google_mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, google.CallbackPath+"?"+tt.query(t), nil)
			if tt.principal != nil {
				req = req.WithContext(auth.WithPrincipal(req.Context(), tt.principal))
			}
			handler.ServeHTTP(recorder, req)
			assert.Equal(t, http.StatusSeeOther, recorder.Code)
			assert.Equal(t, tt.want, recorder.Header().Get("Location"))
		})
	}
}
