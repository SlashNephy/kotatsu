package logo

import (
	"errors"
	"math"
	"slices"
)

// ErrFewPoints は、型の点が少なすぎて、その回はロゴを使えないことを表す (#1505 の C)。
var ErrFewPoints = errors.New("the logo has too few edge points")

// point は、型の点。位置は切り出し範囲の中の座標で、向きは、積算を正規化した 2 倍角の単位ベクトル。
type point struct {
	x, y   int
	cx, cy float64
}

// Template は、コマごとにロゴが出ているかを測る型 (#1505 の C)。
type Template struct {
	// Crop は、コマから切り出す範囲 (コマの座標)。ロゴの枠の四方を Shift + 1 画素広げたもの。
	Crop   Box
	points []point
	shift  int
	// edge は、縁とみなす強さの 2 乗。cos2 は、2 倍角の余弦の下限 (2c² − 1)。
	edge float64
	cos2 float64
}

// NewTemplate は、ロゴの枠 box (コマの座標) の積算 model から、型を作る。model は box の範囲を切り出したもの。
// frameWidth と frameHeight は、コマの大きさ。点が MinTemplatePoints 未満なら ErrFewPoints を返す。
func NewTemplate(model *Accumulator, box Box, frameWidth, frameHeight int, p *Params) (*Template, error) {
	coherence := model.Coherence()
	sorted := slices.Clone(coherence)
	slices.Sort(sorted)
	threshold := max(p.MinCornerCoherence, quantile(sorted, p.TemplateQuantile)*p.TemplatePointRatio)

	pad := p.Shift + 1
	crop := Box{X: max(box.X-pad, 0), Y: max(box.Y-pad, 0), W: 0, H: 0}
	crop.W = min(box.right()+pad, frameWidth) - crop.X
	crop.H = min(box.bottom()+pad, frameHeight) - crop.Y
	t := &Template{Crop: crop, points: []point{}, shift: p.Shift, edge: p.EdgeStrength * p.EdgeStrength, cos2: 2*p.AngleCos*p.AngleCos - 1}
	for y := range box.H {
		for x := range box.W {
			i := y*box.W + x
			if coherence[i] < threshold {
				continue
			}
			// ずらしても Sobel を掛けられる位置の点だけを使う。
			cx, cy := box.X+x-crop.X, box.Y+y-crop.Y
			if cx-pad < 0 || cy-pad < 0 || cx+pad >= crop.W || cy+pad >= crop.H {
				continue
			}
			sx, sy := float64(model.Sx[i]), float64(model.Sy[i])
			norm := math.Hypot(sx, sy)
			t.points = append(t.points, point{x: cx, y: cy, cx: sx / norm, cy: sy / norm})
		}
	}
	if len(t.points) < p.MinTemplatePoints {
		return t, ErrFewPoints
	}

	return t, nil
}

// Points は、型の点の数を返す。
func (t *Template) Points() int {
	return len(t.points)
}

// Measure は、1 コマの測り (#1505 の D)。
type Measure struct {
	// Score は、ロゴが出ているかの点数。型の点のうち向きの合う割合から、型を斜め 4 方向にずらしたときの割合の平均を引いたもの。
	// 背景の細かい柄はずらしても揃うので、引き算で消える。
	Score float64
	// Brightness は、型の点での画素値の平均 (0〜255)。
	Brightness float64
	// Texture は、切り出し範囲で、横に隣り合う画素の差の絶対値の平均 (0〜255)。
	Texture float64
}

// Measure は、切り出し範囲 crop (t.Crop の大きさのグレースケール) の測りを返す。
func (t *Template) Measure(crop *Gray) Measure {
	s := t.shift
	center := t.match(crop, 0, 0)
	shifted := (t.match(crop, -s, -s) + t.match(crop, s, -s) + t.match(crop, -s, s) + t.match(crop, s, s)) / 4
	brightness := 0.0
	for _, pt := range t.points {
		brightness += crop.at(pt.x, pt.y)
	}
	if len(t.points) > 0 {
		brightness /= float64(len(t.points))
	}
	texture, n := 0.0, 0
	for y := range crop.Height {
		for x := 1; x < crop.Width; x++ {
			texture += math.Abs(crop.at(x, y) - crop.at(x-1, y))
			n++
		}
	}
	if n > 0 {
		texture /= float64(n)
	}

	return Measure{Score: center - shifted, Brightness: brightness, Texture: texture}
}

// match は、型を (dx, dy) ずらしたとき、向きの合う点の割合を返す。強さが足りない点は外れとして数え、分母は型の全点にする。
func (t *Template) match(crop *Gray, dx, dy int) float64 {
	if len(t.points) == 0 {
		return 0
	}
	hits := 0
	for _, pt := range t.points {
		gx, gy := crop.sobel(pt.x+dx, pt.y+dy)
		m := gx*gx + gy*gy
		if m < t.edge || m == 0 {
			continue
		}
		// 2 倍角の単位ベクトルどうしの内積が、2 倍角の余弦。
		if ((gx*gx-gy*gy)*pt.cx+2*gx*gy*pt.cy)/m >= t.cos2 {
			hits++
		}
	}

	return float64(hits) / float64(len(t.points))
}
