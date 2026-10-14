package graph_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/id"
)

const searchQuery = `query($conditions: ProgramConditionsInput!, $archive: ArchiveRangeInput, $first: Int!, $after: String) {
	searchPrograms(conditions: $conditions, archive: $archive, first: $first, after: $after) {
		edges { cursor node { id name archived startAt service { name channelType { type } } } }
		pageInfo { hasNextPage hasPreviousPage endCursor }
	}
}`

type searchResult struct {
	SearchPrograms struct {
		Edges []struct {
			Node struct {
				ID       string    `json:"id"`
				Name     string    `json:"name"`
				Archived bool      `json:"archived"`
				StartAt  time.Time `json:"startAt"`
				Service  struct {
					Name string `json:"name"`
				} `json:"service"`
			} `json:"node"`
		} `json:"edges"`
		PageInfo struct {
			HasNextPage     bool    `json:"hasNextPage"`
			HasPreviousPage bool    `json:"hasPreviousPage"`
			EndCursor       *string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"searchPrograms"`
}

func (r *searchResult) names() []string {
	names := make([]string, 0, len(r.SearchPrograms.Edges))
	for _, edge := range r.SearchPrograms.Edges {
		names = append(names, edge.Node.Name)
	}

	return names
}

type keywordInput struct {
	Text          string   `json:"text"`
	Regexp        bool     `json:"regexp"`
	CaseSensitive bool     `json:"caseSensitive"`
	Fields        []string `json:"fields"`
}

type genreInput struct {
	Lv1 int  `json:"lv1"`
	Lv2 *int `json:"lv2,omitempty"`
}

type conditionsInput struct {
	Keywords        []keywordInput `json:"keywords,omitempty"`
	ExcludeKeywords []keywordInput `json:"excludeKeywords,omitempty"`
	ChannelTypes    []string       `json:"channelTypes,omitempty"`
	Services        []string       `json:"services,omitempty"`
	Genres          []genreInput   `json:"genres,omitempty"`
	ExcludeGenres   []genreInput   `json:"excludeGenres,omitempty"`
}

// programName は、こたつテスト1 の eventId の番組の名前を返す。
func programName(eventID int) string {
	return fmt.Sprintf("こたつテスト1 テスト番組 %d", eventID)
}

type archiveInput struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// searchVariables は、searchPrograms の query の変数を作る。
func searchVariables(conditions conditionsInput, archive *archiveInput, first int, after *string) map[string]any {
	variables := map[string]any{"conditions": conditions, varFirst: first, varAfter: after}
	if archive != nil {
		variables["archive"] = archive
	}

	return variables
}

func keyword(text string, regexp bool) keywordInput {
	return keywordInput{Text: text, Regexp: regexp, CaseSensitive: false, Fields: []string{fieldName}}
}

func TestSearchPrograms(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)

	tests := []struct {
		name       string
		conditions conditionsInput
		want       []string
	}{
		{
			// 開始時刻が同じなら、サービス ID (networkId × 100000 + serviceId) の順。BS の networkId は地上波より小さい。
			name:       "regular expression across services in start time order",
			conditions: conditionsInput{Keywords: []keywordInput{keyword("テスト番組 5$", true)}},
			want: []string{
				"こたつ BS テスト番組 5", programName(5), "こたつテスト1 サブ テスト番組 5", "こたつテスト2 テスト番組 5", "こたつテスト3 テスト番組 5",
			},
		},
		{
			name:       "full-width keyword matches after NFKC",
			conditions: conditionsInput{Keywords: []keywordInput{keyword("こたつテスト１　テスト番組　１４", false)}},
			want:       []string{programName(14)},
		},
		{
			name:       "channel type",
			conditions: conditionsInput{Keywords: []keywordInput{keyword("テスト番組 1[0-1]$", true)}, ChannelTypes: []string{"BS"}},
			want:       []string{"こたつ BS テスト番組 10", "こたつ BS テスト番組 11"},
		},
		{
			name: "exclude keyword",
			conditions: conditionsInput{
				Keywords: []keywordInput{keyword("テスト番組 5$", true)}, ExcludeKeywords: []keywordInput{keyword("サブ|BS", true)},
			},
			want: []string{programName(5), "こたつテスト2 テスト番組 5", "こたつテスト3 テスト番組 5"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got searchResult
			require.Empty(t, f.query(t, searchQuery, searchVariables(tt.conditions, nil, 50, nil), &got))
			assert.Equal(t, tt.want, got.names())
			assert.False(t, got.SearchPrograms.PageInfo.HasNextPage)
		})
	}
}

// TestSearchProgramsPagination は、カーソルで続きのページを取得できることを確かめる。
func TestSearchProgramsPagination(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	conditions := conditionsInput{Keywords: []keywordInput{keyword("^こたつテスト1 テスト番組", true)}}

	pages := [][]string{}
	var after *string
	for {
		var got searchResult
		require.Empty(t, f.query(t, searchQuery, searchVariables(conditions, nil, 5, after), &got))
		pages = append(pages, got.names())
		assert.Equal(t, after != nil, got.SearchPrograms.PageInfo.HasPreviousPage)
		if !got.SearchPrograms.PageInfo.HasNextPage {
			break
		}
		after = got.SearchPrograms.PageInfo.EndCursor
	}

	// こたつテスト1 の番組は 14 件あるが、1 と 2 はテストの時刻より前に終わっているので返さない。
	// 残りの 12 件は、5 件、5 件、2 件に分かれる。
	require.Len(t, pages, 3)
	assert.Equal(t, []int{5, 5, 2}, []int{len(pages[0]), len(pages[1]), len(pages[2])})
	assert.Equal(t, programName(3), pages[0][0])
	assert.Equal(t, programName(14), pages[2][1])
}

// TestSearchArchivedPrograms は、過去の番組を、期間を指定したときだけ検索することを確かめる (設計書 3.4)。
func TestSearchArchivedPrograms(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	for _, daysAgo := range []int{5, 40} {
		start := testNow().Add(-time.Duration(daysAgo) * 24 * time.Hour)
		_, err := f.pool.Exec(t.Context(), `INSERT INTO program_archives (
			service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free, name, description,
			name_normalized, description_normalized, extended, genres, related_items, created_at, updated_at, archived_at
		) SELECT id, 9000, 0, $1, $2, false, true, '過去の番組', '', '過去の番組', '', '{}', '[]', '[]', $1, $1, $2
		FROM services WHERE name = 'こたつテスト1'`, start, start.Add(30*time.Minute))
		require.NoError(t, err)
	}
	conditions := conditionsInput{Keywords: []keywordInput{keyword("過去の番組", false)}}

	tests := []struct {
		name    string
		archive *archiveInput
		want    int
	}{
		{name: "without archive", archive: nil, want: 0},
		{name: "default range is the last 30 days", archive: &archiveInput{}, want: 1},
		{name: "explicit range", archive: &archiveInput{From: testNow().Add(-60 * 24 * time.Hour).Format(time.RFC3339)}, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got searchResult
			require.Empty(t, f.query(t, searchQuery, searchVariables(conditions, tt.archive, 50, nil), &got))
			require.Len(t, got.SearchPrograms.Edges, tt.want)
			for _, edge := range got.SearchPrograms.Edges {
				assert.True(t, edge.Node.Archived)
				assert.Equal(t, "こたつテスト1", edge.Node.Service.Name)

				// アーカイブの番組も、ID で引ける。
				var node struct {
					Node struct {
						Name     string `json:"name"`
						Archived bool   `json:"archived"`
					} `json:"node"`
				}
				require.Empty(t, f.query(t, `query($id: ID!) { node(id: $id) { ... on Program { name archived } } }`, map[string]any{"id": edge.Node.ID}, &node))
				assert.Equal(t, "過去の番組", node.Node.Name)
				assert.True(t, node.Node.Archived)
			}
		})
	}
}

func TestSearchProgramsRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	tests := []struct {
		name      string
		variables map[string]any
	}{
		{name: "invalid regular expression", variables: searchVariables(conditionsInput{Keywords: []keywordInput{keyword("(", true)}}, nil, 10, nil)},
		{name: "too many results", variables: searchVariables(conditionsInput{}, nil, 101, nil)},
		{name: "invalid cursor", variables: searchVariables(conditionsInput{}, nil, 10, new("bogus"))},
		{name: "empty archive range", variables: searchVariables(conditionsInput{}, &archiveInput{From: noon, To: noon}, 10, nil)},
		{name: "invalid service", variables: searchVariables(conditionsInput{Services: []string{id.New().String()}}, nil, 10, nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []string{codeBadUserInput}, f.query(t, searchQuery, tt.variables, nil))
		})
	}
}

// TestSearchProgramsReadsAllCandidates は、一致する番組が少なくても、候補を最後まで照合して 1 回で返し、
// 続きがないときは hasNextPage を false にすることを確かめる。
func TestSearchProgramsReadsAllCandidates(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)

	tests := []struct {
		name       string
		conditions conditionsInput
		want       int
	}{
		// 地上波のサービスの eventId 14 の番組は、候補の最後のほうにある。
		{name: "候補の最後のほうに一致する", conditions: conditionsInput{Keywords: []keywordInput{keyword("テスト番組 14$", true)}}, want: 5},
		{name: "どれにも一致しない", conditions: conditionsInput{Keywords: []keywordInput{keyword("存在しない番組", false)}}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got searchResult
			require.Empty(t, f.query(t, searchQuery, searchVariables(tt.conditions, nil, 50, nil), &got))
			assert.Len(t, got.names(), tt.want)
			assert.False(t, got.SearchPrograms.PageInfo.HasNextPage)
		})
	}
}

// TestSearchProgramsByGenre は、ジャンルの包含と除外で絞り込めることを確かめる。
func TestSearchProgramsByGenre(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	// eventId 5 をアニメ (大分類 7)、6 をアニメの中分類 1 にする。
	_, err := f.pool.Exec(t.Context(), `UPDATE programs SET genres = '[{"lv1":7,"lv2":0,"un1":15,"un2":15}]' WHERE event_id = 5`)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `UPDATE programs SET genres = '[{"lv1":7,"lv2":1,"un1":15,"un2":15}]' WHERE event_id = 6`)
	require.NoError(t, err)
	base := []keywordInput{keyword("^こたつテスト1 テスト番組 [4-7]$", true)}

	tests := []struct {
		name       string
		conditions conditionsInput
		want       []string
	}{
		{
			name:       "major genre",
			conditions: conditionsInput{Keywords: base, Genres: []genreInput{{Lv1: 7}}},
			want:       []string{programName(5), programName(6)},
		},
		{
			name:       "minor genre",
			conditions: conditionsInput{Keywords: base, Genres: []genreInput{{Lv1: 7, Lv2: new(1)}}},
			want:       []string{programName(6)},
		},
		{
			name:       "excluded major genre",
			conditions: conditionsInput{Keywords: base, ExcludeGenres: []genreInput{{Lv1: 7}}},
			want:       []string{programName(4), programName(7)},
		},
		{
			name:       "excluded minor genre",
			conditions: conditionsInput{Keywords: base, ExcludeGenres: []genreInput{{Lv1: 7, Lv2: new(0)}}},
			want:       []string{programName(4), programName(6), programName(7)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got searchResult
			require.Empty(t, f.query(t, searchQuery, searchVariables(tt.conditions, nil, 50, nil), &got))
			assert.Equal(t, tt.want, got.names())
		})
	}
}

// TestSearchProgramsNarrowsKeywordsInSQL は、SQL でキーワードを粗く絞っても、照合関数と同じ番組が見つかることを確かめる。
// LIKE の記号は文字どおりに扱い、大文字と小文字は、区別するかどうかの指定に従う。
func TestSearchProgramsNarrowsKeywordsInSQL(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	const name = `Kotatsu 100%_OFF \ 特番`
	// まだ終わっていない番組の名前にする。いちばん遅く始まる番組を使う。
	_, err := f.pool.Exec(t.Context(), `UPDATE programs SET name = $1, name_normalized = $1
		WHERE (service_id, event_id) = (SELECT service_id, event_id FROM programs ORDER BY start_at DESC, service_id, event_id LIMIT 1)`, name)
	require.NoError(t, err)

	tests := []struct {
		name          string
		text          string
		caseSensitive bool
		want          int
	}{
		{name: "パーセントとアンダースコアを文字どおりに含む", text: "100%_OFF", caseSensitive: false, want: 1},
		{name: "アンダースコアは任意の 1 文字に一致しない", text: "100%xOFF", caseSensitive: false, want: 0},
		{name: "パーセントは任意の文字列に一致しない", text: "100%OFF", caseSensitive: false, want: 0},
		{name: "バックスラッシュを文字どおりに含む", text: `\ 特番`, caseSensitive: false, want: 1},
		{name: "大文字と小文字を区別しない", text: "kotatsu", caseSensitive: false, want: 1},
		{name: "大文字と小文字を区別する", text: "kotatsu", caseSensitive: true, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conditions := conditionsInput{Keywords: []keywordInput{{Text: tt.text, Regexp: false, CaseSensitive: tt.caseSensitive, Fields: []string{fieldName}}}}
			var got searchResult
			require.Empty(t, f.query(t, searchQuery, searchVariables(conditions, nil, 50, nil), &got))
			assert.Len(t, got.names(), tt.want)
		})
	}
}

// TestSearchProgramsExcludesEndedPrograms は、アーカイブへ移す前の、放送の終わった番組を、
// 過去の放送を含めないときは返さず、含めるときは期間で絞って返すことを確かめる。
func TestSearchProgramsExcludesEndedPrograms(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	// 1 つの番組を、2 時間前に終わったことにする。まだ programs に残っている。
	const name = "終わったばかりの番組"
	_, err := f.pool.Exec(t.Context(), `UPDATE programs SET name = $1, name_normalized = $1, start_at = $2, end_at = $3
		WHERE (service_id, event_id) = (SELECT service_id, event_id FROM programs ORDER BY start_at, service_id, event_id LIMIT 1)`,
		name, testNow().Add(-3*time.Hour), testNow().Add(-2*time.Hour))
	require.NoError(t, err)
	conditions := conditionsInput{Keywords: []keywordInput{keyword(name, false)}}

	tests := []struct {
		name    string
		archive *archiveInput
		want    int
	}{
		{name: "過去の放送を含めない", archive: nil, want: 0},
		{name: "過去の放送を含める", archive: &archiveInput{}, want: 1},
		{name: "期間の外", archive: &archiveInput{From: testNow().Add(-48 * time.Hour).Format(time.RFC3339), To: testNow().Add(-24 * time.Hour).Format(time.RFC3339)}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got searchResult
			require.Empty(t, f.query(t, searchQuery, searchVariables(conditions, tt.archive, 50, nil), &got))
			assert.Len(t, got.names(), tt.want)
		})
	}
}

// TestSearchProgramsOmitsSharedEvents は、サブチャンネルが元のサービスと同じ番組を流している (イベント共有) とき、
// 元の番組も一致していれば、サブチャンネルの番組を返さないことを確かめる。ページをまたいでも省く。
func TestSearchProgramsOmitsSharedEvents(t *testing.T) {
	t.Parallel()

	const (
		// こたつテスト1 (0x0400) とそのサブチャンネル (0x0401) の、Mirakurun のサービス ID。
		mainKey = 0x7FE0*100000 + 0x0400
		subKey  = 0x7FE0*100000 + 0x0401
	)
	main5, sub5 := programName(5), "こたつテスト1 サブ テスト番組 5"
	main6, sub6 := programName(6), "こたつテスト1 サブ テスト番組 6"

	tests := []struct {
		name    string
		keyword string
		first   int
		want    []string
	}{
		{name: "元の番組も一致すれば、サブチャンネルの番組を省く", keyword: "^こたつテスト1 (サブ )?テスト番組 5$", first: 50, want: []string{main5}},
		{name: "元の番組が一致しなければ、サブチャンネルの番組を残す", keyword: "^こたつテスト1 サブ テスト番組 5$", first: 50, want: []string{sub5}},
		{name: "共有していない番組は、どちらも返す", keyword: "^こたつテスト1 (サブ )?テスト番組 6$", first: 50, want: []string{main6, sub6}},
		{name: "ページをまたいでも省く", keyword: "^こたつテスト1 (サブ )?テスト番組 [56]$", first: 1, want: []string{main5, main6, sub6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			f.syncEPG(t)
			// eventId 5 の番組だけ、サブチャンネルがメインと同じ番組を流していることにする。
			for key, items := range map[int64]string{
				mainKey: `[{"type":"shared","serviceId":1024,"eventId":5},{"type":"shared","serviceId":1025,"eventId":5}]`,
				subKey:  `[{"type":"shared","serviceId":1024,"eventId":5}]`,
			} {
				_, err := f.pool.Exec(t.Context(), `UPDATE programs SET related_items = $1 WHERE service_id = $2 AND event_id = 5`, items, key)
				require.NoError(t, err)
			}

			conditions := conditionsInput{Keywords: []keywordInput{keyword(tt.keyword, true)}}
			found := []string{}
			var after *string
			for {
				var got searchResult
				require.Empty(t, f.query(t, searchQuery, searchVariables(conditions, nil, tt.first, after), &got))
				found = append(found, got.names()...)
				if !got.SearchPrograms.PageInfo.HasNextPage {
					break
				}
				after = got.SearchPrograms.PageInfo.EndCursor
			}
			assert.Equal(t, tt.want, found)
		})
	}
}
