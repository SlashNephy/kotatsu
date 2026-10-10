// Package thumbnail は、録画の索引から I フレームの位置を引き、代表画像と、シークバー用のプレビュー画像を作る (設計書 6.1)。
//
// 画像は、I フレームから始まる TS の一部を FFmpeg に渡し、最初のフレームを 1 枚取り出して作る。
// 録画全体をデコードしないので、長い録画でも速く作れる。プレビュー画像は、一定間隔のフレームを 1 枚のタイル画像にまとめる。
package thumbnail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"math"
	"os"
	"time"

	"github.com/SlashNephy/kotatsu/internal/ts"
)

const (
	// ThumbnailWidth と ThumbnailHeight は、代表画像の大きさ。16:9 に収め、余白は黒で埋める。
	// FFmpeg に渡すフィルター (thumbnailFilter、tileFilter) の大きさと揃える。
	ThumbnailWidth  = 480
	ThumbnailHeight = 270
	// TileWidth と TileHeight は、プレビュー画像の 1 コマの大きさ。
	TileWidth  = 160
	TileHeight = 90
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

// NewGenerator は Generator を返す。
func NewGenerator(extractor Extractor, opts *Options) *Generator {
	o := *opts
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
	var errs []error
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
	var frames []image.Image
	manifest := &Manifest{Width: TileWidth, Height: TileHeight, Columns: Columns, Times: []float64{}}
	last := -1
	var errs []error
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
	preview, err := tile(frames)
	if err != nil {
		return nil, nil, err
	}

	return preview, manifest, nil
}

// extract は、I フレームから始まる TS の一部を、Extractor に渡して 1 枚にする。
func (g *Generator) extract(ctx context.Context, segments []Segment, k keyframe, size Size) ([]byte, error) {
	file, err := os.Open(segments[k.segment].Path)
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
func keyframesOf(segments []Segment) []keyframe {
	var keyframes []keyframe
	for i := range segments {
		index := segments[i].Index
		for _, entry := range index {
			at := segments[i].Start + time.Duration(ts.PTSDiff(index[0].PTS, entry.PTS))*time.Second/ts.PTSClock
			keyframes = append(keyframes, keyframe{segment: i, offset: entry.Offset, at: at})
		}
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

// tile は、コマを Columns ずつ並べた、1 枚の JPEG にする。コマは TileWidth × TileHeight に収めてある。
func tile(frames []image.Image) ([]byte, error) {
	rows := (len(frames) + Columns - 1) / Columns
	columns := min(len(frames), Columns)
	canvas := image.NewRGBA(image.Rect(0, 0, columns*TileWidth, rows*TileHeight))
	for i, frame := range frames {
		at := image.Pt(i%Columns*TileWidth, i/Columns*TileHeight)
		draw.Draw(canvas, image.Rectangle{Min: at, Max: at.Add(image.Pt(TileWidth, TileHeight))}, frame, frame.Bounds().Min, draw.Src)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("failed to encode the preview: %w", err)
	}

	return buf.Bytes(), nil
}
