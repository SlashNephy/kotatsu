package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// TestAPITokens は、API トークンを発行し、そのユーザーのロールのまま認証し、取り消したら使えなくなることを確かめる (#431)。
func TestAPITokens(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	users := auth.NewUsers(clk)
	viewer, err := users.Create(t.Context(), db, &auth.NewUser{Username: "kiririn", DisplayName: "kiririn", Role: auth.RoleViewer, PasswordHash: nil})
	require.NoError(t, err)
	tokens := auth.NewAPITokens(db, clk, audit.NewRecorder(clk))
	actor := audit.User(viewer.ID)

	token, record, err := tokens.Create(t.Context(), actor, viewer.ID, "  macOS の kiririn  ")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(token, "kotatsu_"))
	assert.Equal(t, "macOS の kiririn", record.Name)

	principal, err := tokens.Authenticate(t.Context(), token)
	require.NoError(t, err)
	assert.Equal(t, viewer.ID, principal.UserID)
	assert.Equal(t, auth.RoleViewer, principal.Role)
	// セッションを持たないので、再認証の要る操作には使えない。
	require.ErrorIs(t, auth.RequireRecentAuthentication(auth.WithPrincipal(t.Context(), principal), clk), auth.ErrReauthenticationRequired)

	// 使った日時は、1 分ごとにしか書き直さない。
	listed, err := tokens.List(t.Context(), viewer.ID)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.NotNil(t, listed[0].LastUsedAt)
	assert.Equal(t, testNow(), listed[0].LastUsedAt.UTC())
	clk.Advance(30 * time.Second)
	_, err = tokens.Authenticate(t.Context(), token)
	require.NoError(t, err)
	listed, err = tokens.List(t.Context(), viewer.ID)
	require.NoError(t, err)
	assert.Equal(t, testNow(), listed[0].LastUsedAt.UTC())
	clk.Advance(30 * time.Second)
	_, err = tokens.Authenticate(t.Context(), token)
	require.NoError(t, err)
	listed, err = tokens.List(t.Context(), viewer.ID)
	require.NoError(t, err)
	assert.Equal(t, testNow().Add(time.Minute), listed[0].LastUsedAt.UTC())

	// 新しい順に並べる。
	_, newer, err := tokens.Create(t.Context(), actor, viewer.ID, "iPhone の kiririn")
	require.NoError(t, err)
	listed, err = tokens.List(t.Context(), viewer.ID)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	assert.Equal(t, []uuid.UUID{newer.ID, record.ID}, []uuid.UUID{listed[0].ID, listed[1].ID})
	require.NoError(t, tokens.Delete(t.Context(), actor, viewer.ID, newer.ID))

	// DB にはトークンそのものを残さない。
	var stored int
	require.NoError(t, db.QueryRow(t.Context(), "SELECT count(*) FROM api_tokens WHERE token_hash = $1::text::bytea", token).Scan(&stored))
	assert.Zero(t, stored)

	// ほかのユーザーのトークンは消せない。
	require.ErrorIs(t, tokens.Delete(t.Context(), actor, uuid.New(), record.ID), auth.ErrAPITokenNotFound)
	require.NoError(t, tokens.Delete(t.Context(), actor, viewer.ID, record.ID))
	_, err = tokens.Authenticate(t.Context(), token)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)

	var events int
	require.NoError(t, db.QueryRow(t.Context(),
		"SELECT count(*) FROM audit_events WHERE action IN ('api_token.create', 'api_token.delete') AND summary->>'name' = 'macOS の kiririn'").Scan(&events))
	assert.Equal(t, 2, events)
}

// TestAPITokenRejects は、名前の長さと、使えないトークンを確かめる。
func TestAPITokenRejects(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	users := auth.NewUsers(clk)
	user, err := users.Create(t.Context(), db, &auth.NewUser{Username: "rejects", DisplayName: "rejects", Role: auth.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	tokens := auth.NewAPITokens(db, clk, audit.NewRecorder(clk))
	actor := audit.User(user.ID)

	for _, name := range []string{"", "   ", strings.Repeat("あ", 65)} {
		_, _, err := tokens.Create(t.Context(), actor, user.ID, name)
		require.ErrorIs(t, err, auth.ErrInvalidAPITokenName, name)
	}

	token, _, err := tokens.Create(t.Context(), actor, user.ID, "kiririn")
	require.NoError(t, err)
	tests := []struct {
		name      string
		presented string
	}{
		{name: "印のないトークン", presented: strings.TrimPrefix(token, "kotatsu_")},
		{name: "知らないトークン", presented: "kotatsu_unknown"},
	}
	for _, tt := range tests {
		_, err := tokens.Authenticate(t.Context(), tt.presented)
		require.ErrorIs(t, err, auth.ErrUnauthenticated, tt.name)
	}

	// 無効にしたユーザーのトークンは使えない。
	_, err = db.Exec(t.Context(), "UPDATE users SET disabled_at = $1 WHERE id = $2", testNow(), user.ID)
	require.NoError(t, err)
	_, err = tokens.Authenticate(t.Context(), token)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
}

// TestAPITokenBearerMiddleware は、Bearer のトークンでログインしたことにし、違うトークンは 401 で断ることを確かめる。
func TestAPITokenBearerMiddleware(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	users := auth.NewUsers(clk)
	user, err := users.Create(t.Context(), db, &auth.NewUser{Username: "bearer", DisplayName: "bearer", Role: auth.RoleViewer, PasswordHash: nil})
	require.NoError(t, err)
	tokens := auth.NewAPITokens(db, clk, audit.NewRecorder(clk))
	token, _, err := tokens.Create(t.Context(), audit.User(user.ID), user.ID, "kiririn")
	require.NoError(t, err)
	handler := tokens.BearerMiddleware(auth.RequirePermission(auth.PermissionWatch)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	tests := []struct {
		name          string
		authorization string
		want          int
	}{
		{name: "正しいトークン", authorization: "Bearer " + token, want: http.StatusOK},
		{name: "違うトークン", authorization: "Bearer kotatsu_wrong", want: http.StatusUnauthorized},
		{name: "トークンなし", authorization: "", want: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/channels", http.NoBody)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}
