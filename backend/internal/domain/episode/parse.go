package episode

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

// Source は、回を読む元の、放送か録画の項目。
type Source struct {
	// Name と Description は、正規化する前の番組名と概要。
	Name        string
	Description string
	// Extended は、詳細の本文をつないだもの。概要が空のときだけ、概要の代わりに読む。
	Extended string
	// SeriesEpisode は、シリーズの記述子の話数。局が回として送っている値なので、番組名の話数より先に使う。
	SeriesEpisode *int32
	// Anime は、しょぼいカレンダーの作品と話数 (設計書 3.8)。1 つの話に結びついたときだけ渡す。
	Anime *Anime
}

// Anime は、しょぼいカレンダーの作品 (TID) と話数。
type Anime struct {
	TID   int32
	Count float64
}

// Episode は、番組名と概要から読んだ回。文字列は、半角に寄せ、小文字にし、空白を除いて揃えたもの。
type Episode struct {
	// Series は、作品名と編を揃えたもの (シリーズ名)。
	Series string
	// Work は、先頭の枠 (「アニメ・」など) を除いた作品名を揃えたもの。
	Work string
	// Arc は、編を揃えたもの。期を読んだときは、期も含む。
	Arc string
	// HasSeason は、編に期の表記 (第2期など) が入っているかどうか。
	HasSeason bool
	// Core は、シリーズ名を空白と括弧で区切った語のうち、一番長い語 (芯)。
	Core string
	// Number は話数。読めなければ 0。
	Number int
	// Subtitle は副題。UncertainSubtitle は、副題が後ろの枠名や出演者かもしれない (確かでない) かどうか。
	Subtitle          string
	UncertainSubtitle bool
	// Fingerprint は、揃えた概要。概要が短ければ空。
	Fingerprint string
	// Key は、飾りの印を除いて揃えた番組名。回の分からない放送を、同時刻の同じ番組名で束ねるのに使う。
	Key string
	// Special は、番組名か概要に、総集編や特別編のような、通常の回でない印があるかどうか。
	Special bool
	// Anime は、しょぼいカレンダーの作品と話数。なければ nil。
	Anime *Anime
}

// Known は、回が分かる (話数か副題を読めた) かどうかを返す。
func (e *Episode) Known() bool {
	return e.Number > 0 || e.Subtitle != "" || e.Anime != nil
}

var (
	// decorationPattern は、番組名の、括弧で囲んだ放送の種類の印。
	decorationPattern = regexp.MustCompile(`[\[【(](?:新|終|再|字|解|デ|二|多|SS|初|最終回|無料|無|生|映)[\]】)]`)
	// framePattern は、番組名の先頭の <…> (枠の名前)。
	framePattern = regexp.MustCompile(`^<([^<>]{1,` + strconv.Itoa(MaxFrameNameLength) + `})>`)
	// leadingQuotePattern は、番組名の先頭の鍵括弧 (作品名)。
	leadingQuotePattern = regexp.MustCompile(`^(?:「([^「」]+)」|『([^『』]+)』)`)
	// trailingQuotePattern は、末尾の鍵括弧 (副題)。
	trailingQuotePattern = regexp.MustCompile(`(?:「([^「」]*)」|『([^『』]*)』)$`)
	// wholeQuotePattern は、全体が鍵括弧 1 つだけのもの。
	wholeQuotePattern = regexp.MustCompile(`^(?:「([^「」]*)」|『([^『』]*)』)$`)
	// trailingFramesPattern は、末尾の【…】の並び (枠の名前)。
	trailingFramesPattern = regexp.MustCompile(`(?:\s*【[^【】]*】)+$`)
	// seasonPattern は、期 (第2期、第二シーズンなど)。括弧で囲まれていてもよい。
	seasonPattern = regexp.MustCompile(`[(\[【]?第([0-9]+|[〇一二三四五六七八九十]+)(期|シーズン|クール|章|部)[)\]】]?`)
	// seasonFollowPattern は、期のすぐ後ろに来てよいもの (話数の印か、第＋数字)。
	seasonFollowPattern = regexp.MustCompile(`^(?:#|第[0-9])`)
	// titleNumberPattern は、番組名の話数 (#N、第N話、第N回)。NFKC で ＃ は # になる。
	titleNumberPattern = regexp.MustCompile(`#([0-9]+)|第([0-9]+)[話回]`)
	// englishNumberPattern は、番組名の英語の話数の呼び方。
	englishNumberPattern = regexp.MustCompile(`(?i)(?:chapter|episode|ep|request|file|case|act|lesson|mission|karte)\.?([0-9]+)`)
	// trailingArcPattern は、番組名の末尾の「空白＋『編』で終わる語」。
	trailingArcPattern = regexp.MustCompile(`\s(\S+編)$`)
	// workFramePattern は、作品名の先頭の「アニメ」「ドラマ」の枠。地上波と BS で、先頭の枠名だけが違う局に合わせる。
	workFramePattern = regexp.MustCompile(`^(?:アニメ|ドラマ)[A-Za-z]?(?:・|\s)+`)
	// specialPattern は、通常の回でない印。大文字と小文字は区別しない。SP は前後に英字がないときだけ見る。
	specialPattern = regexp.MustCompile(`(?i)総集編|特別[編篇版回]|番外|一挙|振り返り|スペシャル`)
	spPattern      = regexp.MustCompile(`(?i)sp`)
	// captionPattern は、字幕付きの印。
	captionPattern = regexp.MustCompile(`\[字\]|【字】|🈑`)
	// excludedArcPattern は、末尾の「…編」に含まれていれば、編として読まない語。
	excludedArcPattern = regexp.MustCompile(`総集|特別|番外|完結`)
)

const (
	// uncertainMarks は、副題の先頭にあれば、その副題を確かでない副題とする記号。後ろの枠名や出演者の可能性がある。
	uncertainMarks = "▼▽◆◇■□★☆"
	// coreSeparators は、芯を取り出すときに語を区切る括弧。
	coreSeparators = "「」『』【】[]()<>〈〉《》〔〕"
	// kanjiDigits は、漢数字の 0 から 9 を並べたもの。
	kanjiDigits = "〇一二三四五六七八九"
)

// Parse は、番組名、概要、記述子の話数から、回を読む。
func Parse(src *Source) *Episode {
	name := normalizeName(src.Name)
	stripped := decorationPattern.ReplaceAllString(name, " ")
	t := parseTitle(strings.TrimSpace(stripped))

	description := domain.Normalize(src.Description)
	ep := &Episode{
		Series:            canon(t.work + t.arc),
		Work:              canon(t.work),
		Arc:               canon(t.arc),
		HasSeason:         t.hasSeason,
		Core:              core(t.work + " " + t.arc),
		Number:            t.number,
		Subtitle:          canon(t.subtitle),
		UncertainSubtitle: isUncertain(t.subtitle),
		Fingerprint:       fingerprint(description),
		Key:               canon(stripped),
		Special:           hasSpecialMark(name) || hasSpecialMark(description),
		Anime:             src.Anime,
	}
	if src.SeriesEpisode != nil && *src.SeriesEpisode > 0 {
		ep.Number = int(*src.SeriesEpisode)
	}
	if ep.Subtitle != "" && !ep.UncertainSubtitle {
		return ep
	}

	summary := description
	if strings.TrimSpace(summary) == "" {
		summary = domain.Normalize(src.Extended)
	}
	applySummary(ep, readSummary(summary))

	return ep
}

// Captioned は、番組名が字幕付きの印を含むかどうかを返す。
func Captioned(name string) bool {
	return captionPattern.MatchString(norm(name))
}

// norm は、NFKC で半角に寄せる。囲み文字は括弧の書き方にする。
func norm(s string) string {
	return domain.Normalize(s)
}

// normalizeName は、囲み漢字 (U+1F210〜U+1F23B) を空白にしてから、番組名を半角に寄せる。
func normalizeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r >= 0x1F210 && r <= 0x1F23B {
			return ' '
		}

		return r
	}, name)

	return domain.Normalize(name)
}

// canon は、小文字にし、空白をすべて除く。
func canon(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}

		return r
	}, strings.ToLower(s))
}

// core は、空白と括弧で区切った語のうち、一番長い語を揃えて返す。
func core(s string) string {
	longest := ""
	for _, word := range strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(coreSeparators, r)
	}) {
		word = canon(word)
		if utf8.RuneCountInString(word) > utf8.RuneCountInString(longest) {
			longest = word
		}
	}

	return longest
}

func isUncertain(subtitle string) bool {
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(subtitle))

	return subtitle != "" && strings.ContainsRune(uncertainMarks, r)
}

// fingerprint は、揃えた概要が MinFingerprintLength 以上の長さならそれを、短ければ空を返す。
func fingerprint(description string) string {
	c := canon(description)
	if utf8.RuneCountInString(c) < MinFingerprintLength {
		return ""
	}

	return c
}

func hasSpecialMark(s string) bool {
	if specialPattern.MatchString(s) {
		return true
	}
	for _, loc := range spPattern.FindAllStringIndex(s, -1) {
		before, _ := utf8.DecodeLastRuneInString(s[:loc[0]])
		after, _ := utf8.DecodeRuneInString(s[loc[1]:])
		if !isLatin(before) && !isLatin(after) {
			return true
		}
	}

	return false
}

func isLatin(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// title は、番組名を分けた部品。
type title struct {
	work      string
	arc       string
	hasSeason bool
	number    int
	subtitle  string
}

// parseTitle は、飾りの印を除いた番組名を、作品名、編、期、話数、副題に分ける。
func parseTitle(name string) *title {
	if m := framePattern.FindStringIndex(name); m != nil {
		name = strings.TrimSpace(name[m[1]:])
	}
	t := &title{work: "", arc: "", hasSeason: false, number: 0, subtitle: ""}
	rest := name
	quoted := false
	if m := leadingQuotePattern.FindStringSubmatch(name); m != nil {
		t.work = m[1] + m[2]
		rest = name[len(m[0]):]
		quoted = true
	}

	// full は、before を先頭に持つ番組名の残り。期のすぐ後ろを見るのに使う。
	var before string
	full := rest
	if m, ok := findTitleNumber(rest); ok {
		t.number = m.number
		before = rest[:m.start]
		t.subtitle = trailingSubtitle(rest[m.end:])
	} else {
		full = strings.TrimSpace(trailingFramesPattern.ReplaceAllString(rest, ""))
		before = full
		// 先頭の鍵括弧は作品名なので、副題にしない。
		if m := trailingQuotePattern.FindStringSubmatchIndex(full); m != nil && (quoted || m[0] > 0) {
			t.subtitle = submatch(full, m, 1) + submatch(full, m, 2)
			before = full[:m[0]]
		}
	}

	before, season := extractSeason(before, full)
	t.hasSeason = season != ""
	before = strings.TrimSpace(before)
	if quoted {
		t.arc = strings.TrimSpace(before + " " + season)
	} else {
		before, arc := extractArc(before)
		t.work = before
		t.arc = strings.TrimSpace(arc + " " + season)
	}
	if loc := workFramePattern.FindStringIndex(t.work); loc != nil && strings.TrimSpace(t.work[loc[1]:]) != "" {
		t.work = t.work[loc[1]:]
	}
	t.work = strings.TrimSpace(t.work)

	return t
}

// numberMatch は、番組名で読んだ話数と、その位置。
type numberMatch struct {
	start, end int
	number     int
}

// findTitleNumber は、番組名の話数を探す。「#N」「第N話」「第N回」を先に探し、無ければ英語の呼び方を読む。
func findTitleNumber(s string) (*numberMatch, bool) {
	for _, m := range titleNumberPattern.FindAllStringSubmatchIndex(s, -1) {
		digits := submatch(s, m, 1) + submatch(s, m, 2)
		if n, ok := parseDigits(digits); ok {
			return &numberMatch{start: m[0], end: m[1], number: n}, true
		}
	}
	for _, m := range englishNumberPattern.FindAllStringSubmatchIndex(s, -1) {
		before, _ := utf8.DecodeLastRuneInString(s[:m[0]])
		after, _ := utf8.DecodeRuneInString(s[m[1]:])
		// 直前が英字、数字、括弧なら語の一部、直後が「年」なら年なので読まない。
		if m[0] > 0 && (isLatin(before) || unicode.IsDigit(before) || strings.ContainsRune("【[(", before)) {
			continue
		}
		if after == '年' {
			continue
		}
		if n, ok := parseDigits(s[m[2]:m[3]]); ok {
			return &numberMatch{start: m[0], end: m[1], number: n}, true
		}
	}

	return nil, false
}

// submatch は、i 番目の部分一致を返す。一致しなければ空。
func submatch(s string, m []int, i int) string {
	if m[2*i] < 0 {
		return ""
	}

	return s[m[2*i]:m[2*i+1]]
}

// parseDigits は、MaxNumberDigits 桁までの数字を読む。0 は話数とみなさない。
func parseDigits(digits string) (int, bool) {
	if digits == "" || len(digits) > MaxNumberDigits {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n <= 0 {
		return 0, false
	}

	return n, true
}

// trailingSubtitle は、話数の後ろから副題を読む。末尾の【…】の並び (枠の名前) は除き、全体が鍵括弧 1 つなら括弧を外す。
func trailingSubtitle(after string) string {
	s := strings.TrimSpace(trailingFramesPattern.ReplaceAllString(after, ""))
	if m := wholeQuotePattern.FindStringSubmatch(s); m != nil {
		return m[1] + m[2]
	}

	return s
}

// extractSeason は、before から期を取り出し、期を除いた before と、「第N期」の形にした期を返す。
// 期のすぐ後ろ (rest の中で) が「#」か「第＋数字」か番組名の終わりのときだけ取り出す。
func extractSeason(before, rest string) (string, string) {
	for _, m := range seasonPattern.FindAllStringSubmatchIndex(before, -1) {
		following := strings.TrimSpace(rest[m[1]:])
		if following != "" && !seasonFollowPattern.MatchString(following) {
			continue
		}
		n, ok := parseSeasonNumber(before[m[2]:m[3]])
		if !ok {
			continue
		}

		return before[:m[0]] + " " + before[m[1]:], "第" + strconv.Itoa(n) + before[m[4]:m[5]]
	}

	return before, ""
}

func parseSeasonNumber(s string) (int, bool) {
	if s[0] >= '0' && s[0] <= '9' {
		return parseDigits(s)
	}
	if utf8.RuneCountInString(s) > MaxSeasonKanjiLength {
		return 0, false
	}

	return parseKanji(s)
}

// extractArc は、作品名の末尾の「空白＋『編』で終わる語」を編として取り出す。
func extractArc(before string) (string, string) {
	m := trailingArcPattern.FindStringSubmatchIndex(before)
	if m == nil {
		return before, ""
	}
	word := before[m[2]:m[3]]
	length := utf8.RuneCountInString(word)
	if length < MinArcWordLength || length > MaxArcWordLength {
		return before, ""
	}
	if excludedArcPattern.MatchString(word) {
		return before, ""
	}

	return strings.TrimSpace(before[:m[0]]), word
}

// kanjiDigit は、漢数字 1 文字の値を返す。
func kanjiDigit(r rune) int {
	for i, c := range []rune(kanjiDigits) {
		if c == r {
			return i
		}
	}

	return 0
}

// parseKanji は、漢数字を読む。十と百は位取りとして読み (十八は 18)、位取りのないものは 1 文字ずつ並べて読む (一〇は 10)。
func parseKanji(s string) (int, bool) {
	positional := strings.ContainsAny(s, "十百")
	total, current := 0, -1
	for _, r := range s {
		switch {
		case r == '百' || r == '十':
			unit := 10
			if r == '百' {
				unit = 100
			}
			total += max(current, 1) * unit
			current = -1
		case positional:
			current = kanjiDigit(r)
		default:
			total = total*10 + kanjiDigit(r)
		}
	}
	if current > 0 {
		total += current
	}

	return total, total > 0
}
