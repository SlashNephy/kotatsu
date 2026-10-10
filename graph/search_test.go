package graph_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
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
	Text   string   `json:"text"`
	Regexp bool     `json:"regexp"`
	Fields []string `json:"fields"`
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
	return keywordInput{Text: text, Regexp: regexp, Fields: []string{fieldName}}
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

	// こたつテスト1 の番組は 14 件あるので、5 件、5 件、4 件に分かれる。
	require.Len(t, pages, 3)
	assert.Equal(t, []int{5, 5, 4}, []int{len(pages[0]), len(pages[1]), len(pages[2])})
	assert.Equal(t, programName(1), pages[0][0])
	assert.Equal(t, programName(14), pages[2][3])
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

// TestSearchProgramsStopsAtCandidateLimit は、照合する候補が上限に達したら途中で返し、カーソルから続きを検索できることを確かめる。
func TestSearchProgramsStopsAtCandidateLimit(t *testing.T) {
	t.Parallel()

	// 候補を 7 件ずつしか照合しない。地上波のサービスの eventId 14 の番組は 70 件中の最後のほうにある。
	f := newFixture(t, &graph.Options{MaxSearchCandidates: 7})
	f.syncEPG(t)
	conditions := conditionsInput{Keywords: []keywordInput{keyword("テスト番組 14$", true)}}

	found := []string{}
	requests := 0
	var after *string
	for {
		var got searchResult
		require.Empty(t, f.query(t, searchQuery, searchVariables(conditions, nil, 50, after), &got))
		requests++
		found = append(found, got.names()...)
		if !got.SearchPrograms.PageInfo.HasNextPage {
			break
		}
		require.NotNil(t, got.SearchPrograms.PageInfo.EndCursor)
		after = got.SearchPrograms.PageInfo.EndCursor
	}

	// 上限で区切っても、続きを検索すれば、すべて見つかる。
	assert.Len(t, found, 5)
	assert.Greater(t, requests, 5)
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
