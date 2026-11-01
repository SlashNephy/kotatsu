package domain

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	// keywordLeadingPattern は、番組名の頭に付く、作品名の一部でない飾り。【50周年記念】のような隅付き括弧と、
	// ＜日本初＞のような山括弧、[新] のような印、「華◆」のような枠の名前、「TVアニメ」のような前置き。NFKC で正規化した番組名から探す。
	// かぎ括弧の前の「TVアニメ」は、括弧を残して除く。
	keywordLeadingPattern = regexp.MustCompile(`^(?:\s+|【[^】]*】|<[^>]*>|\[[^\[\]]{1,3}\]|[^\s◆◇「『]{1,4}[◆◇]|(?:TV|テレビ)?アニメ(?:\s+|[:：]|([「『])))`)
	// keywordQuotedPattern は、「TVアニメ『作品名』」のように、かぎ括弧で囲んだ作品名。前置きを除いた後の頭にあるものだけを読む。
	keywordQuotedPattern = regexp.MustCompile(`^[「『]([^」』]+)[」』]`)
	// keywordEpisodePattern は、話数 (#1、第1話、第六話、(15)、CHAPTER:5、Episode 5)。(1971) のような 4 桁の年は、話数とみなさない。
	keywordEpisodePattern = regexp.MustCompile(`(?i)#\s*[0-9]|第[0-9〇一二三四五六七八九十百]+話|\([0-9]{1,3}\)|\bchapter\s*[:.]?\s*[0-9]|\bep(?:isode)?\.?\s*[0-9]`)
	// keywordInnerQuotePattern は、話数の前にある、かぎ括弧で囲んだ部分。「ドラマ「作品名」 第3話」の括弧は、作品名の一部とする。
	keywordInnerQuotePattern = regexp.MustCompile(`「[^「」]+」`)
	// keywordEndPattern は、作品名の終わりを示すもの。話数、サブタイトルのかぎ括弧、囲み文字の印 (【一挙】のような編成の印を含む)、
	// 注記の山括弧、▽のようなサブタイトルの区切り、出演者の案内。
	keywordEndPattern = regexp.MustCompile(`(?i)#\s*[0-9]|第[0-9〇一二三四五六七八九十百]+話|\([0-9]{1,3}\)|\bchapter\s*[:.]?\s*[0-9]|\bep(?:isode)?\.?\s*[0-9]|「|<|[▽▼]|\s出演[:：]|` +
		`[\[【(（](?:新|再|字|終|二|ニ|多|解|デ|S|SS|無|映|双|天|生|吹|声|手|交|日|N|B|HV|SD|PPV|MV|料|初|販|他|吹替|字幕|無料|一挙|連日|` +
		`2K|4K|8K|3D|HDR|SHV|UHD|5\.1|7\.1|22\.2)[\]】)）]`)
)

// RuleKeyword は、番組名から、ルールの番組名のキーワードにする作品名を読む (設計書 4.2、#1240)。
// 番組表の番組や見たいリストの候補からルールを作るとき、話数やサブタイトルまでキーワードに入れると、その話にしか一致しないため。
//
// 照合はキーワードを正規化した番組名の部分文字列として探すので、正規化した番組名の、ひと続きの部分を返す。
// 頭の飾り (【50周年記念】、＜日本初＞、[新]、「華◆」、「TVアニメ」) を除き、話数、サブタイトル、印、注記の手前までを作品名とする。
// 話数の前のかぎ括弧 (「ドラマ「作品名」 第3話」) は、作品名に含める。
// 「TVアニメ『作品名』」のように、前置きの後の作品名をかぎ括弧で囲んでいれば、括弧の中を作品名とする。
// 作品名が読めなければ、印を除いた番組名を返す。
func RuleKeyword(name string) string {
	normalized := Normalize(name)
	rest := normalized
	prefixed := false
	for {
		loc := keywordLeadingPattern.FindStringSubmatchIndex(rest)
		if loc == nil || loc[1] == 0 {
			break
		}
		end := loc[1]
		if loc[2] >= 0 {
			// 「TVアニメ『作品名』」の括弧は、作品名を読むのに使うので残す。
			end = loc[2]
		}
		prefixed = prefixed || strings.Contains(rest[:end], "アニメ")
		rest = rest[end:]
	}
	// 頭のかぎ括弧の後から、作品名の終わりを探す。「『作品名』Season2 #5」の括弧の後ろも、作品名の一部とする。
	searchFrom := 0
	if match := keywordQuotedPattern.FindStringSubmatch(rest); match != nil {
		if title := strings.TrimSpace(match[1]); prefixed && title != "" {
			return title
		}
		searchFrom = len(match[0])
	}
	// 話数の前のかぎ括弧は、サブタイトルではなく作品名の一部 (「ドラマ「作品名」 第3話」) なので、その後から終わりを探す。
	// 話数のない番組名のかぎ括弧 (「作品名「サブタイトル」」) は、サブタイトルとみなす。
	if episode := keywordEpisodePattern.FindStringIndex(rest[searchFrom:]); episode != nil {
		for _, quote := range keywordInnerQuotePattern.FindAllStringIndex(rest[searchFrom:searchFrom+episode[0]], -1) {
			if quote[0] > 0 {
				searchFrom += quote[1]

				break
			}
		}
	}
	if loc := keywordEndPattern.FindStringIndex(rest[searchFrom:]); loc != nil {
		rest = rest[:searchFrom+loc[0]]
	}
	if title := strings.TrimRightFunc(rest, isKeywordSeparator); strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}

	return strings.Join(strings.Fields(bracketMarkPattern.ReplaceAllString(normalized, " ")), " ")
}

// isKeywordSeparator は、作品名と話数の間に置かれる区切り (空白、コロン、ハイフンなど) かどうか。
// 「〜作品名〜」の閉じに使う波ダッシュは、作品名の一部なので含めない。
func isKeywordSeparator(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune(":：-－/／・|｜", r)
}
