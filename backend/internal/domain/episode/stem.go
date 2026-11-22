package episode

import (
	"regexp"
	"strings"
)

// seasonWordPattern は、揃えた語のうち、期や続編を表すもの。作品名と話数の間に置いても副題ではない。
// 「2nd Season」のように空白で区切ったときの、後ろの語 (season など) だけのものも含む。
var seasonWordPattern = regexp.MustCompile(
	`^(?:[0-9]+|[ivx]+|s[0-9]+|(?:season|シーズン|part|パート)[0-9]*|[0-9]+(?:st|nd|rd|th)(?:season)?)$`)

// Stem は、番組名 name が「作品名 副題 #N」の形なら、副題かもしれない語 (話数の直前の、空白で区切った最後の語) を除いた作品名を、
// Episode.Series と同じく揃えて返す (#2023)。形が違えば空を返す。
//
// 番組名だけでは、最後の語が副題か作品名の一部かを決められない (「ＭＵＳＩＣ　ＢＬＩＳＳ　＃２６」のように、作品名にも空白が入る)。
// そのため、返した作品名を、そのままシリーズ名として使わない。副題のない回 (「作品名 #N」) が別にあり、その回のシリーズ名と
// 一致したときだけ、同じ作品の回とみなす。期や続編を表す語 (数字、Season2、S3、II など) は、副題ではないので、空を返す。
// 話数の後ろに副題があれば、話数の前の語は作品名の一部なので、空を返す。編か期を読んだ番組名も、空を返す。
func Stem(name string) string {
	stripped := strings.TrimSpace(decorationPattern.ReplaceAllString(normalizeName(name), " "))
	t := parseTitle(stripped)
	if t.number == 0 || t.subtitle != "" || t.arc != "" {
		return ""
	}
	words := strings.Fields(t.work)
	if len(words) < 2 {
		return ""
	}
	// canon は長音符を揃えるので、期の表記 (シーズン、パート) は、揃える前の語で見る。
	if seasonWordPattern.MatchString(strings.ToLower(words[len(words)-1])) {
		return ""
	}

	return canon(strings.Join(words[:len(words)-1], " "))
}

// Stems は、番組名 name が「作品名 副題 #N」の形なら、話数の直前の語を後ろから 1 つずつ除いた作品名を、長い順に返す (#2031)。
// 副題に空白が入る番組名 (「作品名 副題 その二 #5」) で、副題のない回の作品名まで届くようにする。
// 1 つ目は Stem と同じ値で、除いた後の最後の語が期や続編を表す語なら、そこで止める。形が違えば空を返す。
func Stems(name string) []string {
	stems := []string{}
	if Stem(name) == "" {
		return stems
	}
	stripped := strings.TrimSpace(decorationPattern.ReplaceAllString(normalizeName(name), " "))
	words := strings.Fields(parseTitle(stripped).work)
	for n := len(words); n >= 2; n-- {
		// 話数を付けて Stem に読ませ、期や続編を表す語の扱いを Stem とそろえる。
		stem := Stem(strings.Join(words[:n], " ") + " #1")
		if stem == "" {
			break
		}
		stems = append(stems, stem)
	}

	return stems
}
