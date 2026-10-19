package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// benchSearchPrograms は、放送前の番組と、過去の番組のそれぞれの件数。1 週間ぶんの EPG が約 2 万番組 (#530)。
const benchSearchPrograms = 20000

// BenchmarkSearchPrograms は、条件の組み合わせごとに、番組の検索 1 回 (50 件) にかかる時間を測る (#852)。
// どの組み合わせも、ほとんどの番組に一致しないので、候補を最後まで照合する。SQL で粗く絞れない条件でも、遅くなりすぎないことを見る。
func BenchmarkSearchPrograms(b *testing.B) {
	allFields := []string{fieldName, "DESCRIPTION", "EXTENDED"}
	tests := []struct {
		name       string
		conditions conditionsInput
		archive    *archiveInput
	}{
		{
			// SQL の部分一致で粗く絞れる。
			name:       "literal keyword in name",
			conditions: conditionsInput{Keywords: []keywordInput{keyword("どこにもない語", false)}},
		},
		{
			// 詳細は SQL で絞れないので、Go で照合する。
			name:       "literal keyword in all fields",
			conditions: conditionsInput{Keywords: []keywordInput{{Text: "どこにもない語", Regexp: false, CaseSensitive: false, Fields: allFields}}},
		},
		{
			name:       "case-insensitive regexp in all fields",
			conditions: conditionsInput{Keywords: []keywordInput{{Text: "nowhere|どこにも", Regexp: true, CaseSensitive: false, Fields: allFields}}},
		},
		{
			// いずれかに一致すればよいキーワードは、SQL で絞れない。
			name: "any of keywords",
			conditions: conditionsInput{KeywordMatch: keywordMatchAny, Keywords: []keywordInput{
				keyword("どこにもない語", false), {Text: "Nowhere", Regexp: false, CaseSensitive: false, Fields: allFields},
			}},
		},
		{
			// ジャンルの中分類は SQL で絞れない。
			name:       "genre subcategory",
			conditions: conditionsInput{Genres: []genreInput{{Lv1: 0, Lv2: new(1)}}},
		},
		{
			name:       "literal keyword in all fields with archives",
			conditions: conditionsInput{Keywords: []keywordInput{{Text: "どこにもない語", Regexp: false, CaseSensitive: false, Fields: allFields}}},
			archive:    &archiveInput{From: "2025-01-01T00:00:00Z"},
		},
	}

	f := newFixture(b, nil)
	f.syncEPG(b)
	seedBenchSearchPrograms(b, f)
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for b.Loop() {
				var got searchResult
				require.Empty(b, f.query(b, searchQuery, searchVariables(tt.conditions, tt.archive, 50, nil), &got))
			}
		})
	}
}

// seedBenchSearchPrograms は、こたつテスト1 に、30 分ごとに並ぶ放送前の番組と過去の番組を benchSearchPrograms 件ずつ入れる。
// 詳細は、全角の英数字を含む 1 KB ほどの本文を 2 つの見出しに持たせる。ジャンルは、大分類を 12、中分類を 16 の周期で散らすので、
// 大分類 0 の中分類 1 には、どの番組も当たらない。
func seedBenchSearchPrograms(b *testing.B, f *fixture) {
	b.Helper()

	const columns = `service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
		name, description, name_normalized, description_normalized, extended_normalized, extended, genres, related_items, created_at, updated_at`
	const values = `
		SELECT (SELECT id FROM services WHERE name = 'こたつテスト1'), 100000 + i, 200000 + i, start_at, start_at + interval '30 minutes', false, true,
			'ベンチ番組　第' || i || '話', 'ＡＢＣ　概要　' || i, 'ベンチ番組 第' || i || '話', 'ABC 概要 ' || i,
			E'内容\n' || repeat('DEF 内容', 40) || E'\n出演者\n' || repeat('ABC 出演者', 40) || E'\n',
			jsonb_build_object('出演者', repeat('ＡＢＣ　出演者', 40), '内容', repeat('ＤＥＦ　内容', 40)),
			jsonb_build_array(jsonb_build_object('lv1', i % 12, 'lv2', i % 16, 'un1', 0, 'un2', 0)),
			'[]', $1::timestamptz, $1::timestamptz`
	_, err := f.pool.Exec(b.Context(), `
		INSERT INTO programs (`+columns+`)
		`+values+`
		FROM generate_series(1, $2::int) AS i, LATERAL (SELECT $1::timestamptz + make_interval(mins => 30 * i) AS start_at) t`,
		testNow(), benchSearchPrograms)
	require.NoError(b, err)
	_, err = f.pool.Exec(b.Context(), `
		INSERT INTO program_archives (`+columns+`, archived_at)
		`+values+`, $1::timestamptz
		FROM generate_series(1, $2::int) AS i, LATERAL (SELECT $1::timestamptz - make_interval(mins => 30 * i + 60) AS start_at) t`,
		testNow(), benchSearchPrograms)
	require.NoError(b, err)
	// 本番のように統計を取ってから測る。自動の ANALYZE を待つと、入れた直後の小さい表の統計で計画が決まる。
	_, err = f.pool.Exec(b.Context(), "ANALYZE programs, program_archives")
	require.NoError(b, err)
}
