package ts_test

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/ts"
)

// segmentBuffer は、Splitter が書いたセグメント。閉じたかどうかも控える。
type segmentBuffer struct {
	bytes.Buffer

	cut    int
	closed bool
}

func (b *segmentBuffer) Close() error {
	b.closed = true

	return nil
}

var _ io.WriteCloser = new(segmentBuffer)

// TestSplitterCutsAtKeyframes は、Splitter が、区切りの PTS に届いた I フレームの PES の先頭でセグメントを切り、
// 各セグメントの先頭に PAT と PMT を写すことを確かめる (#806)。
// test-hevc-main10.ts の IRAP の PTS は、129003 から 45045 (0.5 秒) ごとに並ぶ。
func TestSplitterCutsAtKeyframes(t *testing.T) {
	t.Parallel()

	input := loadTestTSFile(t, "test-hevc-main10.ts")
	keyframes, _ := analyzeWithIndex(t, input)
	require.Equal(t, []int64{129003, 174048, 219093, 264138, 309183, 354228, 399273, 444318}, ptsOf(keyframes))

	tests := []struct {
		name string
		cuts []int64
		// interval は、cuts を使い切った後に切る間隔。
		interval time.Duration
		// chunk は、Splitter に一度に書く大きさ。パケットの境界をまたいで書いても切れることを確かめる。
		chunk int
		// wantCuts は、セグメントを始めた区切りの添え字。
		wantCuts []int
		// wantPTS は、各セグメントの最初の I フレームの PTS。
		wantPTS []int64
	}{
		{
			name:     "every other keyframe",
			cuts:     []int64{129003, 219093, 309183, 399273},
			interval: time.Hour,
			chunk:    ts.PacketSize,
			wantCuts: []int{0, 1, 2, 3},
			wantPTS:  []int64{129003, 219093, 309183, 399273},
		},
		{
			// 区切りが I フレームの間にあれば、その後の最初の I フレームで切る。最初の区切りより前は捨てる。
			name:     "cuts between keyframes",
			cuts:     []int64{150000, 250000},
			interval: time.Hour,
			chunk:    1000,
			wantCuts: []int{0, 1},
			wantPTS:  []int64{174048, 264138},
		},
		{
			// 1 つの I フレームで 2 つの区切りを越えたら、越えた区切りのセグメントは作らない。
			name:     "skipped cut",
			cuts:     []int64{129003, 150000, 160000, 309183},
			interval: time.Hour,
			chunk:    4096,
			wantCuts: []int{0, 2, 3},
			wantPTS:  []int64{129003, 174048, 309183},
		},
		{
			// 区切りを使い切った後は、セグメントを始めた I フレームから interval 以上進んだ最初の I フレームで切る。
			name:     "interval after cuts",
			cuts:     []int64{174048},
			interval: 900 * time.Millisecond,
			chunk:    ts.PacketSize,
			wantCuts: []int{0, 1, 2, 3},
			wantPTS:  []int64{174048, 264138, 354228, 444318},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			segments := []*segmentBuffer{}
			splitter, err := ts.NewSplitter(tt.cuts, tt.interval, func(i int) (io.WriteCloser, error) {
				segment := &segmentBuffer{Buffer: bytes.Buffer{}, cut: i, closed: false}
				segments = append(segments, segment)

				return segment, nil
			})
			require.NoError(t, err)
			for rest := input; len(rest) > 0; {
				n := min(tt.chunk, len(rest))
				_, err := splitter.Write(rest[:n])
				require.NoError(t, err)
				rest = rest[n:]
			}
			require.NoError(t, splitter.Close())

			cuts, firstPTS := make([]int, 0, len(segments)), make([]int64, 0, len(segments))
			// written は、セグメントの先頭に写した PAT と PMT を除いて、書いた大きさ。
			written := 0
			for _, segment := range segments {
				assert.True(t, segment.closed)
				cuts = append(cuts, segment.cut)
				data := segment.Bytes()
				entries, stats := analyzeWithIndex(t, data)
				require.NotEmpty(t, entries)
				require.NotEmpty(t, stats.Programs, "each segment starts with PAT and PMT")
				firstPTS = append(firstPTS, entries[0].PTS)
				assert.Equal(t, int64(2*ts.PacketSize), entries[0].Offset, "the keyframe follows PAT and PMT")
				written += len(data) - 2*ts.PacketSize
			}
			assert.Equal(t, tt.wantCuts, cuts)
			assert.Equal(t, tt.wantPTS, firstPTS)
			// 最初のセグメントの I フレームから後は、1 パケットも欠けない。
			first := keyframes[0]
			for _, k := range keyframes {
				if k.PTS == tt.wantPTS[0] {
					first = k
				}
			}
			assert.Equal(t, len(input)-int(first.Offset), written)
		})
	}
}
