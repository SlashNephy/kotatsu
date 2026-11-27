package ts

import (
	"crypto/md5"
	"encoding/hex"
	"math/bits"
	"strings"
)

// 字幕の文を読む決まり (ARIB STD-B24 第 1 編 第 3 部、#2216、設計書 7.4)。
//
// 字幕の文は、ブラウザの字幕の履歴 (aribb24.js の TextRenderer) と同じ文にする。
//   - 文字の指示と呼び出しは、字幕文をまたいで引き継ぐ。初めは G0 が漢字、G1 が英数、G2 がひらがな、G3 がマクロで、
//     GL が G0、GR が G2 である。aribb24.js と同じく、画面の消去 (CS) でも戻さない。
//   - 小型 (SSZ) の文字は、ルビなので除く。
//   - 書く行が変わったら、改行 ("\n") を入れる。行は、APR、APS、APD、APU で動かす。
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

// captionDefaultMacros は、既定のマクロ文 (ARIB STD-B24 第 1 編 第 2 部) の 0x60〜0x6F。
// 集合の指示 (G0〜G3) と、LS0 と LS2R を並べたもの。aribb24.js と libaribcaption の表と同じ。
var captionDefaultMacros = [16][]byte{
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

// drcsKey は、外字の符号。final は集合の終端の符号 (DRCS-0 は 0x40、DRCS-1〜15 は 0x41〜0x4F)、code は集合の中の符号。
type drcsKey struct {
	final byte
	code  uint16
}

// captionState は、字幕の文を読む状態。字幕文をまたいで引き継ぐ。
type captionState struct {
	// drcs は、字幕で送られた外字の画像の MD5 (小文字の 16 進)。
	drcs map[drcsKey]string
	// replaceDRCS は、外字の画像の MD5 から、置き換える文字を引く。nil なら、外字はすべて「〓」にする。
	replaceDRCS func(md5 string) (string, bool)
	// row は、書く位置の行。lastRow は、最後に文字を書いた行で、まだ書いていなければ -1。
	row, lastRow int
	// cleared は、読んでいる字幕文が画面を消した (CS) かどうか。
	cleared bool
	// macroDepth は、読んでいるマクロの深さ。
	macroDepth int
}

// newCaptionDecoder は、字幕の文を読むデコーダーを返す。replaceDRCS は、外字の画像の MD5 から置き換える文字を引く。
func newCaptionDecoder(replaceDRCS func(md5 string) (string, bool)) *aribDecoder {
	return &aribDecoder{
		g: [4]aribGraphicSet{
			{set: aribSetKanji, bytes: 2, final: 0},
			{set: aribSetAlphanumeric, bytes: 1, final: 0},
			{set: aribSetHiragana, bytes: 1, final: 0},
			{set: aribSetMacro, bytes: 1, final: 0x70},
		},
		gl: 0, gr: 2, single: -1, middle: false, small: false,
		caption: &captionState{drcs: map[drcsKey]string{}, replaceDRCS: replaceDRCS, row: 0, lastRow: -1, cleared: false, macroDepth: 0},
		out:     strings.Builder{},
	}
}

// statement は、字幕文の本文 (データユニットの 0x20) を読み、書いた文と、画面を消したかを返す。
// 画面を消したときは、消す前に書いた文を捨てる。
func (d *aribDecoder) statement(b []byte) (string, bool) {
	d.out.Reset()
	d.caption.cleared = false
	d.decode(b)

	return d.out.String(), d.caption.cleared
}

// write は、字幕の文字 s を書く。小型の文字は除き、行が変わっていたら改行を入れる。
func (c *captionState) write(d *aribDecoder, s string) {
	if d.small {
		return
	}
	if c.lastRow >= 0 && c.row != c.lastRow {
		d.out.WriteByte('\n')
	}
	c.lastRow = c.row
	d.out.WriteString(s)
}

// captionCharacter は、字幕の外字かマクロの集合の文字を読む。
func (d *aribDecoder) captionCharacter(g aribGraphicSet, code uint16) {
	if g.set == aribSetMacro {
		if code < 0x60 || code > 0x6F || d.caption.macroDepth >= captionMaxMacroDepth {
			return
		}
		d.caption.macroDepth++
		d.decode(captionDefaultMacros[code-0x60])
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
		d.out.Reset()
		c.cleared, c.row, c.lastRow = true, 0, -1
	case aribAPR, aribAPD:
		c.row++
	case aribAPU:
		c.row = max(0, c.row-1)
	case aribAPS:
		// 行と列 (それぞれ 0x40 を足した値) が続く。
		if len(b) >= 2 {
			c.row = int(b[1] & 0x3F)
		}

		return min(3, len(b)), true
	case aribAPB, aribAPF:
	default:
		return 0, false
	}

	return 1, true
}

// defineDRCS は、外字のデータユニット (0x30 は 1 バイトの DRCS、0x31 は 2 バイトの DRCS) を読み、外字の画像の MD5 を覚える。
// 画像の MD5 は、aribb24.js と同じく、送られた画素の並び (1 画素あたり、階調の数の 2 を底とする対数のビット) から取る。
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
			length := width * height * bits.Len(uint(depth-1)) / 8
			offset += 4
			if offset+length > len(b) {
				return
			}
			digest := md5.Sum(b[offset : offset+length])
			offset += length
			key := drcsKey{final: 0x40, code: characterCode & 0x7F7F}
			if !twoBytes {
				key = drcsKey{final: byte(characterCode >> 8), code: characterCode & 0x7F}
			}
			d.caption.drcs[key] = hex.EncodeToString(digest[:])
		}
	}
}
