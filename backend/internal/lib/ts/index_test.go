package ts_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// wantIFramePTS は、testdata/ts/test.ts の I フレームの PTS。
// ffprobe -select_streams v:0 -show_entries frame=pict_type,pts で数えた値で、GOP は 15 フレーム (45045 刻み) ごと。
func wantIFramePTS() []int64 {
	return []int64{129003, 174048, 219093, 264138, 309183, 354228, 399273, 444318, 489363, 534408, 579453, 624498}
}

// analyzeWithIndex は、input を解析し、索引の中身と統計を返す。
func analyzeWithIndex(t *testing.T, chunks ...[]byte) ([]ts.IndexEntry, *ts.Stats) {
	t.Helper()

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

func ptsOf(entries []ts.IndexEntry) []int64 {
	out := make([]int64, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.PTS)
	}

	return out
}

func TestAnalyzerIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file string
		// wantPTS は、ffprobe -select_streams v:0 -show_entries frame=pict_type,pts で数えた I フレームの PTS。
		wantPTS []int64
	}{
		{
			name:    "MPEG-2",
			file:    "test.ts",
			wantPTS: wantIFramePTS(),
		},
		{
			// IDR と、IDR でない I スライスのピクチャ (開いた GOP) を含む。
			name:    "H.264",
			file:    "test-h264.ts",
			wantPTS: []int64{132006, 222096, 312186, 402276},
		},
		{
			// IDR と CRA を含む。IRAP でない TRAIL_N の NAL ユニットのヘッダーは 0x00 で始まり、MPEG-2 の picture_start_code と同じ値になる。
			name:    "HEVC",
			file:    "test-hevc.ts",
			wantPTS: []int64{132006, 222096, 312186, 402276},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := loadTestTSFile(t, tt.file)
			entries, stats := analyzeWithIndex(t, input)
			assert.Equal(t, tt.wantPTS, ptsOf(entries))
			assert.Equal(t, int64(len(entries)), stats.IFrames)
			require.NoError(t, stats.IndexError)

			for _, e := range entries {
				require.Zero(t, e.Offset%ts.PacketSize, "the offset must point to a packet boundary")
				p := input[e.Offset : e.Offset+ts.PacketSize]
				assert.Equal(t, uint16(videoPID), pidOf(p), "the offset must point to a video packet")
				assert.NotZero(t, p[1]&0x40, "the offset must point to the start of a PES")
			}

			// 書き込みの区切り方によらず、同じ索引ができる。
			chunked, _ := analyzeWithIndex(t, slices.Collect(slices.Chunk(input, 1000))...)
			assert.Equal(t, entries, chunked)
			// start code が 1 バイトずつ分かれても、同じ索引ができる。
			bytewise, _ := analyzeWithIndex(t, slices.Collect(slices.Chunk(input, 1))...)
			assert.Equal(t, entries, bytewise)
		})
	}
}

func TestAnalyzerIndexGrowsWhileWriting(t *testing.T) {
	t.Parallel()

	input := loadTestTS(t)
	half := len(input) / 2 / ts.PacketSize * ts.PacketSize

	var buf bytes.Buffer
	writer, err := ts.NewIndexWriter(&buf)
	require.NoError(t, err)
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer})

	_, err = analyzer.Write(input[:half])
	require.NoError(t, err)
	partial, err := ts.ReadIndex(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, wantIFramePTS()[:len(partial)], ptsOf(partial))
	assert.NotEmpty(t, partial)
	assert.Less(t, len(partial), len(wantIFramePTS()))

	_, err = analyzer.Write(input[half:])
	require.NoError(t, err)
	full, err := ts.ReadIndex(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, wantIFramePTS(), ptsOf(full))
}

func TestAnalyzerIndexWithBrokenVideo(t *testing.T) {
	t.Parallel()

	original := loadTestTS(t)
	entries, _ := analyzeWithIndex(t, original)
	// 3 番目の I フレームを含む PES の先頭パケット。
	target := int(entries[2].Offset / ts.PacketSize)

	tests := []struct {
		name    string
		mutate  func(pkts [][]byte) [][]byte
		wantPTS []int64
	}{
		{
			name:    "the packet starting an I frame PES is dropped",
			mutate:  func(pkts [][]byte) [][]byte { return slices.Delete(pkts, target, target+1) },
			wantPTS: slices.Delete(wantIFramePTS(), 2, 3),
		},
		{
			name: "the packet starting an I frame PES is scrambled",
			mutate: func(pkts [][]byte) [][]byte {
				pkts[target][3] |= 0x80

				return pkts
			},
			wantPTS: slices.Delete(wantIFramePTS(), 2, 3),
		},
		{
			name: "the packet starting an I frame PES has a transport error",
			mutate: func(pkts [][]byte) [][]byte {
				pkts[target][1] |= 0x80

				return pkts
			},
			wantPTS: slices.Delete(wantIFramePTS(), 2, 3),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, _ := analyzeWithIndex(t, join(tt.mutate(packets(original))))
			assert.Equal(t, tt.wantPTS, ptsOf(got))
		})
	}
}

// failingWriter は、ヘッダーの後の書き込みで失敗する。
type failingWriter struct {
	writes int
}

var errDiskFull = errors.New("disk full")

func (w *failingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > 1 {
		return 0, errDiskFull
	}

	return len(p), nil
}

func TestAnalyzerIndexWriteError(t *testing.T) {
	t.Parallel()

	writer, err := ts.NewIndexWriter(new(failingWriter))
	require.NoError(t, err)
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer})

	// 索引を書けなくても、録画の書き込みを止めないよう、Write はエラーを返さない。
	input := loadTestTS(t)
	n, err := analyzer.Write(input)
	require.NoError(t, err)
	assert.Equal(t, len(input), n)

	stats := analyzer.Stats()
	require.ErrorIs(t, stats.IndexError, errDiskFull)
	assert.Equal(t, int64(len(wantIFramePTS())), stats.IFrames)
}

func TestReadIndex(t *testing.T) {
	t.Parallel()

	written := []ts.IndexEntry{{Offset: 376, PTS: 129003}, {Offset: 176908, PTS: 1<<33 - 1}}
	var buf bytes.Buffer
	writer, err := ts.NewIndexWriter(&buf)
	require.NoError(t, err)
	for _, e := range written {
		require.NoError(t, writer.Append(e))
	}
	file := buf.Bytes()
	headerOnly := file[:8]

	tests := []struct {
		name    string
		input   []byte
		want    []ts.IndexEntry
		wantErr error
	}{
		{name: "round trip", input: file, want: written},
		{name: "header only", input: headerOnly, want: []ts.IndexEntry{}},
		{name: "the last record is cut while appending", input: file[:len(file)-5], want: written[:1]},
		{name: "empty file", input: []byte{}, wantErr: ts.ErrInvalidIndex},
		{name: "wrong magic", input: append([]byte("XXXX"), file[4:]...), wantErr: ts.ErrInvalidIndex},
		{name: "unsupported version", input: append(append([]byte("KTIX"), 0x02, 0x00), file[6:]...), wantErr: ts.ErrInvalidIndex},
		{name: "unexpected record size", input: append(append([]byte("KTIX"), 0x01, 0x00, 0x20, 0x00), file[8:]...), wantErr: ts.ErrInvalidIndex},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ts.ReadIndex(bytes.NewReader(tt.input))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPTSDiff(t *testing.T) {
	t.Parallel()

	const wrap = int64(1) << 33

	tests := []struct {
		name     string
		from, to int64
		want     int64
	}{
		{name: "forward", from: 129003, to: 174048, want: 45045},
		{name: "backward", from: 174048, to: 129003, want: -45045},
		{name: "forward across the wrap", from: wrap - 45045, to: 0, want: 45045},
		{name: "backward across the wrap", from: 100, to: wrap - 100, want: -200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, ts.PTSDiff(tt.from, tt.to))
		})
	}
}

// pesHeader は、PTS を持つ映像の PES のヘッダー。pts は PTS の 5 バイトの符号化。
func pesHeader(pts [5]byte) []byte {
	return append([]byte{0x00, 0x00, 0x01, 0xE0, 0x00, 0x00, 0x80, 0x80, 0x05}, pts[:]...)
}

// pictureHeader は、picture_coding_type が pictureType の MPEG-2 のピクチャヘッダー。
func pictureHeader(pictureType byte) []byte {
	return []byte{0x00, 0x00, 0x01, 0x00, 0x00, pictureType << 3}
}

// videoPacket は、映像の PID の合成のパケットを作る。ペイロードの残りは 0xFF で埋める。
func videoPacket(cc *byte, payloadUnitStart, scrambled bool, payload ...[]byte) []byte {
	p := make([]byte, ts.PacketSize)
	for i := range p {
		p[i] = 0xFF
	}
	p[0] = 0x47
	p[1] = videoPID >> 8
	if payloadUnitStart {
		p[1] |= 0x40
	}
	p[2] = videoPID & 0xFF
	p[3] = 0x10 | *cc&0x0F
	if scrambled {
		p[3] |= 0x80
	}
	*cc++
	copy(p[4:], slices.Concat(payload...))

	return p
}

func TestAnalyzerIndexIgnoresScrambledPESStart(t *testing.T) {
	t.Parallel()

	var (
		pts1000 = [5]byte{0x21, 0x00, 0x01, 0x07, 0xD1}
		pts3000 = [5]byte{0x21, 0x00, 0x01, 0x17, 0x71}
	)
	const (
		pictureI = 1
		pictureP = 2
	)

	tests := []struct {
		name    string
		video   func(cc *byte) [][]byte
		wantPTS []int64
	}{
		{
			name: "I picture after a P picture",
			video: func(cc *byte) [][]byte {
				return [][]byte{
					videoPacket(cc, true, false, pesHeader(pts1000), pictureHeader(pictureP)),
					videoPacket(cc, true, false, pesHeader(pts3000), pictureHeader(pictureI)),
				}
			},
			wantPTS: []int64{3000},
		},
		{
			// PES の始まりが読めなければ、その PES の位置と PTS は分からない。
			// 続きのパケットで I ピクチャを見つけても、前の PES の位置と PTS で索引に書いてはいけない。
			name: "the start of the I picture PES is scrambled",
			video: func(cc *byte) [][]byte {
				return [][]byte{
					videoPacket(cc, true, false, pesHeader(pts1000), pictureHeader(pictureP)),
					videoPacket(cc, true, true, pesHeader(pts3000)),
					videoPacket(cc, false, false, pictureHeader(pictureI)),
				}
			},
			wantPTS: []int64{},
		},
		{
			name: "the I picture header straddles two packets",
			video: func(cc *byte) [][]byte {
				header := pictureHeader(pictureI)
				first := videoPacket(cc, true, false, pesHeader(pts3000))
				// 先頭の 3 バイト (00 00 01) を 1 つ目のパケットの末尾に置き、残りを 2 つ目のパケットに置く。
				copy(first[ts.PacketSize-3:], header[:3])

				return [][]byte{first, videoPacket(cc, false, false, header[3:])}
			},
			wantPTS: []int64{3000},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cc := map[uint16]*byte{0: new(byte), pmtPID: new(byte)}
			pkts := append(psiPackets(0, cc[0], patSection(0, map[uint16]uint16{programNum: pmtPID})),
				psiPackets(pmtPID, cc[pmtPID], pmtSection(0, esEntry{streamType: 0x02, pid: videoPID}))...)
			entries, _ := analyzeWithIndex(t, join(append(pkts, tt.video(new(byte))...)))
			assert.Equal(t, tt.wantPTS, ptsOf(entries))
		})
	}
}
