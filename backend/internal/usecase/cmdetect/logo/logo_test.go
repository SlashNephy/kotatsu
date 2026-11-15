package logo_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

// コマの大きさは、実放送と同じく、ロゴの縁が隅の 1% ほどになる大きさにする。
const (
	frameW = 960
	frameH = 540
)

// rng は、テストの絵を毎回同じに作る、決まった種から始める xorshift の乱数。
type rng struct{ state uint64 }

func newRNG(seed uint64) *rng { return &rng{state: seed*2654435761 + 1} }

// Float64 は、[0, 1) の値を返す。
func (r *rng) Float64() float64 { return float64(r.next()>>11) / (1 << 53) }

// Pick は、values のどれかを返す。
func (r *rng) Pick(values []byte) byte { return values[r.next()%uint64(len(values))] }

func (r *rng) next() uint64 {
	r.state ^= r.state << 13
	r.state ^= r.state >> 7
	r.state ^= r.state << 17

	return r.state
}

// background は、背景の種類。
type background int

const (
	// randomBlocks は、コマごとに向きの変わる波の背景。縁の向きは、コマをまたぐと打ち消し合う。
	randomBlocks background = iota
	// bright は、白っぽい背景。
	bright
	// stripes は、細かい縦縞の背景。ロゴがなくても縁の向きが揃う。
	stripes
	// black は、真っ黒の背景。
	black
)

// canvas は、コマの絵を作る。
type canvas struct {
	img *logo.Gray
}

func newCanvas(bg background, rng *rng) *canvas {
	img := &logo.Gray{Width: frameW, Height: frameH, Pix: make([]byte, frameW*frameH)}
	switch bg {
	case randomBlocks:
		// 向きと細かさと位相がコマごとにでたらめな、3 つの波を重ねる。縁の向きは、コマをまたぐと打ち消し合う。
		type wave struct{ kx, ky, phase float64 }
		waves := [3]wave{}
		for i := range waves {
			angle := rng.Float64() * 2 * math.Pi
			freq := 0.08 + rng.Float64()*0.2
			waves[i] = wave{kx: freq * math.Cos(angle), ky: freq * math.Sin(angle), phase: rng.Float64() * 2 * math.Pi}
		}
		for y := range frameH {
			for x := range frameW {
				v := 100.0
				for _, w := range waves {
					v += 30 * math.Sin(w.kx*float64(x)+w.ky*float64(y)+w.phase)
				}
				img.Pix[y*frameW+x] = byte(min(max(v, 0), 255))
			}
		}
	case bright:
		// 白いロゴ (240) とほとんど同じ明るさ。半透明のロゴが白っぽい背景で見えなくなるのに当たる。
		noise := []byte{238, 239, 240, 241, 242}
		for i := range img.Pix {
			img.Pix[i] = rng.Pick(noise)
		}
	case stripes:
		for y := range frameH {
			for x := range frameW {
				img.Pix[y*frameW+x] = byte(60 + 120*((x/2)%2))
			}
		}
	case black:
	}

	return &canvas{img: img}
}

// fill は、box を白 (ロゴの明るさ) で塗る。
func (c *canvas) fill(b logo.Box) {
	for y := b.Y; y < b.Y+b.H; y++ {
		for x := b.X; x < b.X+b.W; x++ {
			c.img.Pix[y*frameW+x] = logoLuma
		}
	}
}

// logoLuma は、ロゴの明るさ。
const logoLuma = 240

// logoBox は、右上のロゴの在り処。
func logoBox() logo.Box { return logo.Box{X: 860, Y: 24, W: 56, H: 20} }

// drawLogo は、右上に、白い文字のような形のロゴを描く。
func (c *canvas) drawLogo() {
	b := logoBox()
	// 3 文字の縦棒と横棒。
	for i := range 3 {
		x := b.X + i*20
		c.fill(logo.Box{X: x, Y: b.Y, W: 4, H: b.H})
		c.fill(logo.Box{X: x, Y: b.Y + 8, W: 14, H: 4})
	}
}

// band は、img の上端 (top) か下端の、高さ h の帯を返す。
func band(img *logo.Gray, h int, top bool) *logo.Gray {
	y0 := 0
	if !top {
		y0 = img.Height - h
	}

	return &logo.Gray{Width: img.Width, Height: h, Pix: img.Pix[y0*img.Width : (y0+h)*img.Width]}
}

// crop は、img の b の範囲を返す。
func crop(img *logo.Gray, b logo.Box) *logo.Gray {
	out := &logo.Gray{Width: b.W, Height: b.H, Pix: make([]byte, b.W*b.H)}
	for y := range b.H {
		copy(out.Pix[y*b.W:(y+1)*b.W], img.Pix[(b.Y+y)*img.Width+b.X:(b.Y+y)*img.Width+b.X+b.W])
	}

	return out
}

// learn は、コマ frames から上下の帯を積算して学習する。
func learn(t *testing.T, frames []*logo.Gray, p *logo.Params) (*logo.LearnResult, *logo.Accumulator, error) {
	t.Helper()

	h := logo.Band(frameH, p)
	top, bottom := logo.NewAccumulator(frameW, h), logo.NewAccumulator(frameW, h)
	for _, f := range frames {
		top.Add(band(f, h, true), p.EdgeStrength)
		bottom.Add(band(f, h, false), p.EdgeStrength)
	}
	result, err := logo.Learn(top, bottom, frameW, frameH, p)

	return result, top, err
}

// TestCoherence は、ロゴの縁では揃い方がロゴの出ていた割合に近く、コマごとに変わる背景の縁では 0 に近いことを確かめる。
// 背景が暗くても明るくても、白いロゴの縁の向きは同じに数える (2 倍角)。
func TestCoherence(t *testing.T) {
	t.Parallel()

	p := logo.DefaultParams()
	rng := newRNG(12)
	acc := logo.NewAccumulator(frameW, frameH)
	for i := range 100 {
		bg := randomBlocks
		if i%4 == 0 {
			bg = bright
		}
		c := newCanvas(bg, rng)
		c.drawLogo()
		acc.Add(c.img, p.EdgeStrength)
	}
	coherence := acc.Coherence()
	// ロゴの左の縦棒の左の縁。
	edge := (logoBox().Y+4)*frameW + logoBox().X
	assert.Greater(t, coherence[edge], 0.7)
	// 背景。
	assert.Less(t, coherence[300*frameW+400], 0.3)
}

// TestLearn は、右上のロゴの枠を割り出し、枠がロゴを囲むことを確かめる。
func TestLearn(t *testing.T) {
	t.Parallel()

	p := logo.DefaultParams()
	rng := newRNG(34)
	frames := make([]*logo.Gray, 0, 120)
	for range 120 {
		c := newCanvas(randomBlocks, rng)
		c.drawLogo()
		frames = append(frames, c.img)
	}
	result, _, err := learn(t, frames, p)
	require.NoError(t, err)
	assert.Equal(t, logo.TopRight, result.Corner)
	b := result.Box
	assert.LessOrEqual(t, b.X, logoBox().X)
	assert.LessOrEqual(t, b.Y, logoBox().Y)
	assert.GreaterOrEqual(t, b.X+b.W, logoBox().X+logoBox().W)
	assert.GreaterOrEqual(t, b.Y+b.H, logoBox().Y+logoBox().H)
	// 締めて余白を足した枠は、ロゴより大きく外れない。
	assert.Less(t, b.W, logoBox().W*2)
	assert.Less(t, b.H, logoBox().H*3)
}

// TestLearnGivesUp は、ロゴと張り合う動かない縁がある隅や、ロゴのない録画では、学習を諦めることを確かめる。
func TestLearnGivesUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		draw     func(c *canvas)
		wantErr  error
		wantWhy  error
		wantSeen int
	}{
		{
			name:    "ロゴがなければ、どの隅も諦める",
			draw:    func(*canvas) {},
			wantErr: logo.ErrNoLogo, wantWhy: logo.ErrWeakCorner, wantSeen: 4,
		},
		{
			name: "ロゴの隣に、同じくらい強い動かない縁 (出し続けるテロップ) があれば、その隅を諦める",
			draw: func(c *canvas) {
				c.drawLogo()
				// ロゴと同じ形で同じ強さのものを、括らない距離だけ左に。
				for i := range 3 {
					x := logoBox().X - 120 + i*20
					c.fill(logo.Box{X: x, Y: logoBox().Y, W: 4, H: logoBox().H})
					c.fill(logo.Box{X: x, Y: logoBox().Y + 8, W: 14, H: 4})
				}
			},
			wantErr: logo.ErrNoLogo, wantWhy: logo.ErrSharedCorner, wantSeen: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := logo.DefaultParams()
			rng := newRNG(56)
			frames := make([]*logo.Gray, 0, 120)
			for range 120 {
				c := newCanvas(randomBlocks, rng)
				tt.draw(c)
				frames = append(frames, c.img)
			}
			result, _, err := learn(t, frames, p)
			require.ErrorIs(t, err, tt.wantErr)
			require.Len(t, result.Corners, tt.wantSeen)
			assert.ErrorIs(t, result.Corners[0].Err, tt.wantWhy)
		})
	}
}

// TestLearnSkipsLargeBlobs は、テロップの帯のような大きすぎる塊を比較から外し、隣のロゴを採ることを確かめる。
func TestLearnSkipsLargeBlobs(t *testing.T) {
	t.Parallel()

	p := logo.DefaultParams()
	rng := newRNG(78)
	frames := make([]*logo.Gray, 0, 120)
	for i := range 120 {
		c := newCanvas(randomBlocks, rng)
		c.drawLogo()
		// 隅を横切る、テロップの帯の下の線。9 割のコマで出す。
		if i%10 != 0 {
			c.fill(logo.Box{X: 0, Y: 80, W: frameW, H: 2})
		}
		frames = append(frames, c.img)
	}
	result, _, err := learn(t, frames, p)
	require.NoError(t, err)
	assert.Equal(t, logo.TopRight, result.Corner, "%+v", result)
	assert.Less(t, result.Box.Y+result.Box.H, 80, "%+v", result)
}

// TestMeasure は、ロゴの出ているコマの点数が高く、ロゴのないコマでは、細かい柄の背景でも点数が 0 に近いことを確かめる。
func TestMeasure(t *testing.T) {
	t.Parallel()

	p := logo.DefaultParams()
	rng := newRNG(910)
	frames := make([]*logo.Gray, 0, 120)
	for range 120 {
		c := newCanvas(randomBlocks, rng)
		c.drawLogo()
		frames = append(frames, c.img)
	}
	result, top, err := learn(t, frames, p)
	require.NoError(t, err)
	template, err := logo.NewTemplate(top.Crop(result.Box), result.Box, frameW, frameH, p)
	require.NoError(t, err)

	tests := []struct {
		name     string
		bg       background
		withLogo bool
		check    func(t *testing.T, m logo.Measure)
	}{
		{name: "暗い背景のロゴ", bg: randomBlocks, withLogo: true, check: func(t *testing.T, m logo.Measure) {
			t.Helper()
			assert.Greater(t, m.Score, 0.5)
		}},
		{name: "ロゴのない背景", bg: randomBlocks, withLogo: false, check: func(t *testing.T, m logo.Measure) {
			t.Helper()
			assert.Less(t, m.Score, 0.15)
		}},
		{name: "ロゴのない細かい縦縞", bg: stripes, withLogo: false, check: func(t *testing.T, m logo.Measure) {
			t.Helper()
			assert.Less(t, m.Score, 0.15)
		}},
		{
			name: "白っぽい背景の白いロゴは、点数が落ち、明るさが高い", bg: bright, withLogo: true,
			check: func(t *testing.T, m logo.Measure) {
				t.Helper()
				assert.Less(t, m.Score, 0.15)
				assert.Greater(t, m.Brightness, 220.0)
			},
		},
		{name: "真っ黒は、のっぺりしている", bg: black, withLogo: false, check: func(t *testing.T, m logo.Measure) {
			t.Helper()
			assert.Less(t, m.Texture, 1.0)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newCanvas(tt.bg, newRNG(1112))
			if tt.withLogo {
				c.drawLogo()
			}
			tt.check(t, template.Measure(crop(c.img, template.Crop)))
		})
	}
}

// TestLearnBottomRight は、下の帯の隅のロゴも見つけ、枠をコマの座標で返すことを確かめる。
func TestLearnBottomRight(t *testing.T) {
	t.Parallel()

	p := logo.DefaultParams()
	rng := newRNG(1314)
	frames := make([]*logo.Gray, 0, 120)
	below := logo.Box{X: 860, Y: frameH - 50, W: 56, H: 20}
	for range 120 {
		c := newCanvas(randomBlocks, rng)
		for i := range 3 {
			x := below.X + i*20
			c.fill(logo.Box{X: x, Y: below.Y, W: 4, H: below.H})
			c.fill(logo.Box{X: x, Y: below.Y + 8, W: 14, H: 4})
		}
		frames = append(frames, c.img)
	}
	result, _, err := learn(t, frames, p)
	require.NoError(t, err)
	assert.Equal(t, logo.BottomRight, result.Corner)
	b := result.Box
	assert.LessOrEqual(t, b.Y, below.Y)
	assert.GreaterOrEqual(t, b.Y+b.H, below.Y+below.H)
	assert.LessOrEqual(t, b.Y+b.H, frameH)
}

// TestNewTemplateTooFewPoints は、型の点が少なすぎれば、その回はロゴを使わないことを確かめる。
func TestNewTemplateTooFewPoints(t *testing.T) {
	t.Parallel()

	p := logo.DefaultParams()
	p.MinTemplatePoints = 100000
	rng := newRNG(1516)
	frames := make([]*logo.Gray, 0, 120)
	for range 120 {
		c := newCanvas(randomBlocks, rng)
		c.drawLogo()
		frames = append(frames, c.img)
	}
	result, top, err := learn(t, frames, p)
	require.NoError(t, err)
	_, err = logo.NewTemplate(top.Crop(result.Box), result.Box, frameW, frameH, p)
	assert.ErrorIs(t, err, logo.ErrFewPoints)
}
