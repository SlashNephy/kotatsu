package rule

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/SlashNephy/kotatsu/internal/epg"
)

// markPattern は、番組名に付く放送の種類の記号 ([新]、【再】、(字) など)。
// 括弧の中が、ARIB の記号に使われる 1〜3 文字の語に限って取り除く。番組名の一部の括弧は残す。
var markPattern = regexp.MustCompile(`[\[【(（]` +
	`(?:新|再|字|終|二|多|解|デ|S|SS|無|映|双|天|生|吹|手|N|B|HV|SD|PPV|MV|料|前|後|初|販|他|吹替|字幕|5\.1|ﾃﾞ|無料|劇場版?)` +
	`[\]】)）]`)

// TitleKey は、重複の回避に使う番組名のキーを返す (設計書 4.4)。
//
// 🈟 や 🈑 のような ARIB の囲み文字は、NFKC で「新」や「字」に分解されて番組名と区別できなくなるので、正規化の前に取り除く。
// その後、NFKC で表記をそろえ、[新] や【再】のような記号を取り除き、空白をまとめる。
// 話数は残す。話数まで取り除くと、2 話目以降がすべて録画済みと判定されてしまうからである。
func TitleKey(name string) string {
	name = strings.Map(func(r rune) rune {
		if isEnclosedMark(r) {
			return -1
		}

		return r
	}, name)
	name = epg.Normalize(name)
	name = markPattern.ReplaceAllString(name, " ")

	return strings.Join(strings.FieldsFunc(name, unicode.IsSpace), " ")
}

// isEnclosedMark は、ARIB の囲み文字 (Enclosed Alphanumeric Supplement と Enclosed Ideographic Supplement) かどうかを返す。
func isEnclosedMark(r rune) bool {
	return (r >= 0x1F100 && r <= 0x1F1FF) || (r >= 0x1F200 && r <= 0x1F2FF)
}
