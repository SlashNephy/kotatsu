package domain

import (
	"regexp"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// bracketMarkPattern は、番組名に括弧で書いた囲み文字 ([新]、【再】、(字) など)。NFKC で正規化した番組名から探す。
// 局によっては、囲み文字の代わりに括弧で書くので、同じ囲み文字とみなす。
// 括弧の中が、ARIB の記号に使われる語に限る。[前] と [後] (前編と後編) や [劇場版] は、番組名の一部とみなす。
var bracketMarkPattern = regexp.MustCompile(`[\[【(（]` +
	`(新|再|字|終|二|多|解|デ|S|SS|無|映|双|天|生|吹|声|手|交|N|B|HV|SD|PPV|MV|料|初|販|吹替|字幕|無料|` +
	`2K|4K|8K|3D|HDR|SHV|UHD|5\.1|7\.1|22\.2)` +
	`[\]】)）]`)

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

// replaceEnclosedMarks は、表の囲み文字を、EPGStation と同じく [字] のような括弧の書き方に置き換える。
func replaceEnclosedMarks(s string) string {
	if !strings.ContainsFunc(s, mayBeEnclosed) {
		return s
	}

	var b strings.Builder
	for _, r := range s {
		if label, ok := enclosedLabel(r); ok {
			b.WriteByte('[')
			b.WriteString(label)
			b.WriteByte(']')

			continue
		}
		b.WriteRune(r)
	}

	return b.String()
}

// Marks は、正規化する前の番組名から、囲み文字の中身 (括弧で書いたものを含む) を取り出す (設計書 4.2)。
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
		if label, ok := enclosedLabel(r); ok {
			add(label)
		}
	}
	for _, match := range bracketMarkPattern.FindAllStringSubmatch(norm.NFKC.String(name), -1) {
		add(bracketMarkLabel(match[1]))
	}

	return marks
}
