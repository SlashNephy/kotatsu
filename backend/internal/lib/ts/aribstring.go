package ts

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
)

// ARIB STD-B24 第 1 編 第 2 部 7 章の 8 単位符号で使う、符号化の集合 (終端の符号)。
const (
	aribSetKanji             = 0x42
	aribSetAlphanumeric      = 0x4A
	aribSetHiragana          = 0x30
	aribSetKatakana          = 0x31
	aribSetMosaicA           = 0x32
	aribSetMosaicD           = 0x35
	aribSetPAlphanumeric     = 0x36
	aribSetPHiragana         = 0x37
	aribSetPKatakana         = 0x38
	aribSetJISX0201Katakana  = 0x49
	aribSetJISKanji1         = 0x39
	aribSetJISKanji2         = 0x3A
	aribSetAdditionalSymbols = 0x3B
	// aribSetDRCS は、DRCS の集合を表す仮の値。DRCS は外字なので、文字にしない。
	aribSetDRCS = 0x100
	// aribSetMacro は、マクロの集合を表す仮の値。番組情報では使われないので、文字にしない。
	aribSetMacro = 0x101
	// aribSetUnknown は、知らない集合を表す仮の値。
	aribSetUnknown = 0x102
)

// ARIB STD-B24 の制御符号。
const (
	aribAPR   = 0x0D
	aribLS1   = 0x0E
	aribLS0   = 0x0F
	aribPAPF  = 0x16
	aribSS2   = 0x19
	aribESC   = 0x1B
	aribAPS   = 0x1C
	aribSS3   = 0x1D
	aribSP    = 0x20
	aribDEL   = 0x7F
	aribSSZ   = 0x88
	aribMSZ   = 0x89
	aribNSZ   = 0x8A
	aribSZX   = 0x8B
	aribCOL   = 0x90
	aribFLC   = 0x91
	aribCDC   = 0x92
	aribPOL   = 0x93
	aribWMM   = 0x94
	aribMACRO = 0x95
	aribHLC   = 0x97
	aribRPC   = 0x98
	aribCSI   = 0x9B
	aribTIME  = 0x9D
)

// aribGraphicSet は、G0〜G3 に指示した符号化の集合。
type aribGraphicSet struct {
	set   int
	bytes int
}

// aribDecoder は、ARIB STD-B24 の 8 単位符号の文字列を読む状態。
type aribDecoder struct {
	g [4]aribGraphicSet
	// gl と gr は、GL と GR に呼び出した G の番号。
	gl, gr int
	// single は、SS2 か SS3 で、次の 1 文字だけに使う G の番号。なければ -1。
	single int
	// middle は、中型 (MSZ) の文字の大きさかどうか。英数を半角にする。
	middle bool
	out    strings.Builder
}

// DecodeARIBString は、ARIB STD-B24 の 8 単位符号の文字列 (番組情報の番組名や概要など) を、UTF-8 の文字列にする (#1500)。
// 文字の大きさ (中型) で書かれた英数と空白は半角に、標準の大きさで書かれたものは全角にする。チューナーサーバーの番組情報と同じ書き方にそろえる。
// 外字 (DRCS) とモザイクは読まずに飛ばし、色や位置などの制御符号は除く。改行 (APR) は "\n" にする。
func DecodeARIBString(b []byte) string {
	d := &aribDecoder{
		g: [4]aribGraphicSet{
			{set: aribSetKanji, bytes: 2},
			{set: aribSetAlphanumeric, bytes: 1},
			{set: aribSetHiragana, bytes: 1},
			{set: aribSetKatakana, bytes: 1},
		},
		gl: 0, gr: 2, single: -1, middle: false, out: strings.Builder{},
	}
	d.decode(b)

	return d.out.String()
}

func (d *aribDecoder) decode(b []byte) {
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == aribSP:
			if d.middle {
				d.out.WriteByte(' ')
			} else {
				d.out.WriteRune('　')
			}
			i++
		case c > aribSP && c < aribDEL:
			i += d.character(b[i:], false)
		case c > 0xA0 && c < 0xFF:
			i += d.character(b[i:], true)
		case c < aribSP:
			i += d.c0(b[i:])
		default:
			i += d.c1(b[i:])
		}
	}
}

// character は、GL (gr が false) か GR の文字を 1 つ読み、読んだバイト数を返す。
func (d *aribDecoder) character(b []byte, gr bool) int {
	index := d.gl
	if gr {
		index = d.gr
	}
	if d.single >= 0 {
		index = d.single
		d.single = -1
	}
	g := d.g[index]
	if len(b) < g.bytes {
		return len(b)
	}
	c1 := b[0] & 0x7F
	if g.bytes == 1 {
		d.writeOne(g.set, c1)

		return 1
	}
	d.writeTwo(g.set, c1, b[1]&0x7F)

	return 2
}

// writeOne は、1 バイトの集合の文字を書く。
func (d *aribDecoder) writeOne(set int, c byte) {
	switch set {
	case aribSetAlphanumeric, aribSetPAlphanumeric:
		d.out.WriteString(alphanumeric(c, d.middle))
	case aribSetHiragana, aribSetPHiragana:
		d.out.WriteString(kana(c, 0x3041, 0x73))
	case aribSetKatakana, aribSetPKatakana:
		d.out.WriteString(kana(c, 0x30A1, 0x76))
	case aribSetJISX0201Katakana:
		if c <= 0x5F {
			d.out.WriteRune(rune(0xFF61 + int(c) - 0x21))
		}
	}
}

// writeTwo は、2 バイトの集合の文字を書く。
func (d *aribDecoder) writeTwo(set int, c1, c2 byte) {
	switch set {
	case aribSetKanji, aribSetJISKanji1, aribSetJISKanji2, aribSetAdditionalSymbols:
	default:
		return
	}
	// 85 区から後ろは、ARIB の追加漢字と追加記号にする。
	if c1 >= 0x75 {
		if s, ok := aribSymbol(uint16(c1)<<8 | uint16(c2)); ok {
			d.out.WriteString(s)
		}

		return
	}
	if set == aribSetJISKanji2 {
		// JIS X 0213 の第 2 面は、表を持たないので読まない。
		return
	}
	if s, ok := jisOverride(uint16(c1)<<8 | uint16(c2)); ok {
		d.out.WriteString(s)

		return
	}
	decoded, err := japanese.EUCJP.NewDecoder().Bytes([]byte{c1 | 0x80, c2 | 0x80})
	if err != nil || !utf8.Valid(decoded) || bytes.ContainsRune(decoded, utf8.RuneError) {
		return
	}
	d.out.Write(decoded)
}

// jisOverride は、EUC-JP の復号器 (WHATWG の表) が JIS X 0208 の対応と違う文字にする符号を、JIS X 0208 の対応にそろえる。
// チューナーサーバー (Mahiron) の番組情報は、地上波で 0x2141 を U+301C (〜) に、0x213D を U+2014 (—) にしている
// (2026 年 11 月に実放送で確かめた)。ほかの符号も、Mirakurun (aribts) と同じく JIS X 0208 の対応にする。
// 0x2141 と 0x213D のほかは、実放送ではまだ確かめていない。BS の一部の局と BS4K では、Mahiron の番組情報が U+FF5E (～) になる番組もあり、
// どちらにもそろえることはできない。
func jisOverride(code uint16) (string, bool) {
	switch code {
	case 0x213D:
		return "—", true // EM DASH (WHATWG は U+2015)
	case 0x2141:
		return "〜", true // WAVE DASH (WHATWG は U+FF5E)
	case 0x2142:
		return "‖", true // DOUBLE VERTICAL LINE (WHATWG は U+2225)
	case 0x215D:
		return "−", true // MINUS SIGN (WHATWG は U+FF0D)
	case 0x2171:
		return "¢", true // CENT SIGN (WHATWG は U+FFE0)
	case 0x2172:
		return "£", true // POUND SIGN (WHATWG は U+FFE1)
	case 0x224C:
		return "¬", true // NOT SIGN (WHATWG は U+FFE2)
	default:
		return "", false
	}
}

// alphanumericFull は、英数の集合の 0x21〜0x7E を全角にしたもの。0x22 と 0x27 は右の引用符 (チューナーサーバーにそろえる。
// 2026 年 11 月に実放送で確かめた)、0x5C は円記号、0x7E は波ダッシュ (aribb24.js にそろえる) にする。
const alphanumericFull = "！”＃＄％＆’（）＊＋，－．／０１２３４５６７８９：；＜＝＞？＠ＡＢＣＤＥＦＧＨＩＪＫＬＭＮＯＰＱＲＳＴＵＶＷＸＹＺ［￥］＾＿｀ａｂｃｄｅｆｇｈｉｊｋｌｍｎｏｐｑｒｓｔｕｖｗｘｙｚ｛｜｝～"

// alphanumeric は、英数の集合の文字を返す。中型なら半角にする。
func alphanumeric(c byte, middle bool) string {
	if c < 0x21 || c > 0x7E {
		return ""
	}
	if middle {
		switch c {
		case 0x5C:
			return "¥"
		case 0x7E:
			return "~"
		default:
			return string(rune(c))
		}
	}
	runes := []rune(alphanumericFull)

	return string(runes[c-0x21])
}

// kana は、ひらがなかカタカナの集合の文字を返す。0x21〜last は、first からの並び。後ろの 0x77〜0x7E は、繰り返しの記号と句読点。
func kana(c byte, first rune, last byte) string {
	switch {
	case c >= 0x21 && c <= last:
		return string(first + rune(c-0x21))
	case c == 0x77 && first == 0x3041:
		return "ゝ"
	case c == 0x78 && first == 0x3041:
		return "ゞ"
	case c == 0x77:
		return "ヽ"
	case c == 0x78:
		return "ヾ"
	case c >= 0x79 && c <= 0x7E:
		return string([]rune(kanaPunctuation)[c-0x79])
	default:
		return ""
	}
}

// kanaPunctuation は、ひらがなとカタカナの集合の 0x79〜0x7E の文字。
const kanaPunctuation = "ー。「」、・"

// c0 は、C0 の制御符号を読み、読んだバイト数を返す。
func (d *aribDecoder) c0(b []byte) int {
	switch b[0] {
	case aribAPR:
		d.out.WriteByte('\n')
	case aribLS1:
		d.gl = 1
	case aribLS0:
		d.gl = 0
	case aribPAPF:
		return min(2, len(b))
	case aribAPS:
		return min(3, len(b))
	case aribSS2:
		d.single = 2
	case aribSS3:
		d.single = 3
	case aribESC:
		return d.escape(b)
	}

	return 1
}

// c1 は、C1 の制御符号を読み、読んだバイト数を返す。色や大きさなどの見た目は、大きさのほかは読まずに飛ばす。
func (d *aribDecoder) c1(b []byte) int {
	switch b[0] {
	case aribMSZ:
		d.middle = true
	case aribNSZ, aribSSZ:
		d.middle = false
	case aribSZX, aribFLC, aribPOL, aribWMM, aribHLC, aribRPC:
		return min(2, len(b))
	case aribCOL, aribCDC:
		// パラメーターが 0x20 なら、もう 1 バイト続く。
		if len(b) >= 2 && b[1] == aribSP {
			return min(3, len(b))
		}

		return min(2, len(b))
	case aribMACRO:
		// マクロの定義は、終わりの MACRO 0x4F まで飛ばす。
		for i := 1; i+1 < len(b); i++ {
			if b[i] == aribMACRO && b[i+1] == 0x4F {
				return i + 2
			}
		}

		return len(b)
	case aribCSI:
		// 終端の文字 (0x40〜0x7E) まで飛ばす。
		return skipUntilFinal(b, 1, 0x40, 0x7E)
	case aribTIME:
		if len(b) >= 2 && b[1] == aribSP {
			return min(3, len(b))
		}

		return skipUntilFinal(b, 2, 0x40, 0x43)
	}

	return 1
}

// skipUntilFinal は、b の from から、終端の文字 (low〜high) までの長さを返す。終端がなければ b の全部を飛ばす。
func skipUntilFinal(b []byte, from int, low, high byte) int {
	for i := from; i < len(b); i++ {
		if b[i] >= low && b[i] <= high {
			return i + 1
		}
	}

	return len(b)
}

// escape は、ESC で始まる、集合の指示か呼び出しを読み、読んだバイト数を返す。
func (d *aribDecoder) escape(b []byte) int {
	if len(b) < 2 {
		return len(b)
	}
	switch c := b[1]; c {
	case 0x6E:
		d.gl = 2

		return 2
	case 0x6F:
		d.gl = 3

		return 2
	case 0x7E:
		d.gr = 1

		return 2
	case 0x7D:
		d.gr = 2

		return 2
	case 0x7C:
		d.gr = 3

		return 2
	case 0x28, 0x29, 0x2A, 0x2B:
		// 1 バイトの集合を G0〜G3 に指示する。
		return d.designate(b[2:], int(c-0x28), 1) + 2
	case 0x24:
		if len(b) < 3 {
			return len(b)
		}
		if b[2] >= 0x29 && b[2] <= 0x2B {
			return d.designate(b[3:], int(b[2]-0x28), 2) + 3
		}
		if b[2] == 0x28 {
			return d.designate(b[3:], 0, 2) + 3
		}

		return d.designate(b[2:], 0, 2) + 2
	default:
		return 2
	}
}

// designate は、終端の符号 (DRCS なら 0x20 と終端の符号) を読み、G の index に集合を指示する。読んだバイト数を返す。
func (d *aribDecoder) designate(b []byte, index, bytes int) int {
	if len(b) == 0 {
		return 0
	}
	if b[0] == aribSP {
		if len(b) < 2 {
			return len(b)
		}
		set := aribSetDRCS
		if b[1] == 0x70 {
			set = aribSetMacro
		}
		if b[1] == 0x40 {
			bytes = 2
		}
		d.g[index] = aribGraphicSet{set: set, bytes: bytes}

		return 2
	}
	set := int(b[0])
	switch {
	case set == aribSetKanji || set == aribSetJISKanji1 || set == aribSetJISKanji2 || set == aribSetAdditionalSymbols:
		bytes = 2
	case set >= aribSetHiragana && set <= aribSetMosaicD, set == aribSetPAlphanumeric, set == aribSetPHiragana, set == aribSetPKatakana,
		set == aribSetAlphanumeric, set == aribSetJISX0201Katakana:
		bytes = 1
	default:
		set = aribSetUnknown
	}
	d.g[index] = aribGraphicSet{set: set, bytes: bytes}

	return 1
}
