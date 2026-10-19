package match

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/SlashNephy/kotatsu/internal/epg"
)

// Program は、照合する番組。文字列の項目は、NewProgram で NFKC で正規化しておくか、DB の正規化した列から入れる。
type Program struct {
	ServiceID   int64
	ChannelType string
	Name        string
	Description string
	// Extended は、詳細の見出しと本文を改行でつないだもの。
	Extended string
	Genres   []*Genre
	StartAt  time.Time
	Duration time.Duration
	IsFree   bool
	// Marks は、番組名の印。囲み文字を括弧の書き方にする前に正規化した番組名では、囲み文字が「新」のような文字になり、
	// 番組名の文字と見分けられないので、正規化する前の番組名から epg.Marks で取り出しておく。
	Marks []string

	// folded は、大文字と小文字を区別しないキーワードで探すために、項目ごとに畳んだ文字列。照合のときに初めて作る。
	folded map[Field]string
}

// ProgramInput は、NewProgram に渡す番組の情報。文字列は正規化する前のものでよい。
type ProgramInput struct {
	ServiceID   int64
	ChannelType string
	Name        string
	Description string
	// Extended は、詳細の見出しから本文への対応。
	Extended map[string]string
	Genres   []*Genre
	StartAt  time.Time
	Duration time.Duration
	IsFree   bool
}

// NewProgram は、照合する番組を作る。文字列を NFKC で正規化する (設計書 3.5)。
// 詳細は、見出しの順に「見出し\n本文」を改行でつなぐ。
func NewProgram(in *ProgramInput) *Program {
	return &Program{
		ServiceID:   in.ServiceID,
		ChannelType: in.ChannelType,
		Name:        epg.Normalize(in.Name),
		Description: epg.Normalize(in.Description),
		Extended:    epg.NormalizeExtended(in.Extended),
		Genres:      in.Genres,
		StartAt:     in.StartAt,
		Duration:    in.Duration,
		IsFree:      in.IsFree,
		Marks:       epg.Marks(in.Name),
		folded:      nil,
	}
}

// foldedText は、項目 field の文字列を、大文字と小文字を区別しない形に畳んで返す。
// 番組 1 つを多くのルールと照合するので、項目ごとに 1 度だけ畳む。
func (p *Program) foldedText(field Field) string {
	if text, ok := p.folded[field]; ok {
		return text
	}
	if p.folded == nil {
		p.folded = map[Field]string{}
	}
	text := foldCase(p.text(field))
	p.folded[field] = text

	return text
}

// text は、項目 field の文字列を返す。
func (p *Program) text(field Field) string {
	switch field {
	case FieldName:
		return p.Name
	case FieldDescription:
		return p.Description
	case FieldExtended:
		return p.Extended
	}

	return ""
}

// Matcher は、検証してコンパイルした条件。複数の goroutine から使ってよい。
type Matcher struct {
	conditions        *Conditions
	keywords          []*compiledKeyword
	excludeKeywords   []*compiledKeyword
	channelTypes      map[string]bool
	serviceIDs        map[int64]bool
	excludeServiceIDs map[int64]bool
	location          *time.Location
}

// compiledKeyword は、照合できる形にしたキーワード。正規表現なら pattern を、文字どおりのキーワードなら literal を使う。
// 文字どおりのキーワードを正規表現で探すと、照合の時間の大半を占めるので、文字列の検索にする (#530)。
type compiledKeyword struct {
	pattern *regexp.Regexp
	literal string
	// fold は、literal を、大文字と小文字を区別せずに探すかどうか。literal は畳んで持つ。
	fold   bool
	fields []Field
}

// Compile は、条件を検証し、照合できる形にする。location は、曜日と時間帯を判定するタイムゾーン。
func Compile(c *Conditions, location *time.Location) (*Matcher, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if location == nil {
		return nil, errors.New("location must not be nil")
	}

	m := &Matcher{
		conditions:        c,
		keywords:          make([]*compiledKeyword, 0, len(c.Keywords)),
		excludeKeywords:   make([]*compiledKeyword, 0, len(c.ExcludeKeywords)),
		channelTypes:      map[string]bool{},
		serviceIDs:        map[int64]bool{},
		excludeServiceIDs: map[int64]bool{},
		location:          location,
	}
	for _, keyword := range c.Keywords {
		compiled, err := compileKeyword(keyword)
		if err != nil {
			return nil, err
		}
		m.keywords = append(m.keywords, compiled)
	}
	for _, keyword := range c.ExcludeKeywords {
		compiled, err := compileKeyword(keyword)
		if err != nil {
			return nil, err
		}
		m.excludeKeywords = append(m.excludeKeywords, compiled)
	}
	for _, channelType := range c.ChannelTypes {
		m.channelTypes[channelType] = true
	}
	for _, serviceID := range c.ServiceIDs {
		m.serviceIDs[serviceID] = true
	}
	for _, serviceID := range c.ExcludeServiceIDs {
		m.excludeServiceIDs[serviceID] = true
	}

	return m, nil
}

// compileKeyword は、キーワードを照合できる形にする。
// 正規表現でないキーワードは、NFKC で正規化してから、文字どおりに探す。
func compileKeyword(k *Keyword) (*compiledKeyword, error) {
	if !k.Regexp {
		literal := epg.Normalize(k.Text)
		// 大文字と小文字の区別がない文字 (かなや漢字、数字) だけのキーワードは、区別してもしなくても同じなので、番組の文字列を畳まずに探す。
		fold := !k.CaseSensitive && hasCase(literal)
		if fold {
			literal = foldCase(literal)
		}

		return &compiledKeyword{pattern: nil, literal: literal, fold: fold, fields: k.Fields}, nil
	}
	pattern := k.Text
	if !k.CaseSensitive {
		pattern = "(?i)" + pattern
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid regular expression: %w", ErrInvalidConditions, err)
	}

	return &compiledKeyword{pattern: compiled, literal: "", fold: false, fields: k.Fields}, nil
}

// hasCase は、s に、大文字と小文字で別の形を持つ文字があるかどうか。
func hasCase(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return unicode.SimpleFold(r) != r })
}

// foldCase は、大文字と小文字を区別しない比較のために、文字ごとに、正規表現の (?i) と同じく同じ文字とみなす組
// (unicode.SimpleFold でたどれる文字) の中で最も小さい文字にそろえる。
func foldCase(s string) string {
	return strings.Map(func(r rune) rune {
		if r < utf8.RuneSelf {
			// ASCII の小文字の組で最も小さいのは、大文字。k と s の組の ASCII でない文字 (K と ſ) も、ASCII の大文字にそろう。
			if 'a' <= r && r <= 'z' {
				return r - 'a' + 'A'
			}

			return r
		}
		smallest := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			smallest = min(smallest, f)
		}

		return smallest
	}, s)
}

// Match は、番組が条件に一致するかどうかを返す。安い条件から順に調べる。
func (m *Matcher) Match(p *Program) bool {
	c := m.conditions
	if c.FreeOnly && !p.IsFree {
		return false
	}
	if c.MinDurationSeconds != nil && p.Duration < time.Duration(*c.MinDurationSeconds)*time.Second {
		return false
	}
	if c.MaxDurationSeconds != nil && p.Duration > time.Duration(*c.MaxDurationSeconds)*time.Second {
		return false
	}
	if (len(m.channelTypes) > 0 || len(m.serviceIDs) > 0) && !m.channelTypes[p.ChannelType] && !m.serviceIDs[p.ServiceID] {
		return false
	}
	if m.excludeServiceIDs[p.ServiceID] {
		return false
	}
	if len(c.Genres) > 0 && !anyGenre(c.Genres, p.Genres) {
		return false
	}
	if anyGenre(c.ExcludeGenres, p.Genres) {
		return false
	}
	if !m.matchMarks(p) {
		return false
	}
	if len(c.TimeRanges) > 0 && !m.inTimeRanges(p.StartAt) {
		return false
	}
	if !m.matchKeywords(p) {
		return false
	}

	return !slices.ContainsFunc(m.excludeKeywords, func(k *compiledKeyword) bool { return k.match(p) })
}

// matchKeywords は、番組が含めるキーワードに一致するかどうかを、照合のしかた (KeywordMatch) に従って返す。
// キーワードがなければ、絞り込まない。
func (m *Matcher) matchKeywords(p *Program) bool {
	if len(m.keywords) == 0 {
		return true
	}
	matches := func(k *compiledKeyword) bool { return k.match(p) }
	if m.conditions.KeywordMatch == KeywordMatchAny {
		return slices.ContainsFunc(m.keywords, matches)
	}

	return !slices.ContainsFunc(m.keywords, func(k *compiledKeyword) bool { return !matches(k) })
}

func (k *compiledKeyword) match(p *Program) bool {
	for _, field := range k.fields {
		var found bool
		switch {
		case k.pattern != nil:
			found = k.pattern.MatchString(p.text(field))
		case k.fold:
			found = strings.Contains(p.foldedText(field), k.literal)
		default:
			found = strings.Contains(p.text(field), k.literal)
		}
		if found {
			return true
		}
	}

	return false
}

// anyGenre は、番組のジャンルのどれかが、条件のジャンルのどれかに当たるかどうかを返す。
func anyGenre(conditions, genres []*Genre) bool {
	for _, condition := range conditions {
		for _, genre := range genres {
			if condition.Lv1 == genre.Lv1 && (condition.Lv2 == nil || (genre.Lv2 != nil && *condition.Lv2 == *genre.Lv2)) {
				return true
			}
		}
	}

	return false
}

// matchMarks は、番組が、含める印のどれかを持ち (含める印がなければ絞り込まない)、除外する印をどれも持たないかどうかを返す。
func (m *Matcher) matchMarks(p *Program) bool {
	c := m.conditions
	if len(c.Marks) > 0 && !anyMark(c.Marks, p.Marks) {
		return false
	}

	return !anyMark(c.ExcludeMarks, p.Marks)
}

// anyMark は、番組の印のどれかが、条件の印のどれかに当たるかどうかを返す。
func anyMark(conditions, marks []string) bool {
	return slices.ContainsFunc(conditions, func(mark string) bool { return slices.Contains(marks, mark) })
}

// inTimeRanges は、開始時刻が、曜日と時間帯の組のどれかに入るかどうかを返す。
// 週の中の位置 (日曜日 0 時からの分) で比べるので、日付や週をまたぐ範囲も扱える。
func (m *Matcher) inTimeRanges(start time.Time) bool {
	local := start.In(m.location)
	minute := int(local.Weekday())*minutesPerDay + local.Hour()*60 + local.Minute()
	for _, r := range m.conditions.TimeRanges {
		for _, weekday := range r.Weekdays {
			from := weekday*minutesPerDay + r.StartMinute
			if (minute-from+minutesPerWeek)%minutesPerWeek < r.DurationMinutes {
				return true
			}
		}
	}

	return false
}
