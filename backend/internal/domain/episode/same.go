package episode

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// digitsPattern は、数字の並び。
var digitsPattern = regexp.MustCompile(`[0-9]+`)

// digitSequence は、文字列の中の数字の並びを返す。
func digitSequence(s string) string {
	return strings.Join(digitsPattern.FindAllString(s, -1), ",")
}

// SameSeries は、2 つの回のシリーズが同じかどうかを返す。
// 期を持つ側と、編を書かない側は同じとみない。期を書かない番組名は 1 期の回かもしれないので、2 期を録り逃すより 1 本多く録る。
func SameSeries(a, b *Episode) bool {
	if a.Series == b.Series {
		return true
	}

	return a.Work != "" && a.Work == b.Work && ((a.Arc == "" && !b.HasSeason) || (b.Arc == "" && !a.HasSeason))
}

// subtitlesConflict は、確かな副題どうしが食い違っているかどうかを返す。
func subtitlesConflict(a, b *Episode) bool {
	return a.Subtitle != "" && b.Subtitle != "" && !a.UncertainSubtitle && !b.UncertainSubtitle && a.Subtitle != b.Subtitle
}

// summaryMatches は、2 つの回の概要が一致するかどうかを返す。
func summaryMatches(a, b *Episode) bool {
	switch {
	case a.Number == 0 || a.Number != b.Number:
		return false
	case a.Fingerprint == "" || a.Fingerprint != b.Fingerprint:
		return false
	case subtitlesConflict(a, b):
		return false
	case a.Series != b.Series && digitSequence(a.Series) != digitSequence(b.Series):
		// 期の数字が違う別シリーズが、同じ番組紹介を持つことがある。
		return false
	}

	return SameSeries(a, b) || (a.Series != "" && b.Series != "" && (strings.Contains(a.Series, b.Series) || strings.Contains(b.Series, a.Series)))
}

// sameAnime は、両方にしょぼいカレンダーの作品と話数があれば、それで同じ回かを決める。2 つ目の値は、決まったかどうか。
func sameAnime(a, b *Episode) (bool, bool) {
	if a.Anime == nil || b.Anime == nil {
		return false, false
	}

	return *a.Anime == *b.Anime, true
}

// Same は、2 つの回が同じ回かどうかを返す。
func Same(a, b *Episode) bool {
	if same, decided := sameAnime(a, b); decided {
		return same
	}
	switch {
	case a.Number != b.Number:
		return false
	case a.Arc != "" && b.Arc != "" && a.Arc != b.Arc:
		return false
	case summaryMatches(a, b):
		return true
	case a.Subtitle == "" || b.Subtitle == "":
		return SameSeries(a, b)
	case a.Subtitle != b.Subtitle:
		return (a.UncertainSubtitle || b.UncertainSubtitle) && SameSeries(a, b)
	case SameSeries(a, b):
		return true
	}

	// 副題が一致してシリーズ名が違うときは、片方の芯がもう片方のシリーズ名に含まれれば同じ回とする。
	return coreContained(a, b) || coreContained(b, a)
}

func coreContained(a, b *Episode) bool {
	return utf8.RuneCountInString(a.Core) >= MinCoreLength && strings.Contains(b.Series, a.Core)
}

// certainlySame は、2 つの回が確かに同じ回かどうかを返す。束ね方の最初に、推移的につなぐのに使う。
func certainlySame(a, b *Episode) bool {
	if same, decided := sameAnime(a, b); decided {
		return same
	}
	if a.Number != b.Number {
		return false
	}
	switch {
	case summaryMatches(a, b):
		return true
	case a.Subtitle != "" && a.Subtitle == b.Subtitle:
		return Same(a, b)
	case a.Subtitle == "":
		return (b.Subtitle == "" || b.UncertainSubtitle) && a.Series == b.Series
	case b.Subtitle == "":
		return a.UncertainSubtitle && a.Series == b.Series
	}

	return false
}

// weeklyKey は、概要の指紋の数字の並びを同じ記号に置き換えたもの。話数の数字だけを差し替える紹介文をまとめる。
func weeklyKey(fingerprint string) string {
	return digitsPattern.ReplaceAllString(fingerprint, "#")
}

// SuppressWeeklySummaries は、毎週同じ概要 (番組紹介) を、関わる回の概要の指紋から除く。
// 同じ鍵の中で話数が 2 種類以上あれば、その概要は回を見分けないので使わない。episodes の指紋を書き換える。
func SuppressWeeklySummaries(episodes []*Episode) {
	numbers := map[string]map[int]bool{}
	for _, ep := range episodes {
		if ep.Fingerprint == "" || ep.Number == 0 {
			continue
		}
		key := weeklyKey(ep.Fingerprint)
		if numbers[key] == nil {
			numbers[key] = map[int]bool{}
		}
		numbers[key][ep.Number] = true
	}
	for _, ep := range episodes {
		if ep.Fingerprint != "" && len(numbers[weeklyKey(ep.Fingerprint)]) >= 2 {
			ep.Fingerprint = ""
		}
	}
}
