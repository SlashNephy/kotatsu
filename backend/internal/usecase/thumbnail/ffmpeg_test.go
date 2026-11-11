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

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/thumbnail"
)

// testTS は、合成した 720×480 (画素の縦横比 1:1) の約 6 秒の TS。I フレームは 0.5 秒ごとにある。
const testTS = "../../../testdata/ts/test.ts"

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
		{name: "H.264", path: "../../../testdata/ts/test-h264.ts"},
		{name: "HEVC", path: "../../../testdata/ts/test-hevc.ts"},
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

// TestFFmpegDeinterlace は、インターレースの映像から作る画像が、2 つのフィールドの混ざった二重写しにならないことを確かめる。
// 映像は、黒地を幅 48 の白い棒がフィールドごとに 48 ずつ右へ動く、640×480 のインターレースの MPEG-2 で、次のように作った。
//
//	ffmpeg -f lavfi -i color=c=black:s=640x480:r=60000/1001:d=1 -f lavfi -i color=c=white:s=48x480:r=60000/1001:d=1 \
//	  -filter_complex "[0][1]overlay=x='mod(n*48,560)':y=0,tinterlace=mode=interleave_top,setfield=tff" \
//	  -c:v mpeg2video -flags +ilme+ildct -g 15 -bf 2 -q:v 2 -an -f mpegts test-mpeg2-480i-motion.ts
//
// 2 つのフィールドで棒の位置が重ならないので、解除せずに縮めると、棒が 2 本の灰色になる。
// 解除すれば、棒は 1 本の白のままで、灰色は縁にしか出ない。FFmpeg のない環境 (CI など) では飛ばす。
func TestFFmpegDeinterlace(t *testing.T) {
	t.Parallel()

	ffmpeg, err := thumbnail.NewFFmpeg()
	if errors.Is(err, thumbnail.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)

	tests := []struct {
		name string
		size thumbnail.Size
	}{
		{name: "thumbnail", size: thumbnail.SizeThumbnail},
		{name: "tile", size: thumbnail.SizeTile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, err := os.Open("../../../testdata/ts/test-mpeg2-480i-motion.ts")
			require.NoError(t, err)
			t.Cleanup(func() { _ = file.Close() })
			data, err := ffmpeg.Extract(t.Context(), file, tt.size)
			require.NoError(t, err)

			img := decode(t, data)
			bounds := img.Bounds()
			gray, white := 0, 0
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				// 輝度 (ITU-R BT.601) を 0〜255 で求める。
				r, g, b, _ := img.At(x, bounds.Dy()/2).RGBA()
				switch y := (299*r + 587*g + 114*b) / 1000 >> 8; {
				case y >= 192:
					white++
				case y >= 64:
					gray++
				}
			}
			assert.Positive(t, white, "the bar stays white")
			assert.LessOrEqual(t, gray, 4, "the bar is not doubled into gray")
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
