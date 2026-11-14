package domain

import (
	"regexp"
	"strings"
)

var (
	// workSlotPattern は、作品名の前に付く、ドラマや映画の放送枠の名前 (土曜ドラマ、日曜劇場、ドラマ24、金曜ロードショーなど)。
	// NFKC で正規化した名前から探す。枠の名前の後には、空白かかぎ括弧が続くものだけを除く (「ドラマチック」のような作品名を削らないため)。
	workSlotPattern = regexp.MustCompile(`^(?:` +
		`[月火水木金土日]曜(?:ナイト|よる|夜)?(?:ドラマ|劇場|ロードショー|プレミアム|シネマ)(?:\d+)?|` +
		`[月火水木金土日]ドラ|夜ドラ|連続テレビ小説|連続ドラマ|大河ドラマ|特集ドラマ|スペシャルドラマ|ドラマスペシャル|` +
		`ドラマ(?:\d+|イズム|ストリーム|NEO|Paravi|L)?|映画)(?:\s+|([「『]))`)
	// workQuotedPattern は、枠の名前の後の、かぎ括弧で囲んだ作品名 (土曜ドラマ「作品名」 第3話)。
	workQuotedPattern = regexp.MustCompile(`^[「『]([^」』]+)[」』]`)
)

// WorkTitle は、番組名から、外部のサービスで作品を探すのに使う作品名を読む (#1650)。
// 頭の飾り (【新】のような印) の後に、ドラマや映画の放送枠の名前 (土曜ドラマ、ドラマ24、映画など) があれば除き、
// 続くかぎ括弧の中を作品名とする (土曜ドラマ「作品名」 第3話)。かぎ括弧がなければ、枠の名前の後から RuleKeyword で読む。
// 枠の名前がなければ、RuleKeyword と同じ作品名を返す。ルールのキーワードと違い、番組名のひと続きの部分になるとは限らない。
func WorkTitle(name string) string {
	rest := Normalize(name)
	for {
		loc := keywordLeadingPattern.FindStringIndex(rest)
		// 「TVアニメ」の前置きは、RuleKeyword が括弧の作品名を読むので、そちらに任せる。
		if loc == nil || loc[1] == 0 || strings.Contains(rest[:loc[1]], "アニメ") {
			break
		}
		rest = rest[loc[1]:]
	}
	if loc := workSlotPattern.FindStringSubmatchIndex(rest); loc != nil {
		end := loc[1]
		if loc[2] >= 0 {
			// 枠の名前の後のかぎ括弧は、作品名を読むのに使うので残す。
			end = loc[2]
		}
		after := rest[end:]
		if match := workQuotedPattern.FindStringSubmatch(after); match != nil && strings.TrimSpace(match[1]) != "" {
			return strings.TrimSpace(match[1])
		}
		if strings.TrimSpace(after) != "" {
			return RuleKeyword(after)
		}
	}

	return RuleKeyword(name)
}
