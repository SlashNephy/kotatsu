package match_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
)

// catArrived は、照らす字幕の文。
const catArrived = "猫が来た"

// TestTextMatcher は、字幕の文の照合 (#2214) が、正規化、正規表現、大文字と小文字の区別、すべてといずれかの一致を、
// 番組の照合と同じに扱うことと、gram の絞り込みで一致する文の集まりを落とさないことを確認する。
func TestTextMatcher(t *testing.T) {
	t.Parallel()

	literal := func(text string) *match.TextKeyword {
		return &match.TextKeyword{Text: text, Regexp: false, CaseSensitive: false}
	}
	tests := []struct {
		name         string
		texts        []string
		keywords     []*match.TextKeyword
		keywordMatch match.KeywordMatch
		wantMatch    bool
		wantContains []bool // 文ごとに、どれかのキーワードを含むか
		wantDropped  bool   // 一致しない文の集まりを、gram で落とせるか
	}{
		{
			name: "全角の英字の文と、半角の小文字のキーワード", texts: []string{"ＮＨＫの番組です", "次の話題"},
			keywords: []*match.TextKeyword{literal("nhk")}, keywordMatch: match.KeywordMatchAll,
			wantMatch: true, wantContains: []bool{true, false},
		},
		{
			name: "大文字と小文字を区別するキーワード", texts: []string{"NHKの番組"},
			keywords:     []*match.TextKeyword{{Text: "nhk", Regexp: false, CaseSensitive: true}},
			keywordMatch: match.KeywordMatchAll, wantMatch: false, wantContains: []bool{false}, wantDropped: false,
		},
		{
			name: "すべてに一致: キーワードが別々の文にある", texts: []string{catArrived, "犬も来た"},
			keywords: []*match.TextKeyword{literal("猫"), literal("犬")}, keywordMatch: match.KeywordMatchAll,
			wantMatch: true, wantContains: []bool{true, true},
		},
		{
			name: "すべてに一致: 片方のキーワードがない", texts: []string{catArrived},
			keywords: []*match.TextKeyword{literal("猫"), literal("犬")}, keywordMatch: match.KeywordMatchAll,
			wantMatch: false, wantContains: []bool{true}, wantDropped: true,
		},
		{
			name: "いずれかに一致", texts: []string{catArrived},
			keywords: []*match.TextKeyword{literal("猫"), literal("犬")}, keywordMatch: match.KeywordMatchAny,
			wantMatch: true, wantContains: []bool{true},
		},
		{
			name: "文をまたぐ語には一致しない", texts: []string{"今日は晴", "れです"},
			keywords: []*match.TextKeyword{literal("晴れ")}, keywordMatch: match.KeywordMatchAll,
			wantMatch: false, wantContains: []bool{false, false}, wantDropped: true,
		},
		{
			name: "正規表現", texts: []string{"第12話の予告"},
			keywords:     []*match.TextKeyword{{Text: `第\d+話`, Regexp: true, CaseSensitive: false}},
			keywordMatch: match.KeywordMatchAll, wantMatch: true, wantContains: []bool{true},
		},
		{
			name: "大文字と小文字を区別しない正規表現", texts: []string{"tokyo mx"},
			keywords:     []*match.TextKeyword{{Text: "TOKYO", Regexp: true, CaseSensitive: false}},
			keywordMatch: match.KeywordMatchAll, wantMatch: true, wantContains: []bool{true},
		},
		{
			name: "波ダッシュをそろえる", texts: []string{"10時〜11時"},
			keywords: []*match.TextKeyword{literal("10時~11時")}, keywordMatch: match.KeywordMatchAll,
			wantMatch: true, wantContains: []bool{true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			matcher, err := match.CompileText(tt.keywords, tt.keywordMatch)
			require.NoError(t, err)
			texts := make([]string, 0, len(tt.texts))
			for _, text := range tt.texts {
				texts = append(texts, domain.Normalize(text))
			}
			assert.Equal(t, tt.wantMatch, matcher.Match(texts))
			contains := make([]bool, 0, len(texts))
			for _, text := range texts {
				contains = append(contains, matcher.Contains(text))
			}
			assert.Equal(t, tt.wantContains, contains)

			grams := match.TextGrams(texts)
			kept := !slices.ContainsFunc(matcher.RequiredGrams(), func(gram string) bool { return !slices.Contains(grams, gram) })
			if tt.wantMatch {
				assert.True(t, kept, "the grams must keep the matching texts")
			} else {
				assert.Equal(t, tt.wantDropped, !kept)
			}
		})
	}
}

// TestCompileTextRejectsInvalidKeywords は、空のキーワードと読めない正規表現を拒むことを確認する。
func TestCompileTextRejectsInvalidKeywords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		keywords     []*match.TextKeyword
		keywordMatch match.KeywordMatch
	}{
		{name: "キーワードがない", keywords: []*match.TextKeyword{}, keywordMatch: match.KeywordMatchAll},
		{name: "空のキーワード", keywords: []*match.TextKeyword{{Text: "", Regexp: false, CaseSensitive: false}}, keywordMatch: match.KeywordMatchAll},
		{name: "読めない正規表現", keywords: []*match.TextKeyword{{Text: "(", Regexp: true, CaseSensitive: false}}, keywordMatch: match.KeywordMatchAll},
		{name: "照合のしかたがない", keywords: []*match.TextKeyword{{Text: "猫", Regexp: false, CaseSensitive: false}}, keywordMatch: match.KeywordMatch("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := match.CompileText(tt.keywords, tt.keywordMatch)
			assert.ErrorIs(t, err, match.ErrInvalidConditions)
		})
	}
}
