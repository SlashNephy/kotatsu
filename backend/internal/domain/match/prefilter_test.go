package match_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/match"
)

func TestNewPrefilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		// want は、絞り込みに使う文字列。nil なら絞り込まない。
		want     []string
		wantFold bool
	}{
		{name: "literal", pattern: "みかん", want: []string{"みかん"}},
		{name: "case-insensitive literal is folded", pattern: "(?i)Season", want: []string{"SEASON"}, wantFold: true},
		{name: "anchors are ignored", pattern: "^ない番組1$", want: []string{"ない番組1"}},
		{name: "alternation collects every branch", pattern: "(?i)nowhere|どこにも", want: []string{"NOWHERE", "どこにも"}, wantFold: true},
		{name: "concatenation picks the longest part", pattern: "ab[0-9]+cdef", want: []string{"cdef"}},
		{name: "repetition at least once keeps the literal", pattern: "(ゆる)+キャン?", want: []string{"ゆる"}},
		{
			// 区別する選択肢も、区別しない選択肢に合わせて畳んで探す。
			name: "mixed case sensitivity is folded", pattern: "(?i:abc)|DEF", want: []string{"ABC", "DEF"}, wantFold: true,
		},
		{name: "optional part does not narrow", pattern: "(みかん)?", want: nil},
		{name: "branch without a literal does not narrow", pattern: "みかん|[0-9]+", want: nil},
		{name: "character class does not narrow", pattern: "[あ-ん]+", want: nil},
		{name: "empty branch does not narrow", pattern: "みかん|", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			literals, fold, ok, err := match.PrefilterLiterals(tt.pattern)
			require.NoError(t, err)
			assert.Equal(t, tt.want != nil, ok)
			assert.Equal(t, tt.want, literals)
			assert.Equal(t, tt.wantFold, fold)
		})
	}
}

// TestUnfilterableKeywords は、詳細まで探す正規表現のうち、前段で絞り込めないキーワードだけを返すことを確かめる (#904)。
func TestUnfilterableKeywords(t *testing.T) {
	t.Parallel()

	const hiragana = "[あ-ん]+"
	extended := []match.Field{match.FieldName, match.FieldExtended}
	nameOnly := []match.Field{match.FieldName}
	tests := []struct {
		name       string
		conditions *match.Conditions
		want       []match.KeywordRef
	}{
		{
			name: "regexp without a required literal over the extended",
			conditions: &match.Conditions{
				Keywords:        []*match.Keyword{{Text: "こたつ", Regexp: true, Fields: extended}, {Text: hiragana, Regexp: true, Fields: extended}},
				ExcludeKeywords: []*match.Keyword{{Text: "[a-z]{20}[0-9]", Regexp: true, Fields: extended}},
			},
			want: []match.KeywordRef{{Exclude: false, Index: 1}, {Exclude: true, Index: 0}},
		},
		{
			name: "not searching the extended",
			conditions: &match.Conditions{
				Keywords: []*match.Keyword{{Text: hiragana, Regexp: true, Fields: nameOnly}},
			},
			want: []match.KeywordRef{},
		},
		{
			name: "plain keyword",
			conditions: &match.Conditions{
				Keywords: []*match.Keyword{{Text: hiragana, Regexp: false, Fields: extended}},
			},
			want: []match.KeywordRef{},
		},
		{
			name: "invalid regexp is left to the validation",
			conditions: &match.Conditions{
				Keywords: []*match.Keyword{{Text: "(", Regexp: true, Fields: extended}},
			},
			want: []match.KeywordRef{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, match.UnfilterableKeywords(tt.conditions))
		})
	}
}
