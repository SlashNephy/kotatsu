package match_test

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
)

// actorDescription は、出演者を書いた概要。
const actorDescription = "ＡＢＣ太郎が出演"

// TestRequiredGramsKeepsMatchingPrograms は、照合関数がキーワードに一致するとみなす番組の gram (SearchGrams) が、
// キーワードの gram (RequiredGrams) をすべて含むこと、つまり索引の絞り込みで一致する番組を落とさないことを確かめる (#966)。
// 一致しない番組のうち、索引で落とせるものは、gram を含まないことも確かめる。
func TestRequiredGramsKeepsMatchingPrograms(t *testing.T) {
	t.Parallel()

	nameOnly := []match.Field{match.FieldName}
	nameAndDescription := []match.Field{match.FieldName, match.FieldDescription}
	tests := []struct {
		name        string
		program     [2]string // 番組名と概要
		extended    map[string]string
		keyword     *match.Keyword
		wantMatch   bool
		wantDropped bool // 一致しない番組を、索引で落とせるか
	}{
		{
			name: "漢字 1 文字", program: [2]string{"猫の島", ""},
			keyword:   &match.Keyword{Text: "猫", Regexp: false, CaseSensitive: false, Fields: nameOnly},
			wantMatch: true,
		},
		{
			name: "全角の英字の番組名と、半角の小文字のキーワード", program: [2]string{"ＮＨＫスペシャル", ""},
			keyword:   &match.Keyword{Text: "nhk", Regexp: false, CaseSensitive: false, Fields: nameOnly},
			wantMatch: true,
		},
		{
			name: "大文字と小文字を区別するキーワード", program: [2]string{"NHKスペシャル", ""},
			keyword:   &match.Keyword{Text: "NHK", Regexp: false, CaseSensitive: true, Fields: nameOnly},
			wantMatch: true,
		},
		{
			name: "大文字と小文字を区別して一致しない番組は、索引では落とさない", program: [2]string{"nhkスペシャル", ""},
			keyword:   &match.Keyword{Text: "NHK", Regexp: false, CaseSensitive: true, Fields: nameOnly},
			wantMatch: false, wantDropped: false,
		},
		{
			name: "ギリシャ文字の大文字と小文字", program: [2]string{"Σの物語", ""},
			keyword:   &match.Keyword{Text: "σの", Regexp: false, CaseSensitive: false, Fields: nameOnly},
			wantMatch: true,
		},
		{
			name: "概要だけに一致する、番組名と概要を見るキーワード", program: [2]string{"相棒", actorDescription},
			keyword:   &match.Keyword{Text: "abc太郎", Regexp: false, CaseSensitive: false, Fields: nameAndDescription},
			wantMatch: true,
		},
		{
			name: "概要だけを見るキーワード", program: [2]string{"相棒", actorDescription},
			keyword:   &match.Keyword{Text: "太郎が", Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldDescription}},
			wantMatch: true,
		},
		{
			name: "番組名だけを見るキーワードは、概要の gram では残らない", program: [2]string{"相棒", actorDescription},
			keyword:   &match.Keyword{Text: "太郎", Regexp: false, CaseSensitive: false, Fields: nameOnly},
			wantMatch: false, wantDropped: true,
		},
		{
			name: "正規表現の必ず含む文字列", program: [2]string{"猫の島 #3", ""},
			keyword:   &match.Keyword{Text: `猫の島 #\d+`, Regexp: true, CaseSensitive: false, Fields: nameOnly},
			wantMatch: true,
		},
		{
			name: "大文字と小文字を区別しない正規表現", program: [2]string{"F1グランプリ", ""},
			keyword:   &match.Keyword{Text: `f1グラ`, Regexp: true, CaseSensitive: false, Fields: nameOnly},
			wantMatch: true,
		},
		{
			name: "詳細だけに一致する", program: [2]string{"相棒", ""}, extended: map[string]string{"出演者": "ＡＢＣ太郎"},
			keyword:   &match.Keyword{Text: "abc太郎", Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldExtended}},
			wantMatch: true,
		},
		{
			name: "キーワードにない語は、索引で落とす", program: [2]string{"猫の島", "島の猫"},
			keyword:   &match.Keyword{Text: "ぬるぽ", Regexp: false, CaseSensitive: false, Fields: nameAndDescription},
			wantMatch: false, wantDropped: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conditions := &match.Conditions{Keywords: []*match.Keyword{tt.keyword}}
			matcher, err := match.Compile(conditions, time.UTC)
			require.NoError(t, err)
			program := match.NewProgram(&match.ProgramInput{Name: tt.program[0], Description: tt.program[1], Extended: tt.extended})
			assert.Equal(t, tt.wantMatch, matcher.Match(program))

			required := match.RequiredGrams(conditions)
			require.NotEmpty(t, required)
			grams := match.SearchGrams(domain.Normalize(tt.program[0]), domain.Normalize(tt.program[1]), domain.NormalizeExtended(tt.extended))
			kept := !slices.ContainsFunc(required, func(gram string) bool { return !slices.Contains(grams, gram) })
			if tt.wantMatch {
				assert.True(t, kept, "a matching program must keep all the required grams: %q not in %q", required, grams)
			} else {
				assert.Equal(t, !tt.wantDropped, kept)
			}
		})
	}
}

// TestRequiredGramsWithoutNarrowing は、索引で絞れない条件では、gram を返さないことを確かめる (#966)。
func TestRequiredGramsWithoutNarrowing(t *testing.T) {
	t.Parallel()

	nameOnly := []match.Field{match.FieldName}
	tests := []struct {
		name       string
		conditions *match.Conditions
	}{
		{name: "キーワードがない", conditions: &match.Conditions{}},
		{
			name: "いずれかのキーワードに一致すればよい",
			conditions: &match.Conditions{KeywordMatch: match.KeywordMatchAny, Keywords: []*match.Keyword{
				{Text: "猫", Regexp: false, CaseSensitive: false, Fields: nameOnly},
				{Text: "犬", Regexp: false, CaseSensitive: false, Fields: nameOnly},
			}},
		},
		{
			name: "空のキーワード",
			conditions: &match.Conditions{Keywords: []*match.Keyword{
				{Text: "", Regexp: false, CaseSensitive: false, Fields: nameOnly},
			}},
		},
		{
			name: "必ず含む文字列が 1 つに決まらない正規表現",
			conditions: &match.Conditions{Keywords: []*match.Keyword{
				{Text: "猫|犬", Regexp: true, CaseSensitive: false, Fields: nameOnly},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, match.RequiredGrams(tt.conditions))
		})
	}
}

// TestRequiredGramsSkipsUnnarrowableKeywords は、すべてに一致する条件で、絞れないキーワードを飛ばし、絞れるキーワードの gram だけを返すことを確かめる (#966)。
func TestRequiredGramsSkipsUnnarrowableKeywords(t *testing.T) {
	t.Parallel()

	conditions := &match.Conditions{Keywords: []*match.Keyword{
		{Text: "猫", Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldName, match.FieldDescription}},
		{Text: "", Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldExtended}},
		{Text: "犬|鳥", Regexp: true, CaseSensitive: false, Fields: []match.Field{match.FieldName}},
	}}
	assert.Equal(t, []string{"猫"}, match.RequiredGrams(conditions))
}
