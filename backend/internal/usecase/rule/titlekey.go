package rule

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

// markPattern は、番組名に付く放送の種類の記号 ([新]、【再】、(字) など)。
// 括弧の中が、ARIB の記号に使われる語に限って取り除く。番組名の一部の括弧は残す。
// [前] と [後] (前編と後編) や [劇場版] は、別の番組を見分ける語なので取り除かない。
var markPattern = regexp.MustCompile(`[\[【(（]` +
	`(?:新|再|字|終|二|多|解|デ|S|SS|無|映|双|天|生|吹|手|N|B|HV|SD|PPV|MV|料|初|販|他|吹替|字幕|5\.1|ﾃﾞ|無料)` +
	`[\]】)）]`)

// TitleKey は、重複の回避に使う番組名のキーを返す (設計書 4.4)。
//
// 🈟 や 🈑 のような ARIB の囲み文字は、NFKC で「新」や「字」に分解されて番組名と区別できなくなるので、正規化の前に取り除く。
// その後、NFKC で表記をそろえ、[新] や【再】のような記号を取り除き、空白をまとめる。
// 話数は残す。話数まで取り除くと、2 話目以降がすべて録画済みと判定されてしまうからである。
func TitleKey(name string) string {
	// 括弧の記号と同じく空白に置き換え、前後の語がつながらないようにする。
	name = strings.Map(func(r rune) rune {
		if isEnclosedMark(r) {
			return ' '
		}

		return r
	}, name)
	name = domain.Normalize(name)
	name = markPattern.ReplaceAllString(name, " ")

	return strings.Join(strings.FieldsFunc(name, unicode.IsSpace), " ")
}

// isEnclosedMark は、ARIB の囲み文字 (Enclosed Alphanumeric Supplement と Enclosed Ideographic Supplement) かどうかを返す。
func isEnclosedMark(r rune) bool {
	return (r >= 0x1F100 && r <= 0x1F1FF) || (r >= 0x1F200 && r <= 0x1F2FF)
}
