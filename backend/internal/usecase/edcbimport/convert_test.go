package edcbimport_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
	"github.com/SlashNephy/kotatsu/internal/usecase/edcbimport"
)

const nhk = 32736<<32 | 32736<<16 | 1024

func autoAdd(edit func(*edcb.AutoAdd)) *edcb.AutoAdd {
	a := &edcb.AutoAdd{
		ID: 7,
		Search: edcb.SearchKey{
			AndKey: "ニュース", NotKey: "", Regexp: false, TitleOnly: true, Contents: []edcb.ContentData{}, Dates: []edcb.SearchDate{},
			Services: []uint64{nhk}, Fuzzy: false, NotContents: false, NotDates: false, FreeCA: 0, CheckRecorded: false,
		},
		RecSetting: edcb.RecSetting{RecMode: 1, Priority: 2},
	}
	edit(a)

	return a
}

func titleKeyword(text string) *match.Keyword {
	return &match.Keyword{Text: text, Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldName}}
}

func TestRuleInput(t *testing.T) {
	t.Parallel()

	services := map[int64]bool{3273601024: true}
	tests := []struct {
		name  string
		edit  func(*edcb.AutoAdd)
		check func(t *testing.T, conditions *match.Conditions, enabled bool, name string)
		// wantSkip なら、Kotatsu で表せないので飛ばす。
		wantSkip bool
	}{
		{
			// 印を外し、無効と大小の区別と番組の長さを読む。
			name: "marks",
			edit: func(a *edcb.AutoAdd) { a.Search.AndKey = "^!{999}C!{999}D!{100150090}アニメ　再放送" },
			check: func(t *testing.T, c *match.Conditions, enabled bool, name string) {
				t.Helper()
				assert.False(t, enabled)
				assert.Equal(t, "アニメ 再放送", name)
				require.Len(t, c.Keywords, 2)
				assert.True(t, c.Keywords[0].CaseSensitive)
				assert.Equal(t, new(15*60), c.MinDurationSeconds)
				assert.Equal(t, new(90*60), c.MaxDurationSeconds)
			},
		},
		{
			// :note: のメモを外し、残りの語をどれか 1 つに一致したら除外する。
			name: "note",
			edit: func(a *edcb.AutoAdd) { a.Search.NotKey = ":note:メモ 天気 交通" },
			check: func(t *testing.T, c *match.Conditions, _ bool, _ string) {
				t.Helper()
				assert.Equal(t, []*match.Keyword{titleKeyword("天気"), titleKeyword("交通")}, c.ExcludeKeywords)
			},
		},
		{
			name: "OR of single words",
			edit: func(a *edcb.AutoAdd) { a.Search.AndKey = "ニュース | 天気" },
			check: func(t *testing.T, c *match.Conditions, _ bool, _ string) {
				t.Helper()
				assert.Equal(t, match.KeywordMatchAny, c.KeywordMatch)
				assert.Equal(t, []*match.Keyword{titleKeyword("ニュース"), titleKeyword("天気")}, c.Keywords)
			},
		},
		{name: "OR of groups", edit: func(a *edcb.AutoAdd) { a.Search.AndKey = "A B | C" }, check: nil, wantSkip: true},
		{name: "per-word field", edit: func(a *edcb.AutoAdd) { a.Search.AndKey = ":title:ニュース" }, check: nil, wantSkip: true},
		{name: "regexp RE2 cannot read", edit: func(a *edcb.AutoAdd) { a.Search.AndKey, a.Search.Regexp = `(?<=a)b`, true }, check: nil, wantSkip: true},
		// EDCB では、局が空のルールは何にも当たらない。全局にしない。
		{name: "no services", edit: func(a *edcb.AutoAdd) { a.Search.Services = []uint64{} }, check: nil, wantSkip: true},
		{name: "unknown services only", edit: func(a *edcb.AutoAdd) { a.Search.Services = []uint64{4<<32 | 4<<16 | 101} }, check: nil, wantSkip: true},
		{name: "paid only", edit: func(a *edcb.AutoAdd) { a.Search.FreeCA = 2 }, check: nil, wantSkip: true},
		{
			// ジャンル情報なし (0xFF/0xFF) と拡張ジャンルは外し、中分類 0xFF は大分類の全部にする。
			name: "genres",
			edit: func(a *edcb.AutoAdd) {
				a.Search.Contents = []edcb.ContentData{{Level1: 0xFF, Level2: 0xFF}, {Level1: 0x60, Level2: 0}, {Level1: 7, Level2: 0xFF}, {Level1: 0, Level2: 1}}
				a.Search.NotContents = true
			},
			check: func(t *testing.T, c *match.Conditions, _ bool, _ string) {
				t.Helper()
				assert.Empty(t, c.Genres)
				assert.Equal(t, []*match.Genre{{Lv1: 7, Lv2: nil}, {Lv1: 0, Lv2: new(1)}}, c.ExcludeGenres)
			},
		},
		{name: "unrepresentable genres only", edit: func(a *edcb.AutoAdd) { a.Search.Contents = []edcb.ContentData{{Level1: 0xFF, Level2: 0xFF}} }, check: nil, wantSkip: true},
		{
			name: "recording mode watch",
			edit: func(a *edcb.AutoAdd) { a.RecSetting.RecMode = 4 },
			check: func(t *testing.T, _ *match.Conditions, enabled bool, _ string) {
				t.Helper()
				assert.False(t, enabled)
			},
		},
		{
			name: "empty keyword name",
			edit: func(a *edcb.AutoAdd) { a.Search.AndKey = "" },
			check: func(t *testing.T, _ *match.Conditions, _ bool, name string) {
				t.Helper()
				assert.Equal(t, "EDCB #7", name)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input, err := edcbimport.RuleInput(autoAdd(tt.edit), services)
			if tt.wantSkip {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, []int64{3273601024}, input.Conditions.ServiceIDs)
			require.NoError(t, input.Conditions.Validate())
			tt.check(t, input.Conditions, input.Enabled, input.Name)
		})
	}
}

func TestTimeRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		date edcb.SearchDate
		want *match.TimeRange
	}{
		// 両端を含むので、長さに 1 分を足す。
		{name: "evening", date: edcb.SearchDate{StartDayOfWeek: 1, StartHour: 19, StartMinute: 0, EndDayOfWeek: 1, EndHour: 22, EndMinute: 59}, want: &match.TimeRange{Weekdays: []int{1}, StartMinute: 19 * 60, DurationMinutes: 4 * 60}},
		{name: "whole day", date: edcb.SearchDate{StartDayOfWeek: 3, StartHour: 0, StartMinute: 0, EndDayOfWeek: 3, EndHour: 23, EndMinute: 59}, want: &match.TimeRange{Weekdays: []int{3}, StartMinute: 0, DurationMinutes: 24 * 60}},
		// 日をまたぐ時間帯は、開始の曜日と長さで表す。翌日の曜日は足さない。
		{name: "past midnight", date: edcb.SearchDate{StartDayOfWeek: 6, StartHour: 23, StartMinute: 0, EndDayOfWeek: 0, EndHour: 1, EndMinute: 0}, want: &match.TimeRange{Weekdays: []int{6}, StartMinute: 23 * 60, DurationMinutes: 2*60 + 1}},
		// 範囲外の値は、EDCB と同じく丸める。
		{name: "wrapped values", date: edcb.SearchDate{StartDayOfWeek: 8, StartHour: 25, StartMinute: 61, EndDayOfWeek: 1, EndHour: 1, EndMinute: 1}, want: &match.TimeRange{Weekdays: []int{1}, StartMinute: 61, DurationMinutes: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, edcbimport.TimeRangeOf(tt.date))
		})
	}
}

func TestComplementTimeRanges(t *testing.T) {
	t.Parallel()

	// 月曜の 19:00〜22:59 を除くと、月曜の 23:00 から翌週の月曜の 18:59 までになる。週の終わりをまたいでも 1 つにまとめる。
	got := edcbimport.ComplementTimeRanges([]edcb.SearchDate{{StartDayOfWeek: 1, StartHour: 19, StartMinute: 0, EndDayOfWeek: 1, EndHour: 22, EndMinute: 59}})
	assert.Equal(t, []*match.TimeRange{{Weekdays: []int{1}, StartMinute: 23 * 60, DurationMinutes: 7*24*60 - 4*60}}, got)
}
