package procprio_test

import (
	"bytes"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
)

// niceOf は、/proc/<pid>/stat から、プロセスの nice 値を読む。
func niceOf(t *testing.T, pid int) int {
	t.Helper()

	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	require.NoError(t, err)
	// 2 番目の項目 (コマンド名) は括弧で囲まれ、空白を含み得るので、閉じ括弧の後から数える。nice は 19 番目の項目。
	fields := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:]))
	nice, err := strconv.Atoi(fields[16])
	require.NoError(t, err)

	return nice
}

func TestLowererStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		lowerer  *procprio.Lowerer
		wantNice func(parent int) int
	}{
		{name: "最低の優先度に下げる", lowerer: procprio.New(slog.New(slog.DiscardHandler)), wantNice: func(int) int { return procprio.LowestNice }},
		{name: "nil なら、優先度を変えない", lowerer: nil, wantNice: func(parent int) int { return parent }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.CommandContext(t.Context(), "sleep", "5")
			require.NoError(t, tt.lowerer.Start(cmd))
			t.Cleanup(func() {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			})
			assert.Equal(t, tt.wantNice(niceOf(t, os.Getpid())), niceOf(t, cmd.Process.Pid))
		})
	}
}

// TestLowerReportsExitedProcess は、優先度を下げる前にプロセスが終わっていたら、警告しない理由 (errProcessGone) を返すことを確かめる。
func TestLowerReportsExitedProcess(t *testing.T) {
	t.Parallel()

	assert.True(t, procprio.IsProcessGone(procprio.Lower(math.MaxInt32)))
}
