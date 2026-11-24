package thumbnail_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/thumbnail"
)

const (
	// keyframeSize は、テストのセグメントで、I フレーム 1 つ分の大きさ。
	keyframeSize = 1000
	// redStep は、I フレームの番号ごとに変える、モックの画像の赤の値の幅。JPEG のずれより十分に大きくする。
	redStep = 20
)

// errBroken は、壊れていて取り出せない I フレームを表す。
var errBroken = errors.New("broken keyframe")

// fakeExtractor は、I フレームの先頭のバイト (通し番号) に redStep を掛けた値を赤にした、単色の画像を返す。
type fakeExtractor struct {
	// broken は、取り出せない I フレームの通し番号。
	broken map[byte]bool
}

func (f *fakeExtractor) Extract(_ context.Context, r io.Reader, size thumbnail.Size) ([]byte, error) {
	width, height := thumbnail.TileWidth, thumbnail.TileHeight
	if size == thumbnail.SizeThumbnail {
		width, height = thumbnail.ThumbnailWidth, thumbnail.ThumbnailHeight
	}
	marker := make([]byte, 1)
	if _, err := io.ReadFull(r, marker); err != nil {
		return nil, err
	}
	if f.broken[marker[0]] {
		return nil, errBroken
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	red := uint8(min(int(marker[0])*redStep, 255))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: red, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// segment は、録画の始まりから start の位置で始まり、I フレームを step ごとに n 個持つセグメントを作る。
// I フレームの先頭のバイトは、通し番号 (first + 1 から始まる) にする。
func segment(t *testing.T, start time.Duration, n int, step time.Duration, first int) thumbnail.Segment {
	t.Helper()

	return segmentAt(t, filepath.Join(t.TempDir(), "segment.m2ts"), start, n, step, first)
}

// segmentAt は、segment と同じセグメントを、path に作る。
func segmentAt(t *testing.T, path string, start time.Duration, n int, step time.Duration, first int) thumbnail.Segment {
	t.Helper()

	data := make([]byte, n*keyframeSize)
	index := make([]ts.IndexEntry, 0, n)
	// PTS は 33 ビットで一周する。途中で一周させても、時間を正しく求めることを確認する。
	base := int64(1<<33) - 2*ts.PTSClock
	for i := range n {
		// 通し番号は 1 バイトに収める。収まらない長い録画では、画像の色を確認しない。
		if n := first + i + 1; n >= 0 && n <= math.MaxUint8 {
			data[i*keyframeSize] = byte(n)
		}
		pts := (base + int64(i)*int64(step/time.Millisecond)*ts.PTSClock/1000) % (1 << 33)
		index = append(index, ts.IndexEntry{Offset: int64(i * keyframeSize), PTS: pts})
	}
	require.NoError(t, os.WriteFile(path, data, 0o600))

	return thumbnail.Segment{Path: path, Start: start, Index: index, Open: nil}
}

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()

	img, err := jpeg.Decode(bytes.NewReader(data))
	require.NoError(t, err)

	return img
}

// marker は、画像の (x, y) の赤の値から、モックの画像にした I フレームの通し番号を求める。
func marker(img image.Image, x, y int) int {
	rgba, ok := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
	if !ok {
		return -1
	}

	return (int(rgba.R) + redStep/2) / redStep
}

// tileMarkers は、プレビュー画像の各コマの中央から、コマにした I フレームの通し番号を求める。
func tileMarkers(t *testing.T, data []byte, count int) []int {
	t.Helper()

	img := decode(t, data)
	markers := make([]int, 0, count)
	for i := range count {
		markers = append(markers, marker(img, i%thumbnail.Columns*thumbnail.TileWidth+thumbnail.TileWidth/2,
			i/thumbnail.Columns*thumbnail.TileHeight+thumbnail.TileHeight/2))
	}

	return markers
}

// TestGenerate は、インデックスの I フレームから、代表画像と、一定間隔のコマを並べたプレビュー画像を作ることを確認する (設計書 6.1)。
func TestGenerate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		segments func(t *testing.T) []thumbnail.Segment
		interval time.Duration
		broken   map[byte]bool
		// wantThumbnail は、代表画像にした I フレームの通し番号。
		wantThumbnail int
		wantTimes     []float64
		// wantMarkers は、プレビュー画像の各コマにした I フレームの通し番号。
		wantMarkers []int
	}{
		{
			name: "1 秒ごとの I フレームから、2 秒ごとのコマを作る",
			segments: func(t *testing.T) []thumbnail.Segment {
				t.Helper()

				return []thumbnail.Segment{segment(t, 0, 9, time.Second, 0)}
			},
			interval: 2 * time.Second,
			// 代表画像は、8 秒の 1/4 の、2 秒の I フレームにする。
			wantThumbnail: 3,
			wantTimes:     []float64{0, 2, 4, 6, 8},
			wantMarkers:   []int{1, 3, 5, 7, 9},
		},
		{
			name: "I フレームの間隔がコマの間隔より広ければ、同じ I フレームを二度使わない",
			segments: func(t *testing.T) []thumbnail.Segment {
				t.Helper()

				return []thumbnail.Segment{segment(t, 0, 3, 5*time.Second, 0)}
			},
			interval: time.Second,
			// 10 秒の 1/4 の 2.5 秒は、0 秒と 5 秒の I フレームから同じ距離なので、先の I フレームにする。
			wantThumbnail: 1,
			wantTimes:     []float64{0, 5, 10},
			wantMarkers:   []int{1, 2, 3},
		},
		{
			name: "途切れた録画は、途切れた区間を詰めて、時間を求める",
			segments: func(t *testing.T) []thumbnail.Segment {
				t.Helper()

				return []thumbnail.Segment{segment(t, 0, 3, time.Second, 0), segment(t, 10*time.Second, 3, time.Second, 3)}
			},
			interval: 5 * time.Second,
			// 2 秒から 10 秒までの途切れは、I フレームの間隔の 1 秒に詰めるので、後のセグメントは 3 秒から始まる。
			// 5 秒の 1/4 の 1.25 秒に最も近いのは、前のセグメントの 1 秒の I フレーム。
			wantThumbnail: 2,
			wantTimes:     []float64{0, 5},
			wantMarkers:   []int{1, 6},
		},
		{
			name: "セグメントが始まりの順に並んでいなくても、時間の順に並べ直す",
			segments: func(t *testing.T) []thumbnail.Segment {
				t.Helper()

				return []thumbnail.Segment{segment(t, 10*time.Second, 3, time.Second, 3), segment(t, 0, 3, time.Second, 0)}
			},
			interval:      5 * time.Second,
			wantThumbnail: 2,
			wantTimes:     []float64{0, 5},
			wantMarkers:   []int{1, 6},
		},
		{
			name: "取り出せない I フレームは、代表画像では後ろで試し直し、プレビュー画像では飛ばす",
			segments: func(t *testing.T) []thumbnail.Segment {
				t.Helper()

				return []thumbnail.Segment{segment(t, 0, 9, time.Second, 0)}
			},
			interval:      2 * time.Second,
			broken:        map[byte]bool{3: true},
			wantThumbnail: 4,
			wantTimes:     []float64{0, 4, 6, 8},
			wantMarkers:   []int{1, 5, 7, 9},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			generator := thumbnail.NewGenerator(&fakeExtractor{broken: tt.broken}, &thumbnail.Options{Preview: true, Interval: tt.interval})
			result, err := generator.Generate(t.Context(), tt.segments(t))
			require.NoError(t, err)

			thumb := decode(t, result.Thumbnail)
			assert.Equal(t, image.Pt(thumbnail.ThumbnailWidth, thumbnail.ThumbnailHeight), thumb.Bounds().Size())
			assert.Equal(t, tt.wantThumbnail, marker(thumb, thumbnail.ThumbnailWidth/2, thumbnail.ThumbnailHeight/2))

			assert.Equal(t, &thumbnail.Manifest{
				Width: thumbnail.TileWidth, Height: thumbnail.TileHeight, Columns: thumbnail.Columns, Times: tt.wantTimes,
			}, result.Manifest)
			assert.Equal(t, image.Pt(len(tt.wantTimes)*thumbnail.TileWidth, thumbnail.TileHeight), decode(t, result.Preview).Bounds().Size())
			assert.Equal(t, tt.wantMarkers, tileMarkers(t, result.Preview, len(tt.wantTimes)))
		})
	}
}

// TestGenerateCapsFrames は、長い録画では、コマの間隔を広げて、コマの数を MaxFrames に収めることを確認する。
func TestGenerateCapsFrames(t *testing.T) {
	t.Parallel()

	// 1 秒ごとの I フレームが 1000 個ある、999 秒の録画。
	segments := []thumbnail.Segment{segment(t, 0, 1000, time.Second, 0)}
	result, err := thumbnail.NewGenerator(&fakeExtractor{broken: nil}, &thumbnail.Options{Preview: true, Interval: time.Second}).
		Generate(t.Context(), segments)
	require.NoError(t, err)
	require.Len(t, result.Manifest.Times, thumbnail.MaxFrames)
	for i := 1; i < len(result.Manifest.Times); i++ {
		assert.GreaterOrEqual(t, result.Manifest.Times[i]-result.Manifest.Times[i-1], 9.0, "the interval is widened to fit")
	}
	assert.Equal(t, image.Pt(thumbnail.Columns*thumbnail.TileWidth, thumbnail.MaxFrames/thumbnail.Columns*thumbnail.TileHeight),
		decode(t, result.Preview).Bounds().Size())
}

// TestGenerateWithoutPreview は、プレビュー画像を無効にしたら、代表画像だけを作ることを確認する。
func TestGenerateWithoutPreview(t *testing.T) {
	t.Parallel()

	result, err := thumbnail.NewGenerator(&fakeExtractor{broken: nil}, &thumbnail.Options{Preview: false, Interval: 0}).
		Generate(t.Context(), []thumbnail.Segment{segment(t, 0, 3, time.Second, 0)})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Thumbnail)
	assert.Nil(t, result.Preview)
	assert.Nil(t, result.Manifest)
}

// TestGenerateFails は、I フレームがないか、どの I フレームからも取り出せなければ、失敗することを確認する。
func TestGenerateFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		segments func(t *testing.T) []thumbnail.Segment
		broken   map[byte]bool
		wantErr  error
	}{
		{
			name: "索引に I フレームがない",
			segments: func(t *testing.T) []thumbnail.Segment {
				t.Helper()

				return []thumbnail.Segment{{Path: filepath.Join(t.TempDir(), "empty.m2ts"), Start: 0, Index: nil}}
			},
			wantErr: thumbnail.ErrNoKeyframe,
		},
		{
			name: "どの I フレームからも取り出せない",
			segments: func(t *testing.T) []thumbnail.Segment {
				t.Helper()

				return []thumbnail.Segment{segment(t, 0, 3, time.Second, 0)}
			},
			broken:  map[byte]bool{1: true, 2: true, 3: true},
			wantErr: thumbnail.ErrNoFrame,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := thumbnail.NewGenerator(&fakeExtractor{broken: tt.broken}, &thumbnail.Options{Preview: true, Interval: 0}).
				Generate(t.Context(), tt.segments(t))
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestGenerateLatest は、録画中の代表画像 (#1236) を、書いた終わりの少し前の I フレームから作り、取り出せなければ前で再試行することを確認する。
func TestGenerateLatest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		broken map[byte]bool
		// want は、代表画像にした I フレームの通し番号。
		want int
	}{
		// 1 秒ごとの 15 個の I フレーム (0〜14 秒) の、終わりの 10 秒前は 4 秒の I フレーム (通し番号 5)。
		{name: "書いた終わりの少し前から作る", broken: nil, want: 5},
		{name: "取り出せなければ前で試し直す", broken: map[byte]bool{5: true}, want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			generator := thumbnail.NewGenerator(&fakeExtractor{broken: tt.broken}, &thumbnail.Options{Preview: false, Interval: 0, Latest: true})
			result, err := generator.Generate(t.Context(), []thumbnail.Segment{segment(t, 0, 15, time.Second, 0)})
			require.NoError(t, err)
			assert.Nil(t, result.Preview, "no preview is made while recording")
			assert.Equal(t, tt.want, marker(decode(t, result.Thumbnail), thumbnail.ThumbnailWidth/2, thumbnail.ThumbnailHeight/2))
		})
	}
}

// TestGenerateAvoidsCM は、代表画像を、CM の区間 (前後に 1 秒の余白を足したもの) にかからない I フレームのうち、
// ThumbnailPosition に最も近いものから作ることを確認する (#1810)。
func TestGenerateAvoidsCM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		avoid  []thumbnail.Span
		broken map[byte]bool
		latest bool
		// want は、代表画像にした I フレームの通し番号。
		want int
	}{
		// 1 秒ごとの 30 個の I フレーム (0〜29 秒) の 25% は 7.25 秒で、7 秒の I フレーム (通し番号 8)。
		{name: "CM の区間がなければ 25% の位置", avoid: nil, broken: nil, latest: false, want: 8},
		{name: "CM の区間の外なら動かさない", avoid: []thumbnail.Span{{Start: 20 * time.Second, End: 25 * time.Second}}, broken: nil, latest: false, want: 8},
		// 余白を足した 4〜11 秒を避け、7.25 秒に近い 11 秒 (3 秒の I フレームより近い) を選ぶ。
		{name: "CM の区間にかかれば外の近いものにずらす", avoid: []thumbnail.Span{{Start: 5 * time.Second, End: 10 * time.Second}}, broken: nil, latest: false, want: 12},
		{name: "前の方が近ければ前にずらす", avoid: []thumbnail.Span{{Start: 7 * time.Second, End: 15 * time.Second}}, broken: nil, latest: false, want: 6},
		// 余白を足した 6〜10 秒を避けて 5 秒を選び、取り出せなければ、後ろの 6〜9 秒を飛ばして 10 秒で再試行する。
		{name: "試し直しも CM の区間を飛ばす", avoid: []thumbnail.Span{{Start: 7 * time.Second, End: 9 * time.Second}}, broken: map[byte]bool{6: true}, latest: false, want: 11},
		{name: "全部が CM なら気にせずに選ぶ", avoid: []thumbnail.Span{{Start: 0, End: 30 * time.Second}}, broken: nil, latest: false, want: 8},
		// 録画中は、終わりの 10 秒前 (19 秒、通し番号 20) から作り、CM の区間を使わない。通し番号 13 以上は色で見分けられないので、
		// 選び直すと 13 未満になる区間にする (避ければ 7 秒、通し番号 8)。
		{name: "録画中の代表画像には使わない", avoid: []thumbnail.Span{{Start: 9 * time.Second, End: 30 * time.Second}}, broken: nil, latest: true, want: 13},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			generator := thumbnail.NewGenerator(&fakeExtractor{broken: tt.broken}, &thumbnail.Options{Preview: false, Interval: 0, Latest: tt.latest, Avoid: tt.avoid})
			result, err := generator.Generate(t.Context(), []thumbnail.Segment{segment(t, 0, 30, time.Second, 0)})
			require.NoError(t, err)
			assert.Equal(t, tt.want, marker(decode(t, result.Thumbnail), thumbnail.ThumbnailWidth/2, thumbnail.ThumbnailHeight/2))
		})
	}
}

// TestGenerateAvoidsCMAcrossGap は、CM の区間を、途切れた区間も含めて数えた時間 (CM の区間と同じ数え方) で比較することを確認する (#1810)。
// 再生の時間は途切れた区間を詰めるので、途切れた後の I フレームは、再生の時間と数え方が食い違う。
func TestGenerateAvoidsCMAcrossGap(t *testing.T) {
	t.Parallel()

	// 0〜1 秒の I フレームのあと、100 秒から 10 個 (再生の時間では 2〜11 秒) 続く。25% は再生の時間の 2.75 秒で、
	// 101 秒の I フレーム (通し番号 4)。
	segments := []thumbnail.Segment{segment(t, 0, 2, time.Second, 0), segment(t, 100*time.Second, 10, time.Second, 2)}
	tests := []struct {
		name  string
		avoid []thumbnail.Span
		want  int
	}{
		{name: "CM の区間がなければ 25% の位置", avoid: nil, want: 4},
		// 余白を足した 100〜104 秒を避け、再生の時間で 2.75 秒に近い 1 秒 (通し番号 2) を選ぶ。
		{name: "途切れた後の区間を避ける", avoid: []thumbnail.Span{{Start: 101 * time.Second, End: 103 * time.Second}}, want: 2},
		// 再生の時間の 2〜4 秒にあたる区間は、途切れた区間も含めて数えると、どの I フレームにもかからない。
		{name: "再生の時間では比べない", avoid: []thumbnail.Span{{Start: 2 * time.Second, End: 4 * time.Second}}, want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			generator := thumbnail.NewGenerator(&fakeExtractor{broken: nil}, &thumbnail.Options{Preview: false, Interval: 0, Latest: false, Avoid: tt.avoid})
			result, err := generator.Generate(t.Context(), segments)
			require.NoError(t, err)
			assert.Equal(t, tt.want, marker(decode(t, result.Thumbnail), thumbnail.ThumbnailWidth/2, thumbnail.ThumbnailHeight/2))
		})
	}
}
