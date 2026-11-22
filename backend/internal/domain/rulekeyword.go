package domain

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// keywordMark は、作品名の終わりを示す、括弧で書いた印の正規表現を返す。囲み文字 (正規化で [新] のような括弧の書き方になる) と、
// 括弧で書いた囲み文字 (BracketMarkWords) に加え、表にない編成の印 (【一挙】、【連日】)、(日)、(他)、カタカナで書いた (ニ) を含む。
// [前] と [後] (前編と後編) は、BracketMarkWords と同じく番組名の一部とみなす。中身は、大文字と小文字を区別する。
func keywordMark() string {
	return `[\[【(（](?:ニ|日|他|一挙|連日|` + BracketMarkWords() + `)[\]】)）]`
}

var (
	// keywordLeadingPattern は、番組名の頭に付く、作品名の一部でない飾り。【50周年記念】のような隅付き括弧と、
	// ＜日本初＞のような山括弧、[新] や [120P] のような印、「華◆」のような枠の名前、「TVアニメ」のような前置き。NFKC で正規化した番組名から探す。
	// 印は、中身が 3 文字までの角括弧か、作品名の終わりとみなす印 (keywordMark) なら、長さによらず除く。
	// かぎ括弧の前の「TVアニメ」は、括弧を残して除く。
	keywordLeadingPattern = regexp.MustCompile(`^(?:\s+|【[^】]*】|<[^>]*>|\[[^\[\]]{1,3}\]|` + keywordMark() + `|[^\s◆◇「『]{1,4}[◆◇]|(?:TV|テレビ)?アニメ(?:\s+|[:：]|([「『])))`)
	// keywordQuotedPattern は、「TVアニメ『作品名』」のように、かぎ括弧で囲んだ作品名。前置きを除いた後の頭にあるものだけを読む。
	keywordQuotedPattern = regexp.MustCompile(`^[「『]([^」』]+)[」』]`)
	// keywordEpisodePattern は、話数 (#1、第1話、第六話、第1回、(15)、CHAPTER:5、Episode: 5)。(1971) のような 4 桁の年は、話数とみなさない。
	keywordEpisodePattern = regexp.MustCompile(`#\s*[0-9]|第[0-9〇一二三四五六七八九十百]+[話回]|\([0-9]{1,3}\)|(?i:\bchapter\s*[:.]?\s*[0-9]|\bep(?:isode)?\s*[:.]?\s*[0-9])`)
	// keywordInnerQuotePattern は、話数の前にある、かぎ括弧で囲んだ部分。「作品名「題」 第3話」の括弧は、作品名の一部とする。
	keywordInnerQuotePattern = regexp.MustCompile(`「[^「」]+」`)
	// keywordEndPattern は、作品名の終わりを示すもの。話数、サブタイトルのかぎ括弧、印 (keywordMark)、
	// 注記の山括弧、▽のようなサブタイトルの区切り、出演者の案内。
	keywordEndPattern = regexp.MustCompile(`#\s*[0-9]|第[0-9〇一二三四五六七八九十百]+[話回]|\([0-9]{1,3}\)|(?i:\bchapter\s*[:.]?\s*[0-9]|\bep(?:isode)?\s*[:.]?\s*[0-9])|「|<|[▽▼]|\s出演[:：]|` +
		keywordMark())
	// keywordMarkSplitPattern は、作品名が読めないときに、番組名を区切る印。
	keywordMarkSplitPattern = regexp.MustCompile(keywordMark())
	// keywordWaveSubtitlePattern は、作品名の後ろの、波ダッシュで囲んだ副題 (作品名〜副題〜)。開きの波ダッシュの位置を 1 つ目のグループで返す。
	// 局や回によって〜と~ (全角チルダを正規化したもの) が入れ替わり、副題も回ごとに変わるので、キーワードに入れない (#2041)。
	// 作品名の終わりにあるものだけを除く。後ろに続きがあれば (2026 〜全英への道〜ミズノオープン)、作品名の一部とみなす。
	keywordWaveSubtitlePattern = regexp.MustCompile(`\S\s*([~〜])[^~〜]+[~〜]\s*$`)
	// keywordDashSubtitlePattern は、空白の後の、ダッシュか長音符で囲んだ副題 (作品名 ー副題ー)。開きの位置を 1 つ目のグループで返す。
	// 空白を前に求めるのは、語の中の長音符 (ゴーストライター) を副題の区切りと取り違えないためである。
	keywordDashSubtitlePattern = regexp.MustCompile(`\S(\s+[ー―—−-])[^ー―—−\s-][^ー―—−-]*[ー―—−-]\s*$`)
)

// stripKeywordSlot は、rest の頭の、ドラマや映画の放送枠の名前を除く (#2041)。
// 枠の名前の後にかぎ括弧が来れば (間に空白があってもよい)、括弧の中を quoted に返す (木曜ドラマ「作品名 ー副題ー」第8話)。
// 括弧がなければ、枠の名前の後ろを after に返す。枠の名前がないときと、後ろに話数や印しか残らないとき (土曜ドラマ #3) は、rest をそのまま返す。
func stripKeywordSlot(rest string) (string, string) {
	loc := workSlotPattern.FindStringSubmatchIndex(rest)
	if loc == nil {
		return "", rest
	}
	if loc[2] >= 0 {
		if match := keywordQuotedPattern.FindStringSubmatch(rest[loc[2]:]); match != nil {
			if title := strings.TrimSpace(match[1]); title != "" {
				return title, rest
			}
		}

		return "", rest
	}
	after := strings.TrimSpace(rest[loc[1]:])
	// 枠の名前とかぎ括弧の間に空白があっても (木曜ドラマ 「作品名」 第8話)、括弧の中を作品名とする。
	if match := keywordQuotedPattern.FindStringSubmatch(after); match != nil {
		if title := strings.TrimSpace(match[1]); title != "" {
			return title, rest
		}
	}
	if end := keywordEndPattern.FindStringIndex(after); after == "" || (end != nil && end[0] == 0) {
		return "", rest
	}

	return "", after
}

// trimEnclosedSubtitle は、作品名 title の終わりの、波ダッシュかダッシュで囲んだ副題を除く (#2041)。除いて何も残らなければ、そのまま返す。
// かぎ括弧の中の副題 (作品名「題〜副題〜」) は除かない。除くと、括弧が閉じないキーワードになるからである。
func trimEnclosedSubtitle(title string) string {
	cut := len(title)
	for _, pattern := range []*regexp.Regexp{keywordWaveSubtitlePattern, keywordDashSubtitlePattern} {
		loc := pattern.FindStringSubmatchIndex(title)
		if loc == nil || loc[2] >= cut {
			continue
		}
		if before := title[:loc[2]]; strings.Count(before, "「")+strings.Count(before, "『") > strings.Count(before, "」")+strings.Count(before, "』") {
			continue
		}
		cut = loc[2]
	}
	// 除いて 1 文字しか残らなければ (声〜あなたと読売テレビ〜)、除かない。1 文字のキーワードは、ほとんどの番組に一致するからである。
	if trimmed := strings.TrimSpace(strings.TrimRightFunc(title[:cut], isKeywordSeparator)); utf8.RuneCountInString(trimmed) >= minTrimmedKeywordLength {
		return trimmed
	}

	return title
}

// minTrimmedKeywordLength は、副題を除いた後のキーワードに残す、最も短い文字の数 (#2041)。
const minTrimmedKeywordLength = 2

// RuleKeyword は、番組名から、ルールの番組名のキーワードにする作品名を読む (設計書 4.2、#1240)。
// 番組表の番組や見たいリストの候補からルールを作るとき、話数やサブタイトルまでキーワードに入れると、その話にしか一致しないため。
//
// 照合はキーワードを正規化した番組名の部分文字列として探すので、正規化した番組名の、ひと続きの部分を返す。
// 頭の飾り (【50周年記念】、＜日本初＞、[新]、「華◆」、「TVアニメ」) を除き、話数、サブタイトル、印、注記の手前までを作品名とする。
// 話数の前のかぎ括弧 (「作品名「題」 第3話」) は、作品名に含める。
// 「TVアニメ『作品名』」のように、前置きの後の作品名をかぎ括弧で囲んでいれば、括弧の中を作品名とする。
// 作品名が読めなければ、正規化した番組名を印で区切った、最初の空でない部分を返す。
// 印を空白に置き換えたり空白をまとめたりすると、正規化した番組名のひと続きの部分でなくなり、元の番組に一致しなくなるため (#1984)。
func RuleKeyword(name string) string {
	return ruleKeyword(name, true)
}

// ruleKeyword は、RuleKeyword の読み方で作品名を読む。trimSubtitle なら、波ダッシュかダッシュで囲んだ副題も除く (#2041)。
// WorkTitle は、外部のサービスで探す作品名と、保存した作品の鍵 (drama.TitleKey) を変えないよう、副題を除かずに読む。
func ruleKeyword(name string, trimSubtitle bool) string {
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
	// ドラマや映画の放送枠の名前 (木曜ドラマ、連続ドラマW、おはよう!時代劇など。WorkTitle と同じ一覧) を除く (#2041)。
	quoted, after := stripKeywordSlot(rest)
	if quoted != "" {
		return finishKeyword(trimSubtitle, quoted)
	}
	rest = after
	// 頭のかぎ括弧の後から、作品名の終わりを探す。「『作品名』Season2 #5」の括弧の後ろも、作品名の一部とする。
	searchFrom := 0
	if match := keywordQuotedPattern.FindStringSubmatch(rest); match != nil {
		if title := strings.TrimSpace(match[1]); prefixed && title != "" {
			return finishKeyword(trimSubtitle, title)
		}
		searchFrom = len(match[0])
	}
	// 話数の前のかぎ括弧は、サブタイトルではなく作品名の一部 (「作品名「題」 第3話」) なので、その後から終わりを探す。
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
		return finishKeyword(trimSubtitle, strings.TrimSpace(title))
	}

	for _, part := range keywordMarkSplitPattern.Split(normalized, -1) {
		if title := strings.TrimSpace(part); title != "" {
			return title
		}
	}

	return ""
}

// finishKeyword は、読んだ作品名 title を、trimSubtitle なら trimEnclosedSubtitle で仕上げる。
func finishKeyword(trimSubtitle bool, title string) string {
	if !trimSubtitle {
		return title
	}

	return trimEnclosedSubtitle(title)
}

// isKeywordSeparator は、作品名と話数の間に置かれる区切り (空白、コロン、ハイフンなど) かどうか。
// 「〜作品名〜」の閉じに使う波ダッシュは、作品名の一部なので含めない。
func isKeywordSeparator(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune(":：-－/／・|｜", r)
}

// HasEpisodeNumber は、番組名 name に話数 (#1、第1話、(15) など) があるかを返す (#2041)。正規化する前の番組名を渡す。
func HasEpisodeNumber(name string) bool {
	return keywordEpisodePattern.MatchString(Normalize(name))
}

// RuleKeywordPrefixes は、ルールのキーワード keyword を空白で区切った語の並びの、頭からの部分を、keyword より短いものから長い順に返す (#2041)。
// 回ごとに変わる副題 (作品名 副題 #7) を除いた作品名の候補にする。どれも keyword の頭のひと続きの部分なので、番組名の部分文字列になる。
func RuleKeywordPrefixes(keyword string) []string {
	prefixes := []string{}
	for i := len(keyword) - 1; i > 0; i-- {
		// 正規化した番組名の空白は ASCII の空白なので、バイトで見てよい。
		if keyword[i] != ' ' || keyword[i-1] == ' ' {
			continue
		}
		// 1 文字の作品名は、ほとんどの番組に一致するので、候補にしない。
		if prefix := strings.TrimRightFunc(keyword[:i], isKeywordSeparator); utf8.RuneCountInString(strings.TrimSpace(prefix)) >= minTrimmedKeywordLength {
			prefixes = append(prefixes, prefix)
		}
	}

	return prefixes
}
