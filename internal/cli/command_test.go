package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
)

func TestHelpListsRolesWithAlias(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	cmd := cli.NewCommand(slog.New(slog.DiscardHandler), cli.DefaultRoles())
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	require.NoError(t, cmd.Execute())

	tests := []string{
		"tenban (server)",
		"neko (scheduler)",
		"mikan (recorder)",
		"nekonote (worker)",
		"kyusu (transcoder)",
		"all ",
	}
	for _, want := range tests {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, out.String(), "\n  "+want)
		})
	}
}

func TestSubcommandAcceptsCodenameAndAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		codename string
		alias    string
	}{
		{codename: "tenban", alias: "server"},
		{codename: "neko", alias: "scheduler"},
		{codename: "mikan", alias: "recorder"},
		{codename: "nekonote", alias: "worker"},
		{codename: "kyusu", alias: "transcoder"},
	}
	for _, tt := range tests {
		for _, arg := range []string{tt.codename, tt.alias} {
			t.Run(arg, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, []string{tt.codename}, startedRoles(t, arg))
			})
		}
	}
}

func TestAllStartsEveryRole(t *testing.T) {
	t.Parallel()

	roles := cli.DefaultRoles()
	want := make([]string, 0, len(roles))
	for _, role := range roles {
		want = append(want, role.Name)
	}

	assert.ElementsMatch(t, want, startedRoles(t, "all"))
}

// startedRoles は arg をサブコマンドとして実行し、起動したロールのコードネームを返す。
func startedRoles(t *testing.T, arg string) []string {
	t.Helper()

	var (
		mu      sync.Mutex
		started = []string{}
	)
	roles := cli.DefaultRoles()
	for _, role := range roles {
		role.Run = func(_ context.Context, _ *slog.Logger) error {
			mu.Lock()
			defer mu.Unlock()
			started = append(started, role.Name)

			return nil
		}
	}

	cmd := cli.NewCommand(slog.New(slog.DiscardHandler), roles)
	cmd.SetArgs([]string{arg})
	require.NoError(t, cmd.ExecuteContext(t.Context()))

	return started
}

func TestRoleFailureStopsOtherRoles(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	roles := cli.DefaultRoles()
	roles[0].Run = func(_ context.Context, _ *slog.Logger) error {
		return errBoom
	}

	cmd := cli.NewCommand(slog.New(slog.DiscardHandler), roles)
	cmd.SetArgs([]string{"all"})

	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(t.Context())
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, errBoom)
	case <-time.After(5 * time.Second):
		t.Fatal("other roles did not stop after a role failed")
	}
}

func TestRoleLogsDisplayNameUntilCanceled(t *testing.T) {
	t.Parallel()

	var (
		mu  sync.Mutex
		buf bytes.Buffer
	)
	logger := slog.New(slog.NewJSONHandler(&lockedWriter{mu: &mu, w: &buf}, nil))

	ctx, cancel := context.WithCancel(t.Context())
	cmd := cli.NewCommand(logger, cli.DefaultRoles())
	cmd.SetArgs([]string{"scheduler"})

	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(ctx)
	}()

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()

		return strings.Contains(buf.String(), `"msg":"role started"`)
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("role did not stop after the context was canceled")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, buf.String(), `"role":"neko (scheduler)"`)
	assert.Contains(t, buf.String(), `"msg":"role stopped"`)
}

func TestUnknownSubcommandFails(t *testing.T) {
	t.Parallel()

	cmd := cli.NewCommand(slog.New(slog.DiscardHandler), cli.DefaultRoles())
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"kago"})
	require.Error(t, cmd.Execute())
}

var _ io.Writer = new(lockedWriter)

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.w.Write(p)
}
