package auth_test

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/lib/password"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
)

const (
	adminPhrase       = "neko-to-mikan-no-fuyu-2026"
	rejectedSetupCode = `"code":"INVALID_SETUP_TOKEN"`
)

type setupFixture struct {
	db     *pgxpool.Pool
	setup  *auth.Setup
	client *auth.ClientInfo
}

func newSetupFixture(t *testing.T) *setupFixture {
	t.Helper()

	clk := clocktest.NewFake(testNow())
	policy := newTestPolicy(t)

	return &setupFixture{
		db:     newDB(t),
		setup:  auth.NewSetup(clk, audit.NewRecorder(clk), policy, slog.New(slog.DiscardHandler)),
		client: &auth.ClientInfo{UserAgent: browserUA, SourceIP: netip.MustParseAddr("192.168.1.10"), Host: testHost, Secure: true},
	}
}

func (f *setupFixture) admin(phrase string) *auth.NewAdmin {
	return &auth.NewAdmin{Username: "admin", DisplayName: "管理者", Password: phrase}
}

// TestSetup は、セットアップトークンがあるときだけ最初の管理者を作れ、作った後はトークンが無効になることを確かめる (設計書 9.2)。
func TestSetup(t *testing.T) {
	t.Parallel()

	f := newSetupFixture(t)
	needed, err := f.setup.Needed(t.Context(), f.db)
	require.NoError(t, err)
	assert.True(t, needed)

	// トークンがなければ、管理者を作れない。
	_, err = f.setup.CreateAdmin(t.Context(), f.db, "no-such-token", f.admin(adminPhrase), f.client)
	require.ErrorIs(t, err, auth.ErrInvalidSetupToken)

	token, err := f.setup.IssueToken(t.Context(), f.db)
	require.NoError(t, err)
	// DB には、トークンのハッシュだけを保存する。
	var stored []byte
	require.NoError(t, f.db.QueryRow(t.Context(), "SELECT token_hash FROM setup_tokens").Scan(&stored))
	assert.NotContains(t, string(stored), token)
	assert.Len(t, stored, 32)

	// 弱いパスワードの管理者は作れない。
	_, err = f.setup.CreateAdmin(t.Context(), f.db, token, f.admin("kotatsukotatsu"), f.client)
	rejected, ok := errors.AsType[*password.RejectedError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, password.ReasonTooWeak, rejected.Reason)

	admin, err := f.setup.CreateAdmin(t.Context(), f.db, token, f.admin(adminPhrase), f.client)
	require.NoError(t, err)
	assert.Equal(t, auth.RoleAdmin, admin.Role)

	// 作った後は、セットアップは要らず、トークンは消え、新しいトークンも発行できない。
	needed, err = f.setup.Needed(t.Context(), f.db)
	require.NoError(t, err)
	assert.False(t, needed)
	var tokens int
	require.NoError(t, f.db.QueryRow(t.Context(), "SELECT count(*) FROM setup_tokens").Scan(&tokens))
	assert.Zero(t, tokens)
	_, err = f.setup.CreateAdmin(t.Context(), f.db, token, &auth.NewAdmin{Username: otherName, DisplayName: otherName, Password: adminPhrase}, f.client)
	require.ErrorIs(t, err, auth.ErrSetupDone)
	_, err = f.setup.IssueToken(t.Context(), f.db)
	require.ErrorIs(t, err, auth.ErrSetupDone)

	// 管理者を無効にしても、初回セットアップは開き直さない。
	require.NoError(t, auth.NewUsers(clocktest.NewFake(testNow())).Disable(t.Context(), f.db, admin.ID))
	needed, err = f.setup.Needed(t.Context(), f.db)
	require.NoError(t, err)
	assert.False(t, needed)

	var audited int
	require.NoError(t, f.db.QueryRow(t.Context(), "SELECT count(*) FROM audit_events WHERE action = 'user.setup' AND source_ip = '192.168.1.10'").Scan(&audited))
	assert.Equal(t, 1, audited)
}

// TestSetupCreatesOnlyOneAdmin は、別々のトークンで同時に管理者を作っても、1 人しか作られないことを確かめる。
func TestSetupCreatesOnlyOneAdmin(t *testing.T) {
	t.Parallel()

	f := newSetupFixture(t)
	const attempts = 4
	tokens := make([]string, 0, attempts)
	for range attempts {
		token, err := f.setup.IssueToken(t.Context(), f.db)
		require.NoError(t, err)
		tokens = append(tokens, token)
	}

	var (
		mu      sync.Mutex
		created int
		done    sync.WaitGroup
		errs    = []error{}
	)
	for i, token := range tokens {
		done.Go(func() {
			_, err := f.setup.CreateAdmin(t.Context(), f.db, token, &auth.NewAdmin{
				Username: fmt.Sprintf("admin%d", i), DisplayName: "管理者", Password: adminPhrase,
			}, f.client)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				created++
			} else {
				errs = append(errs, err)
			}
		})
	}
	done.Wait()
	assert.Equal(t, 1, created)
	for _, err := range errs {
		require.ErrorIs(t, err, auth.ErrSetupDone)
	}
}

func TestSetupHandlers(t *testing.T) {
	t.Parallel()

	f := newSetupFixture(t)
	client := func(*http.Request) *auth.ClientInfo { return f.client }
	status := f.setup.StatusHandler(f.db)
	create := f.setup.CreateHandler(f.db, client)
	token, err := f.setup.IssueToken(t.Context(), f.db)
	require.NoError(t, err)

	serve := func(handler http.Handler, method, body string) (int, string) {
		req := httptest.NewRequestWithContext(t.Context(), method, "/auth/setup", strings.NewReader(body))
		req.Header.Set("Content-Type", jsonType)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		got, err := io.ReadAll(rec.Body)
		require.NoError(t, err)

		return rec.Code, string(got)
	}
	body := func(token, phrase string) string {
		return `{"token":"` + token + `","username":"admin","displayName":"管理者","password":"` + phrase + `"}`
	}

	code, got := serve(status, http.MethodGet, "")
	assert.Equal(t, http.StatusOK, code)
	assert.JSONEq(t, `{"required":true}`, got)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		want       string
	}{
		{name: "missing token", body: body("", adminPhrase), wantStatus: http.StatusUnauthorized, want: rejectedSetupCode},
		{name: "wrong token", body: body("wrong", adminPhrase), wantStatus: http.StatusUnauthorized, want: rejectedSetupCode},
		// トークンがなければ、パスワードの規定に当たるかどうかも見せない。
		{name: "wrong token and weak password", body: body("wrong", "kotatsukotatsu"), wantStatus: http.StatusUnauthorized, want: rejectedSetupCode},
		{name: "weak password", body: body(token, "kotatsukotatsu"), wantStatus: http.StatusBadRequest, want: `{"error":{"code":"WEAK_PASSWORD","reason":"too_weak"}}`},
		{name: "created", body: body(token, adminPhrase), wantStatus: http.StatusCreated, want: `{"username":"admin","next":"login"}`},
		{name: "already done", body: body(token, adminPhrase), wantStatus: http.StatusConflict, want: `"code":"SETUP_DONE"`},
	}
	for _, tt := range tests {
		code, got := serve(create, http.MethodPost, tt.body)
		assert.Equal(t, tt.wantStatus, code, tt.name)
		assert.Contains(t, got, tt.want, tt.name)
	}

	code, got = serve(status, http.MethodGet, "")
	assert.Equal(t, http.StatusOK, code)
	assert.JSONEq(t, `{"required":false}`, got)
}
