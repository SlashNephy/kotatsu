package thumbnail_test

import (
	"bytes"
	"errors"
	"image"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/thumbnail"
	"github.com/SlashNephy/kotatsu/internal/ts"
)

// testTS は、合成した 720×480 (画素の縦横比 1:1) の約 6 秒の TS。I フレームは 0.5 秒ごとにある。
const testTS = "../../testdata/ts/test.ts"

// indexOf は、録画と同じく、TS を解析して索引を作る。
func indexOf(t *testing.T, path string) []ts.IndexEntry {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var buf bytes.Buffer
	writer, err := ts.NewIndexWriter(&buf)
	require.NoError(t, err)
	_, err = ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer}).Write(data)
	require.NoError(t, err)
	index, err := ts.ReadIndex(&buf)
	require.NoError(t, err)

	return index
}

// TestFFmpeg は、本物の FFmpeg で、合成した TS から代表画像とプレビュー画像を作ることを確かめる。
// FFmpeg のない環境 (CI など) では飛ばす。
func TestFFmpeg(t *testing.T) {
	t.Parallel()

	ffmpeg, err := thumbnail.NewFFmpeg()
	if errors.Is(err, thumbnail.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)
	index := indexOf(t, testTS)
	require.Len(t, index, 12)

	result, err := thumbnail.NewGenerator(ffmpeg, &thumbnail.Options{Preview: true, Interval: time.Second}).
		Generate(t.Context(), []thumbnail.Segment{{Path: testTS, Start: 0, Index: index}})
	require.NoError(t, err)
	assert.Equal(t, image.Pt(thumbnail.ThumbnailWidth, thumbnail.ThumbnailHeight), decode(t, result.Thumbnail).Bounds().Size())
	// 0.5 秒ごとの I フレームから、1 秒ごとのコマを作る。
	assert.Equal(t, []float64{0, 1.001, 2.002, 3.003, 4.004, 5.005}, result.Manifest.Times)
	assert.Equal(t, image.Pt(6*thumbnail.TileWidth, thumbnail.TileHeight), decode(t, result.Preview).Bounds().Size())
	// 3:2 の映像を 16:9 に収めるので、左右の端は黒で埋める。
	left := decode(t, result.Thumbnail).At(2, thumbnail.ThumbnailHeight/2)
	r, g, b, _ := left.RGBA()
	assert.Less(t, max(r, g, b)>>8, uint32(16), "the pillarbox is black")
}

// TestFFmpegCodecs は、本物の FFmpeg で、H.264 と HEVC の TS からも、索引を使って代表画像とプレビュー画像を作れることを確かめる。
// どちらも 320×180 の約 4 秒の TS で、I フレームは約 1 秒ごとにある。FFmpeg のない環境 (CI など) では飛ばす。
func TestFFmpegCodecs(t *testing.T) {
	t.Parallel()

	ffmpeg, err := thumbnail.NewFFmpeg()
	if errors.Is(err, thumbnail.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)

	tests := []struct {
		name string
		path string
	}{
		{name: "H.264", path: "../../testdata/ts/test-h264.ts"},
		{name: "HEVC", path: "../../testdata/ts/test-hevc.ts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			index := indexOf(t, tt.path)
			require.Len(t, index, 4)

			result, err := thumbnail.NewGenerator(ffmpeg, &thumbnail.Options{Preview: true, Interval: time.Second}).
				Generate(t.Context(), []thumbnail.Segment{{Path: tt.path, Start: 0, Index: index}})
			require.NoError(t, err)
			assert.Equal(t, image.Pt(thumbnail.ThumbnailWidth, thumbnail.ThumbnailHeight), decode(t, result.Thumbnail).Bounds().Size())
			assert.Equal(t, []float64{0, 1.001, 2.002, 3.003}, result.Manifest.Times)
			assert.Equal(t, image.Pt(4*thumbnail.TileWidth, thumbnail.TileHeight), decode(t, result.Preview).Bounds().Size())
		})
	}
}

// TestNewFFmpegNotFound は、FFmpeg が PATH に見つからなければ、ErrFFmpegNotFound を返すことを確かめる。
// 環境変数を変えるので、ほかのテストと並べて動かさない。
func TestNewFFmpegNotFound(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := thumbnail.NewFFmpeg()
	require.ErrorIs(t, err, thumbnail.ErrFFmpegNotFound)
}
