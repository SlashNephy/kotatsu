package rule_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/match"
	"github.com/SlashNephy/kotatsu/internal/rule"
)

// BenchmarkEngineRun は、1 週間ぶんの EPG (約 2 万番組) と 50 のルールで、照合 1 回にかかる時間を測る (#530)。
// どのルールも、番組名、概要、詳細のすべてから探すキーワードを持ち、ほとんどの番組には一致しない。
func BenchmarkEngineRun(b *testing.B) {
	b.Run("normalized", func(b *testing.B) {
		benchmarkEngineRun(b, "")
	})
	// 詳細の正規化した列を足す前に書いた番組は、照合のたびに正規化し直す。
	b.Run("written before the column", func(b *testing.B) {
		benchmarkEngineRun(b, "UPDATE programs SET extended_normalized = NULL")
	})
}

func benchmarkEngineRun(b *testing.B, update string) {
	b.Helper()

	const (
		programs = 20000
		rules    = 50
	)
	f := newEngineFixture(b)
	// 詳細は、全角の英数字を含む 1 KB ほどの本文を 2 つの見出しに持たせる。全角は正規化で半角になる。
	_, err := f.pool.Exec(b.Context(), `
		INSERT INTO programs (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
			name, description, name_normalized, description_normalized, extended_normalized, extended, genres, related_items, created_at, updated_at)
		SELECT $1, 100000 + i, 200000 + i, $2::timestamptz + make_interval(mins => 30 * i), $2::timestamptz + make_interval(mins => 30 * i + 30), false, true,
			'ベンチ番組　第' || i || '話', 'ＡＢＣ　概要　' || i, 'ベンチ番組 第' || i || '話', 'ABC 概要 ' || i,
			E'内容\n' || repeat('DEF 内容', 40) || E'\n出演者\n' || repeat('ABC 出演者', 40) || E'\n',
			jsonb_build_object('出演者', repeat('ＡＢＣ　出演者', 40), '内容', repeat('ＤＥＦ　内容', 40)),
			'[]', '[]', $2, $2
		FROM generate_series(1, $3::int) AS i`,
		serviceID(), testNow(), programs)
	require.NoError(b, err)
	if update != "" {
		_, err := f.pool.Exec(b.Context(), update)
		require.NoError(b, err)
	}
	for i := range rules {
		f.createRule(b, fmt.Sprintf("どこにもない語%d", i), func(in *rule.Input) {
			in.Conditions.Keywords[0].Regexp = false
			in.Conditions.Keywords[0].Fields = []match.Field{match.FieldName, match.FieldDescription, match.FieldExtended}
		})
	}
	f.run(b)

	for b.Loop() {
		f.run(b)
	}
}
