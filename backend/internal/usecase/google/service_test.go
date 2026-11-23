package google_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/googletest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/google"
)

const (
	// origin は、テストで Kotatsu に接続したオリジン。
	origin = "http://kotatsu.test"
	// clientID と clientSecret は、モックの Google に登録したクライアント。
	clientID     = "client"
	clientSecret = "secret"
	// reservedPool は、設定の保存先プールの ID。Google Drive のライブラリの名前と重ねない。
	reservedPool = "main"
)

type fixture struct {
	pool    *pgxpool.Pool
	clock   *clocktest.Fake
	fake    *googletest.Server
	service *google.Service
	admin   *auth.Principal
	other   *auth.Principal
	client  *auth.ClientInfo
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC))
	fake := googletest.New(clientID, clientSecret)
	fake.RedirectURIs = []string{origin + google.CallbackPath}
	fake.SetNow(clk.Now)
	fake.AddFile(&googletest.File{ID: "a", Name: "a.ts", MIMEType: "video/mp2t", Parent: googletest.MyDriveID, Content: []byte("a")})
	server := httptest.NewServer(fake.Handler())
	t.Cleanup(server.Close)
	service, err := google.NewService(&google.Options{
		Clock: clk, Recorder: audit.NewRecorder(clk), AppSecret: strings.Repeat("k", 32), HTTPClient: server.Client(),
		Logger: slog.New(slog.DiscardHandler), URL: server.URL, ReservedLibraryNames: []string{reservedPool},
	})
	require.NoError(t, err)
	f := &fixture{
		pool: pool, clock: clk, fake: fake, service: service, admin: nil, other: nil,
		client: &auth.ClientInfo{UserAgent: "test", SourceIP: netip.MustParseAddr("127.0.0.1"), Host: "kotatsu.test", Secure: false},
	}
	f.admin = f.login(t, "admin", domain.RoleAdmin)
	f.other = f.login(t, "other", domain.RoleAdmin)

	return f
}

func (f *fixture) login(t *testing.T, name string, role domain.Role) *auth.Principal {
	t.Helper()
	user, err := auth.NewUsers(f.clock).Create(t.Context(), f.pool, &auth.NewUser{Username: name, DisplayName: name, Role: role, PasswordHash: nil})
	require.NoError(t, err)
	_, session, err := auth.NewSessions(&auth.SessionsOptions{Clock: f.clock}).Create(t.Context(), f.pool, user.ID, f.client)
	require.NoError(t, err)

	return &auth.Principal{UserID: user.ID, Role: role, SessionID: session.ID, AuthenticatedAt: session.CreatedAt, OAuthClientID: ""}
}

func (f *fixture) registerApp(t *testing.T) {
	t.Helper()
	require.NoError(t, f.service.UpdateApp(t.Context(), f.pool, audit.User(f.admin.UserID),
		&google.AppChanges{ClientID: clientID, ClientSecret: new(clientSecret)}))
}

// authorize は、管理者が認可を始め、モックの Google で許して戻ってきたときのクエリを返す。
func (f *fixture) authorize(t *testing.T) url.Values {
	t.Helper()
	authURL, err := f.service.BeginDrive(t.Context(), f.pool, f.admin, origin)
	require.NoError(t, err)
	parsed, err := url.Parse(authURL)
	require.NoError(t, err)
	query := parsed.Query()
	assert.Equal(t, google.DriveScope, query.Get("scope"))
	assert.Equal(t, "offline", query.Get("access_type"))
	assert.Equal(t, "consent", query.Get("prompt"))
	assert.Equal(t, "S256", query.Get("code_challenge_method"))
	assert.Equal(t, origin+"/internal/auth/google/callback", query.Get("redirect_uri"))
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, authURL, http.NoBody)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusFound, res.StatusCode)
	location, err := url.Parse(res.Header.Get("Location"))
	require.NoError(t, err)

	return location.Query()
}

func (f *fixture) complete(t *testing.T, principal *auth.Principal, query url.Values) (string, error) {
	t.Helper()

	return f.service.Complete(t.Context(), f.pool, principal, &google.Callback{
		State: query.Get("state"), Code: query.Get("code"), Error: query.Get("error"), Scope: query.Get("scope"),
	}, f.client)
}

// TestConnectDrive は、管理者が登録したクライアントで Google Drive を接続し、リフレッシュトークンからアクセストークンを作って
// Drive を読めることを確認する。
func TestConnectDrive(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	_, err := f.service.BeginDrive(t.Context(), f.pool, f.admin, origin)
	require.ErrorIs(t, err, google.ErrUnavailable)
	f.registerApp(t)
	app, err := f.service.App(t.Context(), f.pool)
	require.NoError(t, err)
	assert.Equal(t, clientID, app.ClientID)
	assert.True(t, app.Configured)

	purpose, err := f.complete(t, f.admin, f.authorize(t))
	require.NoError(t, err)
	assert.Equal(t, google.PurposeDrive, purpose)
	drive, err := f.service.Drive(t.Context(), f.pool)
	require.NoError(t, err)
	assert.Equal(t, "admin@example.com", drive.AccountName)
	assert.Nil(t, drive.RevokedAt)

	// リフレッシュトークンは、暗号化して保存する。
	row, err := sqlc.New(f.pool).GetGoogleDriveConnection(t.Context())
	require.NoError(t, err)
	assert.NotEmpty(t, row.RefreshTokenEncrypted)

	tokens := f.service.DriveTokens(f.pool)
	connected, err := tokens.Connected(t.Context())
	require.NoError(t, err)
	assert.True(t, connected)
	client := f.service.DriveClient(tokens)
	file, err := client.Get(t.Context(), "a")
	require.NoError(t, err)
	assert.Equal(t, "a.ts", file.Name)
	assert.Equal(t, 1, f.fake.Refreshes())
	// 期限まで、アクセストークンを使い回す。
	_, err = client.Get(t.Context(), "a")
	require.NoError(t, err)
	assert.Equal(t, 1, f.fake.Refreshes())
	// 期限が近づいたら、作り直す。
	f.clock.Advance(time.Hour)
	_, err = client.Get(t.Context(), "a")
	require.NoError(t, err)
	assert.Equal(t, 2, f.fake.Refreshes())

	// Google でアクセスを取り消されたら、接続に印を付け、読まない。
	f.fake.RevokeAll()
	tokens.Invalidate()
	_, err = client.Get(t.Context(), "a")
	require.ErrorIs(t, err, google.ErrNotConnected)
	drive, err = f.service.Drive(t.Context(), f.pool)
	require.NoError(t, err)
	assert.NotNil(t, drive.RevokedAt)
	connected, err = tokens.Connected(t.Context())
	require.NoError(t, err)
	assert.False(t, connected)

	// 接続し直せば、印は消える。同じ部品の DriveTokens は、覚えた接続を待たずに読み直す。
	_, err = f.complete(t, f.admin, f.authorize(t))
	require.NoError(t, err)
	drive, err = f.service.Drive(t.Context(), f.pool)
	require.NoError(t, err)
	assert.Nil(t, drive.RevokedAt)
	connected, err = tokens.Connected(t.Context())
	require.NoError(t, err)
	assert.True(t, connected)

	// 接続し直したら、前のリフレッシュトークンを取り消す。外すと、今のリフレッシュトークンを Google で取り消す。
	assert.Len(t, f.fake.Revoked(), 1)
	require.NoError(t, f.service.DisconnectDrive(t.Context(), f.pool, audit.User(f.admin.UserID)))
	assert.Len(t, f.fake.Revoked(), 2)
	_, err = f.service.Drive(t.Context(), f.pool)
	require.ErrorIs(t, err, google.ErrNotConnected)
	require.ErrorIs(t, f.service.DisconnectDrive(t.Context(), f.pool, audit.User(f.admin.UserID)), google.ErrNotConnected)
}

// TestCompleteRejects は、始めたセッションと違う、使い回した、断った、などの戻りを接続しないことを確認する。
func TestCompleteRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// change は、戻ってきたクエリと、戻ってきたセッションを変える。
		change           func(f *fixture, query url.Values) *auth.Principal
		omitRefreshToken bool
		wantErr          error
	}{
		{name: "another session", change: func(f *fixture, _ url.Values) *auth.Principal { return f.other }, wantErr: google.ErrAuthorizationMismatch},
		{name: "no session", change: func(*fixture, url.Values) *auth.Principal { return nil }, wantErr: google.ErrAuthorizationMismatch},
		{
			name: "an unknown state",
			change: func(f *fixture, query url.Values) *auth.Principal {
				query.Set("state", "forged")

				return f.admin
			},
			wantErr: google.ErrAuthorizationExpired,
		},
		{
			name: "denied",
			change: func(f *fixture, query url.Values) *auth.Principal {
				query.Del("code")
				query.Set("error", "access_denied")

				return f.admin
			},
			wantErr: google.ErrAccessDenied,
		},
		{
			name: "the Drive scope not granted",
			change: func(f *fixture, query url.Values) *auth.Principal {
				query.Set("scope", "openid")

				return f.admin
			},
			wantErr: google.ErrAccessDenied,
		},
		{name: "no refresh token", change: func(f *fixture, _ url.Values) *auth.Principal { return f.admin }, omitRefreshToken: true, wantErr: google.ErrNoRefreshToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.registerApp(t)
			f.fake.OmitRefreshToken = tt.omitRefreshToken
			query := f.authorize(t)
			principal := tt.change(f, query)
			_, err := f.complete(t, principal, query)
			require.ErrorIs(t, err, tt.wantErr)
			_, err = f.service.Drive(t.Context(), f.pool)
			require.ErrorIs(t, err, google.ErrNotConnected)
		})
	}

	// 同じ state は、2 度使えない。
	f := newFixture(t)
	f.registerApp(t)
	query := f.authorize(t)
	_, err := f.complete(t, f.admin, query)
	require.NoError(t, err)
	_, err = f.complete(t, f.admin, query)
	require.ErrorIs(t, err, google.ErrAuthorizationExpired)

	// 10 分を過ぎて戻ってきたら、接続しない。
	query = f.authorize(t)
	f.clock.Advance(11 * time.Minute)
	_, err = f.complete(t, f.admin, query)
	require.ErrorIs(t, err, google.ErrAuthorizationExpired)

	// API トークンや MCP のクライアント (セッションのない操作) では、始められない。
	token := *f.admin
	token.SessionID = uuid.Nil()
	_, err = f.service.BeginDrive(t.Context(), f.pool, &token, origin)
	require.ErrorIs(t, err, google.ErrSessionRequired)
}

// TestUpdateApp は、クライアントの登録と、消したときに Drive の接続も外すことを確認する。
func TestUpdateApp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		changes *google.AppChanges
		wantErr error
	}{
		{name: "no secret at first", changes: &google.AppChanges{ClientID: clientID, ClientSecret: nil}, wantErr: google.ErrInvalidApp},
		{name: "empty secret", changes: &google.AppChanges{ClientID: clientID, ClientSecret: new(" ")}, wantErr: google.ErrInvalidApp},
		{name: "too long client ID", changes: &google.AppChanges{ClientID: strings.Repeat("a", 257), ClientSecret: new("s")}, wantErr: google.ErrInvalidApp},
		{name: "valid", changes: &google.AppChanges{ClientID: " " + clientID + " ", ClientSecret: new(clientSecret)}, wantErr: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			err := f.service.UpdateApp(t.Context(), f.pool, audit.User(f.admin.UserID), tt.changes)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			app, err := f.service.App(t.Context(), f.pool)
			require.NoError(t, err)
			assert.Equal(t, clientID, app.ClientID)
		})
	}

	f := newFixture(t)
	f.registerApp(t)
	_, err := f.complete(t, f.admin, f.authorize(t))
	require.NoError(t, err)
	// シークレットを省けば、登録したものを変えない。
	require.NoError(t, f.service.UpdateApp(t.Context(), f.pool, audit.User(f.admin.UserID), &google.AppChanges{ClientID: "renamed", ClientSecret: nil}))
	_, err = f.service.Drive(t.Context(), f.pool)
	require.NoError(t, err)
	// クライアント ID を空にすると、登録を消し、Drive の接続も外して、リフレッシュトークンを取り消す。
	require.NoError(t, f.service.UpdateApp(t.Context(), f.pool, audit.User(f.admin.UserID), &google.AppChanges{ClientID: "", ClientSecret: nil}))
	app, err := f.service.App(t.Context(), f.pool)
	require.NoError(t, err)
	assert.False(t, app.Configured)
	_, err = f.service.Drive(t.Context(), f.pool)
	require.ErrorIs(t, err, google.ErrNotConnected)
	assert.Len(t, f.fake.Revoked(), 1)
}
