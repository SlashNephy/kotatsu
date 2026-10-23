package stream_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/stream"
)

const (
	// keyMaterial は、テストのアプリの秘密キー。
	keyMaterial = "kotatsu-stream-test-application-key-0123456789"
	// resource は、署名するリソース。
	resource = "/streams/services/" + serviceID
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
		{name: "別のリソース", signer: newSigner(t, keyMaterial), resource: "/streams/services/3273601025", query: query.Clone, now: now, wantErr: true},
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

// TestAuthorizer は、ログイン中のセッションか、無効にしていないユーザーの署名付き URL のどちらかで、映像を見られることを確かめる。
func TestAuthorizer(t *testing.T) {
	t.Parallel()

	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	users := auth.NewUsers(clk)
	viewer, err := users.Create(t.Context(), pool, &auth.NewUser{Username: "mikan", DisplayName: "みかん", Role: auth.RoleViewer, PasswordHash: nil})
	require.NoError(t, err)
	disabled, err := users.Create(t.Context(), pool, &auth.NewUser{Username: "neko", DisplayName: "ねこ", Role: auth.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	require.NoError(t, users.Disable(t.Context(), pool, disabled.ID))
	sessions := auth.NewSessions(&auth.SessionsOptions{IdleTimeout: 0, AbsoluteTimeout: 0, IsSecure: nil, Clock: clk})
	token, _, err := sessions.Create(t.Context(), pool, viewer.ID, nil)
	require.NoError(t, err)

	signer := newSigner(t, keyMaterial)
	mux := http.NewServeMux()
	mux.Handle(livePattern, stream.NewAuthorizer(signer, pool, clk).Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	server := httptest.NewServer(sessions.Middleware(pool)(mux))
	t.Cleanup(server.Close)
	expiresAt := clk.Now().Add(stream.DefaultURLLifetime)

	tests := []struct {
		name       string
		path       string
		cookie     string
		wantStatus int
	}{
		{name: "ログイン中のセッション", path: resource, cookie: token, wantStatus: http.StatusOK},
		{name: "署名付き URL", path: signer.Sign(resource, viewer.ID, expiresAt).URL + "&start=60", cookie: "", wantStatus: http.StatusOK},
		{name: "認証がない", path: resource, cookie: "", wantStatus: http.StatusUnauthorized},
		{name: "無効にしたユーザーの署名付き URL", path: signer.Sign(resource, disabled.ID, expiresAt).URL, cookie: "", wantStatus: http.StatusUnauthorized},
		{name: "いないユーザーの署名付き URL", path: signer.Sign(resource, uuid.New(), expiresAt).URL, cookie: "", wantStatus: http.StatusUnauthorized},
		{
			name: "別のリソースの署名付き URL", cookie: "", wantStatus: http.StatusUnauthorized,
			path: resource + "?" + mustQuery(t, signer.Sign("/streams/services/3273601025", viewer.ID, expiresAt).URL),
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
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

// mustQuery は、URL のクエリの部分を返す。
func mustQuery(t *testing.T, rawURL string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)

	return parsed.RawQuery
}
