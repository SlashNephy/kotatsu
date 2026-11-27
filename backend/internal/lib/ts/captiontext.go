package ts

import (
	"crypto/md5" //nolint:gosec // G501: 外字の置き換えのテーブルは画像の MD5 で検索する (設計書 7.4)。暗号には使わない。
	"encoding/hex"
	"math"
	"math/bits"
	"strings"

	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"
)

// 字幕の文を読む決まり (ARIB STD-B24 第 1 編 第 3 部、#2216、設計書 7.4)。
//
// 字幕の文は、ブラウザの字幕の履歴 (aribb24.js の TextRenderer) と同じ文にする。
//   - 文字の指示と呼び出しは、字幕文ごとに初めの状態から読む。初めは G0 が漢字、G1 が英数、G2 がひらがな、G3 がマクロで、
//     GL が G0、GR が G2 である。画面の消去 (CS) では戻さない。
//   - 小型 (SSZ) の文字は、ルビなので除く。中型 (MSZ) の文字は、半角にする。
//   - 字幕文の中で、文字を書く縦の位置が変わったら、改行 ("\n") を入れる。字幕文と字幕文の間には入れない。
//     書く位置は、aribb24.js の Parser と同じく画素で持ち (captionLayout)、文字の大きさ、字間と行間、表示領域から計算する。
//     行の終わりで折り返したときにも、縦の位置が変わるので改行を入れる。
//   - 画面の消去 (CS) は、時間の指定 (TIME) で待った後なら、文を消さない。
//   - 外字 (DRCS) は、画像の MD5 で置き換えの文字を引き、引けなければ「〓」にする。
//   - マクロの集合の 0x60〜0x6F は、既定のマクロ (captionDefaultMacros) として読む。

// ARIB STD-B24 の制御符号のうち、字幕で読むもの。
const (
	aribAPB = 0x08
	aribAPF = 0x09
	aribAPD = 0x0A
	aribAPU = 0x0B
	aribCS  = 0x0C
)

// captionUnknownDRCS は、置き換えの文字を引けなかった外字。aribb24.js の TextRenderer と同じ文字にする。
const captionUnknownDRCS = "〓"

// captionMaxMacroDepth は、マクロの中で呼び出したマクロを読む深さの上限。既定のマクロはマクロを呼ばないが、壊れたデータで止まらないようにする。
const captionMaxMacroDepth = 4

// captionDefaultMacro は、既定のマクロ文 (ARIB STD-B24 第 1 編 第 2 部) の 0x60〜0x6F のうち、code のものを返す。
// 集合の指示 (G0〜G3) と、LS0 と LS2R を並べたもの。値は規格のもので、aribb24.js (MIT) と libaribcaption (MIT) のテーブルと同じ (設計書 付録 C)。
func captionDefaultMacro(code uint16) []byte {
	macros := [16][]byte{
		{0x1B, 0x24, 0x42, 0x1B, 0x29, 0x4A, 0x1B, 0x2A, 0x30, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x24, 0x42, 0x1B, 0x29, 0x31, 0x1B, 0x2A, 0x30, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x24, 0x42, 0x1B, 0x29, 0x20, 0x41, 0x1B, 0x2A, 0x30, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x28, 0x32, 0x1B, 0x29, 0x34, 0x1B, 0x2A, 0x35, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x28, 0x32, 0x1B, 0x29, 0x33, 0x1B, 0x2A, 0x35, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x28, 0x32, 0x1B, 0x29, 0x20, 0x41, 0x1B, 0x2A, 0x35, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x28, 0x20, 0x41, 0x1B, 0x29, 0x20, 0x42, 0x1B, 0x2A, 0x20, 0x43, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x28, 0x20, 0x44, 0x1B, 0x29, 0x20, 0x45, 0x1B, 0x2A, 0x20, 0x46, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x28, 0x20, 0x47, 0x1B, 0x29, 0x20, 0x48, 0x1B, 0x2A, 0x20, 0x49, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x28, 0x20, 0x4A, 0x1B, 0x29, 0x20, 0x4B, 0x1B, 0x2A, 0x20, 0x4C, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x28, 0x20, 0x4D, 0x1B, 0x29, 0x20, 0x4E, 0x1B, 0x2A, 0x20, 0x4F, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x24, 0x42, 0x1B, 0x29, 0x20, 0x42, 0x1B, 0x2A, 0x30, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x24, 0x42, 0x1B, 0x29, 0x20, 0x43, 0x1B, 0x2A, 0x30, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x24, 0x42, 0x1B, 0x29, 0x20, 0x44, 0x1B, 0x2A, 0x30, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x28, 0x31, 0x1B, 0x29, 0x30, 0x1B, 0x2A, 0x4A, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
		{0x1B, 0x28, 0x4A, 0x1B, 0x29, 0x32, 0x1B, 0x2A, 0x20, 0x41, 0x1B, 0x2B, 0x20, 0x70, 0x0F, 0x1B, 0x7D},
	}

	return macros[code-0x60]
}

// drcsKey は、外字の符号。final は集合の終端の符号 (DRCS-0 は 0x40、DRCS-1〜15 は 0x41〜0x4F)、code は集合の中の符号。
type drcsKey struct {
	final byte
	code  uint16
}

// captionState は、字幕文 1 つを読む状態。
type captionState struct {
	// drcs は、字幕で送られた外字の画像の MD5 (小文字の 16 進)。
	drcs map[drcsKey]string
	// replaceDRCS は、外字の画像の MD5 から、置き換える文字を検索する。nil なら、外字はすべて「〓」にする。
	replaceDRCS func(md5 string) (string, bool)
	// layout は、文字を書く位置と、位置の計算に使う大きさ。
	layout captionLayout
	// lastY は、最後に文字を書いた縦の位置。written は、字幕文の中で文字を書いたかどうか。
	lastY   float64
	written bool
	// cleared は、読んでいる字幕文が画面を消した (CS) かどうか。
	cleared bool
	// waited は、読んでいる字幕文が、時間の指定 (TIME) で待ったかどうか。待った後の画面の消去は、文を消さない。
	waited bool
	// macroDepth は、読んでいるマクロの深さ。
	macroDepth int
}

// newCaptionDecoder は、字幕の文を読むデコーダーを返す。replaceDRCS は、外字の画像の MD5 から置き換える文字を検索する。
func newCaptionDecoder(replaceDRCS func(md5 string) (string, bool)) *aribDecoder {
	return &aribDecoder{
		g: [4]aribGraphicSet{
			{set: aribSetKanji, bytes: 2, final: 0},
			{set: aribSetAlphanumeric, bytes: 1, final: 0},
			{set: aribSetHiragana, bytes: 1, final: 0},
			{set: aribSetMacro, bytes: 1, final: 0x70},
		},
		gl: 0, gr: 2, single: -1, middle: false, small: false,
		caption: &captionState{
			drcs: map[drcsKey]string{}, replaceDRCS: replaceDRCS, layout: newCaptionLayout(),
			lastY: 0, written: false, cleared: false, waited: false, macroDepth: 0,
		},
		out: strings.Builder{},
	}
}

// statement は、字幕文の本文 (データユニットの 0x20) を読み、書いた文と、画面を消したかを返す。
// 画面を消したときは、消す前に書いた文を破棄する。
func (d *aribDecoder) statement(b []byte) (string, bool) {
	d.out.Reset()
	d.caption.cleared = false
	d.decode(b)

	return d.out.String(), d.caption.cleared
}

// write は、字幕の文字 s を書き、書く位置を 1 文字進める。小型の文字は除き (位置は進める)、縦の位置が変わっていたら改行を入れる。
// aribb24.js と同じく、置き換えた外字は、何文字でも 1 文字として進める。
func (c *captionState) write(d *aribDecoder, s string) {
	defer c.layout.moveRelative(1, 0)
	if d.small {
		return
	}
	// 縦の位置が NaN (大きさが特殊1、特殊2) のときも、aribb24.js と同じく、変わったものとして改行を入れる。
	if c.written && c.layout.y != c.lastY {
		d.out.WriteByte('\n')
	}
	c.lastY, c.written = c.layout.y, true
	if d.middle {
		s = narrowCaption(s)
	}
	d.out.WriteString(s)
}

// captionCharacter は、字幕の外字かマクロの集合の文字を読む。
func (d *aribDecoder) captionCharacter(g aribGraphicSet, code uint16) {
	if g.set == aribSetMacro {
		if code < 0x60 || code > 0x6F || d.caption.macroDepth >= captionMaxMacroDepth {
			return
		}
		d.caption.macroDepth++
		d.decode(captionDefaultMacro(code))
		d.caption.macroDepth--

		return
	}
	digest, ok := d.caption.drcs[drcsKey{final: g.final, code: code}]
	if !ok {
		// 送られていない外字は、aribb24.js と同じく書かない。
		return
	}
	if d.caption.replaceDRCS != nil {
		if text, ok := d.caption.replaceDRCS(digest); ok && text != "" {
			d.write(text)

			return
		}
	}
	d.write(captionUnknownDRCS)
}

// captionC0 は、字幕でだけ読む C0 の制御符号を読み、読んだバイト数と true を返す。字幕で読まない符号なら false を返す。
func (d *aribDecoder) captionC0(b []byte) (int, bool) {
	c := d.caption
	switch b[0] {
	case aribCS:
		// aribb24.js は、画面を消しても、書く位置と、最後に書いた縦の位置を戻さない。
		if !c.waited {
			d.out.Reset()
			c.cleared = true
		}
	case aribAPB:
		c.layout.moveRelative(-1, 0)
	case aribAPF:
		c.layout.moveRelative(1, 0)
	case aribAPD:
		c.layout.moveRelative(0, 1)
	case aribAPU:
		c.layout.moveRelative(0, -1)
	case aribAPR:
		c.layout.x = 0
		c.layout.moveRelative(0, 1)
	case aribPAPF:
		if len(b) >= 2 {
			c.layout.moveRelative(float64(b[1]&0x3F), 0)
		}

		return min(2, len(b)), true
	case aribAPS:
		// 行と、行の中の位置 (それぞれ 0x40 を足した値) が続く。
		if len(b) >= 3 {
			c.layout.moveAbsolute(float64(b[2]&0x3F), float64(b[1]&0x3F))
		}

		return min(3, len(b)), true
	default:
		return 0, false
	}

	return 1, true
}

// defineDRCS は、外字のデータユニット (0x30 は 1 バイトの DRCS、0x31 は 2 バイトの DRCS) を読み、外字の画像の MD5 を保存する。
// 画像の MD5 は、aribb24.js と同じく、送られた画素の並び (1 画素あたり、階調の数の 2 を底とする対数のビット) から取る。
// 画素のビット数がバイトの区切りにそろわないとき、aribb24.js は端数のバイトを除いて MD5 を取るので、同じにする。
// 管理者が登録する外字の MD5 は、ブラウザが取ったものだからである。次の外字は、端数のバイトの後から読む。
// 圧縮した形 (幾何図形) の外字は読まない。
func (d *aribDecoder) defineDRCS(b []byte, twoBytes bool) {
	if len(b) < 1 {
		return
	}
	offset := 1
	for range int(b[0]) {
		if offset+3 > len(b) {
			return
		}
		characterCode := uint16(b[offset])<<8 | uint16(b[offset+1])
		fonts := int(b[offset+2])
		offset += 3
		for range fonts {
			if offset+4 > len(b) {
				return
			}
			mode := b[offset] & 0x0F
			if mode != 0 && mode != 1 {
				return
			}
			depth := int(b[offset+1]) + 2
			width, height := int(b[offset+2]), int(b[offset+3])
			pixelBits := width * height * bits.Len(uint(depth-1))
			length := (pixelBits + 7) / 8
			offset += 4
			if offset+length > len(b) {
				return
			}
			digest := md5.Sum(b[offset : offset+pixelBits/8]) //nolint:gosec // G401: 理由は import のとおり。
			offset += length
			key := drcsKey{final: 0x40, code: characterCode & 0x7F7F}
			if !twoBytes {
				key = drcsKey{final: byte(characterCode >> 8), code: characterCode & 0x7F}
			}
			d.caption.drcs[key] = hex.EncodeToString(digest[:])
		}
	}
}

// narrowCaption は、中型の文字を、aribb24.js の TextRenderer と同じく半角にする。濁点と半濁点の付いたカタカナは、半角の文字と
// 半角の濁点 (ﾞ) と半濁点 (ﾟ) に分ける。半角にない文字 (ひらがなや漢字) は、そのままにする。
func narrowCaption(s string) string {
	var out strings.Builder
	for _, r := range s {
		if decomposed := []rune(norm.NFD.String(string(r))); len(decomposed) == 2 {
			if base := width.Narrow.String(string(decomposed[0])); base != string(decomposed[0]) {
				switch decomposed[1] {
				case '\u3099':
					out.WriteString(base + "ﾞ")

					continue
				case '\u309A':
					out.WriteString(base + "ﾟ")

					continue
				}
			}
		}
		out.WriteString(width.Narrow.String(string(r)))
	}

	return out.String()
}

// 字幕で読む CSI の終端の文字。
const (
	aribSDF  = 0x56
	aribSSM  = 0x57
	aribSHS  = 0x58
	aribSVS  = 0x59
	aribSDP  = 0x5F
	aribACPS = 0x61
)

// csi は、字幕の CSI の制御符号 (b は CSI から終端の文字まで) のうち、書く位置の計算に使うものを読む。
// パラメーターは、aribb24.js のトークナイザーと同じく、0x20 と 0x3B で区切り、各バイトの下位 4 ビットを 10 進の桁として読む。
func (c *captionState) csi(b []byte) {
	params := []float64{0}
	var final byte
	for _, v := range b[1:] {
		if v == aribSP || v == 0x3B {
			params = append(params, 0)

			continue
		}
		if v&0x40 != 0 {
			final = v

			break
		}
		params[len(params)-1] = params[len(params)-1]*10 + float64(v&0x0F)
	}
	param := func(i int) float64 {
		if i < len(params) {
			return params[i]
		}

		return 0
	}
	l := &c.layout
	switch final {
	case aribSDF:
		l.areaW, l.areaH = param(0)*captionMagnification, param(1)*captionMagnification
	case aribSDP:
		l.setMargin(param(0)*captionMagnification, param(1)*captionMagnification)
	case aribSSM:
		l.fontW, l.fontH = param(0)*captionMagnification, param(1)*captionMagnification
	case aribSHS:
		l.hspace = param(0) * captionMagnification
	case aribSVS:
		l.vspace = param(0) * captionMagnification
	case aribACPS:
		l.moveAbsoluteDot(param(0)*captionMagnification, param(1)*captionMagnification)
	}
}

// sizeControl は、字幕の文字の大きさの制御符号 (SSZ、MSZ、NSZ、SZX) を読み、書く位置の計算に使う倍率を変える。
// 倍率は aribb24.js と同じ。特殊1と特殊2は、aribb24.js と同じく NaN にする。番組情報を読むとき (c が nil) は何もしない。
func (c *captionState) sizeControl(b []byte) {
	if c == nil {
		return
	}
	l := &c.layout
	switch b[0] {
	case aribSSZ:
		l.scaleX, l.scaleY = 0.5, 0.5
	case aribMSZ:
		l.scaleX, l.scaleY = 0.5, 1
	case aribNSZ:
		l.scaleX, l.scaleY = 1, 1
	case aribSZX:
		if len(b) < 2 {
			return
		}
		switch b[1] {
		case 0x60: // 超小型
			l.scaleX, l.scaleY = 1.0/4, 1.0/6
		case 0x41: // 縦倍
			l.scaleX, l.scaleY = 1, 2
		case 0x44: // 横倍
			l.scaleX, l.scaleY = 2, 1
		case 0x45: // 縦横倍
			l.scaleX, l.scaleY = 2, 2
		case 0x6B, 0x64: // 特殊1、特殊2
			l.scaleX, l.scaleY = math.NaN(), math.NaN()
		}
	}
}

// captionMagnification は、aribb24.js の Parser の倍率 (magnification) の既定値。TextRenderer は既定値で位置を計算するので、
// 位置の端数の切り捨てまで同じになるよう、同じ倍率で計算する。
const captionMagnification = 2

// captionLayout は、aribb24.js の Parser と同じく、文字を書く位置を、表示領域の左上からの画素で持つ (Kotatsu のパッチを当てたもの、
// 設計書 7.4)。値は、すべて captionMagnification 倍にしてある。
type captionLayout struct {
	// areaW と areaH は表示領域 (SDF) の大きさ、marginX と marginY は表示領域の位置 (SDP)。
	areaW, areaH, marginX, marginY float64
	// fontW と fontH は文字の大きさ (SSM)、hspace と vspace は字間 (SHS) と行間 (SVS)。
	fontW, fontH, hspace, vspace float64
	// scaleX と scaleY は、文字の大きさの制御 (SSZ、MSZ、NSZ、SZX) の倍率。
	scaleX, scaleY float64
	// x と y は、文字を書く位置。y は文字の下の端の位置。
	x, y float64
	// positioned は、字幕文の中で書く位置を決めたかどうか。決めた後の SDP では、位置を画面の上で動かさない。
	positioned bool
}

// newCaptionLayout は、aribb24.js の初めの状態 (initialState) を返す。
func newCaptionLayout() captionLayout {
	const m = captionMagnification

	return captionLayout{
		areaW: 960 * m, areaH: 540 * m, marginX: 0, marginY: 0,
		fontW: 36 * m, fontH: 36 * m, hspace: 4 * m, vspace: 24 * m,
		scaleX: 1, scaleY: 1,
		x: 0, y: 59 * m,
		positioned: false,
	}
}

// box は、文字 1 つが占める横と縦の大きさ (字間と行間を含む) を返す。
func (l *captionLayout) box() (float64, float64) {
	return math.Floor((l.fontW + l.hspace) * l.scaleX), math.Floor((l.fontH + l.vspace) * l.scaleY)
}

// moveAbsolute は、書く位置を、row 行目の col 文字目にする (APS)。
func (l *captionLayout) moveAbsolute(col, row float64) {
	w, h := l.box()
	l.positioned = true
	l.x, l.y = col*w, (row+1)*h-captionMagnification
}

// moveAbsoluteDot は、書く位置を、画面の上の画素 (x, y) にする (ACPS)。
func (l *captionLayout) moveAbsoluteDot(x, y float64) {
	l.positioned = true
	l.x, l.y = x-l.marginX, y-l.marginY
}

// moveRelative は、書く位置を、横に dx 文字、縦に dy 行動かす。進む向きの表示領域の端を越えたら、反対の端へ折り返す。
// 横に折り返したときは、行も動かす。
func (l *captionLayout) moveRelative(dx, dy float64) {
	w, h := l.box()
	l.positioned = true
	l.x += w * dx
	if wraps := math.Floor(l.x / areaOrInf(l.areaW)); dx > 0 && wraps > 0 || dx < 0 && wraps < 0 {
		l.x -= wraps * l.areaW
		dy += wraps
	}
	l.y += h * dy
	if wraps := math.Floor(l.y / areaOrInf(l.areaH)); dy > 0 && wraps > 0 || dy < 0 && wraps < 0 {
		l.y -= wraps * l.areaH
	}
}

// setMargin は、表示領域の位置を (x, y) にする (SDP)。書く位置を決めた後なら、画面の上の位置を保つよう、動いた分を戻す。
func (l *captionLayout) setMargin(x, y float64) {
	if l.positioned {
		l.x -= x - l.marginX
		l.y -= y - l.marginY
	}
	l.marginX, l.marginY = x, y
}

// areaOrInf は、表示領域の大きさ size を返す。大きさが 0 なら、折り返さないよう無限大を返す。
func areaOrInf(size float64) float64 {
	if size == 0 {
		return math.Inf(1)
	}

	return size
}
