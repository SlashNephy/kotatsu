package ts_test

import (
	"bytes"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// splitAt は、input を、rapid が選んだ位置で区切る。空の書き込みも混ぜる。
func splitAt(t *rapid.T, input []byte) [][]byte {
	cuts := rapid.SliceOfN(rapid.IntRange(0, len(input)), 0, 12).Draw(t, "cuts")
	slices.Sort(cuts)
	chunks := make([][]byte, 0, len(cuts)+1)
	prev := 0
	for _, cut := range cuts {
		chunks = append(chunks, input[prev:cut])
		prev = cut
	}

	return append(chunks, input[prev:])
}

// corrupt は、パケットの並び pkts に、rapid が選んだ壊れ方 (0x47 を含むゴミ、途中で切れたパケット) を混ぜた並びを返す。
func corrupt(t *rapid.T, pkts [][]byte) [][]byte {
	garbage := rapid.SliceOfN(rapid.SampledFrom([]byte{0x00, 0x47, 0xAA, 0xFF}), 1, 400)
	out := make([][]byte, 0, len(pkts))
	for i, p := range pkts {
		switch rapid.IntRange(0, 39).Draw(t, fmt.Sprintf("kind%d", i)) {
		case 0:
			out = append(out, garbage.Draw(t, fmt.Sprintf("garbage%d", i)))
		case 1:
			p = p[:rapid.IntRange(1, ts.PacketSize-1).Draw(t, fmt.Sprintf("truncate%d", i))]
		}
		out = append(out, p)
	}

	return out
}

// analyzeChunks は、chunks を順に書いた Analyzer の索引と統計を返す。
func analyzeChunks(t require.TestingT, chunks [][]byte) ([]ts.IndexEntry, *ts.Stats) {
	var buf bytes.Buffer
	writer, err := ts.NewIndexWriter(&buf)
	require.NoError(t, err)
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer})
	for _, chunk := range chunks {
		_, err := analyzer.Write(chunk)
		require.NoError(t, err)
	}
	entries, err := ts.ReadIndex(&buf)
	require.NoError(t, err)

	return entries, analyzer.Stats()
}

// TestAnalyzerWriteBoundaries は、壊れた TS でも、書き込みの切れ目によらず、1 回で書いたときと同じに読むことを確かめる (#1986)。
func TestAnalyzerWriteBoundaries(t *testing.T) {
	t.Parallel()

	pkts := packets(loadTestTS(t))[:400]
	rapid.Check(t, func(t *rapid.T) {
		pieces := corrupt(t, pkts)
		input := join(pieces)
		wantEntries, wantStats := analyzeChunks(t, [][]byte{input})
		// 区切り方は、rapid が選んだ位置、壊れたパケットやゴミごと (パケットの終わりで書き込みが終わる)、1 バイトずつの 3 つ。
		for _, chunks := range [][][]byte{splitAt(t, input), pieces, slices.Collect(slices.Chunk(input, 1))} {
			gotEntries, gotStats := analyzeChunks(t, chunks)
			assert.Equal(t, wantStats, gotStats)
			assert.Equal(t, wantEntries, gotEntries)
		}
	})
}

// TestAnalyzerResyncAtWriteEnd は、同期を失っている間に、書き込みの末尾の 0x47 を、次のパケットの頭を確かめずにパケットの先頭としないことを確かめる (#1986)。
func TestAnalyzerResyncAtWriteEnd(t *testing.T) {
	t.Parallel()

	pkts := packets(loadTestTS(t))
	head := join(slices.Concat(pkts[:50], [][]byte{{0x00, 0x47, 0x02}}))
	tail := join(pkts[50:])
	_, want := analyzeChunks(t, [][]byte{slices.Concat(head, tail)})
	_, got := analyzeChunks(t, [][]byte{head, tail})
	assert.Equal(t, want, got)
	assert.Equal(t, int64(1), got.SyncLosses)
	assert.Equal(t, int64(len(pkts)), got.Packets)
}

// TestPCRClockTruncatedPacket は、途中で切れたパケットの後に続く、正常なパケットの PCR を読むことを確かめる (#1986)。
func TestPCRClockTruncatedPacket(t *testing.T) {
	t.Parallel()

	before := pcrAt(videoPID, pcrSeconds(29))
	truncated := pcrAt(videoPID, pcrSeconds(30))[:120]
	next := pcrAt(videoPID, pcrSeconds(31))
	tests := []struct {
		name   string
		chunks [][]byte
	}{
		// 切れたパケットと正常なパケットが、1 回の書き込みに続く。
		{name: "in one write", chunks: [][]byte{slices.Concat(before, truncated, next)}},
		// 切れたパケットを持ち越している間に、次のパケットが 2 つに分かれて届く。
		{name: "the next packet split in two", chunks: [][]byte{slices.Concat(before, truncated), next[:50], next[50:]}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clock := ts.NewPCRClock(programNum, 30*time.Second)
			for _, chunk := range tt.chunks {
				clock.Write(chunk, at(100))
			}
			pair, ok := clock.Pair()
			require.True(t, ok)
			// 同じ時刻に受け取ったので、最も新しい PCR の組を採る。
			assert.InDelta(t, 31, pair.PCR, 1e-6)
		})
	}
}

// TestPCRClockWriteBoundaries は、整ったストリームなら、書き込みの切れ目によらず同じ組を選ぶことを確かめる (#1986)。
func TestPCRClockWriteBoundaries(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		seconds := rapid.SliceOfN(rapid.Float64Range(0, 1000), 1, 20).Draw(t, "pcrs")
		pkts := make([][]byte, 0, len(seconds))
		for _, s := range seconds {
			pkts = append(pkts, pcrAt(videoPID, pcrSeconds(s)))
		}
		input := join(pkts)
		whole := ts.NewPCRClock(programNum, 30*time.Second)
		whole.Write(input, at(100))
		split := ts.NewPCRClock(programNum, 30*time.Second)
		for _, chunk := range splitAt(t, input) {
			split.Write(chunk, at(100))
		}
		want, wantOK := whole.Pair()
		got, gotOK := split.Pair()
		assert.Equal(t, wantOK, gotOK)
		assert.Equal(t, want, got)
	})
}

// TestScrambleDetectorResyncAtWriteEnd は、同期を失っている間に、書き込みの末尾の 0x47 を、次のパケットの頭を確かめずにパケットの先頭としないことを確かめる (#1986)。
func TestScrambleDetectorResyncAtWriteEnd(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_763_000_000, 0)
	d := ts.NewScrambleDetector(ts.ScrambleDetectorOptions{
		WindowPackets: 2, Windows: 1, Hold: time.Second, MinPartialPackets: 1,
		Now: func() time.Time {
			now = now.Add(time.Hour)

			return now
		},
		OnScrambled: nil,
	})
	// 0x47 で始まるが、188 バイト先が 0x47 でないゴミで、同期を失わせる。
	junk := bytes.Repeat([]byte{0xAA}, 200)
	junk[0] = 0x47
	// 末尾の 0x47 から読むと、スクランブルされていない映像のパケットに見える。
	fake := []byte{0x47, videoPID >> 8, videoPID & 0xFF, 0x10}
	first := join(slices.Concat(scrambleHeader(), [][]byte{esPacket(videoPID, 0, true, true), junk, fake}))
	second := join([][]byte{esPacket(videoPID, 1, true, true), esPacket(videoPID, 2, true, true)})

	_, err := d.Write(first)
	require.NoError(t, err)
	_, err = d.Write(second)
	require.ErrorIs(t, err, ts.ErrScrambled)
	assert.Equal(t, ts.ScrambleVerdictScrambled, d.Verdict())
}

// TestScrambleDetectorWriteBoundaries は、整ったストリームなら、書き込みの切れ目によらず同じ判定になることを確かめる (#1986)。
func TestScrambleDetectorWriteBoundaries(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		flags := rapid.SliceOfN(rapid.Bool(), 1, 300).Draw(t, "scrambled")
		input := join(scrambleStream(len(flags), func(i int) bool { return flags[i] }))
		newDetector := func() *ts.ScrambleDetector {
			// 時計を止め、抑止時間を越えないようにして、窓の割合だけで決まるようにする。
			return ts.NewScrambleDetector(ts.ScrambleDetectorOptions{
				WindowPackets: 50, Windows: 0, Hold: 0, MinPartialPackets: 10,
				Now:         func() time.Time { return time.Unix(1_763_000_000, 0) },
				OnScrambled: nil,
			})
		}
		whole := newDetector()
		_, err := whole.Write(input)
		require.NoError(t, err)
		split := newDetector()
		for _, chunk := range splitAt(t, input) {
			_, err := split.Write(chunk)
			require.NoError(t, err)
		}
		assert.Equal(t, whole.Finish(), split.Finish())
	})
}

// TestAnalyzerIndexH264OverlappingStartCode は、NAL ヘッダーの 0x00 が次の start code の先頭のゼロを兼ねても、
// その start code を見落とさないことを確かめる (#1986)。
func TestAnalyzerIndexH264OverlappingStartCode(t *testing.T) {
	t.Parallel()

	pts3000 := [5]byte{0x21, 0x00, 0x01, 0x17, 0x71}
	cc := map[uint16]*byte{0: new(byte), pmtPID: new(byte)}
	pkts := append(psiPackets(0, cc[0], patSection(0, map[uint16]uint16{programNum: pmtPID})),
		psiPackets(pmtPID, cc[pmtPID], pmtSection(0, esEntry{streamType: 0x1B, pid: videoPID}))...)
	// nal_unit_type が 0 の NAL ヘッダー (0x00) の直後に、IDR のスライス (0x65) の start code が続く。
	video := videoPacket(new(byte), true, false, pesHeader(pts3000), []byte{0x00, 0x00, 0x01, 0x00, 0x00, 0x01, 0x65})
	entries, _ := analyzeWithIndex(t, join(append(pkts, video)))
	assert.Equal(t, []int64{3000}, ptsOf(entries))
}
