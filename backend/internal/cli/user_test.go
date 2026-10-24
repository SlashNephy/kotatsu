package cli_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/lib/password"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
)

const adminUsername = "kotatsu"

// TestUserSetPassword は、kotatsu user set-password が、標準入力のパスワードを規定に照らして設定することを確かめる。
func TestUserSetPassword(t *testing.T) {
	t.Parallel()

	dbConfig := databasetest.New(t)
	env := testEnv()
	env["KOTATSU_DATABASE_URL"] = dbConfig.URL.Value()
	opts := cli.DefaultOptions(slog.New(slog.DiscardHandler))
	// -race では、パスワードの強度の判定が 10 倍以上遅くなる。
	opts.PasswordStrengthTimeout = 10 * time.Minute
	opts.LookupEnv = func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	}
	migrateCmd := cli.NewCommand(opts)
	migrateCmd.SetArgs([]string{subcommandMigrate})
	require.NoError(t, migrateCmd.ExecuteContext(t.Context()))

	pool, err := database.Open(t.Context(), dbConfig)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	user, err := auth.NewUsers(clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))).Create(t.Context(), pool, &auth.NewUser{
		Username: adminUsername, DisplayName: adminUsername, Role: auth.RoleAdmin, PasswordHash: nil,
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		username string
		stdin    string
		wantOut  string
		wantErr  string
	}{
		{name: "weak password is rejected", username: adminUsername, stdin: "kotatsukotatsu\n", wantErr: "the password was rejected: too_weak"},
		{name: "empty input", username: adminUsername, stdin: "", wantErr: "the password must be given on the standard input: EOF"},
		{name: "empty line", username: adminUsername, stdin: "\n", wantErr: "the password must be given on the standard input"},
		{name: "unknown user", username: "nobody", stdin: "neko-to-mikan-no-fuyu-2026\n", wantErr: "user not found"},
		{name: "strong password is set", username: "KOTATSU", stdin: "neko-to-mikan-no-fuyu-2026\n", wantOut: "set the password of KOTATSU\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			cmd := cli.NewCommand(opts)
			cmd.SetArgs([]string{"user", "set-password", tt.username})
			cmd.SetIn(strings.NewReader(tt.stdin))
			cmd.SetOut(&out)
			err := cmd.ExecuteContext(t.Context())
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, out.String())

			var encoded string
			require.NoError(t, pool.QueryRow(t.Context(), "SELECT password_hash FROM users WHERE id = $1", user.ID).Scan(&encoded))
			verification, err := password.Verify(strings.TrimSpace(tt.stdin), encoded)
			require.NoError(t, err)
			assert.True(t, verification.OK)
		})
	}
}

// TestAdminSetupToken は、kotatsu admin setup-token が、管理者がいない間だけトークンを発行することを確かめる。
func TestAdminSetupToken(t *testing.T) {
	t.Parallel()

	dbConfig := databasetest.New(t)
	env := testEnv()
	env["KOTATSU_DATABASE_URL"] = dbConfig.URL.Value()
	opts := cli.DefaultOptions(slog.New(slog.DiscardHandler))
	opts.LookupEnv = func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	}
	migrateCmd := cli.NewCommand(opts)
	migrateCmd.SetArgs([]string{subcommandMigrate})
	require.NoError(t, migrateCmd.ExecuteContext(t.Context()))
	pool, err := database.Open(t.Context(), dbConfig)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	issue := func() (string, error) {
		var out bytes.Buffer
		cmd := cli.NewCommand(opts)
		cmd.SetArgs([]string{"admin", "setup-token"})
		cmd.SetOut(&out)
		err := cmd.ExecuteContext(t.Context())

		return out.String(), err
	}
	token, err := issue()
	require.NoError(t, err)
	assert.Regexp(t, `^[A-Za-z0-9_-]{43}\n$`, token)

	// all の起動時にも、管理者がいなければ、トークンをログに出す。
	var logs bytes.Buffer
	require.NoError(t, opts.LogSetupToken(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)), opts.Clock, pool))
	assert.Regexp(t, `"setup_token":"[A-Za-z0-9_-]{43}"`, logs.String())

	_, err = auth.NewUsers(clocktest.NewFake(time.Now())).Create(t.Context(), pool, &auth.NewUser{
		Username: adminUsername, DisplayName: adminUsername, Role: auth.RoleAdmin, PasswordHash: nil,
	})
	require.NoError(t, err)
	_, err = issue()
	require.ErrorContains(t, err, "an administrator already exists")
	logs.Reset()
	require.NoError(t, opts.LogSetupToken(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)), opts.Clock, pool))
	assert.Empty(t, logs.String())
}

// TestUserLinkOIDC は、kotatsu user link-oidc が、設定の IdP のユーザーとローカルのアカウントを紐付けることを確かめる。
func TestUserLinkOIDC(t *testing.T) {
	t.Parallel()

	const issuer = "https://idp.example.com"
	dbConfig := databasetest.New(t)
	env := testEnv()
	env["KOTATSU_DATABASE_URL"] = dbConfig.URL.Value()
	opts := cli.DefaultOptions(slog.New(slog.DiscardHandler))
	opts.LookupEnv = func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	}
	migrateCmd := cli.NewCommand(opts)
	migrateCmd.SetArgs([]string{subcommandMigrate})
	require.NoError(t, migrateCmd.ExecuteContext(t.Context()))
	pool, err := database.Open(t.Context(), dbConfig)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = auth.NewUsers(clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))).Create(t.Context(), pool, &auth.NewUser{
		Username: adminUsername, DisplayName: adminUsername, Role: auth.RoleAdmin, PasswordHash: nil,
	})
	require.NoError(t, err)

	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		cmd := cli.NewCommand(opts)
		cmd.SetArgs(append([]string{"user", "link-oidc"}, args...))
		cmd.SetOut(&out)
		err := cmd.ExecuteContext(t.Context())

		return out.String(), err
	}

	// IdP を設定していなければ、紐付けられない。
	_, err = run(adminUsername, "sub-1")
	require.ErrorIs(t, err, auth.ErrOIDCDisabled)

	env["KOTATSU_AUTH_OIDC_ISSUER"] = issuer
	env["KOTATSU_AUTH_OIDC_CLIENT_ID"] = "kotatsu"
	out, err := run(adminUsername, "sub-1")
	require.NoError(t, err)
	assert.Equal(t, "linked kotatsu to sub-1 at "+issuer+"\n", out)
	var linked int
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM user_oidc_identities WHERE issuer = $1 AND subject = 'sub-1'", issuer).Scan(&linked))
	assert.Equal(t, 1, linked)

	_, err = run(adminUsername, "sub-1")
	require.ErrorIs(t, err, auth.ErrOIDCIdentityLinked)
}
