package epg

import (
	"regexp"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// bracketMarkPattern は、番組名に括弧で書いた印 ([新]、【再】、(字) など)。NFKC で正規化した番組名から探す。
// 局によっては、囲み文字の代わりに括弧で書くので、囲み文字と同じ印とみなす。
// 括弧の中が、ARIB の記号に使われる語に限る。[前] と [後] (前編と後編) や [劇場版] は、番組名の一部とみなす。
var bracketMarkPattern = regexp.MustCompile(`[\[【(（]` +
	`(新|再|字|終|二|多|解|デ|S|SS|無|映|双|天|生|吹|声|手|交|N|B|HV|SD|PPV|MV|料|初|販|吹替|字幕|無料|` +
	`2K|4K|8K|3D|HDR|SHV|UHD|5\.1|7\.1|22\.2)` +
	`[\]】)）]`)

// isEnclosedMark は、番組名に入る ARIB の囲み文字かどうかを返す (#434)。
//   - 英字の囲み文字 (🄰〜🅏、HV や SD などを含む)
//   - 4K や 5.1 などの囲み文字 (🆛〜🆬)
//   - 仮名や漢字の囲み文字 (🈁〜)。🈀 (ほか) は、番組名の一部 (「〜ほか」) なので含めない。
//   - 鍵の囲み文字 (⚿)
func isEnclosedMark(r rune) bool {
	return (r >= 0x1F130 && r <= 0x1F14F) || (r >= 0x1F19B && r <= 0x1F1AC) || (r >= 0x1F201 && r <= 0x1F2FF) || r == 0x26BF
}

// markLabel は、囲み文字の中身 (🈑 なら「字」、🅊 なら「HV」) を返す。
// NFKC で中身を取り出せない囲み文字は、Unicode の文字名 (SQUARED FOUR K など) から決めた。web の programMarks.ts と同じ表にする。
func markLabel(r rune) string {
	switch r {
	case '\U0001F19B':
		return "3D"
	case '\U0001F19C':
		return "2ndScr"
	case '\U0001F19D':
		return "2K"
	case '\U0001F19E':
		return "4K"
	case '\U0001F19F':
		return "8K"
	case '\U0001F1A0':
		return "5.1"
	case '\U0001F1A1':
		return "7.1"
	case '\U0001F1A2':
		return "22.2"
	case '\U0001F1A3':
		return "60P"
	case '\U0001F1A4':
		return "120P"
	case '\U0001F1A5':
		return "d"
	case '\U0001F1A6':
		return "HC"
	case '\U0001F1A7':
		return "HDR"
	case '\U0001F1A8':
		return "Hi-Res"
	case '\U0001F1A9':
		return "Lossless"
	case '\U0001F1AA':
		return "SHV"
	case '\U0001F1AB':
		return "UHD"
	case '\U0001F1AC':
		return "VOD"
	case '\u26BF':
		return "鍵"
	}

	return norm.NFKC.String(string(r))
}

// bracketMarkLabel は、括弧で書いた語を、囲み文字の中身にそろえる。
func bracketMarkLabel(word string) string {
	switch word {
	case "吹替":
		return "吹"
	case "字幕":
		return "字"
	case "無料":
		// 🈚 (無) は無料放送、🈛 (料) は有料放送を表す。
		return "無"
	}

	return word
}

// replaceEnclosedMarks は、囲み文字を、EPGStation と同じく [字] のような括弧の書き方に置き換える。
func replaceEnclosedMarks(s string) string {
	if !strings.ContainsFunc(s, isEnclosedMark) {
		return s
	}

	var b strings.Builder
	for _, r := range s {
		if isEnclosedMark(r) {
			b.WriteByte('[')
			b.WriteString(markLabel(r))
			b.WriteByte(']')

			continue
		}
		b.WriteRune(r)
	}

	return b.String()
}

// Marks は、正規化する前の番組名から、印 (囲み文字の中身と、括弧で書いた印) を取り出す (設計書 4.2)。
// 番組名に出てきた順に、囲み文字を先に、重複を除いて並べる。
// 囲み文字を括弧の書き方にする前に正規化した番組名では、囲み文字が「新」のような文字になり、番組名の文字と見分けられないので、
// 正規化する前の番組名を使う。
func Marks(name string) []string {
	marks := []string{}
	add := func(mark string) {
		if !slices.Contains(marks, mark) {
			marks = append(marks, mark)
		}
	}
	for _, r := range name {
		if isEnclosedMark(r) {
			add(markLabel(r))
		}
	}
	for _, match := range bracketMarkPattern.FindAllStringSubmatch(norm.NFKC.String(name), -1) {
		add(bracketMarkLabel(match[1]))
	}

	return marks
}
