package ts_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// TestAnalyzerTOT は、TOT を読み、放送の現在の時刻を OnTOT に渡すことを確かめる (#1309)。
// OnTOT を渡さなければ、TOT を読まない。
func TestAnalyzerTOT(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ts", "test.ts"))
	require.NoError(t, err)
	received := []time.Time{}
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: nil, CutoffPTS: nil, OnTOT: func(at time.Time) { received = append(received, at) }})
	_, err = analyzer.Write(data)
	require.NoError(t, err)
	require.NotEmpty(t, received)
	// テストの TS の TOT は、2026-01-01 00:00:00 (日本標準時) を送る (scripts/testts)。
	assert.True(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("JST", 9*60*60)).Equal(received[0]), received[0])

	_, err = ts.NewAnalyzer(nil).Write(data)
	require.NoError(t, err)
}

// TestParseTOT は、JST_time の MJD と BCD を読み、壊れた値は読まないことを確かめる (#1309)。
func TestParseTOT(t *testing.T) {
	t.Parallel()

	jst := time.FixedZone("JST", 9*60*60)
	tests := []struct {
		name    string
		section []byte
		want    time.Time
		ok      bool
	}{
		// ARIB STD-B10 の例: MJD 0xC079 (1993-10-13)、12:45:00。
		{name: "TOT", section: []byte{0x73, 0x70, 0x0a, 0xc0, 0x79, 0x12, 0x45, 0x00}, want: time.Date(1993, 10, 13, 12, 45, 0, 0, jst), ok: true},
		{name: "TDT", section: []byte{0x70, 0x70, 0x05, 0xc0, 0x79, 0x23, 0x59, 0x59}, want: time.Date(1993, 10, 13, 23, 59, 59, 0, jst), ok: true},
		{name: "not BCD", section: []byte{0x73, 0x70, 0x0a, 0xc0, 0x79, 0x1a, 0x45, 0x00}, ok: false},
		{name: "other table", section: []byte{0x42, 0x70, 0x0a, 0xc0, 0x79, 0x12, 0x45, 0x00}, ok: false},
		{name: "short", section: []byte{0x73, 0x70}, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := ts.ParseTOTForTest(tt.section)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.True(t, tt.want.Equal(got), got)
			}
		})
	}
}
