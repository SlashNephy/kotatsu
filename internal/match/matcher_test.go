package match_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/match"
)

const (
	channelGR = "GR"
	channelBS = "BS"
	// serviceGR と serviceBS は、テストの番組のサービス ID。
	serviceGR = 3273601024
	serviceBS = 400101
)

func tokyo(t *testing.T) *time.Location {
	t.Helper()

	location, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	return location
}

// program は、2026-10-10 (土) 23:30 (JST) に始まる 1 時間の地上波のアニメ。
func program(t *testing.T) *match.ProgramInput {
	t.Helper()

	return &match.ProgramInput{
		ServiceID:   serviceGR,
		ChannelType: channelGR,
		Name:        "［新］ゆるキャン△　ＳＥＡＳＯＮ３　第１話",
		Description: "なでしこたちのキャンプの物語。",
		Extended:    map[string]string{"出演者": "花守ゆみり", "スタッフ": "監督：京極尚彦"},
		Genres:      []*match.Genre{{Lv1: 7, Lv2: new(0)}},
		StartAt:     time.Date(2026, 10, 10, 23, 30, 0, 0, tokyo(t)),
		Duration:    time.Hour,
		IsFree:      true,
	}
}

func keyword(text string, fields ...match.Field) *match.Keyword {
	return &match.Keyword{Text: text, Regexp: false, CaseSensitive: false, Fields: fields}
}

func TestMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		conditions *match.Conditions
		// modify は、照合する番組を変える。nil なら変えない。
		modify func(p *match.ProgramInput)
		want   bool
	}{
		{
			name:       "empty conditions match everything",
			conditions: &match.Conditions{},
			want:       true,
		},
		{
			name:       "half-width keyword matches full-width name after NFKC",
			conditions: &match.Conditions{Keywords: []*match.Keyword{keyword("SEASON3", match.FieldName)}},
			want:       true,
		},
		{
			name:       "full-width keyword matches too",
			conditions: &match.Conditions{Keywords: []*match.Keyword{keyword("ゆるキャン△　ＳＥＡＳＯＮ", match.FieldName)}},
			want:       true,
		},
		{
			name: "case-sensitive keyword does not match different case",
			conditions: &match.Conditions{Keywords: []*match.Keyword{
				{Text: "season3", Regexp: false, CaseSensitive: true, Fields: []match.Field{match.FieldName}},
			}},
			want: false,
		},
		{
			name:       "case-insensitive keyword matches different case",
			conditions: &match.Conditions{Keywords: []*match.Keyword{keyword("season3", match.FieldName)}},
			want:       true,
		},
		{
			name:       "keyword only in description does not match when only name is searched",
			conditions: &match.Conditions{Keywords: []*match.Keyword{keyword("キャンプ", match.FieldName)}},
			want:       false,
		},
		{
			name:       "keyword matches any of the given fields",
			conditions: &match.Conditions{Keywords: []*match.Keyword{keyword("キャンプ", match.FieldName, match.FieldDescription)}},
			want:       true,
		},
		{
			name:       "extended matches headings and bodies",
			conditions: &match.Conditions{Keywords: []*match.Keyword{keyword("出演者", match.FieldExtended), keyword("花守", match.FieldExtended)}},
			want:       true,
		},
		{
			name:       "all keywords must match",
			conditions: &match.Conditions{Keywords: []*match.Keyword{keyword("ゆるキャン", match.FieldName), keyword("劇場版", match.FieldName)}},
			want:       false,
		},
		{
			name: "any keyword is enough when keywordMatch is any",
			conditions: &match.Conditions{
				Keywords:     []*match.Keyword{keyword("ゆるキャン", match.FieldName), keyword("劇場版", match.FieldName)},
				KeywordMatch: match.KeywordMatchAny,
			},
			want: true,
		},
		{
			name: "no keyword matches when keywordMatch is any",
			conditions: &match.Conditions{
				Keywords:     []*match.Keyword{keyword("劇場版", match.FieldName), keyword("総集編", match.FieldName)},
				KeywordMatch: match.KeywordMatchAny,
			},
			want: false,
		},
		{
			name: "exclude keywords still exclude when keywordMatch is any",
			conditions: &match.Conditions{
				Keywords:        []*match.Keyword{keyword("ゆるキャン", match.FieldName), keyword("劇場版", match.FieldName)},
				KeywordMatch:    match.KeywordMatchAny,
				ExcludeKeywords: []*match.Keyword{keyword("SEASON", match.FieldName)},
			},
			want: false,
		},
		{
			name:       "regular expression is evaluated against the normalized text",
			conditions: &match.Conditions{Keywords: []*match.Keyword{{Text: `第\d+話$`, Regexp: true, CaseSensitive: false, Fields: []match.Field{match.FieldName}}}},
			want:       true,
		},
		{
			name:       "any exclude keyword excludes",
			conditions: &match.Conditions{ExcludeKeywords: []*match.Keyword{keyword("再放送", match.FieldName), keyword("[新]", match.FieldName)}},
			want:       false,
		},
		{
			name:       "enclosed character 新 from NFKC is also excluded",
			conditions: &match.Conditions{ExcludeKeywords: []*match.Keyword{keyword("新", match.FieldName)}},
			modify:     func(p *match.ProgramInput) { p.Name = "🈟ゆるキャン△" },
			want:       false,
		},
		{
			name:       "channel type in the set",
			conditions: &match.Conditions{ChannelTypes: []string{channelBS, channelGR}},
			want:       true,
		},
		{
			name:       "channel type not in the set",
			conditions: &match.Conditions{ChannelTypes: []string{channelBS}},
			want:       false,
		},
		{
			name:       "service in the set matches even if its channel type is not",
			conditions: &match.Conditions{ChannelTypes: []string{channelBS}, ServiceIDs: []int64{serviceGR}},
			want:       true,
		},
		{
			name:       "service not in the set",
			conditions: &match.Conditions{ServiceIDs: []int64{serviceBS}},
			want:       false,
		},
		{
			name:       "excluded service is excluded even if its channel type is in the set",
			conditions: &match.Conditions{ChannelTypes: []string{channelGR}, ExcludeServiceIDs: []int64{serviceGR}},
			want:       false,
		},
		{
			name:       "excluded service wins over the same service in the set",
			conditions: &match.Conditions{ServiceIDs: []int64{serviceGR}, ExcludeServiceIDs: []int64{serviceGR}},
			want:       false,
		},
		{
			name:       "excluding another service keeps the program",
			conditions: &match.Conditions{ChannelTypes: []string{channelGR}, ExcludeServiceIDs: []int64{serviceBS}},
			want:       true,
		},
		{
			name:       "major genre matches every minor genre",
			conditions: &match.Conditions{Genres: []*match.Genre{{Lv1: 7, Lv2: nil}}},
			want:       true,
		},
		{
			name:       "different minor genre does not match",
			conditions: &match.Conditions{Genres: []*match.Genre{{Lv1: 7, Lv2: new(1)}}},
			want:       false,
		},
		{
			name:       "excluded genre",
			conditions: &match.Conditions{ExcludeGenres: []*match.Genre{{Lv1: 7, Lv2: new(0)}}},
			want:       false,
		},
		{
			name:       "program without genres does not match a genre condition",
			conditions: &match.Conditions{Genres: []*match.Genre{{Lv1: 7, Lv2: nil}}},
			modify:     func(p *match.ProgramInput) { p.Genres = []*match.Genre{} },
			want:       false,
		},
		{
			name:       "time range on Saturday night in JST",
			conditions: &match.Conditions{TimeRanges: []*match.TimeRange{{Weekdays: []int{6}, StartMinute: 23 * 60, DurationMinutes: 60}}},
			want:       true,
		},
		{
			name:       "time range on another weekday",
			conditions: &match.Conditions{TimeRanges: []*match.TimeRange{{Weekdays: []int{5}, StartMinute: 23 * 60, DurationMinutes: 60}}},
			want:       false,
		},
		{
			name: "time range crossing midnight into the next week",
			// 土曜日 23:00 から 2 時間 (日曜日 1:00 まで)。番組は日曜日 0:30 に始まる。
			conditions: &match.Conditions{TimeRanges: []*match.TimeRange{{Weekdays: []int{6}, StartMinute: 23 * 60, DurationMinutes: 120}}},
			modify:     func(p *match.ProgramInput) { p.StartAt = p.StartAt.Add(time.Hour) },
			want:       true,
		},
		{
			name:       "time range end is exclusive",
			conditions: &match.Conditions{TimeRanges: []*match.TimeRange{{Weekdays: []int{6}, StartMinute: 22*60 + 30, DurationMinutes: 60}}},
			want:       false,
		},
		{
			name:       "shorter than the minimum duration",
			conditions: &match.Conditions{MinDurationSeconds: new(3601)},
			want:       false,
		},
		{
			name:       "within the duration range",
			conditions: &match.Conditions{MinDurationSeconds: new(3600), MaxDurationSeconds: new(3600)},
			want:       true,
		},
		{
			name:       "longer than the maximum duration",
			conditions: &match.Conditions{MaxDurationSeconds: new(3599)},
			want:       false,
		},
		{
			name:       "free only excludes a pay program",
			conditions: &match.Conditions{FreeOnly: true},
			modify:     func(p *match.ProgramInput) { p.IsFree = false },
			want:       false,
		},
		{
			name: "combination of conditions",
			conditions: &match.Conditions{
				Keywords:        []*match.Keyword{keyword("ゆるキャン", match.FieldName)},
				ExcludeKeywords: []*match.Keyword{keyword("再放送", match.FieldName, match.FieldDescription)},
				ChannelTypes:    []string{channelGR},
				Genres:          []*match.Genre{{Lv1: 7, Lv2: nil}},
				TimeRanges:      []*match.TimeRange{{Weekdays: []int{5, 6}, StartMinute: 22 * 60, DurationMinutes: 180}},
				FreeOnly:        true,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := program(t)
			if tt.modify != nil {
				tt.modify(in)
			}
			matcher, err := match.Compile(tt.conditions, tokyo(t))
			require.NoError(t, err)
			assert.Equal(t, tt.want, matcher.Match(match.NewProgram(in)))
		})
	}
}

// TestMatchFoldsCase は、正規表現でないキーワードを文字列で探すときも、大文字と小文字の扱いが正規表現の (?i) と同じであることを確かめる (#530)。
func TestMatchFoldsCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		conditions *match.Conditions
		modify     func(p *match.ProgramInput)
		want       bool
	}{
		{
			// 正規表現の (?i) と同じく、大文字と小文字の組をたどって同じ文字とみなす (#530)。ς と σ は、どちらも Σ の小文字。
			name:       "case-insensitive keyword treats letters folding to the same letter as equal",
			conditions: &match.Conditions{Keywords: []*match.Keyword{keyword("ΚΑΛΟς", match.FieldName)}},
			modify:     func(p *match.ProgramInput) { p.Name = "καλοσ" },
			want:       true,
		},
		{
			name: "case-sensitive keyword does not fold letters",
			conditions: &match.Conditions{Keywords: []*match.Keyword{
				{Text: "ΚΑΛΟς", Regexp: false, CaseSensitive: true, Fields: []match.Field{match.FieldName}},
			}},
			modify: func(p *match.ProgramInput) { p.Name = "καλοσ" },
			want:   false,
		},
		{
			name:       "case-insensitive exclude keyword excludes different case",
			conditions: &match.Conditions{ExcludeKeywords: []*match.Keyword{keyword("season3", match.FieldName)}},
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := program(t)
			if tt.modify != nil {
				tt.modify(in)
			}
			matcher, err := match.Compile(tt.conditions, tokyo(t))
			require.NoError(t, err)
			assert.Equal(t, tt.want, matcher.Match(match.NewProgram(in)))
		})
	}
}

// TestMatchMarks は、番組名の印 (囲み文字と、括弧で書いた印) での照合を確かめる。
func TestMatchMarks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		conditions *match.Conditions
		modify     func(p *match.ProgramInput)
		want       bool
	}{
		{
			// EPGStation から移したルールのキーワード [再] は、囲み文字の 🈞 に一致する。
			name:       "bracketed keyword matches the enclosed character",
			conditions: &match.Conditions{Keywords: []*match.Keyword{keyword("[再]", match.FieldName)}},
			modify:     func(p *match.ProgramInput) { p.Name = "ゆるキャン△🈞" },
			want:       true,
		},
		{
			name:       "mark from the enclosed character",
			conditions: &match.Conditions{Marks: []string{"字", "新"}},
			modify:     func(p *match.ProgramInput) { p.Name = "🈟ゆるキャン△" },
			want:       true,
		},
		{
			name:       "mark from the bracketed text",
			conditions: &match.Conditions{Marks: []string{"新"}},
			want:       true,
		},
		{
			// 番組名の文字の「新」は印ではない。
			name:       "plain character is not a mark",
			conditions: &match.Conditions{Marks: []string{"新"}},
			modify:     func(p *match.ProgramInput) { p.Name = "新ゆるキャン△" },
			want:       false,
		},
		{
			name:       "program without the mark",
			conditions: &match.Conditions{Marks: []string{"再"}},
			want:       false,
		},
		{
			name:       "exclude mark excludes",
			conditions: &match.Conditions{ExcludeMarks: []string{"再"}},
			modify:     func(p *match.ProgramInput) { p.Name = "ゆるキャン△🈞" },
			want:       false,
		},
		{
			name:       "exclude mark does not exclude a program without the mark",
			conditions: &match.Conditions{ExcludeMarks: []string{"再"}},
			want:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := program(t)
			if tt.modify != nil {
				tt.modify(in)
			}
			matcher, err := match.Compile(tt.conditions, tokyo(t))
			require.NoError(t, err)
			assert.Equal(t, tt.want, matcher.Match(match.NewProgram(in)))
		})
	}
}
