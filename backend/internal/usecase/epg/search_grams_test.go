package epg_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
)

// TestSyncAllStoresSearchGrams は、同期が番組の検索の gram (#966) を書き、番組名か概要が変わったときだけ作り直すことを確認する。
func TestSyncAllStoresSearchGrams(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	grams := func() []string {
		var got []string
		require.NoError(t, f.pool.QueryRow(t.Context(),
			"SELECT search_grams FROM programs WHERE service_id = $1 AND event_id = 6", serviceID()).Scan(&got))

		return got
	}
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	var name, description, extended string
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT name_normalized, description_normalized, extended_normalized FROM programs WHERE service_id = $1 AND event_id = 6", serviceID()).
		Scan(&name, &description, &extended))
	assert.Equal(t, match.SearchGrams(name, description, extended), grams())

	// 番組名、概要、詳細が変わらない書き換えでは、DB の gram を残す。目印の gram を置いて、書き直さないことを見る。
	_, err = f.pool.Exec(t.Context(), "UPDATE programs SET search_grams = '{目印}' WHERE service_id = $1 AND event_id = 6", serviceID())
	require.NoError(t, err)
	require.NoError(t, f.fake.ChangeProgramVideo(programID(6), &faketuner.Video{Type: "hevc", Resolution: "2160p"}))
	result, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.ProgramsUpserted)
	assert.Equal(t, []string{"目印"}, grams())

	// 正規化すると同じになる書き換え (全角と半角) でも、DB の gram を残す。
	require.NoError(t, f.fake.RenameProgram(programID(6), widen(name)))
	result, err = f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.ProgramsUpserted)
	assert.Equal(t, []string{"目印"}, grams())

	// 詳細が変わったら、作り直す。
	require.NoError(t, f.fake.ChangeProgramExtended(programID(6), map[string]string{"出演者": "こたつ太郎"}))
	_, err = f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Equal(t, match.SearchGrams(name, description, "出演者\nこたつ太郎\n"), grams())

	// 番組名が変わったら、作り直す。
	require.NoError(t, f.fake.RenameProgram(programID(6), "猫の島"))
	_, err = f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	assert.Equal(t, match.SearchGrams("猫の島", description, "出演者\nこたつ太郎\n"), grams())
}

// TestSyncAllFillsSearchGrams は、gram を埋めていない番組とアーカイブ (カラムを足す前に書いたもの) を、全量の照合が埋めることを確認する (#966)。
func TestSyncAllFillsSearchGrams(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "UPDATE programs SET search_grams = NULL")
	require.NoError(t, err)
	start := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO program_archives (
		service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free, name, description,
		name_normalized, description_normalized, extended_normalized, extended, genres, related_items, created_at, updated_at, archived_at
	) VALUES ($1, 9000, 0, $2, $3, false, true, '過去の番組', '猫が出る', '過去の番組', '猫が出る', NULL, '{"出演者": "ＡＢＣ太郎"}', '[]', '[]', $2, $2, $3),
		-- 番組名も概要も空の番組には、空の配列を書き、埋め直さない。
		($1, 9001, 0, $2, $3, false, true, '', '', '', '', '', '{}', '[]', '[]', $2, $2, $3)`,
		serviceID(), start, start.Add(30*time.Minute))
	require.NoError(t, err)

	result, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	// 中身は変わらないので、番組を書き直したとは数えない。
	assert.Equal(t, 0, result.ProgramsUpserted)
	assert.Equal(t, 0, f.count(t, "SELECT count(*) FROM programs WHERE search_grams IS NULL"))
	var archived []string
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT search_grams FROM program_archives WHERE event_id = 9000").Scan(&archived))
	// 詳細を正規化した列のない番組は、上流の詳細から正規化して作る。
	assert.Equal(t, match.SearchGrams("過去の番組", "猫が出る", "出演者\nABC太郎\n"), archived)
	assert.Zero(t, f.count(t, "SELECT count(*) FROM program_archives WHERE search_grams IS NULL"))
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM program_archives WHERE event_id = 9001 AND search_grams = '{}'"))
}

// widen は、ASCII の英数字を全角にする。NFKC で正規化すると元に戻る。
func widen(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if ('0' <= r && r <= '9') || ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z') {
			r += 0xFEE0
		}
		out = append(out, r)
	}

	return string(out)
}
