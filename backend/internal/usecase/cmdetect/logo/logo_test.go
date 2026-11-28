package logo_test

import (
	"math"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

// フレームの大きさは、実放送と同じく、ロゴの縁が隅の 1% ほどになる大きさにする。
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
	// randomBlocks は、フレームごとに向きの変わる波の背景。縁の向きは、フレームをまたぐと打ち消し合う。
	randomBlocks background = iota
	// bright は、白っぽい背景。
	bright
	// stripes は、細かい縦縞の背景。ロゴがなくても縁の向きが揃う。
	stripes
	// black は、真っ黒の背景。
	black
)

// canvas は、フレームの絵を作る。
type canvas struct {
	img *logo.Gray
}

// newCanvas は、bg のフレームを作る。波の背景は上下の帯の行にだけ描く (paintWaves)。ほかの背景は軽いので、すべての行に描く。
func newCanvas(bg background, rng *rng) *canvas {
	img := &logo.Gray{Width: frameW, Height: frameH, Pix: make([]byte, frameW*frameH)}
	switch bg {
	case randomBlocks:
		paintWaves(img, rng)
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

// paintWaves は、img に波の背景 (randomBlocks) を描く。
// テストが読む上下の帯の行にだけ描き、帯の外の行は黒のまま残す。テストは帯しか読まないので、結果は全体に描いたフレームと変わらない (#1941)。
func paintWaves(img *logo.Gray, rng *rng) {
	h := logo.Band(frameH, logo.DefaultParams())
	// 向きと細かさと位相がフレームごとにでたらめな、3 つの波を重ねる。縁の向きは、フレームをまたぐと打ち消し合う。
	// 画素ごとに sin を求めると、テストの時間のほとんどを占める (#1941)。
	// sin(kx·x + ky·y + φ) = sin(kx·x)·cos(ky·y + φ) + cos(kx·x)·sin(ky·y + φ) と x と y に分け、
	// x の側はカラムごとのテーブルを前もって作り、y の側は行ごとに求める。
	type wave struct{ kx, ky, phase float64 }
	waves := [3]wave{}
	for i := range waves {
		angle := rng.Float64() * 2 * math.Pi
		freq := 0.08 + rng.Float64()*0.2
		waves[i] = wave{kx: freq * math.Cos(angle), ky: freq * math.Sin(angle), phase: rng.Float64() * 2 * math.Pi}
	}
	// sinCos は、波ごとの sin と cos。
	type sinCos [3]struct{ sin, cos float64 }
	columns := make([]sinCos, frameW)
	for x := range columns {
		for i, w := range waves {
			columns[x][i].sin, columns[x][i].cos = math.Sincos(w.kx * float64(x))
		}
	}
	for y := range frameH {
		if h <= y && y < frameH-h {
			continue
		}
		var r sinCos
		for i, w := range waves {
			r[i].sin, r[i].cos = math.Sincos(w.ky*float64(y) + w.phase)
		}
		row := img.Pix[y*frameW : (y+1)*frameW]
		for x, c := range columns {
			v := 100.0
			for i := range waves {
				v += 30 * (c[i].sin*r[i].cos + c[i].cos*r[i].sin)
			}
			row[x] = byte(min(max(v, 0), 255))
		}
	}
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

// learnFrames は、学習のテストで積算するフレームの数。
const learnFrames = 120

// learn は、frame で作る learnFrames 枚のフレームから上下の帯を積算して学習する。
// フレームは作るたびに積算し、蓄積しない。蓄積すると、並列に走るテストがそれぞれ何十 MB も持ち続ける (#1941)。
func learn(t *testing.T, p *logo.Params, frame func(i int) *logo.Gray) (*logo.LearnResult, *logo.Accumulator, error) {
	t.Helper()

	h := logo.Band(frameH, p)
	// paintWaves は、既定値の帯の行にしか波の背景を描かない。帯の高さを変えると、描いていない黒い行を読んでしまう。
	require.Equal(t, logo.Band(frameH, logo.DefaultParams()), h, "the band height must match the one paintWaves paints")
	top, bottom := logo.NewAccumulator(frameW, h), logo.NewAccumulator(frameW, h)
	for i := range learnFrames {
		f := frame(i)
		top.Add(band(f, h, true), p.EdgeStrength)
		bottom.Add(band(f, h, false), p.EdgeStrength)
	}
	result, err := logo.Learn(top, bottom, frameW, frameH, p)

	return result, top, err
}

// TestCoherence は、ロゴの縁では揃い方がロゴの出ていた割合に近く、フレームごとに変わる背景の縁では 0 に近いことを確認する。
// 背景が暗くても明るくても、白いロゴの縁の向きは同じに数える (2 倍角)。
func TestCoherence(t *testing.T) {
	t.Parallel()

	p := logo.DefaultParams()
	rng := newRNG(12)
	// ロゴの乗る上の帯だけを積算する。
	h := logo.Band(frameH, p)
	acc := logo.NewAccumulator(frameW, h)
	for i := range 100 {
		bg := randomBlocks
		if i%4 == 0 {
			bg = bright
		}
		c := newCanvas(bg, rng)
		c.drawLogo()
		acc.Add(band(c.img, h, true), p.EdgeStrength)
	}
	coherence := acc.Coherence()
	// ロゴの左の縦棒の左の縁。
	edge := (logoBox().Y+4)*frameW + logoBox().X
	assert.Greater(t, coherence[edge], 0.7)
	// 背景。
	assert.Less(t, coherence[60*frameW+400], 0.3)
}

// TestLearn は、右上のロゴの枠を割り出し、枠がロゴを囲むことを確認する。
func TestLearn(t *testing.T) {
	t.Parallel()

	p := logo.DefaultParams()
	rng := newRNG(34)
	result, _, err := learn(t, p, func(int) *logo.Gray {
		c := newCanvas(randomBlocks, rng)
		c.drawLogo()

		return c.img
	})
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

// TestLearnGivesUp は、ロゴと張り合う動かない縁がある隅や、ロゴのない録画では、学習を諦めることを確認する。
func TestLearnGivesUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		draw     func(c *canvas, frame int)
		wantErr  error
		wantWhy  error
		wantSeen int
	}{
		{
			name:    "ロゴがなければ、どの隅も諦める",
			draw:    func(*canvas, int) {},
			wantErr: logo.ErrNoLogo, wantWhy: logo.ErrWeakCorner, wantSeen: 4,
		},
		{
			name: "ロゴの隣に、同じくらい強い動かない縁 (出し続けるテロップ) があれば、その隅を諦める",
			draw: func(c *canvas, _ int) {
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
		{
			name: "ロゴの形のものが、隅を横切る帯 (番組の話題の帯) の端にあれば、その隅を諦める",
			draw: func(c *canvas, frame int) {
				// 帯の端のアイコン。帯の中の文字は変わるので、動かない縁はアイコンに集まる。
				c.drawLogo()
				// 帯の上と下の縁の線。アイコンの行を通って、画面の中央の側へ隅の外まで伸びる。
				// 帯の出ないフレームもあるので、アイコンより揃い方が弱く、強い縁にはならない。
				if frame%2 == 0 {
					c.fill(logo.Box{X: 400, Y: logoBox().Y + 2, W: logoBox().X - 400, H: 2})
					c.fill(logo.Box{X: 400, Y: logoBox().Y + logoBox().H - 4, W: logoBox().X - 400, H: 2})
				}
			},
			wantErr: logo.ErrNoLogo, wantWhy: logo.ErrBandCorner, wantSeen: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := logo.DefaultParams()
			rng := newRNG(56)
			result, _, err := learn(t, p, func(i int) *logo.Gray {
				c := newCanvas(randomBlocks, rng)
				tt.draw(c, i)

				return c.img
			})
			require.ErrorIs(t, err, tt.wantErr)
			require.Len(t, result.Corners, tt.wantSeen)
			assert.ErrorIs(t, result.Corners[0].Err, tt.wantWhy)
		})
	}
}

// mirror は、img を左右に反転する。右上のロゴを左上に移すのに使う。
func mirror(img *logo.Gray) {
	for y := range img.Height {
		row := img.Pix[y*img.Width : (y+1)*img.Width]
		for i, j := 0, len(row)-1; i < j; i, j = i+1, j-1 {
			row[i], row[j] = row[j], row[i]
		}
	}
}

// TestLearnBandCorner は、ロゴの形のものの行を、隅を横切る帯の線が通るときだけ、その隅を諦めることを確認する (#1883)。
// 帯の線は、帯の出ないフレームもあるので、半分のフレームで出す。
func TestLearnBandCorner(t *testing.T) {
	t.Parallel()

	b := logoBox()
	tests := []struct {
		name string
		// line は、帯の線 (フレームの右上にロゴがあるときの座標)。
		line   logo.Box
		mirror bool
		// wantErr が nil なら、ロゴの隅 wantCorner を採る。nil でなければ、ロゴの隅を wantWhy で諦める。
		wantErr    error
		wantCorner logo.Corner
		wantWhy    error
	}{
		{
			name: "左上でも、画面の中央の側 (右) へ伸びる帯の端なら諦める",
			line: logo.Box{X: 400, Y: b.Y + 2, W: b.X - 400, H: 2}, mirror: true,
			wantErr: logo.ErrNoLogo, wantCorner: logo.TopLeft, wantWhy: logo.ErrBandCorner,
		},
		{
			name:    "線が隅の端まで届かなければ、帯の端とみなさない",
			line:    logo.Box{X: b.X - 80, Y: b.Y + 2, W: 80, H: 2},
			wantErr: nil, wantCorner: logo.TopRight, wantWhy: nil,
		},
		{
			name:    "帯の線がロゴの行を通らず、すぐ下を通るなら諦めない",
			line:    logo.Box{X: 400, Y: b.Y + b.H + 6, W: frameW - 400, H: 2},
			wantErr: nil, wantCorner: logo.TopRight, wantWhy: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := logo.DefaultParams()
			rng := newRNG(1883)
			result, _, err := learn(t, p, func(i int) *logo.Gray {
				c := newCanvas(randomBlocks, rng)
				c.drawLogo()
				if i%2 == 0 {
					c.fill(tt.line)
				}
				if tt.mirror {
					mirror(c.img)
				}

				return c.img
			})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				i := slices.IndexFunc(result.Corners, func(c logo.CornerResult) bool { return c.Corner == tt.wantCorner })
				require.GreaterOrEqual(t, i, 0, "%+v", result)
				assert.ErrorIs(t, result.Corners[i].Err, tt.wantWhy, "%+v", result)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantCorner, result.Corner, "%+v", result)
		})
	}
}

// TestLearnWideBand は、隅の幅のほとんどを占める横長のものは、帯の線を見る幅が枠より狭くても、線が埋めれば諦めることを確認する (#1883)。
// 横長の枠は、それ自体が帯であることが多い (2026 年 11 月の録画で、見る幅が狭いときに諦めないようにすると、隅の帯を採った)。
// 揃い方を直に置いた積算で、枠と線を決まった形にする。
func TestLearnWideBand(t *testing.T) {
	t.Parallel()

	p := logo.DefaultParams()
	h := logo.Band(frameH, p)
	top, bottom := logo.NewAccumulator(frameW, h), logo.NewAccumulator(frameW, h)
	const frames = 10
	top.N, bottom.N = frames, frames
	set := func(x, y int, coherence float64) { top.Sx[y*frameW+x] = float32(coherence * frames) }
	// 右上の隅 (672〜960) の内側の端の 20 画素先から、幅 230 の横長の枠の縁 (太さ 3 画素)。揃い方は 1 (いつも出ている)。
	plate := logo.Box{X: 692, Y: 30, W: 230, H: 16}
	for y := plate.Y; y < plate.Y+plate.H; y++ {
		for x := plate.X; x < plate.X+plate.W; x++ {
			if x < plate.X+3 || x >= plate.X+plate.W-3 || y < plate.Y+3 || y >= plate.Y+plate.H-3 {
				set(x, y, 1)
			}
		}
	}
	// 枠の行を通って、画面の中央の側へ隅の外まで伸びる帯の線。揃い方は 0.5 (半分のフレームで出る)。
	for x := 400; x < plate.X; x++ {
		set(x, plate.Y+8, 0.5)
	}
	result, err := logo.Learn(top, bottom, frameW, frameH, p)
	require.ErrorIs(t, err, logo.ErrNoLogo)
	assert.ErrorIs(t, result.Corners[0].Err, logo.ErrBandCorner, "%+v", result)
}

// TestLearnSkipsLargeBlobs は、テロップの帯のような大きすぎるチャンクを比較から外し、隣のロゴを採ることを確認する。
func TestLearnSkipsLargeBlobs(t *testing.T) {
	t.Parallel()

	p := logo.DefaultParams()
	rng := newRNG(78)
	result, _, err := learn(t, p, func(i int) *logo.Gray {
		c := newCanvas(randomBlocks, rng)
		c.drawLogo()
		// 隅を横切る、テロップの帯の下の線。9 割のフレームで出す。
		if i%10 != 0 {
			c.fill(logo.Box{X: 0, Y: 80, W: frameW, H: 2})
		}

		return c.img
	})
	require.NoError(t, err)
	assert.Equal(t, logo.TopRight, result.Corner, "%+v", result)
	assert.Less(t, result.Box.Y+result.Box.H, 80, "%+v", result)
}

// TestMeasure は、ロゴの出ているフレームの点数が高く、ロゴのないフレームでは、細かい柄の背景でも点数が 0 に近いことを確認する。
func TestMeasure(t *testing.T) {
	t.Parallel()

	p := logo.DefaultParams()
	rng := newRNG(910)
	result, top, err := learn(t, p, func(int) *logo.Gray {
		c := newCanvas(randomBlocks, rng)
		c.drawLogo()

		return c.img
	})
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

// TestLearnBottomRight は、下の帯の隅のロゴも見つけ、枠をフレームの座標で返すことを確認する。
func TestLearnBottomRight(t *testing.T) {
	t.Parallel()

	p := logo.DefaultParams()
	rng := newRNG(1314)
	below := logo.Box{X: 860, Y: frameH - 50, W: 56, H: 20}
	result, _, err := learn(t, p, func(int) *logo.Gray {
		c := newCanvas(randomBlocks, rng)
		for i := range 3 {
			x := below.X + i*20
			c.fill(logo.Box{X: x, Y: below.Y, W: 4, H: below.H})
			c.fill(logo.Box{X: x, Y: below.Y + 8, W: 14, H: 4})
		}

		return c.img
	})
	require.NoError(t, err)
	assert.Equal(t, logo.BottomRight, result.Corner)
	b := result.Box
	assert.LessOrEqual(t, b.Y, below.Y)
	assert.GreaterOrEqual(t, b.Y+b.H, below.Y+below.H)
	assert.LessOrEqual(t, b.Y+b.H, frameH)
}

// TestNewTemplateTooFewPoints は、型の点が少なすぎれば、その回はロゴを使わないことを確認する。
func TestNewTemplateTooFewPoints(t *testing.T) {
	t.Parallel()

	p := logo.DefaultParams()
	p.MinTemplatePoints = 100000
	rng := newRNG(1516)
	result, top, err := learn(t, p, func(int) *logo.Gray {
		c := newCanvas(randomBlocks, rng)
		c.drawLogo()

		return c.img
	})
	require.NoError(t, err)
	_, err = logo.NewTemplate(top.Crop(result.Box), result.Box, frameW, frameH, p)
	assert.ErrorIs(t, err, logo.ErrFewPoints)
}
