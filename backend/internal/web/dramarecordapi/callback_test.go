package dramarecordapi_test

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
	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/simkltest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/dramarecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/web/dramarecordapi"
)

// TestCallbackHandler は、Simkl から戻ってきたリクエストを、アカウントの連携の画面へ戻すことを確かめる (#1649)。
// 連携できたら connected=simkl を、できなければ、トークンなどの中身を入れずに error=<理由> を付ける。
func TestCallbackHandler(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	fake := simkltest.New("client", "secret")
	t.Cleanup(fake.Close)
	fake.SetNow(clk.Now)
	service, err := dramarecord.NewService(&dramarecord.Options{
		Clock: clk, Recorder: audit.NewRecorder(clk), AppSecret: strings.Repeat("k", 32),
		Destinations: []dramarecord.Destination{dramarecord.NewSimkl(dramarecord.NewHTTPClient(nil, netactivity.PurposeDramaLink), fake.URL())},
		Logger:       slog.New(slog.DiscardHandler), Inserter: nil, RequestInterval: 0,
	})
	require.NoError(t, err)
	info := &auth.ClientInfo{UserAgent: "test", SourceIP: netip.MustParseAddr("127.0.0.1"), Host: "kotatsu.test", Secure: false}
	user, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{Username: "alice", DisplayName: "alice", Role: domain.RoleViewer, PasswordHash: nil})
	require.NoError(t, err)
	_, session, err := auth.NewSessions(&auth.SessionsOptions{Clock: clk}).Create(t.Context(), pool, user.ID, info)
	require.NoError(t, err)
	principal := &auth.Principal{UserID: user.ID, Role: domain.RoleViewer, SessionID: session.ID, AuthenticatedAt: session.CreatedAt, OAuthClientID: ""}
	_, err = settings.NewStore(clk, audit.NewRecorder(clk)).Update(t.Context(), pool, audit.User(user.ID), &settings.Changes{DramaLinkingEnabled: new(true)})
	require.NoError(t, err)
	require.NoError(t, service.UpdateApp(t.Context(), pool, audit.User(user.ID), dramarecord.DestinationSimkl,
		&dramarecord.AppChanges{ClientID: "client", ClientSecret: new("secret")}))
	handler := dramarecordapi.CallbackHandler(service, pool, func(*http.Request) *auth.ClientInfo { return info }, dramarecord.DestinationSimkl,
		slog.New(slog.DiscardHandler))

	// authorize は、連携を始め、偽の Simkl が戻すリダイレクト先の問い合わせを返す。
	authorize := func(t *testing.T) url.Values {
		t.Helper()
		authorizationURL, err := service.Begin(t.Context(), pool, principal, dramarecord.DestinationSimkl, "http://kotatsu.test")
		require.NoError(t, err)
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, authorizationURL, nil)
		require.NoError(t, err)
		res, err := client.Do(req)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		location, err := url.Parse(res.Header.Get("Location"))
		require.NoError(t, err)

		return location.Query()
	}
	// authorized は、そのまま戻ってきたリクエストの問い合わせを返す。
	authorized := func(t *testing.T) string {
		t.Helper()

		return authorize(t).Encode()
	}

	tests := []struct {
		name string
		// query は、戻ってきたリクエストの問い合わせを作る。
		query func(t *testing.T) string
		// signedIn なら、連携を始めたユーザーのセッションで戻ってくる。
		signedIn bool
		want     string
	}{
		{name: "no state", query: func(*testing.T) string { return "code=abc" }, signedIn: true, want: "/settings/account?error=drama_record_expired"},
		{
			name: "an unknown state", query: func(*testing.T) string { return "state=forged&code=abc" }, signedIn: true,
			want: "/settings/account?error=drama_record_expired",
		},
		{
			name: "another session", query: authorized, signedIn: false,
			want: "/settings/account?error=drama_record_mismatch",
		},
		{
			name: "a denial", signedIn: true, want: "/settings/account?error=drama_record_denied",
			query: func(t *testing.T) string {
				t.Helper()
				q := authorize(t)
				q.Del("code")
				q.Set("error", "access_denied")

				return q.Encode()
			},
		},
		{
			name: "connected", query: authorized, signedIn: true,
			want: "/settings/account?connected=simkl",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/internal/auth/simkl/callback?"+tt.query(t), nil)
			if tt.signedIn {
				req = req.WithContext(auth.WithPrincipal(req.Context(), principal))
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			assert.Equal(t, http.StatusSeeOther, recorder.Code)
			assert.Equal(t, tt.want, recorder.Header().Get("Location"))
		})
	}
}
