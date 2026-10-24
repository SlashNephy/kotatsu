package faketuner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGeneratedTS は、scripts/generate-test-ts.sh で作った TS に、録画やライブ視聴のテストで使う PID が揃っていることを確かめる。
func TestGeneratedTS(t *testing.T) {
	t.Parallel()

	ts, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ts", "test.ts"))
	require.NoError(t, err)
	require.Zero(t, len(ts)%188, "the TS must consist of whole 188-byte packets")

	pids := map[int]bool{}
	for offset := 0; offset < len(ts); offset += 188 {
		packet := ts[offset : offset+188]
		require.Equal(t, byte(0x47), packet[0], "packet at offset %d must start with the sync byte", offset)
		pids[int(packet[1]&0x1F)<<8|int(packet[2])] = true
	}

	tests := []struct {
		name string
		pid  int
	}{
		{name: "PAT", pid: 0x0000},
		{name: "SDT", pid: 0x0011},
		{name: "EIT", pid: 0x0012},
		{name: "TOT", pid: 0x0014},
		{name: "PMT", pid: 0x01F0},
		{name: "video", pid: 0x0111},
		{name: "audio", pid: 0x0112},
		{name: "null", pid: 0x1FFF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.True(t, pids[tt.pid], "PID 0x%04X must be present", tt.pid)
		})
	}
}
