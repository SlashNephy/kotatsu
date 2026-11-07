package ts_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// res180p は、テストの H.264 と HEVC の映像 (320x180 のプログレッシブ) の解像度。
const res180p = "180p"

// TestAnalyzerVideoFormat は、最初の映像のシーケンスヘッダーか SPS から、映像の形式を読むことを確かめる (#1310)。
func TestAnalyzerVideoFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		file           string
		want           ts.VideoFormat
		wantResolution string
	}{
		{file: "test.ts", want: ts.VideoFormat{Codec: "mpeg2", Width: 720, Height: 480, Interlaced: false, FrameRate: 30000.0 / 1001}, wantResolution: "480p"},
		{file: "test-mpeg2-1080i.ts", want: ts.VideoFormat{Codec: "mpeg2", Width: 1440, Height: 1080, Interlaced: true, FrameRate: 30000.0 / 1001}, wantResolution: "1080i"},
		{file: "test-h264.ts", want: ts.VideoFormat{Codec: "h264", Width: 320, Height: 180, Interlaced: false, FrameRate: 30000.0 / 1001}, wantResolution: res180p},
		{file: "test-h264-1080i.ts", want: ts.VideoFormat{Codec: "h264", Width: 1440, Height: 1080, Interlaced: true, FrameRate: 30000.0 / 1001}, wantResolution: "1080i"},
		{file: "test-hevc.ts", want: ts.VideoFormat{Codec: "hevc", Width: 320, Height: 180, Interlaced: false, FrameRate: 0}, wantResolution: res180p},
		{file: "test-hevc-main10.ts", want: ts.VideoFormat{Codec: "hevc", Width: 320, Height: 180, Interlaced: false, FrameRate: 0}, wantResolution: res180p},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ts", tt.file))
			require.NoError(t, err)
			analyzer := ts.NewAnalyzer(nil)
			_, err = analyzer.Write(data)
			require.NoError(t, err)
			got := analyzer.Stats().Video
			require.NotNil(t, got)
			assert.Equal(t, tt.want.Codec, got.Codec)
			assert.Equal(t, tt.want.Width, got.Width)
			assert.Equal(t, tt.want.Height, got.Height)
			assert.Equal(t, tt.want.Interlaced, got.Interlaced)
			assert.InDelta(t, tt.want.FrameRate, got.FrameRate, 0.001)
			assert.Equal(t, tt.wantResolution, got.Resolution())
		})
	}
}

// TestAnalyzerVideoFormatTruncated は、映像の先頭が途中で切れていても、止まらずに形式を読まないことを確かめる (#1310)。
func TestAnalyzerVideoFormatTruncated(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ts", "test-h264.ts"))
	require.NoError(t, err)
	analyzer := ts.NewAnalyzer(nil)
	// 最初の数パケットだけを読む。SPS の後ろの start code が来ないので、形式は決められない。
	_, err = analyzer.Write(data[:ts.PacketSize*3])
	require.NoError(t, err)
	assert.Nil(t, analyzer.Stats().Video)
}

// bitWriter は、テストの SPS を組み立てる。
type bitWriter struct {
	bits []byte
}

func (w *bitWriter) put(value uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		w.bits = append(w.bits, byte(value>>i&1))
	}
}

// ue は、符号なしの指数ゴロム符号を書く。
func (w *bitWriter) ue(value uint64) {
	v := value + 1
	n := 0
	for x := v; x > 1; x >>= 1 {
		n++
	}
	w.put(0, n)
	w.put(v, n+1)
}

func (w *bitWriter) bytes() []byte {
	out := make([]byte, (len(w.bits)+7)/8)
	for i, b := range w.bits {
		out[i/8] |= b << (7 - i%8)
	}

	return out
}

// TestParseH264SPSHugePOCCycle は、壊れた SPS の num_ref_frames_in_pic_order_cnt_cycle が大きくても、
// 長く回らずに、形式を読まないことを確かめる (#1310)。
func TestParseH264SPSHugePOCCycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cycle uint64
		want  bool
	}{
		{name: "within the limit", cycle: 2, want: true},
		{name: "beyond the limit", cycle: 1<<31 - 1, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := &bitWriter{bits: []byte{}}
			w.put(66, 8) // profile_idc (Baseline)
			w.put(30, 16)
			w.ue(0) // seq_parameter_set_id
			w.ue(0) // log2_max_frame_num_minus4
			w.ue(1) // pic_order_cnt_type
			w.put(0, 1)
			w.ue(0) // offset_for_non_ref_pic
			w.ue(0) // offset_for_top_to_bottom_field
			w.ue(tt.cycle)
			if tt.cycle < 16 {
				for range tt.cycle {
					w.ue(0)
				}
			}
			w.ue(1)     // max_num_ref_frames
			w.put(0, 1) // gaps_in_frame_num_value_allowed_flag
			w.ue(119)   // pic_width_in_mbs_minus1 (1920)
			w.ue(67)    // pic_height_in_map_units_minus1 (1088)
			w.put(1, 1) // frame_mbs_only_flag
			w.put(1, 1) // direct_8x8_inference_flag
			w.put(0, 1) // frame_cropping_flag
			w.put(0, 1) // vui_parameters_present_flag
			w.put(1, 1) // rbsp_stop_one_bit
			es := append([]byte{0, 0, 1, 0x67}, w.bytes()...)
			es = append(es, 0, 0, 1, 0x65)
			got := ts.ParseVideoFormatForTest("h264", es)
			if !tt.want {
				assert.Nil(t, got)

				return
			}
			require.NotNil(t, got)
			assert.Equal(t, 1920, got.Width)
			assert.Equal(t, 1088, got.Height)
		})
	}
}
