package match

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/epg"
)

// Program は、照合する番組。文字列の項目は、NewProgram で NFKC で正規化しておく。
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
	var extended strings.Builder
	for _, heading := range slices.Sorted(maps.Keys(in.Extended)) {
		extended.WriteString(heading)
		extended.WriteString("\n")
		extended.WriteString(in.Extended[heading])
		extended.WriteString("\n")
	}

	return &Program{
		ServiceID:   in.ServiceID,
		ChannelType: in.ChannelType,
		Name:        epg.Normalize(in.Name),
		Description: epg.Normalize(in.Description),
		Extended:    epg.Normalize(extended.String()),
		Genres:      in.Genres,
		StartAt:     in.StartAt,
		Duration:    in.Duration,
		IsFree:      in.IsFree,
	}
}

// Matcher は、検証してコンパイルした条件。複数の goroutine から使ってよい。
type Matcher struct {
	conditions      *Conditions
	keywords        []*compiledKeyword
	excludeKeywords []*compiledKeyword
	channelTypes    map[string]bool
	serviceIDs      map[int64]bool
	location        *time.Location
}

type compiledKeyword struct {
	pattern *regexp.Regexp
	fields  []Field
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
		conditions:      c,
		keywords:        make([]*compiledKeyword, 0, len(c.Keywords)),
		excludeKeywords: make([]*compiledKeyword, 0, len(c.ExcludeKeywords)),
		channelTypes:    map[string]bool{},
		serviceIDs:      map[int64]bool{},
		location:        location,
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

	return m, nil
}

// compileKeyword は、キーワードを正規表現にする。
// 正規表現でないキーワードは、NFKC で正規化してから、文字どおりに一致するパターンにする。
func compileKeyword(k *Keyword) (*compiledKeyword, error) {
	pattern := k.Text
	if !k.Regexp {
		pattern = regexp.QuoteMeta(epg.Normalize(k.Text))
	}
	if !k.CaseSensitive {
		pattern = "(?i)" + pattern
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid regular expression: %w", ErrInvalidConditions, err)
	}

	return &compiledKeyword{pattern: compiled, fields: k.Fields}, nil
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
	if len(c.Genres) > 0 && !anyGenre(c.Genres, p.Genres) {
		return false
	}
	if anyGenre(c.ExcludeGenres, p.Genres) {
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
		var text string
		switch field {
		case FieldName:
			text = p.Name
		case FieldDescription:
			text = p.Description
		case FieldExtended:
			text = p.Extended
		}
		if k.pattern.MatchString(text) {
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
