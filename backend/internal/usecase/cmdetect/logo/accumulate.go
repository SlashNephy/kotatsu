package logo

import "math"

// Gray は、グレースケールの画像。Pix は、Width × Height の画素値 (0〜255) を行ごとに並べたもの。
type Gray struct {
	Width  int
	Height int
	Pix    []byte
}

// at は、(x, y) の画素値を返す。
func (g *Gray) at(x, y int) float64 {
	return float64(g.Pix[y*g.Width+x])
}

// sobel は、(x, y) の Sobel (3×3) の勾配を返す。(x, y) は外周 1 画素より内側にする。
func (g *Gray) sobel(x, y int) (float64, float64) {
	a, b, c := g.at(x-1, y-1), g.at(x, y-1), g.at(x+1, y-1)
	d, f := g.at(x-1, y), g.at(x+1, y)
	h, i, j := g.at(x-1, y+1), g.at(x, y+1), g.at(x+1, y+1)
	gx := (c + 2*f + j) - (a + 2*d + h)
	gy := (h + 2*i + j) - (a + 2*b + c)

	return gx, gy
}

// Accumulator は、縁の向きをコマをまたいで足し合わせたもの (#1505 の A)。
// 画素ごとに、縁とみなす強さ以上の勾配の、角を 2 倍にした単位ベクトルの和 (Sx, Sy) と、足したコマの数 N を持つ。
// 白いロゴが暗い背景にも明るい背景にも乗ると勾配の符号が反転するが、2 倍角なら θ と θ+π が同じベクトルになる。
// 和のまま持つので、録画をまたいで足していける。
type Accumulator struct {
	Width  int
	Height int
	Sx     []float32
	Sy     []float32
	N      int
}

// NewAccumulator は、width × height の空の Accumulator を返す。
func NewAccumulator(width, height int) *Accumulator {
	return &Accumulator{Width: width, Height: height, Sx: make([]float32, width*height), Sy: make([]float32, width*height), N: 0}
}

// Add は、img の縁の向きを足す。img は Accumulator と同じ大きさにする。外周 1 画素は数えない。
// 縁がなかった画素も、そのコマは N に数える。
func (a *Accumulator) Add(img *Gray, edgeStrength float64) {
	threshold := edgeStrength * edgeStrength
	for y := 1; y < a.Height-1; y++ {
		for x := 1; x < a.Width-1; x++ {
			gx, gy := img.sobel(x, y)
			m := gx*gx + gy*gy
			if m < threshold || m == 0 {
				continue
			}
			i := y*a.Width + x
			a.Sx[i] += float32((gx*gx - gy*gy) / m)
			a.Sy[i] += float32(2 * gx * gy / m)
		}
	}
	a.N++
}

// Merge は、b を a に足す。b は a と同じ大きさにする。
func (a *Accumulator) Merge(b *Accumulator) {
	for i := range a.Sx {
		a.Sx[i] += b.Sx[i]
		a.Sy[i] += b.Sy[i]
	}
	a.N += b.N
}

// Scale は、a を k 倍に薄める。型を育てるとき、古い分を比で薄めるのに使う (#1505 の G)。
func (a *Accumulator) Scale(k float64) {
	for i := range a.Sx {
		a.Sx[i] = float32(float64(a.Sx[i]) * k)
		a.Sy[i] = float32(float64(a.Sy[i]) * k)
	}
	a.N = int(math.Round(float64(a.N) * k))
}

// Coherence は、画素ごとの揃い方 √(Sx² + Sy²) / N (0〜1) を返す。ロゴの縁では、ロゴが出ていたコマの割合に近い値が残り、
// 本編の中身の縁は向きが打ち消し合って 0 に近づく。N が 0 なら、どの画素も 0 にする。
func (a *Accumulator) Coherence() []float64 {
	out := make([]float64, len(a.Sx))
	if a.N == 0 {
		return out
	}
	for i := range a.Sx {
		sx, sy := float64(a.Sx[i]), float64(a.Sy[i])
		out[i] = math.Sqrt(sx*sx+sy*sy) / float64(a.N)
	}

	return out
}

// Crop は、a の中の box の範囲を切り出した Accumulator を返す。
func (a *Accumulator) Crop(box Box) *Accumulator {
	out := NewAccumulator(box.W, box.H)
	for y := range box.H {
		for x := range box.W {
			src := (box.Y+y)*a.Width + box.X + x
			dst := y*box.W + x
			out.Sx[dst] = a.Sx[src]
			out.Sy[dst] = a.Sy[src]
		}
	}
	out.N = a.N

	return out
}
