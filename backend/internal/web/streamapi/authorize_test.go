package streamapi_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
	"github.com/SlashNephy/kotatsu/internal/web/streamapi"
)

const (
	// keyMaterial は、テストのアプリの秘密キー。
	keyMaterial = "kotatsu-stream-test-application-key-0123456789"
	// resource は、署名するリソース。外部プレイヤーに渡す、署名付き URL の経路。
	resource = "/api/streams/services/" + serviceID
	// signedPattern は、署名付き URL の経路のパターン。
	signedPattern = "GET /api/streams/services/{id}"
	// sessionPath は、画面のプレイヤーが Cookie で使う経路。
	sessionPath = "/internal/streams/services/" + serviceID
)

func newSigner(t *testing.T, key string) *stream.Signer {
	t.Helper()

	signer, err := stream.NewSigner(key)
	require.NoError(t, err)

	return signer
}

// TestSigner は、署名付き URL が、署名したリソース、ユーザー、有効期限の組でだけ通ることを確かめる (設計書 9.6)。
func TestSigner(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	userID := uuid.MustParse("0199d000-0000-7000-8000-0000000000a1")
	signed := newSigner(t, keyMaterial).Sign(resource, userID, now.Add(stream.DefaultURLLifetime))
	assert.Equal(t, now.Add(6*time.Hour), signed.ExpiresAt)
	parsed, err := url.Parse(signed.URL)
	require.NoError(t, err)
	require.Equal(t, resource, parsed.Path)
	query := parsed.Query()

	tests := []struct {
		name     string
		signer   *stream.Signer
		resource string
		query    func() url.Values
		now      time.Time
		wantErr  bool
	}{
		{name: "署名したとおり", signer: newSigner(t, keyMaterial), resource: resource, query: query.Clone, now: now, wantErr: false},
		{
			name: "署名しないクエリは変えてよい", signer: newSigner(t, keyMaterial), resource: resource, now: now, wantErr: false,
			query: func() url.Values {
				q := query.Clone()
				q.Set("start", "120")

				return q
			},
		},
		{name: "別のリソース", signer: newSigner(t, keyMaterial), resource: "/api/streams/services/3273601025", query: query.Clone, now: now, wantErr: true},
		{name: "画面の経路", signer: newSigner(t, keyMaterial), resource: sessionPath, query: query.Clone, now: now, wantErr: true},
		{
			name: "別のユーザー", signer: newSigner(t, keyMaterial), resource: resource, now: now, wantErr: true,
			query: func() url.Values {
				q := query.Clone()
				q.Set("user", "0199d000-0000-7000-8000-0000000000a2")

				return q
			},
		},
		{
			name: "有効期限を延ばした", signer: newSigner(t, keyMaterial), resource: resource, now: now, wantErr: true,
			query: func() url.Values {
				q := query.Clone()
				q.Set("expires", strconv.FormatInt(now.Add(7*24*time.Hour).Unix(), 10))

				return q
			},
		},
		{
			name: "署名がない", signer: newSigner(t, keyMaterial), resource: resource, now: now, wantErr: true,
			query: func() url.Values {
				q := query.Clone()
				q.Del("sig")

				return q
			},
		},
		{name: "期限が切れた", signer: newSigner(t, keyMaterial), resource: resource, query: query.Clone, now: now.Add(6 * time.Hour), wantErr: true},
		{name: "秘密キーが違う", signer: newSigner(t, keyMaterial+"!"), resource: resource, query: query.Clone, now: now, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.signer.Verify(tt.resource, tt.query(), tt.now)
			if tt.wantErr {
				require.ErrorIs(t, err, stream.ErrInvalidSignature)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, userID, got)
		})
	}
}

func TestNewSignerRequiresSecret(t *testing.T) {
	t.Parallel()

	_, err := stream.NewSigner("")
	require.ErrorIs(t, err, stream.ErrNoKey)
}

// TestAuthorizer は、画面の経路はログイン中のセッションでだけ、外部プレイヤーの経路は無効にしていないユーザーの署名付き URL でだけ、
// 映像を見られることを確かめる (設計書 9.6、#1605)。もう一方の経路の認証は受け付けない。
func TestAuthorizer(t *testing.T) {
	t.Parallel()

	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	users := auth.NewUsers(clk)
	viewer, err := users.Create(t.Context(), pool, &auth.NewUser{Username: "mikan", DisplayName: "みかん", Role: domain.RoleViewer, PasswordHash: nil})
	require.NoError(t, err)
	disabled, err := users.Create(t.Context(), pool, &auth.NewUser{Username: "neko", DisplayName: "ねこ", Role: domain.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	require.NoError(t, users.Disable(t.Context(), pool, disabled.ID))
	sessions := auth.NewSessions(&auth.SessionsOptions{IdleTimeout: 0, AbsoluteTimeout: 0, IsSecure: nil, Clock: clk})
	token, _, err := sessions.Create(t.Context(), pool, viewer.ID, nil)
	require.NoError(t, err)

	setExternalPlayerURLsEnabled(t, pool, clk, true)

	signer := newSigner(t, keyMaterial)
	authorizer := streamapi.NewAuthorizer(signer, auth.NewURLUsers(pool, clk, slog.New(slog.DiscardHandler)),
		settings.NewURLGate(pool, clk, slog.New(slog.DiscardHandler)), clk)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux := http.NewServeMux()
	mux.Handle(livePattern, authorizer.Session(ok))
	mux.Handle(signedPattern, authorizer.Signed(ok))
	server := httptest.NewServer(sessions.Middleware(pool)(mux))
	t.Cleanup(server.Close)
	expiresAt := clk.Now().Add(stream.DefaultURLLifetime)

	tests := []struct {
		name       string
		path       string
		cookie     string
		wantStatus int
	}{
		{name: "画面の経路にログイン中のセッション", path: sessionPath, cookie: token, wantStatus: http.StatusOK},
		{name: "画面の経路に認証がない", path: sessionPath, cookie: "", wantStatus: http.StatusUnauthorized},
		{name: "画面の経路に署名だけ", path: signer.Sign(sessionPath, viewer.ID, expiresAt).URL, cookie: "", wantStatus: http.StatusUnauthorized},
		{name: "署名付き URL", path: signer.Sign(resource, viewer.ID, expiresAt).URL + "&start=60", cookie: "", wantStatus: http.StatusOK},
		{name: "署名付き URL の経路にセッションだけ", path: resource, cookie: token, wantStatus: http.StatusUnauthorized},
		{name: "署名付き URL の経路に認証がない", path: resource, cookie: "", wantStatus: http.StatusUnauthorized},
		{name: "無効にしたユーザーの署名付き URL", path: signer.Sign(resource, disabled.ID, expiresAt).URL, cookie: "", wantStatus: http.StatusUnauthorized},
		{name: "いないユーザーの署名付き URL", path: signer.Sign(resource, uuid.New(), expiresAt).URL, cookie: "", wantStatus: http.StatusUnauthorized},
		{
			name: "別のリソースの署名付き URL", cookie: "", wantStatus: http.StatusUnauthorized,
			path: resource + "?" + mustQuery(t, signer.Sign("/api/streams/services/3273601025", viewer.ID, expiresAt).URL),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+tt.path, nil)
			require.NoError(t, err)
			if tt.cookie != "" {
				req.Header.Set("Cookie", auth.CookieName+"="+tt.cookie)
			}
			resp, err := httpclienttest.New(t).Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

// TestAuthorizerWhileDatabaseIsUnreachable は、DB に届かない間は、DB が止まる前に認可できたセッションと署名付き URL のユーザーに限って、
// 覚えた結果で映像を見られることを確かめる (#1084)。
func TestAuthorizerWhileDatabaseIsUnreachable(t *testing.T) {
	t.Parallel()

	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	users := auth.NewUsers(clk)
	viewer, err := users.Create(t.Context(), pool, &auth.NewUser{Username: "mikan", DisplayName: "みかん", Role: domain.RoleViewer, PasswordHash: nil})
	require.NoError(t, err)
	other, err := users.Create(t.Context(), pool, &auth.NewUser{Username: "neko", DisplayName: "ねこ", Role: domain.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	conn := databasetest.NewSwitch(t, pool)
	sessions := auth.NewSessions(&auth.SessionsOptions{IdleTimeout: 0, AbsoluteTimeout: 0, IsSecure: nil, Clock: clk, Logger: nil})
	seen, _, err := sessions.Create(t.Context(), pool, viewer.ID, nil)
	require.NoError(t, err)
	unseen, _, err := sessions.Create(t.Context(), pool, viewer.ID, nil)
	require.NoError(t, err)
	revoked, revokedSession, err := sessions.Create(t.Context(), pool, viewer.ID, nil)
	require.NoError(t, err)

	setExternalPlayerURLsEnabled(t, pool, clk, true)

	signer := newSigner(t, keyMaterial)
	authorizer := streamapi.NewAuthorizer(signer, auth.NewURLUsers(conn.Pool, clk, slog.New(slog.DiscardHandler)),
		settings.NewURLGate(conn.Pool, clk, slog.New(slog.DiscardHandler)), clk)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux := http.NewServeMux()
	mux.Handle(livePattern, authorizer.Session(ok))
	mux.Handle(signedPattern, authorizer.Signed(ok))
	server := httptest.NewServer(sessions.Middleware(conn.Pool)(mux))
	t.Cleanup(server.Close)
	expiresAt := clk.Now().Add(stream.DefaultURLLifetime)
	signedSeen := signer.Sign(resource, viewer.ID, expiresAt).URL
	signedUnseen := signer.Sign(resource, other.ID, expiresAt).URL

	get := func(t *testing.T, path, cookie string) int {
		t.Helper()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, nil)
		require.NoError(t, err)
		if cookie != "" {
			req.Header.Set("Cookie", auth.CookieName+"="+cookie)
		}
		resp, err := httpclienttest.New(t).Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		return resp.StatusCode
	}
	// DB が止まる前に、見ていたセッションと署名付き URL を認可する。取り消したセッションも、取り消す前は見ていた。
	require.Equal(t, http.StatusOK, get(t, sessionPath, seen))
	require.Equal(t, http.StatusOK, get(t, signedSeen, ""))
	require.Equal(t, http.StatusOK, get(t, sessionPath, revoked))
	require.NoError(t, sessions.Revoke(t.Context(), pool, viewer.ID, revokedSession.ID))
	conn.Down()

	tests := []struct {
		name       string
		path       string
		cookie     string
		wantStatus int
	}{
		{name: "認可できたセッション", path: sessionPath, cookie: seen, wantStatus: http.StatusOK},
		{name: "認可できた署名付き URL", path: signedSeen, cookie: "", wantStatus: http.StatusOK},
		{name: "DB が止まってから初めてのセッション", path: sessionPath, cookie: unseen, wantStatus: http.StatusServiceUnavailable},
		{name: "DB が止まってから初めての署名付き URL", path: signedUnseen, cookie: "", wantStatus: http.StatusServiceUnavailable},
		{name: "DB が止まる前に取り消したセッション", path: sessionPath, cookie: revoked, wantStatus: http.StatusServiceUnavailable},
		{name: "認証がない", path: sessionPath, cookie: "", wantStatus: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.wantStatus, get(t, tt.path, tt.cookie))
		})
	}
}

// TestAuthorizerFollowsExternalPlayerURLsSetting は、管理者が外部プレイヤー用の URL を有効にしている間だけ、
// 署名付き URL で映像を見られることを確かめる (#1755)。無効にしたら、期限の前に発行した URL も、その場で断る。
func TestAuthorizerFollowsExternalPlayerURLsSetting(t *testing.T) {
	t.Parallel()

	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	viewer, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{Username: "yuzu", DisplayName: "ゆず", Role: domain.RoleViewer, PasswordHash: nil})
	require.NoError(t, err)

	signer := newSigner(t, keyMaterial)
	authorizer := streamapi.NewAuthorizer(signer, auth.NewURLUsers(pool, clk, slog.New(slog.DiscardHandler)),
		settings.NewURLGate(pool, clk, slog.New(slog.DiscardHandler)), clk)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux := http.NewServeMux()
	mux.Handle(signedPattern, authorizer.Signed(ok))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	signed := signer.Sign(resource, viewer.ID, clk.Now().Add(stream.DefaultURLLifetime)).URL

	// 順に設定を変えるので、行ごとに並行させない。
	tests := []struct {
		name       string
		enabled    *bool
		wantStatus int
	}{
		{name: "既定では無効", enabled: nil, wantStatus: http.StatusForbidden},
		{name: "有効にした", enabled: new(true), wantStatus: http.StatusOK},
		{name: "無効に戻した", enabled: new(false), wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		if tt.enabled != nil {
			setExternalPlayerURLsEnabled(t, pool, clk, *tt.enabled)
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+signed, nil)
		require.NoError(t, err)
		resp, err := httpclienttest.New(t).Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, tt.wantStatus, resp.StatusCode, tt.name)
	}
}

// setExternalPlayerURLsEnabled は、管理者が外部プレイヤー用の URL を有効にするか無効にする (#1755)。
func setExternalPlayerURLsEnabled(t *testing.T, pool *pgxpool.Pool, clk *clocktest.Fake, enabled bool) {
	t.Helper()

	_, err := settings.NewStore(clk, audit.NewRecorder(clk)).Update(t.Context(), pool, audit.System("test"),
		&settings.Changes{ExternalPlayerURLsEnabled: &enabled})
	require.NoError(t, err)
}

// mustQuery は、URL のクエリの部分を返す。
func mustQuery(t *testing.T, rawURL string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)

	return parsed.RawQuery
}
