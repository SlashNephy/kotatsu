package domain

import (
	"regexp"
	"strings"
)

var (
	// workSlotPattern は、作品名の前に付く、ドラマや映画の放送枠の名前 (土曜ドラマ、日曜劇場、ドラマ24、金曜ロードショー、中国ドラマなど)。
	// NFKC で正規化した名前から探す。枠の名前の後には、空白かかぎ括弧が続くものだけを除く (「ドラマチック」のような作品名を削らないため)。
	workSlotPattern = regexp.MustCompile(`^(?:` +
		`[月火水木金土日]曜(?:ナイト|よる|夜)?(?:ドラマ|劇場|ロードショー|プレミアム|シネマ)(?:\d+)?|` +
		`[月火水木金土日]ドラ(?:\d+)?|夜ドラ|連続テレビ小説|連続ドラマW?|大河ドラマ|特集ドラマ|スペシャルドラマ|ドラマスペシャル|` +
		// 海外ドラマや時代劇の再放送の枠 (中国ドラマ「作品名」、懐ドラ「作品名」、おはよう!時代劇 作品名。#1651)。
		`(?:中国|韓国|台湾|タイ|華流|韓流|海外)ドラマ|懐ドラ|(?:おはよう!)?時代劇|[月火水木金土日]曜ミステリー|ドラマブレイク|` +
		`ドラマ(?:\d+|イズム|ストリーム|NEO|Paravi|L)?|映画)(?:\s+|([「『]))`)
	// workQuotedPattern は、枠の名前の後の、かぎ括弧で囲んだ作品名 (土曜ドラマ「作品名」 第3話)。
	workQuotedPattern = regexp.MustCompile(`^[「『]([^」』]+)[」』]`)
	// workLenticularPattern は、作品名の途中や後ろに残る、すみ付き括弧の飾り (作品名【最終章へ…】 #6)。続けて並んだ飾りは、まとめて 1 つの空白にする。
	workLenticularPattern = regexp.MustCompile(`(?:\s*【[^】]*】\s*)+`)
	// workAnimeEpisodePattern は、アニメの番組名で、和文の作品名の後に空白で区切った話数があり、その後にサブタイトルが続くもの
	// (アニメ 天は赤い河のほとり 20 亀裂。#1929)。1 つ目のグループから後ろを除く。ドラマや映画では、同じ書き方の数が続編の番号のことが多い
	// (名探偵キャサリン 12 坂本龍馬殺人事件) ので、頭に「アニメ」の前置きがある番組名だけで除く。
	// 英字の後の数 (ULTRAMAN 2) と、後ろに何も続かない数は、作品名の一部とみなす。
	workAnimeEpisodePattern = regexp.MustCompile(`[^\x00-\x7F](\s+[0-9]{1,3}\s+\S)`)
	// 以下の workEpisode*Pattern は、RuleKeyword が話数とみなさない書き方の話数 (#1929)。それぞれの 1 つ目のグループから後ろを除く。
	// 作品名に数字が入るもの (機動戦士ガンダム00、Season2、相棒 season12、風間昭平15) を削らないよう、書き方ごとに前後を絞る。
	// workEpisodeRequestPattern は、小文字の英語の後に、数とピリオドが続くもの (request 20.、request 20. サブタイトル)。
	// 2 つ目のグループは、英語の言葉。workSeasonWordPattern の言葉 (season 2.) は、作品名の一部とみなす。
	workEpisodeRequestPattern = regexp.MustCompile(`(\s+([a-z]+)\s*[0-9]{1,3}\.)(?:\s|$)`)
	// workEpisodeScorePattern は、小文字の英語に数が続くもの (score08、score04、score05)。2 つ目のグループは、最初の言葉。
	// 大文字で始まる Season2 や PART2 と、workSeasonWordPattern の言葉 (season12) は、作品名の一部とみなす。
	workEpisodeScorePattern = regexp.MustCompile(`(\s+([a-z]+)[0-9]{1,3}(?:[、,]\s*[a-z]+[0-9]{1,3})*)$`)
	// workEpisodePeriodPattern は、空白の後の、数とピリオドの後にサブタイトルが続くもの (機動戦士ガンダムSEED DESTINY 21. PHASE-21)。
	// 作品名の終わりの数とピリオド (作品名 Season 2.) は、シーズンの番号のこともあるので除かない。
	workEpisodePeriodPattern = regexp.MustCompile(`(\s+[0-9]{1,3}\.\s)`)
	// workEpisodeOperationPattern は、作戦の数 (夜桜さんちの大作戦 第2期 作戦44 姉弟奪還作戦開始)。
	workEpisodeOperationPattern = regexp.MustCompile(`(\s+作戦[0-9]{1,3})(?:\s|$)`)
	// workEpisodeCountPattern は、第を付けない話数と、話数の範囲 (49話、130話 サブタイトル、第1〜12話、10話〜12話)。「全13話」や「1話から」は除かない。
	workEpisodeCountPattern = regexp.MustCompile(`(\s+第?[0-9]{1,4}話?[~〜][0-9]{1,4}話|\s+[0-9]{1,4}話)(?:[\s~〜(]|$)`)
	// workSeasonWordPattern は、小文字の英語に数が続いても、話数でなくシーズンや部の番号を表す言葉 (相棒 season12、part1)。
	workSeasonWordPattern = regexp.MustCompile(`^(?:season|part)$`)
)

// WorkTitle は、番組名から、外部のサービスで作品を探すのに使う作品名を読む (#1650)。
// 頭の飾り (【新】のような印) の後に、ドラマや映画の放送枠の名前 (土曜ドラマ、ドラマ24、映画など) があれば除き、
// 続くかぎ括弧の中を作品名とする (土曜ドラマ「作品名」 第3話)。かぎ括弧がなければ、枠の名前の後から RuleKeyword で読む。
// 枠の名前がなければ、RuleKeyword と同じ作品名を返す。ルールのキーワードと違い、番組名のひと続きの部分になるとは限らない。
// 作品名の途中や後ろに残ったすみ付き括弧 (【最終章へ…】) は、外部のサービスで探すときの邪魔になるので除く。
// 続けて、RuleKeyword が話数とみなさない書き方の話数 (49話、request 20.、score08、作戦44 など) から後ろを除く (#1929)。
// 除いて何も残らなければ、除く前の作品名を返す。
func WorkTitle(name string) string {
	title := workTitle(name)
	if stripped := strings.TrimSpace(workLenticularPattern.ReplaceAllString(title, " ")); stripped != "" {
		title = stripped
	}
	if start := workEpisodeStart(title, animePrefixed(name)); start < len(title) {
		// 話数の前の区切りは空白だけを除く。作品名の終わりのハイフン (BLEACH 千年血戦篇-禍進譚- 49話) は残す。
		if stripped := strings.TrimSpace(title[:start]); stripped != "" {
			return stripped
		}
	}

	return title
}

// workEpisodeStart は、作品名 title の中で、workEpisode*Pattern の話数が始まる位置を返す。いくつも当たれば、いちばん前のものを使う。
// anime なら、workAnimeEpisodePattern の話数も探す。当たらなければ、title の長さを返す。
func workEpisodeStart(title string, anime bool) int {
	start := len(title)
	if loc := workAnimeEpisodePattern.FindStringSubmatchIndex(title); anime && loc != nil {
		start = loc[2]
	}
	for _, pattern := range []*regexp.Regexp{
		workEpisodeRequestPattern, workEpisodeScorePattern, workEpisodePeriodPattern, workEpisodeOperationPattern, workEpisodeCountPattern,
	} {
		loc := pattern.FindStringSubmatchIndex(title)
		if loc == nil || loc[2] >= start {
			continue
		}
		if len(loc) > 4 && loc[4] >= 0 && workSeasonWordPattern.MatchString(title[loc[4]:loc[5]]) {
			continue
		}
		start = loc[2]
	}

	return start
}

// animePrefixed は、番組名 name の頭の飾りに、「アニメ」や「TVアニメ」の前置きがあるかを返す。
func animePrefixed(name string) bool {
	rest := Normalize(name)
	for {
		loc := keywordLeadingPattern.FindStringIndex(rest)
		if loc == nil || loc[1] == 0 {
			return false
		}
		if strings.Contains(rest[:loc[1]], "アニメ") {
			return true
		}
		rest = rest[loc[1]:]
	}
}

func workTitle(name string) string {
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
