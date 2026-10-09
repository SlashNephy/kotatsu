package graph_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/id"
)

const guideViewFields = `id name genres { lv1 lv2 } excludeGenres { lv1 lv2 } minDurationSeconds hideEmptyServices services { name }`

type guideViewInput struct {
	Name               string       `json:"name"`
	Genres             []genreInput `json:"genres,omitempty"`
	ExcludeGenres      []genreInput `json:"excludeGenres,omitempty"`
	MinDurationSeconds *int         `json:"minDurationSeconds,omitempty"`
	Services           []string     `json:"services,omitempty"`
	HideEmptyServices  bool         `json:"hideEmptyServices"`
}

type guideView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Genres []struct {
		Lv1 int  `json:"lv1"`
		Lv2 *int `json:"lv2"`
	} `json:"genres"`
	HideEmptyServices bool `json:"hideEmptyServices"`
	Services          []struct {
		Name string `json:"name"`
	} `json:"services"`
}

type guideViewVariables struct {
	ID    string          `json:"id,omitempty"`
	Input *guideViewInput `json:"input,omitempty"`
}

// inputVariables は、input だけを持つ変数を作る。
func inputVariables(input guideViewInput) guideViewVariables {
	return guideViewVariables{ID: "", Input: &input}
}

func (f *fixture) serviceIDs(t *testing.T) map[string]string {
	t.Helper()

	var got struct {
		Services []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"services"`
	}
	require.Empty(t, f.query(t, `{ services(includeHidden: true) { id name } }`, nil, &got))
	ids := map[string]string{}
	for _, s := range got.Services {
		ids[s.Name] = s.ID
	}

	return ids
}

// TestGuideViews は、番組表ビューを保存して取得でき、番組表に当てられることを確かめる。
func TestGuideViews(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	// こたつテスト2 と こたつテスト3 の 12:30 の番組 (eventId 4) をアニメにする。
	_, err := f.pool.Exec(t.Context(), `UPDATE programs SET genres = '[{"lv1":7,"lv2":0,"un1":15,"un2":15}]'
		WHERE event_id = 4 AND service_id IN (SELECT id FROM services WHERE name IN ('こたつテスト2', 'こたつテスト3'))`)
	require.NoError(t, err)
	services := f.serviceIDs(t)

	var created struct {
		CreateGuideView guideView `json:"createGuideView"`
	}
	require.Empty(t, f.query(t, `mutation($input: GuideViewInput!) { createGuideView(input: $input) { `+guideViewFields+` } }`,
		inputVariables(guideViewInput{
			Name: "アニメだけ", Genres: []genreInput{{Lv1: 7}}, HideEmptyServices: true,
			Services: []string{services["こたつテスト3"], services["こたつテスト2"]},
		}), &created))
	view := created.CreateGuideView
	assert.Equal(t, "アニメだけ", view.Name)
	assert.Nil(t, view.Genres[0].Lv2)
	assert.Equal(t, []string{"こたつテスト3", "こたつテスト2"}, []string{view.Services[0].Name, view.Services[1].Name})

	var listed struct {
		GuideViews []guideView `json:"guideViews"`
	}
	require.Empty(t, f.query(t, `{ guideViews { `+guideViewFields+` } }`, nil, &listed))
	assert.Equal(t, []guideView{view}, listed.GuideViews)

	// node でも引ける。
	var node struct {
		Node guideView `json:"node"`
	}
	require.Empty(t, f.query(t, `query($id: ID!) { node(id: $id) { ... on GuideView { `+guideViewFields+` } } }`, map[string]any{"id": view.ID}, &node))
	assert.Equal(t, view, node.Node)

	// ビューを当てると、アニメの番組だけが残り、番組のないサービスは隠れ、ビューの並びになる。
	var guide struct {
		Guide []struct {
			Service struct {
				Name string `json:"name"`
			} `json:"service"`
			Programs []struct {
				Name string `json:"name"`
			} `json:"programs"`
		} `json:"guide"`
	}
	require.Empty(t, f.query(t, `query($view: ID) { guide(from: "2026-10-10T12:00:00Z", to: "2026-10-10T13:00:00Z", view: $view) { service { name } programs { name } } }`,
		map[string]any{"view": view.ID}, &guide))
	rows := []string{}
	for _, row := range guide.Guide {
		for _, p := range row.Programs {
			rows = append(rows, row.Service.Name+": "+p.Name)
		}
	}
	assert.Equal(t, []string{"こたつテスト3: こたつテスト3 テスト番組 4", "こたつテスト2: こたつテスト2 テスト番組 4"}, rows)
	assert.Len(t, guide.Guide, 2)

	// 名前を変えて、消す。
	var updated struct {
		UpdateGuideView guideView `json:"updateGuideView"`
	}
	require.Empty(t, f.query(t, `mutation($id: ID!, $input: GuideViewInput!) { updateGuideView(id: $id, input: $input) { `+guideViewFields+` } }`,
		guideViewVariables{ID: view.ID, Input: &guideViewInput{Name: "ぜんぶ"}}, &updated))
	assert.Equal(t, "ぜんぶ", updated.UpdateGuideView.Name)
	assert.Empty(t, updated.UpdateGuideView.Services)

	var deleted struct {
		DeleteGuideView string `json:"deleteGuideView"`
	}
	require.Empty(t, f.query(t, `mutation($id: ID!) { deleteGuideView(id: $id) }`, map[string]any{"id": view.ID}, &deleted))
	assert.Equal(t, view.ID, deleted.DeleteGuideView)
	require.Empty(t, f.query(t, `{ guideViews { id } }`, nil, &listed))
	assert.Empty(t, listed.GuideViews)
}

func TestGuideViewErrors(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)
	create := `mutation($input: GuideViewInput!) { createGuideView(input: $input) { id } }`
	require.Empty(t, f.query(t, create, inputVariables(guideViewInput{Name: "アニメ"}), nil))

	// 他のユーザーの番組表ビュー。
	var otherUser string
	require.NoError(t, f.pool.QueryRow(t.Context(),
		`INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES ($1, 'other', 'other', 'member', now(), now()) RETURNING id`,
		id.New()).Scan(&otherUser))
	otherView := id.New()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO guide_views (id, user_id, name, settings, hide_empty_services, created_at, updated_at)
		VALUES ($1, $2, 'other', '{"genres":[],"excludeGenres":[],"services":[]}', false, now(), now())`, otherView, otherUser)
	require.NoError(t, err)
	otherViewID := graph.GlobalID("GuideView", otherView.String())

	tests := []struct {
		name      string
		query     string
		variables any
		want      string
	}{
		{name: "duplicate name", query: create, variables: inputVariables(guideViewInput{Name: "アニメ"}), want: codeBadUserInput},
		{name: "empty name", query: create, variables: inputVariables(guideViewInput{Name: ""}), want: codeBadUserInput},
		{name: "blank name", query: create, variables: inputVariables(guideViewInput{Name: " 　"}), want: codeBadUserInput},
		{name: "too long name", query: create, variables: inputVariables(guideViewInput{Name: strings.Repeat("あ", 101)}), want: codeBadUserInput},
		{
			name: "duplicate services", query: create,
			variables: inputVariables(guideViewInput{Name: "x", Services: []string{services["こたつテスト1"], services["こたつテスト1"]}}), want: codeBadUserInput,
		},
		{name: "invalid genre", query: create, variables: inputVariables(guideViewInput{Name: "x", Genres: []genreInput{{Lv1: 16}}}), want: codeBadUserInput},
		{
			name: "other user's view cannot be fetched by node", query: `query($id: ID!) { node(id: $id) { id } }`,
			variables: map[string]any{"id": otherViewID}, want: codeNotFound,
		},
		{
			name: "other user's view cannot be updated", query: `mutation($id: ID!, $input: GuideViewInput!) { updateGuideView(id: $id, input: $input) { id } }`,
			variables: guideViewVariables{ID: otherViewID, Input: &guideViewInput{Name: "x"}}, want: codeNotFound,
		},
		{
			name: "other user's view cannot be deleted", query: `mutation($id: ID!) { deleteGuideView(id: $id) }`,
			variables: map[string]any{"id": otherViewID}, want: codeNotFound,
		},
		{
			name: "other user's view cannot be applied", query: `query($id: ID) { guide(from: "2026-10-10T12:00:00Z", to: "2026-10-10T13:00:00Z", view: $id) { service { name } } }`,
			variables: map[string]any{"id": otherViewID}, want: codeNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []string{tt.want}, f.query(t, tt.query, tt.variables, nil))
		})
	}
}

// TestGuideViewFilters は、番組表ビューの除外するジャンルと番組の最短の長さで、番組表が絞り込まれることを確かめる。
func TestGuideViewFilters(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	// こたつテスト2 の 12:30 の番組 (eventId 4) だけをアニメにする。
	_, err := f.pool.Exec(t.Context(), `UPDATE programs SET genres = '[{"lv1":7,"lv2":0,"un1":15,"un2":15}]'
		WHERE event_id = 4 AND service_id IN (SELECT id FROM services WHERE name = 'こたつテスト2')`)
	require.NoError(t, err)

	type guideRows struct {
		Guide []struct {
			Programs []struct {
				Name string `json:"name"`
			} `json:"programs"`
		} `json:"guide"`
	}
	// count は、番組表のサービスの数と番組の数を返す。
	count := func(t *testing.T, viewID *string) (int, int) {
		t.Helper()

		var got guideRows
		require.Empty(t, f.query(t, `query($view: ID) { guide(from: "2026-10-10T12:00:00Z", to: "2026-10-10T13:00:00Z", view: $view) { programs { name } } }`,
			map[string]any{"view": viewID}, &got))
		programs := 0
		for _, row := range got.Guide {
			programs += len(row.Programs)
		}

		return len(got.Guide), programs
	}
	allServices, allPrograms := count(t, nil)
	require.Positive(t, allPrograms)

	tests := []struct {
		name         string
		input        guideViewInput
		wantServices int
		wantPrograms int
	}{
		{
			name: "excluded genre", input: guideViewInput{Name: "アニメ以外", ExcludeGenres: []genreInput{{Lv1: 7}}},
			wantServices: allServices, wantPrograms: allPrograms - 1,
		},
		{
			name: "programs as long as the minimum remain", input: guideViewInput{Name: "30 分以上", MinDurationSeconds: new(1800)},
			wantServices: allServices, wantPrograms: allPrograms,
		},
		{
			name: "shorter programs are removed", input: guideViewInput{Name: "1 時間以上", MinDurationSeconds: new(3600)},
			wantServices: allServices, wantPrograms: 0,
		},
		{
			name: "services without programs are hidden", input: guideViewInput{Name: "1 時間以上で隠す", MinDurationSeconds: new(3600), HideEmptyServices: true},
			wantServices: 0, wantPrograms: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var created struct {
				CreateGuideView guideView `json:"createGuideView"`
			}
			require.Empty(t, f.query(t, `mutation($input: GuideViewInput!) { createGuideView(input: $input) { id } }`, inputVariables(tt.input), &created))
			services, programs := count(t, &created.CreateGuideView.ID)
			assert.Equal(t, tt.wantServices, services)
			assert.Equal(t, tt.wantPrograms, programs)
		})
	}
}
