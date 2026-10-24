package auth_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/password"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/passwordtest"
)

// newTestPolicy は、パスワードの規定を返す。強度の判定は、-race で遅い zxcvbn-ts の代わりに、
// 決まったパスワード (kotatsukotatsu) と、ユーザー名を含むパスワードを弱いとみなすものに差し替える。
// zxcvbn-ts での判定は、password パッケージのテストで確かめる。
func newTestPolicy(t *testing.T) *password.Policy {
	t.Helper()

	policy, err := password.NewPolicy(&password.PolicyOptions{
		HIBP: nil, StrengthTimeout: 0, Estimator: passwordtest.NewEstimator("kotatsukotatsu"), Logger: slog.New(slog.DiscardHandler),
	})
	require.NoError(t, err)

	return policy
}

func testNow() time.Time {
	return time.Date(2026, 10, 9, 21, 0, 0, 0, time.UTC)
}

// newDB は、マイグレーションを適用した空のデータベースに接続する。
func newDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	return pool
}

// TestAuthorizationMatrix は、設計書 9.7 のロールと権限の表どおりに、ミドルウェアが通すか拒むかを確かめる。
func TestAuthorizationMatrix(t *testing.T) {
	t.Parallel()

	// 設計書 9.7 の表。行が操作、列が管理者、メンバー、視聴者。
	table := []struct {
		permission auth.Permission
		admin      bool
		member     bool
		viewer     bool
	}{
		{permission: auth.PermissionWatch, admin: true, member: true, viewer: true},
		{permission: auth.PermissionManageSchedule, admin: true, member: true, viewer: false},
		{permission: auth.PermissionManageRecordings, admin: true, member: true, viewer: false},
		{permission: auth.PermissionAdminister, admin: true, member: false, viewer: false},
	}

	for _, row := range table {
		for role, allowed := range map[auth.Role]bool{auth.RoleAdmin: row.admin, auth.RoleMember: row.member, auth.RoleViewer: row.viewer} {
			t.Run(string(row.permission)+"/"+string(role), func(t *testing.T) {
				t.Parallel()

				want := http.StatusForbidden
				if allowed {
					want = http.StatusOK
				}
				assert.Equal(t, want, serveWithPrincipal(t, row.permission, &auth.Principal{UserID: id.New(), Role: role, SessionID: id.New()}))
			})
		}
		t.Run(string(row.permission)+"/anonymous", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, http.StatusUnauthorized, serveWithPrincipal(t, row.permission, nil))
		})
	}
}

// serveWithPrincipal は、principal としてログインしたリクエストを、p を要するハンドラーに送り、状態コードを返す。
func serveWithPrincipal(t *testing.T, p auth.Permission, principal *auth.Principal) int {
	t.Helper()

	handler := auth.RequirePermission(p)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	if principal != nil {
		req = req.WithContext(auth.WithPrincipal(req.Context(), principal))
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec.Code
}

func TestParseRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		want    auth.Role
		wantErr bool
	}{
		{input: "admin", want: auth.RoleAdmin},
		{input: "member", want: auth.RoleMember},
		{input: "viewer", want: auth.RoleViewer},
		{input: "Admin", wantErr: true},
		{input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got, err := auth.ParseRole(tt.input)
			if tt.wantErr {
				require.ErrorIs(t, err, auth.ErrInvalidRole)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
