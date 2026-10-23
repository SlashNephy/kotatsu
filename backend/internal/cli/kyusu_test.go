package cli_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
)

// TestKyusuRegistersOnlyWhenItCanTranscode は、FFmpeg と tsreadex がないときは、kyusu を登録簿に載せないことを確かめる。
// 画面は、登録簿に kyusu がいるかで、サーバー側でのトランスコードを使えるかを判断する (設計書 7.1)。
//
// FFmpeg と tsreadex は PATH から探すので、PATH を差し替える。PATH はプロセスで共有するので、並行して動かさない。
func TestKyusuRegistersOnlyWhenItCanTranscode(t *testing.T) {
	tests := []struct {
		name string
		// tools は、PATH に置く実行ファイルの名前。
		tools     []string
		wantRoles []string
	}{
		{name: "FFmpeg と tsreadex がある", tools: []string{"ffmpeg", "tsreadex"}, wantRoles: []string{roleKyusu}},
		{name: "tsreadex がない", tools: []string{"ffmpeg"}, wantRoles: []string{}},
		{name: "FFmpeg がない", tools: []string{"tsreadex"}, wantRoles: []string{}},
		{name: "どちらもない", tools: []string{}, wantRoles: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			for _, tool := range tt.tools {
				// PATH から見つかればよいので、実行できるファイルへのリンクを置く。
				require.NoError(t, os.Symlink("/bin/sh", filepath.Join(bin, tool)))
			}
			t.Setenv("PATH", bin)

			env := testEnv()
			env["KOTATSU_DATABASE_URL"] = databasetest.New(t).URL.Value()
			opts := cli.DefaultOptions(slog.New(slog.DiscardHandler))
			opts.LookupEnv = func(key string) (string, bool) {
				v, ok := env[key]

				return v, ok
			}
			opts.Roles = filterRoles(opts.Roles, roleKyusu)
			registered := make(chan []string, 1)
			opts.RunRegistry = func(ctx context.Context, _ *slog.Logger, _ *cli.Deps, roles []string, _ string) error {
				registered <- roles
				<-ctx.Done()

				return nil
			}
			migrate := cli.NewCommand(opts)
			migrate.SetArgs([]string{subcommandMigrate})
			require.NoError(t, migrate.ExecuteContext(t.Context()))

			ctx, cancel := context.WithCancel(t.Context())
			cmd := cli.NewCommand(opts)
			cmd.SetArgs([]string{roleKyusu})
			done := make(chan error, 1)
			go func() {
				done <- cmd.ExecuteContext(ctx)
			}()
			t.Cleanup(func() {
				cancel()
				assert.NoError(t, <-done)
			})

			select {
			case roles := <-registered:
				assert.Equal(t, tt.wantRoles, roles)
			case <-time.After(10 * time.Second):
				t.Fatal("the registry was not started")
			}
		})
	}
}
