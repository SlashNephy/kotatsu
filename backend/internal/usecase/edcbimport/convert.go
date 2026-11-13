// Package edcbimport は、EDCB (EpgTimerSrv) のキーワード予約、予約、録画済みの一覧を Kotatsu に取り込む (#1503、設計書 6.3)。
//
// EpgTimerSrv からは、EpgTimerNW と同じ TCP の CtrlCmd で読む。録画のファイルは移せない (CtrlCmd にファイルを送るコマンドはない)。
package edcbimport

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
)

const (
	// serviceIDBase は、Kotatsu のサービス ID (ONID × 100000 + SID) の基数。
	serviceIDBase = 100000
	// maxRuleNameLength は、ルールの名前の長さの上限 (rule.Input の検証と同じ)。
	maxRuleNameLength = 200
	// minutesPerWeek は、1 週間の分の数。
	minutesPerWeek = 7 * 24 * 60
	// maxGenreLevel1 は、写すジャンルの大分類の上限。0x60〜0x7F は拡張ジャンル、0xFF はジャンル情報なし。
	maxGenreLevel1 = 0xF
	// anyGenreLevel2 は、中分類のすべてを表す値。
	anyGenreLevel2 = 0xFF
)

// keyword は、AND キーワードの先頭の印を外した結果。
type keyword struct {
	disabled      bool
	caseSensitive bool
	// minMinutes と maxMinutes は、番組の長さの条件 (分)。0 なら条件なし。
	minMinutes, maxMinutes int
	text                   string
}

// parseAndKey は、AND キーワードの先頭の印を外す。印は「^!{999}」(無効)、「C!{999}」(大小を区別)、
// 「D!{1」と数字 8 桁と「}」(番組の長さ。最短と最長を 4 桁ずつの分とみる) の順に付く。
func parseAndKey(s string) keyword {
	k := keyword{disabled: false, caseSensitive: false, minMinutes: 0, maxMinutes: 0, text: s}
	if rest, ok := strings.CutPrefix(k.text, "^!{999}"); ok {
		k.disabled, k.text = true, rest
	}
	if rest, ok := strings.CutPrefix(k.text, "C!{999}"); ok {
		k.caseSensitive, k.text = true, rest
	}
	if rest, ok := strings.CutPrefix(k.text, "D!{1"); ok && len(rest) >= 9 && rest[8] == '}' {
		minMinutes, err1 := strconv.Atoi(rest[:4])
		maxMinutes, err2 := strconv.Atoi(rest[4:8])
		if err1 == nil && err2 == nil {
			k.minMinutes, k.maxMinutes, k.text = minMinutes, maxMinutes, rest[9:]
		}
	}

	return k
}

// parseNotKey は、NOT キーワードの先頭のメモ (「:note:」から最初の空白まで) を外す。
func parseNotKey(s string) string {
	rest, ok := strings.CutPrefix(s, ":note:")
	if !ok {
		return s
	}
	if i := strings.IndexFunc(rest, unicode.IsSpace); i >= 0 {
		return strings.TrimLeftFunc(rest[i:], unicode.IsSpace)
	}

	return ""
}

// unsupportedWordPrefixes は、語ごとの照合範囲や正規表現の印。Kotatsu の条件では表せない。
func unsupportedWordPrefixes() []string {
	return []string{"::", ":title:", ":event:", ":genre:", ":video:", ":audio:"}
}

// unsupportedError は、Kotatsu の条件で表せないことを表す。
type unsupportedError struct {
	what string
}

func (e *unsupportedError) Error() string {
	return "cannot be represented in Kotatsu: " + e.what
}

func unsupported(format string, args ...any) error {
	return &unsupportedError{what: fmt.Sprintf(format, args...)}
}

// keywordsOf は、キーワードの文字列を Kotatsu のキーワードにする。
//   - 正規表現なら、文字列全体を 1 つの正規表現にする。Go の RE2 で読めない式は、表せないとして飛ばす (EDCB の方言は RE2 と違う)。
//   - そうでなければ、空白 (全角も) で区切った語にする。「|」の語で分けた組がどれも 1 語なら、いずれかに一致 (any) にする。
func keywordsOf(text string, regexpFlag, caseSensitive bool, fields []match.Field) ([]*match.Keyword, match.KeywordMatch, error) {
	if text == "" {
		return []*match.Keyword{}, match.KeywordMatchAll, nil
	}
	newKeyword := func(t string) *match.Keyword {
		return &match.Keyword{Text: t, Regexp: regexpFlag, CaseSensitive: caseSensitive, Fields: append([]match.Field{}, fields...)}
	}
	if regexpFlag {
		if _, err := regexp.Compile(text); err != nil {
			return nil, "", unsupported("regular expression %q (%v)", text, err)
		}

		return []*match.Keyword{newKeyword(text)}, match.KeywordMatchAll, nil
	}
	words := strings.Fields(text)
	for _, word := range words {
		for _, prefix := range unsupportedWordPrefixes() {
			if strings.HasPrefix(word, prefix) {
				return nil, "", unsupported("word %q", word)
			}
		}
	}
	groups := [][]string{{}}
	for _, word := range words {
		if word == "|" {
			groups = append(groups, []string{})

			continue
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], word)
	}
	keywords := []*match.Keyword{}
	if len(groups) == 1 {
		for _, word := range words {
			keywords = append(keywords, newKeyword(word))
		}

		return keywords, match.KeywordMatchAll, nil
	}
	for _, group := range groups {
		if len(group) != 1 {
			return nil, "", unsupported("OR groups with more than one word in %q", text)
		}
		keywords = append(keywords, newKeyword(group[0]))
	}

	return keywords, match.KeywordMatchAny, nil
}

// excludeKeywordsOf は、NOT キーワードを Kotatsu の除外のキーワードにする。正規表現なら文字列全体を 1 つの式に、
// そうでなければ空白で区切った語ごとのキーワードにする。
func excludeKeywordsOf(text string, regexpFlag, caseSensitive bool, fields []match.Field) ([]*match.Keyword, error) {
	if text == "" || regexpFlag {
		keywords, _, err := keywordsOf(text, regexpFlag, caseSensitive, fields)

		return keywords, err
	}
	keywords := []*match.Keyword{}
	for word := range strings.FieldsSeq(text) {
		words, _, err := keywordsOf(word, false, caseSensitive, fields)
		if err != nil {
			return nil, err
		}
		keywords = append(keywords, words...)
	}

	return keywords, nil
}

// serviceIDOf は、EDCB の詰めたサービスの値 (上から ONID、TSID、SID を 16 ビットずつ) を、Kotatsu のサービス ID にする。TSID は使わない。
func serviceIDOf(packed uint64) int64 {
	onid, sid := int64(packed>>32&0xFFFF), int64(packed&0xFFFF)

	return onid*serviceIDBase + sid
}

// genreOf は、EDCB のジャンルを Kotatsu のジャンルにする。写せないジャンル (ジャンル情報なしと拡張ジャンル) は false を返す。
func genreOf(c edcb.ContentData) (*match.Genre, bool) {
	if c.Level1 > maxGenreLevel1 {
		return nil, false
	}
	genre := &match.Genre{Lv1: int(c.Level1), Lv2: nil}
	if c.Level2 != anyGenreLevel2 {
		genre.Lv2 = new(int(c.Level2))
	}

	return genre, true
}

// weekMinute は、曜日と時刻を、週の始まり (日曜の 0 時) からの分にする。範囲外の値は、EDCB と同じく丸める。
func weekMinute(dayOfWeek uint8, hour, minute uint16) int {
	return int(dayOfWeek%7)*24*60 + int(hour%24)*60 + int(minute%60)
}

// timeRangeOf は、EDCB の時間帯を Kotatsu の時刻の範囲にする。
// EDCB は、番組の開始が「開始 ≤ 開始時刻 ≤ 終了」に入れば当てる (両端を含む)。Kotatsu は半開区間なので、長さに 1 分を足す。
// 曜日は開始の曜日 1 つにし、日をまたぐ時間帯は長さで表す。翌日の曜日を足すと、当たる番組が増えるからである。
func timeRangeOf(d edcb.SearchDate) *match.TimeRange {
	start, end := weekMinute(d.StartDayOfWeek, d.StartHour, d.StartMinute), weekMinute(d.EndDayOfWeek, d.EndHour, d.EndMinute)
	duration := ((end-start)%minutesPerWeek+minutesPerWeek)%minutesPerWeek + 1

	return &match.TimeRange{Weekdays: []int{start / (24 * 60)}, StartMinute: start % (24 * 60), DurationMinutes: duration}
}

// complementTimeRanges は、時間帯の NOT を、補集合の時刻の範囲にする。週を分の単位で塗り、塗っていない区間を範囲にする。
func complementTimeRanges(dates []edcb.SearchDate) []*match.TimeRange {
	var covered [minutesPerWeek]bool
	for _, d := range dates {
		r := timeRangeOf(d)
		start := r.Weekdays[0]*24*60 + r.StartMinute
		for i := range r.DurationMinutes {
			covered[(start+i)%minutesPerWeek] = true
		}
	}
	// 週の終わりと始まりにまたがる区間を 1 つにするため、塗った分から数え始める。
	origin := -1
	for i, c := range covered {
		if c {
			origin = i

			break
		}
	}
	if origin < 0 {
		return []*match.TimeRange{{Weekdays: []int{0}, StartMinute: 0, DurationMinutes: minutesPerWeek}}
	}
	ranges := []*match.TimeRange{}
	runStart := -1
	for offset := range minutesPerWeek + 1 {
		minute := (origin + offset) % minutesPerWeek
		free := offset < minutesPerWeek && !covered[minute]
		switch {
		case free && runStart < 0:
			runStart = minute
		case !free && runStart >= 0:
			length := ((minute-runStart)%minutesPerWeek + minutesPerWeek) % minutesPerWeek
			ranges = append(ranges, &match.TimeRange{Weekdays: []int{runStart / (24 * 60)}, StartMinute: runStart % (24 * 60), DurationMinutes: length})
			runStart = -1
		}
	}

	return ranges
}

// ruleName は、ルールの名前を、印を外した AND キーワードから作る。空なら「EDCB #ID」にする。
func ruleName(text string, id uint32) string {
	name := strings.Join(strings.Fields(text), " ")
	if name == "" {
		return "EDCB #" + strconv.FormatUint(uint64(id), 10)
	}
	if utf8.RuneCountInString(name) > maxRuleNameLength {
		name = string([]rune(name)[:maxRuleNameLength])
	}

	return name
}
