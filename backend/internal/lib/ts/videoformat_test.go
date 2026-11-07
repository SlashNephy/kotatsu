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
