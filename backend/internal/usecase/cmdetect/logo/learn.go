package logo

import (
	"errors"
	"math"
	"slices"
)

// Box は、画素の矩形。X と Y は左上の位置。
type Box struct {
	X, Y, W, H int
}

func (b Box) right() int  { return b.X + b.W }
func (b Box) bottom() int { return b.Y + b.H }

// union は、b と c を囲む矩形を返す。
func (b Box) union(c Box) Box {
	x, y := min(b.X, c.X), min(b.Y, c.Y)

	return Box{X: x, Y: y, W: max(b.right(), c.right()) - x, H: max(b.bottom(), c.bottom()) - y}
}

// distance は、b と c の間の、縦と横の隙間の大きいほう (重なっていれば 0)。
func (b Box) distance(c Box) int {
	dx := max(c.X-b.right(), b.X-c.right(), 0)
	dy := max(c.Y-b.bottom(), b.Y-c.bottom(), 0)

	return max(dx, dy)
}

// Corner は、ロゴを探す隅。
type Corner int

const (
	TopRight Corner = iota
	TopLeft
	BottomRight
	BottomLeft
)

func (c Corner) String() string {
	return [...]string{"top-right", "top-left", "bottom-right", "bottom-left"}[c]
}

// 隅を諦めた理由。学習できなかった局で、どのパラメーターが原因かを見るのに使う (#1505 の「測ること」)。
var (
	// ErrWeakCorner は、強い縁の閾値が、隅を採る揃い方の下限に届かなかったことを表す。
	ErrWeakCorner = errors.New("the strong edges are too weak")
	// ErrFlatCorner は、強い縁の閾値が、隅の揃い方の中央値に比べて目立たなかったことを表す。
	ErrFlatCorner = errors.New("the strong edges do not stand out from the corner")
	// ErrNoCore は、核にできる塊がなかったことを表す。
	ErrNoCore = errors.New("no blob is large enough for a logo")
	// ErrSharedCorner は、ロゴと張り合う動かない縁が多いことを表す。
	ErrSharedCorner = errors.New("other still edges compete with the logo")
	// ErrNoLogo は、どの隅にもロゴを言い切れなかったことを表す。
	ErrNoLogo = errors.New("no corner has a logo")
)

// CornerResult は、1 つの隅を調べた結果。
type CornerResult struct {
	Corner Corner
	// Threshold は、強い縁の閾値 (隅の揃い方の分位)。Median は、隅の揃い方の中央値。
	Threshold float64
	Median    float64
	// Share は、括った塊の強い縁が、張り合う量に占める割合。塊を作る前に諦めたら 0。
	Share float64
	// Box は、採った枠 (コマの座標)。諦めたら空。
	Box Box
	// Err は、諦めた理由。採ったら nil。
	Err error
}

// LearnResult は、学習の結果。
type LearnResult struct {
	// Box は、ロゴの枠 (コマの座標)。どの隅も諦めたら空。
	Box Box
	// Corner は、ロゴを見つけた隅。
	Corner Corner
	// Corners は、調べた隅ごとの結果。採った隅で止めるので、その後の隅は入らない。
	Corners []CornerResult
}

// Band は、学習で切り出す上下の帯の高さを返す。コマの高さに BandRatio を掛けて丸め、偶数に切り下げる。
func Band(frameHeight int, p *Params) int {
	h := int(math.Round(float64(frameHeight) * p.BandRatio))

	return h &^ 1
}

// Learn は、上の帯 top と下の帯 bottom の縁の向きの積算から、ロゴの枠を割り出す (#1505 の B の 3 と 4)。
// 隅を、右上、左上、右下、左下の順に調べ、最初にロゴと言い切れた隅を採る。どの隅も駄目なら ErrNoLogo を返す。
func Learn(top, bottom *Accumulator, frameWidth, frameHeight int, p *Params) (*LearnResult, error) {
	result := &LearnResult{Box: Box{X: 0, Y: 0, W: 0, H: 0}, Corner: TopRight, Corners: []CornerResult{}}
	cornerWidth := int(math.Round(float64(frameWidth) * p.CornerRatio))
	for _, corner := range []Corner{TopRight, TopLeft, BottomRight, BottomLeft} {
		band, bandY := top, 0
		if corner == BottomRight || corner == BottomLeft {
			band, bandY = bottom, frameHeight-bottom.Height
		}
		region := Box{X: 0, Y: 0, W: cornerWidth, H: band.Height}
		if corner == TopRight || corner == BottomRight {
			region.X = frameWidth - cornerWidth
		}
		r := findInCorner(band, region, frameWidth, frameHeight, p)
		r.Corner = corner
		if r.Err == nil {
			r.Box.Y += bandY
			result.Box, result.Corner = r.Box, corner
		}
		result.Corners = append(result.Corners, r)
		if r.Err == nil {
			return result, nil
		}
	}

	return result, ErrNoLogo
}

// blob は、強い縁の印を太らせた後の、4 近傍でつながった塊。
type blob struct {
	box    Box
	strong int
	large  bool
}

// findInCorner は、band の region (帯の座標) の中から、ロゴの枠を割り出す。返す枠は帯の座標。
func findInCorner(band *Accumulator, region Box, frameWidth, frameHeight int, p *Params) CornerResult {
	coherence := band.Coherence()
	values := make([]float64, 0, region.W*region.H)
	for y := region.Y; y < region.bottom(); y++ {
		for x := region.X; x < region.right(); x++ {
			values = append(values, coherence[y*band.Width+x])
		}
	}
	slices.Sort(values)
	r := CornerResult{
		Corner: TopRight, Threshold: quantile(values, p.StrongQuantile), Median: quantile(values, 0.5), Share: 0,
		Box: Box{X: 0, Y: 0, W: 0, H: 0}, Err: nil,
	}
	switch {
	case r.Threshold < p.MinCornerCoherence:
		r.Err = ErrWeakCorner

		return r
	case r.Threshold < r.Median*p.CornerMedianRatio:
		r.Err = ErrFlatCorner

		return r
	}

	strong := make([]bool, region.W*region.H)
	for y := range region.H {
		for x := range region.W {
			strong[y*region.W+x] = coherence[(region.Y+y)*band.Width+region.X+x] >= r.Threshold
		}
	}
	limits := blobLimits{
		tooLargeW: float64(frameWidth) * p.TooLargeRatio, tooLargeH: float64(frameHeight) * p.TooLargeRatio,
		coreMinW: float64(frameWidth) * p.CoreMinRatio, coreMinH: float64(frameHeight) * p.CoreMinRatio,
	}
	logo, share, ok := pickLogo(findBlobs(strong, region.W, region.H, p.DilateRadius), limits, p)
	r.Share = share
	switch {
	case !ok:
		r.Err = ErrNoCore

		return r
	case share < p.LogoShareMin:
		r.Err = ErrSharedCorner

		return r
	}

	logo = trim(strong, region.W, logo, p.TrimRatio)
	logo = withMargin(logo, region.W, region.H, p)
	logo.X += region.X
	logo.Y += region.Y
	r.Box = logo

	return r
}

// blobLimits は、塊の大きさの条件 (画素)。
type blobLimits struct {
	tooLargeW, tooLargeH float64
	coreMinW, coreMinH   float64
}

func (l blobLimits) tooLarge(b Box) bool {
	return float64(b.W) > l.tooLargeW || float64(b.H) > l.tooLargeH
}

// pickLogo は、塊 blobs から、ロゴの枠と、括った塊の強い縁が張り合う量に占める割合を返す。核にできる塊がなければ false を返す。
// 大きすぎる塊 (テロップの帯や黒帯の縁) は、核を選ぶ前に比較から外す。核を選んでから大きさで弾くと、隣のロゴを見ずに打ち切るからである。
// 図形と文字が離れたロゴのため、核の近くの、十分に強い塊を括る。
func pickLogo(blobs []blob, limits blobLimits, p *Params) (Box, float64, bool) {
	competing := 0
	core := -1
	for i := range blobs {
		b := &blobs[i]
		b.large = limits.tooLarge(b.box)
		if b.large {
			continue
		}
		competing += b.strong
		if float64(b.box.W) >= limits.coreMinW && float64(b.box.H) >= limits.coreMinH && (core < 0 || b.strong > blobs[core].strong) {
			core = i
		}
	}
	if core < 0 {
		return Box{X: 0, Y: 0, W: 0, H: 0}, 0, false
	}
	logo := blobs[core].box
	bundled := blobs[core].strong
	minStrong := int(float64(blobs[core].strong) * p.BundleStrengthRatio)
	for i, b := range blobs {
		if i == core || b.large || b.strong < minStrong || logo.distance(b.box) > p.BundleDistance {
			continue
		}
		if merged := logo.union(b.box); !limits.tooLarge(merged) {
			logo = merged
			bundled += b.strong
		}
	}

	return logo, float64(bundled) / float64(competing), true
}

// findBlobs は、印 mask (width × height) を radius の正方形で太らせ、4 近傍でつながった塊に分ける。
// 塊の枠は、太らせた分と Sobel が縁の外に出す 1 画素の分 (radius + 1) だけ四方から縮め、幅と高さは最低 1 画素残す。
// 塊の強い縁は、太らせる前の印の数。
func findBlobs(mask []bool, width, height, radius int) []blob {
	dilated := dilate(mask, width, height, radius)
	label := make([]bool, len(mask))
	blobs := []blob{}
	stack := []int{}
	for start := range dilated {
		if !dilated[start] || label[start] {
			continue
		}
		minX, minY, maxX, maxY := width, height, -1, -1
		strong := 0
		label[start] = true
		stack = append(stack[:0], start)
		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := i%width, i/width
			minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
			if mask[i] {
				strong++
			}
			stack = appendNeighbors(stack, dilated, label, width, height, x, y)
		}
		box := shrink(Box{X: minX, Y: minY, W: maxX - minX + 1, H: maxY - minY + 1}, radius+1)
		blobs = append(blobs, blob{box: box, strong: strong, large: false})
	}

	return blobs
}

// dilate は、印 mask を radius の正方形で太らせる。
func dilate(mask []bool, width, height, radius int) []bool {
	dilated := make([]bool, len(mask))
	for i, on := range mask {
		if !on {
			continue
		}
		x, y := i%width, i/width
		for ny := max(y-radius, 0); ny <= min(y+radius, height-1); ny++ {
			for nx := max(x-radius, 0); nx <= min(x+radius, width-1); nx++ {
				dilated[ny*width+nx] = true
			}
		}
	}

	return dilated
}

// appendNeighbors は、(x, y) の上下左右のうち、印があってまだ数えていない画素を、数えたことにして stack に足す。
func appendNeighbors(stack []int, dilated, label []bool, width, height, x, y int) []int {
	for _, n := range [4][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}} {
		if n[0] < 0 || n[1] < 0 || n[0] >= width || n[1] >= height {
			continue
		}
		if j := n[1]*width + n[0]; dilated[j] && !label[j] {
			label[j] = true
			stack = append(stack, j)
		}
	}

	return stack
}

// shrink は、b を四方から n 画素ずつ縮める。幅と高さは最低 1 画素残す。
func shrink(b Box, n int) Box {
	w := max(b.W-2*n, 1)
	h := max(b.H-2*n, 1)

	return Box{X: b.X + (b.W-w)/2, Y: b.Y + (b.H-h)/2, W: w, H: h}
}

// trim は、枠 b の中の強い縁を列ごと・行ごとに数え、上下左右のそれぞれの端から、合計の ratio までを切り落とす。
func trim(strong []bool, width int, b Box, ratio float64) Box {
	cols := make([]int, b.W)
	rows := make([]int, b.H)
	total := 0
	for y := range b.H {
		for x := range b.W {
			if strong[(b.Y+y)*width+b.X+x] {
				cols[x]++
				rows[y]++
				total++
			}
		}
	}
	limit := int(float64(total) * ratio)
	left, right := cut(cols, limit)
	top, bottom := cut(rows, limit)

	return Box{X: b.X + left, Y: b.Y + top, W: max(b.W-left-right, 1), H: max(b.H-top-bottom, 1)}
}

// cut は、counts の前と後ろから、和が limit を超えない所まで切り落とす数を返す。
func cut(counts []int, limit int) (int, int) {
	front, sum := 0, 0
	for front < len(counts)-1 && sum+counts[front] <= limit {
		sum += counts[front]
		front++
	}
	back, sum := 0, 0
	for back < len(counts)-1-front && sum+counts[len(counts)-1-back] <= limit {
		sum += counts[len(counts)-1-back]
		back++
	}

	return front, back
}

// withMargin は、b の四方に max(MarginMin, 大きさ × MarginRatio) の余白を足し、width × height の内側に収める。
func withMargin(b Box, width, height int, p *Params) Box {
	mx := max(p.MarginMin, int(math.Ceil(float64(b.W)*p.MarginRatio)))
	my := max(p.MarginMin, int(math.Ceil(float64(b.H)*p.MarginRatio)))
	x0, y0 := max(b.X-mx, 0), max(b.Y-my, 0)
	x1, y1 := min(b.right()+mx, width), min(b.bottom()+my, height)

	return Box{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// quantile は、昇順に並べた sorted の q 分位 (0〜1) を返す。空なら 0。
func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Round(q * float64(len(sorted)-1)))

	return sorted[min(max(i, 0), len(sorted)-1)]
}
