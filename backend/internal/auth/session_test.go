package auth_test

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/id"
)

const day = 24 * time.Hour

// sessionFixture は、ユーザーを 1 人作った DB と、偽の時計で動く Sessions。
type sessionFixture struct {
	db       *pgxpool.Pool
	clock    *clocktest.Fake
	users    *auth.Users
	sessions *auth.Sessions
	userID   uuid.UUID
}

func newSessionFixture(t *testing.T, db *pgxpool.Pool) *sessionFixture {
	t.Helper()

	clk := clocktest.NewFake(testNow())
	users := auth.NewUsers(clk)
	user, err := users.Create(t.Context(), db, &auth.NewUser{Username: "user-" + id.New().String(), DisplayName: "ゆーざー", Role: auth.RoleMember})
	require.NoError(t, err)

	return &sessionFixture{
		db:       db,
		clock:    clk,
		users:    users,
		sessions: auth.NewSessions(&auth.SessionsOptions{Clock: clk}),
		userID:   user.ID,
	}
}

func (f *sessionFixture) login(t *testing.T) (string, *auth.Session) {
	t.Helper()

	token, session, err := f.sessions.Create(t.Context(), f.db, f.userID, &auth.ClientInfo{
		UserAgent: "Mozilla/5.0",
		SourceIP:  netip.MustParseAddr("192.168.1.10"),
		Host:      testHost,
		Secure:    false,
	})
	require.NoError(t, err)

	return token, session
}

func TestAuthenticate(t *testing.T) {
	t.Parallel()

	db := newDB(t)

	tests := []struct {
		name string
		// steps は、ログインした後に行う操作。時計を進めたり、途中で使ったりする。
		steps   func(t *testing.T, f *sessionFixture, token string, session *auth.Session)
		wantErr error
	}{
		{
			name:  "right after login",
			steps: func(*testing.T, *sessionFixture, string, *auth.Session) {},
		},
		{
			name: "just before the idle timeout",
			steps: func(_ *testing.T, f *sessionFixture, _ string, _ *auth.Session) {
				f.clock.Advance(30*day - time.Second)
			},
		},
		{
			name: "idle for 30 days",
			steps: func(_ *testing.T, f *sessionFixture, _ string, _ *auth.Session) {
				f.clock.Advance(30 * day)
			},
			wantErr: auth.ErrSessionInvalid,
		},
		{
			name: "used every 20 days until just before the absolute timeout",
			steps: func(t *testing.T, f *sessionFixture, token string, _ *auth.Session) {
				t.Helper()
				for range 4 {
					f.clock.Advance(20 * day)
					_, err := f.sessions.Authenticate(t.Context(), f.db, token)
					require.NoError(t, err)
				}
				f.clock.Advance(10*day - time.Second)
			},
		},
		{
			name: "used every 20 days but past the absolute timeout",
			steps: func(t *testing.T, f *sessionFixture, token string, _ *auth.Session) {
				t.Helper()
				for range 4 {
					f.clock.Advance(20 * day)
					_, err := f.sessions.Authenticate(t.Context(), f.db, token)
					require.NoError(t, err)
				}
				f.clock.Advance(10 * day)
			},
			wantErr: auth.ErrSessionInvalid,
		},
		{
			name: "revoked",
			steps: func(t *testing.T, f *sessionFixture, _ string, session *auth.Session) {
				t.Helper()
				require.NoError(t, f.sessions.Revoke(t.Context(), f.db, f.userID, session.ID))
			},
			wantErr: auth.ErrSessionInvalid,
		},
		{
			name: "user disabled",
			steps: func(t *testing.T, f *sessionFixture, _ string, _ *auth.Session) {
				t.Helper()
				require.NoError(t, f.users.Disable(t.Context(), f.db, f.userID))
			},
			wantErr: auth.ErrSessionInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newSessionFixture(t, db)
			token, session := f.login(t)
			tt.steps(t, f, token, session)

			principal, err := f.sessions.Authenticate(t.Context(), f.db, token)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			// 使い続けても、認証した日時はログインしたときのまま。
			assert.Equal(t, &auth.Principal{UserID: f.userID, Role: auth.RoleMember, SessionID: session.ID, AuthenticatedAt: testNow()}, principal)
		})
	}

	t.Run("without options and client info", func(t *testing.T) {
		t.Parallel()

		f := newSessionFixture(t, db)
		sessions := auth.NewSessions(nil)
		token, session, err := sessions.Create(t.Context(), f.db, f.userID, nil)
		require.NoError(t, err)
		assert.Empty(t, session.UserAgent)
		assert.False(t, session.SourceIP.IsValid())
		_, err = sessions.Authenticate(t.Context(), f.db, token)
		require.NoError(t, err)
	})

	t.Run("unknown token", func(t *testing.T) {
		t.Parallel()

		f := newSessionFixture(t, db)
		_, err := f.sessions.Authenticate(t.Context(), f.db, "not-a-token")
		require.ErrorIs(t, err, auth.ErrSessionInvalid)
	})
}

func TestAuthenticateThrottlesTouch(t *testing.T) {
	t.Parallel()

	f := newSessionFixture(t, newDB(t))
	token, _ := f.login(t)
	lastSeen := func() time.Time {
		sessions, err := f.sessions.List(t.Context(), f.db, f.userID)
		require.NoError(t, err)
		require.Len(t, sessions, 1)

		return sessions[0].LastSeenAt
	}

	// 1 分以内に使われても、最後に使われた日時は書き換えない。
	f.clock.Advance(30 * time.Second)
	_, err := f.sessions.Authenticate(t.Context(), f.db, token)
	require.NoError(t, err)
	assert.True(t, testNow().Equal(lastSeen()))

	// 1 分を過ぎたら書き換える。
	f.clock.Advance(31 * time.Second)
	_, err = f.sessions.Authenticate(t.Context(), f.db, token)
	require.NoError(t, err)
	assert.True(t, testNow().Add(61*time.Second).Equal(lastSeen()))
}

func TestListAndRevoke(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	alice := newSessionFixture(t, db)
	bob := newSessionFixture(t, db)
	bob.clock = alice.clock
	bob.sessions = alice.sessions

	_, first := alice.login(t)
	alice.clock.Advance(time.Hour)
	_, second := alice.login(t)
	_, bobs := bob.login(t)

	listed, err := alice.sessions.List(t.Context(), db, alice.userID)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	// 最後に使われた順に並ぶ。
	assert.Equal(t, []uuid.UUID{second.ID, first.ID}, []uuid.UUID{listed[0].ID, listed[1].ID})
	assert.Equal(t, "Mozilla/5.0", listed[0].UserAgent)
	assert.Equal(t, netip.MustParseAddr("192.168.1.10"), listed[0].SourceIP)

	tests := []struct {
		name      string
		sessionID uuid.UUID
		wantErr   error
	}{
		{name: "own session", sessionID: first.ID},
		{name: "already revoked", sessionID: first.ID, wantErr: auth.ErrSessionNotFound},
		{name: "another user's session", sessionID: bobs.ID, wantErr: auth.ErrSessionNotFound},
		{name: "unknown session", sessionID: id.New(), wantErr: auth.ErrSessionNotFound},
	}
	for _, tt := range tests {
		// 前のケースで失効させた結果を次のケースで使うので、順に動かす。
		err := alice.sessions.Revoke(t.Context(), db, alice.userID, tt.sessionID)
		if tt.wantErr != nil {
			require.ErrorIs(t, err, tt.wantErr, tt.name)

			continue
		}
		require.NoError(t, err, tt.name)
	}

	listed, err = alice.sessions.List(t.Context(), db, alice.userID)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, second.ID, listed[0].ID)

	// 他のユーザーのセッションは失効していない。
	bobListed, err := bob.sessions.List(t.Context(), db, bob.userID)
	require.NoError(t, err)
	assert.Len(t, bobListed, 1)

	// 無操作の期限を過ぎたセッションは、一覧に出さない。
	alice.clock.Advance(30 * day)
	listed, err = alice.sessions.List(t.Context(), db, alice.userID)
	require.NoError(t, err)
	assert.Empty(t, listed)
}

// newCookieServer は、/login でログインし、/watch で PermissionWatch を要するサーバーを作る。
func newCookieServer(t *testing.T, f *sessionFixture, tls bool) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		token, session, err := f.sessions.Create(r.Context(), f.db, f.userID, &auth.ClientInfo{UserAgent: r.UserAgent(), SourceIP: netip.Addr{}, Host: "", Secure: false})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}
		f.sessions.SetCookie(w, r, token, session)
	})
	mux.Handle("GET /watch", auth.RequirePermission(auth.PermissionWatch)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(auth.PrincipalFrom(r.Context()).UserID.String()))
	})))
	handler := f.sessions.Middleware(f.db)(mux)

	var server *httptest.Server
	if tls {
		server = httptest.NewTLSServer(handler)
	} else {
		server = httptest.NewServer(handler)
	}
	t.Cleanup(server.Close)

	return server
}

// response は、テストで見る応答の部分。本文は読まずに閉じる。
type response struct {
	status  int
	cookies []*http.Cookie
}

func do(t *testing.T, client *http.Client, method, url string, cookie *http.Cookie) *response {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, url, nil)
	require.NoError(t, err)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	return &response{status: resp.StatusCode, cookies: resp.Cookies()}
}

func sessionCookie(t *testing.T, resp *response) *http.Cookie {
	t.Helper()

	for _, c := range resp.cookies {
		if c.Name == auth.CookieName {
			return c
		}
	}
	require.FailNow(t, "the session cookie is not set")

	return nil
}

func TestSessionCookie(t *testing.T) {
	t.Parallel()

	db := newDB(t)

	tests := []struct {
		name       string
		tls        bool
		wantSecure bool
	}{
		{name: "HTTP", tls: false, wantSecure: false},
		{name: "HTTPS", tls: true, wantSecure: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newSessionFixture(t, db)
			server := newCookieServer(t, f, tt.tls)
			client := server.Client()

			resp := do(t, client, http.MethodPost, server.URL+"/login", nil)
			require.Equal(t, http.StatusOK, resp.status)
			cookie := sessionCookie(t, resp)
			assert.True(t, cookie.HttpOnly)
			assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
			assert.Equal(t, tt.wantSecure, cookie.Secure)
			assert.Equal(t, "/", cookie.Path)
			assert.True(t, testNow().Add(90*day).Equal(cookie.Expires), "the cookie expires with the absolute timeout")

			// Cookie があれば通り、なければ 401 になる。
			assert.Equal(t, http.StatusOK, do(t, client, http.MethodGet, server.URL+"/watch", cookie).status)
			assert.Equal(t, http.StatusUnauthorized, do(t, client, http.MethodGet, server.URL+"/watch", nil).status)
		})
	}
}

func TestMiddlewareClearsInvalidCookie(t *testing.T) {
	t.Parallel()

	f := newSessionFixture(t, newDB(t))
	server := newCookieServer(t, f, false)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := server.Client()
	client.Jar = jar

	login := do(t, client, http.MethodPost, server.URL+"/login", nil)
	require.Equal(t, http.StatusOK, login.status)
	assert.Equal(t, http.StatusOK, do(t, client, http.MethodGet, server.URL+"/watch", nil).status)

	// 無操作の期限を過ぎたら 401 になり、Cookie も消える。
	f.clock.Advance(30 * day)
	resp := do(t, client, http.MethodGet, server.URL+"/watch", nil)
	assert.Equal(t, http.StatusUnauthorized, resp.status)
	cleared := sessionCookie(t, resp)
	assert.Empty(t, cleared.Value)
	assert.Negative(t, cleared.MaxAge)
}
