package rule_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/match"
	"github.com/SlashNephy/kotatsu/internal/rule"
)

const (
	// benchPrograms と benchRules は、1 週間ぶんの EPG (約 2 万番組) と、家族で持つくらいのルールの数 (#530)。
	benchPrograms = 20000
	benchRules    = 50
)

// BenchmarkEngineRun は、条件の組み合わせごとに、照合 1 回にかかる時間を測る (#530、#852)。
// どの組み合わせも、照合の時間が番組の数とルールの数に比例する程度に収まることを見る。
func BenchmarkEngineRun(b *testing.B) {
	allFields := []match.Field{match.FieldName, match.FieldDescription, match.FieldExtended}
	tests := []struct {
		name string
		// update は、番組を入れた後に流す SQL。
		update string
		// modify は、i 番目のルールの条件を変える。
		modify func(i int, in *rule.Input)
	}{
		{
			// 番組名、概要、詳細のすべてから探す、正規表現でないキーワード。ほとんどの番組には一致しない。
			name: "literal keyword in all fields",
			modify: func(i int, in *rule.Input) {
				in.Conditions.Keywords = []*match.Keyword{{Text: fmt.Sprintf("どこにもない語%d", i), Regexp: false, CaseSensitive: false, Fields: allFields}}
			},
		},
		{
			// 詳細の正規化した列を足す前に書いた番組は、照合のたびに正規化し直す。
			name:   "literal keyword written before the column",
			update: "UPDATE programs SET extended_normalized = NULL",
			modify: func(i int, in *rule.Input) {
				in.Conditions.Keywords = []*match.Keyword{{Text: fmt.Sprintf("どこにもない語%d", i), Regexp: false, CaseSensitive: false, Fields: allFields}}
			},
		},
		{
			// 大文字と小文字を区別しない正規表現を、詳細まで含めて探す。
			name: "case-insensitive regexp in all fields",
			modify: func(i int, in *rule.Input) {
				in.Conditions.Keywords = []*match.Keyword{{Text: fmt.Sprintf("nowhere%d|どこにも%d", i, i), Regexp: true, CaseSensitive: false, Fields: allFields}}
			},
		},
		{
			// いずれかに一致すればよいキーワードを 3 つと、除外するキーワード。
			name: "any of keywords with excludes",
			modify: func(i int, in *rule.Input) {
				in.Conditions.Keywords = []*match.Keyword{
					{Text: fmt.Sprintf("Nowhere%d", i), Regexp: false, CaseSensitive: false, Fields: allFields},
					{Text: fmt.Sprintf("どこにもない%d", i), Regexp: false, CaseSensitive: false, Fields: allFields},
					{Text: fmt.Sprintf("^ない番組%d", i), Regexp: true, CaseSensitive: false, Fields: []match.Field{match.FieldName}},
				}
				in.Conditions.KeywordMatch = match.KeywordMatchAny
				in.Conditions.ExcludeKeywords = []*match.Keyword{{Text: "再放送", Regexp: false, CaseSensitive: false, Fields: allFields}}
			},
		},
		{
			// キーワードを持たず、ジャンル、曜日と時間帯、囲み文字だけで絞るルール。多くの番組に一致し、多くの予約を持つ。
			name: "broad rules without keywords",
			modify: func(i int, in *rule.Input) {
				in.Conditions.Keywords = []*match.Keyword{}
				in.Conditions.Genres = []*match.Genre{{Lv1: i % 12, Lv2: nil}}
				in.Conditions.TimeRanges = []*match.TimeRange{{Weekdays: []int{i % 7}, StartMinute: 18 * 60, DurationMinutes: 6 * 60}}
				in.Conditions.Marks = []string{"新"}
			},
		},
		{
			// 多くの番組に一致し、重複の回避も見るルール。
			name: "broad rules avoiding duplicates",
			modify: func(i int, in *rule.Input) {
				in.Conditions.Keywords = []*match.Keyword{}
				in.Conditions.Genres = []*match.Genre{{Lv1: i % 12, Lv2: nil}}
				in.AvoidDuplicates = true
				in.DuplicateWindowDays = new(7)
			},
		},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			f := newEngineFixture(b)
			seedBenchPrograms(b, f)
			if tt.update != "" {
				_, err := f.pool.Exec(b.Context(), tt.update)
				require.NoError(b, err)
			}
			for i := range benchRules {
				f.createRule(b, fmt.Sprintf("ルール%d", i), func(in *rule.Input) { tt.modify(i, in) })
			}
			// 1 回目は予約を作るので、測るのは、予約がそろった後の照合にする。
			f.run(b)

			for b.Loop() {
				f.run(b)
			}
		})
	}
}

// seedBenchPrograms は、30 分ごとに並ぶ benchPrograms 件の番組を入れる。
// 詳細は、全角の英数字を含む 1 KB ほどの本文を 2 つの見出しに持たせる。全角は正規化で半角になる。
// ジャンルは 12 の大分類に順に散らし、7 番組に 1 つは新番組の囲み文字を番組名に持たせる。
func seedBenchPrograms(b *testing.B, f *engineFixture) {
	b.Helper()

	_, err := f.pool.Exec(b.Context(), `
		INSERT INTO programs (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
			name, description, name_normalized, description_normalized, extended_normalized, extended, genres, related_items, created_at, updated_at)
		SELECT $1, 100000 + i, 200000 + i, $2::timestamptz + make_interval(mins => 30 * i), $2::timestamptz + make_interval(mins => 30 * i + 30), false, true,
			CASE WHEN i % 7 = 0 THEN '🈟' ELSE '' END || 'ベンチ番組　第' || i || '話',
			'ＡＢＣ　概要　' || i,
			CASE WHEN i % 7 = 0 THEN '[新]' ELSE '' END || 'ベンチ番組 第' || i || '話',
			'ABC 概要 ' || i,
			E'内容\n' || repeat('DEF 内容', 40) || E'\n出演者\n' || repeat('ABC 出演者', 40) || E'\n',
			jsonb_build_object('出演者', repeat('ＡＢＣ　出演者', 40), '内容', repeat('ＤＥＦ　内容', 40)),
			jsonb_build_array(jsonb_build_object('lv1', i % 12, 'lv2', i % 16, 'un1', 0, 'un2', 0)),
			'[]', $2, $2
		FROM generate_series(1, $3::int) AS i`,
		serviceID(), testNow(), benchPrograms)
	require.NoError(b, err)
}
