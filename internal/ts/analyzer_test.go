package ts_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/ts"
)

const (
	// videoPID と audioPID は、testdata/ts/test.ts の映像と音声の PID。scripts/generate-test-ts.sh で作るときに指定している。
	videoPID = 0x0111
	audioPID = 0x0112
)

// loadTestTS は、scripts/generate-test-ts.sh で作ったテスト用の TS (test.ts) を読む。
func loadTestTS(t *testing.T) []byte {
	t.Helper()

	return loadTestTSFile(t, "test.ts")
}

// loadTestTSFile は、scripts/generate-test-ts.sh で作ったテスト用の TS のうち、name のファイルを読む。
func loadTestTSFile(t *testing.T, name string) []byte {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ts", name))
	require.NoError(t, err)
	require.Zero(t, len(b)%ts.PacketSize)

	return b
}

// packets は、TS をパケットに分ける。各パケットは複製なので、書き換えても元の TS は変わらない。
func packets(b []byte) [][]byte {
	out := [][]byte{}
	for offset := 0; offset < len(b); offset += ts.PacketSize {
		out = append(out, slices.Clone(b[offset:offset+ts.PacketSize]))
	}

	return out
}

func pidOf(p []byte) uint16 {
	return uint16(p[1]&0x1F)<<8 | uint16(p[2])
}

// nthOfPID は、pid の n 番目 (0 始まり) のパケットの、全体での位置を返す。
func nthOfPID(t *testing.T, pkts [][]byte, pid uint16, n int) int {
	t.Helper()

	count := 0
	for i, p := range pkts {
		if pidOf(p) == pid {
			if count == n {
				return i
			}
			count++
		}
	}
	require.FailNow(t, "packet not found", "PID 0x%04X has fewer than %d packets", pid, n+1)

	return -1
}

func join(pkts [][]byte) []byte {
	return slices.Concat(pkts...)
}

func statsOf(stats *ts.Stats, pid uint16) ts.PIDStats {
	for _, s := range stats.PIDs {
		if s.PID == pid {
			return *s
		}
	}

	return ts.PIDStats{PID: pid}
}

func TestAnalyzerMutations(t *testing.T) {
	t.Parallel()

	original := loadTestTS(t)

	tests := []struct {
		name string
		// mutate は、パケットの列を書き換える。
		mutate        func(t *testing.T, pkts [][]byte) [][]byte
		wantVideo     ts.PIDStats
		wantAudioDrop int64
	}{
		{
			name:      "unmodified",
			mutate:    func(_ *testing.T, pkts [][]byte) [][]byte { return pkts },
			wantVideo: ts.PIDStats{Drops: 0, Errors: 0, Scrambled: 0},
		},
		{
			name: "one video packet dropped",
			mutate: func(t *testing.T, pkts [][]byte) [][]byte {
				t.Helper()

				i := nthOfPID(t, pkts, videoPID, 100)

				return slices.Delete(pkts, i, i+1)
			},
			wantVideo: ts.PIDStats{Drops: 1},
		},
		{
			name: "three separate video packets dropped",
			mutate: func(t *testing.T, pkts [][]byte) [][]byte {
				t.Helper()

				for _, n := range []int{300, 200, 100} {
					i := nthOfPID(t, pkts, videoPID, n)
					pkts = slices.Delete(pkts, i, i+1)
				}

				return pkts
			},
			wantVideo: ts.PIDStats{Drops: 3},
		},
		{
			name: "five consecutive video packets dropped count as one drop",
			mutate: func(t *testing.T, pkts [][]byte) [][]byte {
				t.Helper()

				for range 5 {
					i := nthOfPID(t, pkts, videoPID, 100)
					pkts = slices.Delete(pkts, i, i+1)
				}

				return pkts
			},
			wantVideo: ts.PIDStats{Drops: 1},
		},
		{
			name: "a packet dropped from both video and audio",
			mutate: func(t *testing.T, pkts [][]byte) [][]byte {
				t.Helper()

				i := nthOfPID(t, pkts, videoPID, 100)
				pkts = slices.Delete(pkts, i, i+1)
				j := nthOfPID(t, pkts, audioPID, 10)

				return slices.Delete(pkts, j, j+1)
			},
			wantVideo:     ts.PIDStats{Drops: 1},
			wantAudioDrop: 1,
		},
		{
			name: "a duplicated packet is not a drop",
			mutate: func(t *testing.T, pkts [][]byte) [][]byte {
				t.Helper()

				i := nthOfPID(t, pkts, videoPID, 100)

				return slices.Insert(pkts, i+1, slices.Clone(pkts[i]))
			},
			wantVideo: ts.PIDStats{Drops: 0},
		},
		{
			name: "a packet sent three times is a drop",
			mutate: func(t *testing.T, pkts [][]byte) [][]byte {
				t.Helper()

				i := nthOfPID(t, pkts, videoPID, 100)

				return slices.Insert(pkts, i+1, slices.Clone(pkts[i]), slices.Clone(pkts[i]))
			},
			wantVideo: ts.PIDStats{Drops: 1},
		},
		{
			name: "transport error",
			mutate: func(t *testing.T, pkts [][]byte) [][]byte {
				t.Helper()

				pkts[nthOfPID(t, pkts, videoPID, 100)][1] |= 0x80

				return pkts
			},
			wantVideo: ts.PIDStats{Errors: 1, Drops: 0},
		},
		{
			name: "scrambled",
			mutate: func(t *testing.T, pkts [][]byte) [][]byte {
				t.Helper()

				for _, n := range []int{100, 101} {
					pkts[nthOfPID(t, pkts, videoPID, n)][3] |= 0x80
				}

				return pkts
			},
			wantVideo: ts.PIDStats{Scrambled: 2, Drops: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := join(tt.mutate(t, packets(original)))
			analyzer := ts.NewAnalyzer(nil)
			n, err := analyzer.Write(input)
			require.NoError(t, err)
			assert.Equal(t, len(input), n)

			stats := analyzer.Stats()
			assert.Equal(t, int64(len(input)), stats.Bytes)
			assert.Equal(t, int64(len(input)/ts.PacketSize), stats.Packets)
			assert.Zero(t, stats.SyncLosses)

			video := statsOf(stats, videoPID)
			assert.Equal(t, tt.wantVideo.Drops, video.Drops, "video drops")
			assert.Equal(t, tt.wantVideo.Errors, video.Errors, "video errors")
			assert.Equal(t, tt.wantVideo.Scrambled, video.Scrambled, "video scrambled")
			assert.Equal(t, tt.wantAudioDrop, statsOf(stats, audioPID).Drops, "audio drops")
			for _, s := range stats.PIDs {
				assert.NotEqual(t, uint16(ts.NullPID), s.PID, "null packets are not counted per PID")
				if s.PID != videoPID && s.PID != audioPID {
					assert.Zero(t, s.Drops, "PID 0x%04X", s.PID)
				}
			}
		})
	}
}

func TestAnalyzerChunkedWrites(t *testing.T) {
	t.Parallel()

	input := loadTestTS(t)
	whole := ts.NewAnalyzer(nil)
	_, err := whole.Write(input)
	require.NoError(t, err)
	want := whole.Stats()
	require.Positive(t, statsOf(want, videoPID).Packets)

	// パケットの大きさで割り切れない単位で、パケットを途中で区切って書く。
	for _, size := range []int{1, 100, 187, 189, 4096, 65_536} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			t.Parallel()

			analyzer := ts.NewAnalyzer(nil)
			for chunk := range slices.Chunk(input, size) {
				_, err := analyzer.Write(chunk)
				require.NoError(t, err)
			}
			assert.Equal(t, want, analyzer.Stats())
		})
	}
}

func TestAnalyzerResync(t *testing.T) {
	t.Parallel()

	pkts := packets(loadTestTS(t))

	tests := []struct {
		name    string
		garbage []byte
	}{
		{name: "garbage without a sync byte", garbage: []byte{0x00, 0x01, 0x02, 0x03, 0x04}},
		// ゴミの中の 0x47 を、パケットの先頭と取り違えない。
		{name: "garbage containing a sync byte", garbage: []byte{0x00, 0x47, 0x02, 0x47, 0x04}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := join(slices.Concat(pkts[:50], [][]byte{tt.garbage}, pkts[50:]))
			analyzer := ts.NewAnalyzer(nil)
			_, err := analyzer.Write(input)
			require.NoError(t, err)

			stats := analyzer.Stats()
			assert.Equal(t, int64(1), stats.SyncLosses)
			assert.Equal(t, int64(len(pkts)), stats.Packets)
			assert.Equal(t, int64(len(input)), stats.Bytes)
			for _, s := range stats.PIDs {
				assert.Zero(t, s.Drops, "PID 0x%04X", s.PID)
			}
		})
	}
}

// syntheticPID は、合成のパケットの PID。
const syntheticPID = 0x0100

// synthetic は、PID が syntheticPID で、ペイロードを持つ合成のパケットを作る。
func synthetic(cc byte, discontinuity bool) []byte {
	p := make([]byte, ts.PacketSize)
	p[0] = 0x47
	p[1] = syntheticPID >> 8
	p[2] = syntheticPID & 0xFF
	// アダプテーションフィールドとペイロードの両方を持つ。
	p[3] = 0x30 | cc&0x0F
	p[4] = 1
	if discontinuity {
		p[5] = 0x80
	}

	return p
}

func TestAnalyzerContinuityRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sequence  [][]byte
		wantDrops int64
	}{
		{
			name:      "wraps from 15 to 0",
			sequence:  [][]byte{synthetic(14, false), synthetic(15, false), synthetic(0, false)},
			wantDrops: 0,
		},
		{
			name:      "discontinuity indicator",
			sequence:  [][]byte{synthetic(1, false), synthetic(9, true), synthetic(10, false)},
			wantDrops: 0,
		},
		{
			name:      "first packet of a PID is never a drop",
			sequence:  [][]byte{synthetic(7, false)},
			wantDrops: 0,
		},
		{
			name: "packets without payload do not advance the counter",
			sequence: func() [][]byte {
				adaptationOnly := synthetic(2, false)
				adaptationOnly[3] = 0x20 | 2

				return [][]byte{synthetic(2, false), adaptationOnly, synthetic(3, false)}
			}(),
			wantDrops: 0,
		},
		{
			name: "discontinuity indicator on a packet without payload",
			sequence: func() [][]byte {
				adaptationOnly := synthetic(2, true)
				adaptationOnly[3] = 0x20 | 2

				return [][]byte{synthetic(2, false), adaptationOnly, synthetic(9, false)}
			}(),
			wantDrops: 0,
		},
		{
			name: "a packet with a broken adaptation field restarts the check",
			sequence: func() [][]byte {
				broken := synthetic(5, false)
				broken[4] = 200

				return [][]byte{synthetic(2, false), broken, synthetic(9, false)}
			}(),
			wantDrops: 0,
		},
		{
			name:      "skipped value",
			sequence:  [][]byte{synthetic(1, false), synthetic(3, false)},
			wantDrops: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := ts.NewAnalyzer(nil)
			_, err := analyzer.Write(join(tt.sequence))
			require.NoError(t, err)
			assert.Equal(t, tt.wantDrops, statsOf(analyzer.Stats(), syntheticPID).Drops)
		})
	}
}
