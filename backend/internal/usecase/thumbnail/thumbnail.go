// Package thumbnail は、録画の索引から I フレームの位置を引き、代表画像と、シークバー用のプレビュー画像を作る (設計書 6.1)。
//
// 画像は、I フレームから始まる TS の一部を FFmpeg に渡し、最初のフレームを 1 枚取り出して作る。
// 録画全体をデコードしないので、長い録画でも速く作れる。プレビュー画像は、一定間隔のフレームを 1 枚のタイル画像にまとめる。
package thumbnail

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"math"
	"slices"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// ThumbnailWidth と ThumbnailHeight は、代表画像の大きさ。16:9 に収め、余白は黒で埋める。
	// FFmpeg に渡すフィルター (thumbnailFilter、tileFilter) の大きさと揃える。
	ThumbnailWidth  = 480
	ThumbnailHeight = 270
	// TileWidth と TileHeight は、プレビュー画像の 1 コマの大きさ。
	TileWidth  = 320
	TileHeight = 180
	// Columns は、プレビュー画像の 1 行のコマの数。
	Columns = 10
	// MaxFrames は、プレビュー画像のコマの数の上限。長い録画では、間隔を広げて収める。
	MaxFrames = 100
	// DefaultInterval は、プレビュー画像のコマの間隔の下限。
	DefaultInterval = 10 * time.Second
	// ThumbnailPosition は、代表画像にするフレームの位置 (録画の長さに対する割合)。
	// 録画の頭は、前の番組の終わりや CM のことが多いので、少し入ったところにする。
	ThumbnailPosition = 0.25
	// chunkSize は、1 枚を取り出すのに FFmpeg へ渡す TS の大きさの上限。GOP 1 つ分 (地上波で 1 MiB ほど) より十分に大きくする。
	chunkSize = 4 << 20
	// jpegQuality は、プレビュー画像を JPEG にするときの品質。
	jpegQuality = 80
	// thumbnailAttempts は、代表画像を取り出せなかったときに、後ろの I フレームで試し直す回数。
	thumbnailAttempts = 3
)

var (
	// ErrNoKeyframe は、索引に I フレームがないので、画像を作れないことを表す。
	ErrNoKeyframe = errors.New("the recording has no keyframe in its index")
	// ErrNoFrame は、どの I フレームからも、画像を取り出せなかったことを表す。
	ErrNoFrame = errors.New("failed to extract any frame from the recording")
)

// Size は、取り出すフレームの大きさ。
type Size int

const (
	// SizeThumbnail は、代表画像の大きさ (ThumbnailWidth × ThumbnailHeight)。
	SizeThumbnail Size = iota
	// SizeTile は、プレビュー画像の 1 コマの大きさ (TileWidth × TileHeight)。
	SizeTile
)

// Extractor は、TS の最初のキーフレームを、size の大きさに収めた JPEG にする。
type Extractor interface {
	Extract(ctx context.Context, r io.Reader, size Size) ([]byte, error)
}

// Segment は、録画の 1 つのセグメントのファイルと、その索引。
type Segment struct {
	// Path は、セグメントのファイルの絶対パス。
	Path string
	// Start は、録画の始まりから、このセグメントの始まりまでの時間。
	Start time.Duration
	Index []ts.IndexEntry
}

// Manifest は、プレビュー画像のコマの並びの説明。画面は、これを見て、シークバーの位置に合うコマを切り出す。
type Manifest struct {
	// Width と Height は、1 コマの大きさ。
	Width  int `json:"width"`
	Height int `json:"height"`
	// Columns は、1 行のコマの数。コマは、左上から右へ、行が埋まったら次の行へ並べる。
	Columns int `json:"columns"`
	// Times は、各コマの、録画の始まりからの時間 (秒)。
	Times []float64 `json:"times"`
}

// Frame は、プレビュー画像から切り出す 1 コマの位置と、プレビュー画像全体の大きさ (ピクセル)。
type Frame struct {
	X, Y          int
	Width, Height int
	// SheetWidth と SheetHeight は、プレビュー画像全体の大きさ。
	SheetWidth, SheetHeight int
}

// Frame は、index 番目のコマの位置を返す。そのコマがなければ false を返す。
// コマは、左上から右へ Columns 個ずつ並ぶ。コマが Columns 個より少なければ、プレビュー画像の幅はその数のコマの幅になる。
// 画面 (frontend/src/recordings/previewFrame.ts) も、同じ並べ方で切り出す。
func (m *Manifest) Frame(index int) (Frame, bool) {
	count := len(m.Times)
	if m.Columns <= 0 || index < 0 || index >= count {
		return Frame{X: 0, Y: 0, Width: 0, Height: 0, SheetWidth: 0, SheetHeight: 0}, false
	}
	rows := (count + m.Columns - 1) / m.Columns

	return Frame{
		X: index % m.Columns * m.Width, Y: index / m.Columns * m.Height,
		Width: m.Width, Height: m.Height,
		SheetWidth: min(count, m.Columns) * m.Width, SheetHeight: rows * m.Height,
	}, true
}

// FrameBefore は、position 以前で、いちばん近いコマの番号を返す。position より前にコマがなければ false を返す。
// 後のコマは選ばないので、まだ見ていない場面を見せない。Times は小さい順に並んでいる。
func (m *Manifest) FrameBefore(position time.Duration) (int, bool) {
	i, found := slices.BinarySearch(m.Times, position.Seconds())
	if found {
		return i, true
	}
	if i == 0 {
		return 0, false
	}

	return i - 1, true
}

// Result は、作った画像。
type Result struct {
	// Thumbnail は、代表画像の JPEG。
	Thumbnail []byte
	// Preview は、プレビュー画像の JPEG。作らないときは nil。
	Preview []byte
	// Manifest は、プレビュー画像の説明。作らないときは nil。
	Manifest *Manifest
}

// Options は Generator の設定。
type Options struct {
	// Preview は、プレビュー画像を作るかどうか。低スペック機では、無効にできる。
	Preview bool
	// Interval は、プレビュー画像のコマの間隔の下限。0 なら DefaultInterval。
	Interval time.Duration
}

// Generator は、録画の代表画像とプレビュー画像を作る。
type Generator struct {
	extractor Extractor
	opts      Options
}

// NewGenerator は Generator を返す。opts が nil なら、プレビュー画像も、既定の間隔で作る。
func NewGenerator(extractor Extractor, opts *Options) *Generator {
	o := Options{Preview: true, Interval: DefaultInterval}
	if opts != nil {
		o = *opts
	}
	if o.Interval <= 0 {
		o.Interval = DefaultInterval
	}

	return &Generator{extractor: extractor, opts: o}
}

// keyframe は、録画の中の 1 つの I フレームの位置。
type keyframe struct {
	segment int
	offset  int64
	// at は、録画の始まりからの時間。
	at time.Duration
}

// Generate は、セグメントの索引から I フレームを選び、代表画像と、設定していればプレビュー画像を作る。
func (g *Generator) Generate(ctx context.Context, segments []Segment) (*Result, error) {
	keyframes := keyframesOf(segments)
	if len(keyframes) == 0 {
		return nil, ErrNoKeyframe
	}
	thumbnail, err := g.thumbnail(ctx, segments, keyframes)
	if err != nil {
		return nil, err
	}
	result := &Result{Thumbnail: thumbnail, Preview: nil, Manifest: nil}
	if !g.opts.Preview {
		return result, nil
	}
	result.Preview, result.Manifest, err = g.preview(ctx, segments, keyframes)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// thumbnail は、録画の ThumbnailPosition の位置に最も近い I フレームから、代表画像を作る。
// 取り出せなければ、後ろの I フレームで試し直す。
func (g *Generator) thumbnail(ctx context.Context, segments []Segment, keyframes []keyframe) ([]byte, error) {
	duration := keyframes[len(keyframes)-1].at
	start := nearest(keyframes, time.Duration(float64(duration)*ThumbnailPosition))
	errs := []error{}
	for i := start; i < len(keyframes) && i < start+thumbnailAttempts; i++ {
		frame, err := g.extract(ctx, segments, keyframes[i], SizeThumbnail)
		if err == nil {
			return frame, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		errs = append(errs, err)
	}

	return nil, fmt.Errorf("%w: %w", ErrNoFrame, errors.Join(errs...))
}

// preview は、一定間隔の I フレームから取り出したコマを、1 枚のタイル画像にまとめる。
// 間隔は Interval を下限に、コマが MaxFrames に収まるよう広げる。取り出せなかったコマは飛ばす。
func (g *Generator) preview(ctx context.Context, segments []Segment, keyframes []keyframe) ([]byte, *Manifest, error) {
	duration := keyframes[len(keyframes)-1].at
	interval := max(g.opts.Interval, time.Duration(math.Ceil(float64(duration)/MaxFrames)))
	frames := []image.Image{}
	manifest := &Manifest{Width: TileWidth, Height: TileHeight, Columns: Columns, Times: []float64{}}
	last := -1
	errs := []error{}
	for target := time.Duration(0); target <= duration && len(frames) < MaxFrames; target += interval {
		i := nearest(keyframes, target)
		// I フレームの間隔が、コマの間隔より広ければ、同じ I フレームを二度使わない。
		if i == last {
			continue
		}
		last = i
		data, err := g.extract(ctx, segments, keyframes[i], SizeTile)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			errs = append(errs, err)

			continue
		}
		frame, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to decode a preview frame: %w", err))

			continue
		}
		frames = append(frames, frame)
		manifest.Times = append(manifest.Times, keyframes[i].at.Seconds())
	}
	if len(frames) == 0 {
		return nil, nil, fmt.Errorf("%w: %w", ErrNoFrame, errors.Join(errs...))
	}
	preview, err := tile(frames, manifest)
	if err != nil {
		return nil, nil, err
	}

	return preview, manifest, nil
}

// extract は、I フレームから始まる TS の一部を、Extractor に渡して 1 枚にする。
func (g *Generator) extract(ctx context.Context, segments []Segment, k keyframe, size Size) ([]byte, error) {
	file, err := sharedfile.Open(segments[k.segment].Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open the segment: %w", err)
	}
	defer file.Close()
	if _, err := file.Seek(k.offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek the segment: %w", err)
	}

	return g.extractor.Extract(ctx, io.LimitReader(file, chunkSize), size)
}

// keyframesOf は、セグメントの索引を、録画の始まりからの時間の順に並べる。
// 放送の PTS が途中で飛んだり、セグメントの時間が重なったりしても、時間の順に並べ直す。
// 時間は、再生と同じく、途切れた区間を含めない (設計書 7.2)。プレビュー画像のコマを、シークバーの位置に合わせるためである。
// 並べた I フレームの間が ts.MaxKeyframeInterval より空いていたら、間で途切れたので、直前の I フレームの間隔だけ進める。
func keyframesOf(segments []Segment) []keyframe {
	keyframes := []keyframe{}
	for i := range segments {
		index := segments[i].Index
		for _, entry := range index {
			at := segments[i].Start + time.Duration(ts.PTSDiff(index[0].PTS, entry.PTS))*time.Second/ts.PTSClock
			keyframes = append(keyframes, keyframe{segment: i, offset: entry.Offset, at: at})
		}
	}
	slices.SortStableFunc(keyframes, func(a, b keyframe) int { return cmp.Compare(a.at, b.at) })
	var removed time.Duration
	interval := ts.DefaultKeyframeInterval
	for i := 1; i < len(keyframes); i++ {
		d := keyframes[i].at - removed - keyframes[i-1].at
		switch {
		case d > ts.MaxKeyframeInterval:
			removed += d - interval
		case d > 0:
			interval = d
		}
		keyframes[i].at -= removed
	}

	return keyframes
}

// nearest は、時間 target に最も近い I フレームの添え字を返す。keyframes は時間の順に並んでいる。
func nearest(keyframes []keyframe, target time.Duration) int {
	best := 0
	for i := range keyframes {
		if (keyframes[i].at - target).Abs() < (keyframes[best].at - target).Abs() {
			best = i
		}
	}

	return best
}

// tile は、コマを、manifest の並べ方 (Manifest.Frame) で 1 枚の JPEG にする。コマは TileWidth × TileHeight に収めてある。
func tile(frames []image.Image, manifest *Manifest) ([]byte, error) {
	first, ok := manifest.Frame(0)
	if !ok {
		return nil, ErrNoFrame
	}
	canvas := image.NewRGBA(image.Rect(0, 0, first.SheetWidth, first.SheetHeight))
	for i, frame := range frames {
		f, ok := manifest.Frame(i)
		if !ok {
			return nil, fmt.Errorf("the manifest has no frame %d", i)
		}
		at := image.Pt(f.X, f.Y)
		draw.Draw(canvas, image.Rectangle{Min: at, Max: at.Add(image.Pt(f.Width, f.Height))}, frame, frame.Bounds().Min, draw.Src)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("failed to encode the preview: %w", err)
	}

	return buf.Bytes(), nil
}
