package episode

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// summaryNumber は、概要の話数の数字。数字か、十と百の位取りを含む漢数字。
const summaryNumber = `([0-9]+|[〇一二三四五六七八九十百]+)`

var (
	// quotedSummaryPattern は、概要の、鍵括弧の副題が付いた話数 (1 回目の読み方)。
	quotedSummaryPattern = regexp.MustCompile(`(?i)(?:#` + summaryNumber + `|第` + summaryNumber + `[話回]|(?:chapter|episode|ep)\.?` + summaryNumber +
		`)\s*(?:「([^「」]*)」|『([^『』]*)』)`)
	// plainSummaryPattern は、概要の、副題の無い話数 (2 回目の読み方)。「第N回」は地の文に紛れやすいので読まない。
	plainSummaryPattern = regexp.MustCompile(`(?i)(?:#` + summaryNumber + `|第` + summaryNumber + `話|(?:chapter|episode|ep)\.?` + summaryNumber + `)`)
	// referencePattern は、ほかの回を指す語。同じ文で、一致した位置より前にあれば、その一致は捨てる。
	referencePattern = regexp.MustCompile(`前回|次回|予告`)
)

const (
	// plainLeaders は、2 回目の読み方で、話数の直前に来てよい文字 (行の先頭も可)。NFKC で ⁉ は !? になる。
	plainLeaders = "。!?⁉【[】>▼▽◆◇■□★☆"
	// plainFollowers は、2 回目の読み方で、話数の直後に来てよい文字 (空白と行の終わりも可)。
	plainFollowers = "「『:.、。)]】▼▽◆◇■□"
	// plainSubtitleEnds は、2 回目の読み方で、副題の終わりとする記号。
	plainSubtitleEnds = "▼▽◆◇■□"
	// plainSubtitleForbidden は、2 回目の読み方で、含めば副題とみない文字。地の文の可能性が高い。
	plainSubtitleForbidden = "。、,!?！？…"
	// sentenceBreaks は、文の区切り。
	sentenceBreaks = "。\n」』"
)

// summaryEpisode は、概要から読んだ回。
type summaryEpisode struct {
	number    int
	subtitle  string
	uncertain bool
}

// readSummary は、概要から回を読む。鍵括弧の副題が付いた話数を先に探し、見つからなければ副題の無い話数を探す。
func readSummary(text string) *summaryEpisode {
	if s := readQuotedSummary(text); s != nil {
		return s
	}

	return readPlainSummary(text)
}

func readQuotedSummary(text string) *summaryEpisode {
	for _, m := range quotedSummaryPattern.FindAllStringSubmatchIndex(text, -1) {
		n, ok := summaryMatchNumber(text, m)
		if !ok || refersOther(text, m[0]) {
			continue
		}
		subtitle := submatch(text, m, 4) + submatch(text, m, 5)
		if length := utf8.RuneCountInString(subtitle); length == 0 || length > MaxQuotedSummarySubtitleLength {
			continue
		}

		return &summaryEpisode{number: n, subtitle: subtitle, uncertain: false}
	}

	return nil
}

func readPlainSummary(text string) *summaryEpisode {
	for _, m := range plainSummaryPattern.FindAllStringSubmatchIndex(text, -1) {
		n, ok := summaryMatchNumber(text, m)
		if !ok || !plainLeads(text[:m[0]]) || !plainFollows(text[m[1]:]) || refersOther(text, m[0]) {
			continue
		}

		return &summaryEpisode{number: n, subtitle: plainSubtitle(text[m[1]:]), uncertain: true}
	}

	return nil
}

// summaryMatchNumber は、一致した話数を読む。英語の呼び方は、直前が英字か数字なら語の一部なので読まない。
func summaryMatchNumber(text string, m []int) (int, bool) {
	if digits := submatch(text, m, 3); digits != "" {
		before, _ := utf8.DecodeLastRuneInString(text[:m[0]])
		if m[0] > 0 && (isLatin(before) || unicode.IsDigit(before)) {
			return 0, false
		}

		return parseSummaryNumber(digits)
	}

	return parseSummaryNumber(submatch(text, m, 1) + submatch(text, m, 2))
}

func parseSummaryNumber(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	if s[0] >= '0' && s[0] <= '9' {
		return parseDigits(s)
	}
	if utf8.RuneCountInString(s) > MaxEpisodeKanjiLength {
		return 0, false
	}

	return parseKanji(s)
}

// plainLeads は、話数の直前が、行の先頭か、置いてよい記号かを返す。間の空白は読み飛ばす。
func plainLeads(before string) bool {
	before = strings.TrimRight(before, " \t")
	if before == "" {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(before)

	return r == '\n' || strings.ContainsRune(plainLeaders, r)
}

// plainFollows は、話数の直後が、空白か、行の終わりか、置いてよい記号かを返す。
func plainFollows(after string) bool {
	if after == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(after)

	return unicode.IsSpace(r) || strings.ContainsRune(plainFollowers, r)
}

// plainSubtitle は、話数の後ろの行の残りから、副題を読む。地の文らしければ空を返す。
func plainSubtitle(after string) string {
	line, _, _ := strings.Cut(after, "\n")
	if i := strings.IndexAny(line, plainSubtitleEnds); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimRightFunc(strings.TrimLeft(line, " \t:.)]】"), unicode.IsSpace)
	if length := utf8.RuneCountInString(line); length == 0 || length > MaxPlainSummarySubtitleLength {
		return ""
	}
	if strings.ContainsAny(line, plainSubtitleForbidden) {
		return ""
	}

	return line
}

// refersOther は、一致した位置より前の同じ文に、ほかの回を指す語 (前回、次回、予告) があるかを返す。
func refersOther(text string, at int) bool {
	sentence := text[:at]
	if i := strings.LastIndexAny(sentence, sentenceBreaks); i >= 0 {
		_, size := utf8.DecodeRuneInString(sentence[i:])
		sentence = sentence[i+size:]
	}

	return referencePattern.MatchString(sentence)
}

// applySummary は、概要から読んだ回を、番組名から読んだ回に当てはめる。
func applySummary(ep *Episode, s *summaryEpisode) {
	switch {
	case s == nil:
		return
	case ep.Number == 0 && ep.Subtitle == "":
		// 番組名で話数を読めず、副題も無ければ、概要から読んだ話数と副題を使う。
		ep.Number = s.number
		ep.Subtitle = canon(s.subtitle)
		ep.UncertainSubtitle = s.uncertain
	case ep.Number > 0 && s.number == ep.Number && s.subtitle != "":
		// 番組名で読んだ話数と同じなら、概要の副題に替える。番組名の副題が無いか、確かでないときだけここに来る。
		ep.Subtitle = canon(s.subtitle)
		ep.UncertainSubtitle = true
	}
}
