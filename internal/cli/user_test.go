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
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/password"
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
		Username: adminUsername, DisplayName: "こたつ", Role: auth.RoleAdmin, PasswordHash: nil,
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
