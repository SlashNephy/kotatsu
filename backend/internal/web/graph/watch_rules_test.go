package graph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/watch"
)

const (
	watchRuleFields = `id name enabled conditions { keywords { text } }`
	createWatchRule = `mutation($input: WatchRuleInput!) { createWatchRule(input: $input) { ` + watchRuleFields + ` } }`
	updateWatchRule = `mutation($id: ID!, $input: WatchRuleInput!) { updateWatchRule(id: $id, input: $input) { ` + watchRuleFields + ` } }`
	setWatchRuleOn  = `mutation($id: ID!, $enabled: Boolean!) { setWatchRuleEnabled(id: $id, enabled: $enabled) { ` + watchRuleFields + ` } }`
	deleteWatchRule = `mutation($id: ID!) { deleteWatchRule(id: $id) }`
	watchRuleNode   = `query($id: ID!) { node(id: $id) { ... on WatchRule { ` + watchRuleFields + ` } } }`
)

type watchRuleResult struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	Conditions struct {
		Keywords []struct {
			Text string `json:"text"`
		} `json:"keywords"`
	} `json:"conditions"`
}

type watchRuleInputVars struct {
	Name       string          `json:"name"`
	Conditions conditionsInput `json:"conditions"`
}

// watchRuleInput は、番組名に keyword を含む番組の視聴ルールの内容を返す。regexp なら keyword を正規表現として扱う。
func watchRuleInput(name, keyword string, regexp bool) *watchRuleInputVars {
	return &watchRuleInputVars{
		Name: name,
		Conditions: conditionsInput{
			Keywords: []keywordInput{{Text: keyword, Regexp: regexp, CaseSensitive: false, Fields: []string{"NAME"}}},
		},
	}
}

func (f *fixture) createWatchRule(t *testing.T, name string) (watchRuleResult, []string) {
	t.Helper()

	var got struct {
		CreateWatchRule watchRuleResult `json:"createWatchRule"`
	}
	codes := f.query(t, createWatchRule, map[string]any{varInput: watchRuleInput(name, name, false)}, &got)

	return got.CreateWatchRule, codes
}

func (f *fixture) watchRules(t *testing.T) []watchRuleResult {
	t.Helper()

	var got struct {
		WatchRules []watchRuleResult `json:"watchRules"`
	}
	require.Empty(t, f.query(t, `{ watchRules { `+watchRuleFields+` } }`, nil, &got))

	return got.WatchRules
}

// TestWatchRules は、閲覧だけのユーザーも、自分の視聴ルールを作り、変え、消せることと、
// 変えるたびに同じユーザーのほかの端末に知らせることを確かめる (設計書 4.8、9.7)。
func TestWatchRules(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil).as(t, "child", auth.RoleViewer)
	version := func() int64 {
		v, err := query.New(f.pool).UserSettingsVersion(t.Context(), f.userID)
		require.NoError(t, err)

		return v
	}

	before := version()
	created, codes := f.createWatchRule(t, "ゆるキャン")
	require.Empty(t, codes)
	assert.Equal(t, "ゆるキャン", created.Name)
	assert.True(t, created.Enabled, "a watch rule is enabled by default")
	assert.Greater(t, version(), before, "the other devices are told to reload")

	var updated struct {
		UpdateWatchRule watchRuleResult `json:"updateWatchRule"`
	}
	require.Empty(t, f.query(t, updateWatchRule, map[string]any{"id": created.ID, varInput: watchRuleInput("のんのんびより", "のんのん", false)}, &updated))
	assert.Equal(t, "のんのんびより", updated.UpdateWatchRule.Name)
	require.Len(t, updated.UpdateWatchRule.Conditions.Keywords, 1)
	assert.Equal(t, "のんのん", updated.UpdateWatchRule.Conditions.Keywords[0].Text)

	var toggled struct {
		SetWatchRuleEnabled watchRuleResult `json:"setWatchRuleEnabled"`
	}
	require.Empty(t, f.query(t, setWatchRuleOn, map[string]any{"id": created.ID, "enabled": false}, &toggled))
	assert.False(t, toggled.SetWatchRuleEnabled.Enabled)

	var node struct {
		Node watchRuleResult `json:"node"`
	}
	require.Empty(t, f.query(t, watchRuleNode, map[string]any{"id": created.ID}, &node))
	assert.Equal(t, toggled.SetWatchRuleEnabled, node.Node)
	assert.Equal(t, []watchRuleResult{toggled.SetWatchRuleEnabled}, f.watchRules(t))

	var deleted struct {
		DeleteWatchRule string `json:"deleteWatchRule"`
	}
	require.Empty(t, f.query(t, deleteWatchRule, map[string]any{"id": created.ID}, &deleted))
	assert.Equal(t, created.ID, deleted.DeleteWatchRule)
	assert.Empty(t, f.watchRules(t))
}

// TestWatchRulesOfOtherUsers は、ほかのユーザーの視聴ルールを、一覧に出さず、読むことも変えることもできない (NOT_FOUND) ことを確かめる。
func TestWatchRulesOfOtherUsers(t *testing.T) {
	t.Parallel()

	owner := newFixture(t, nil)
	created, codes := owner.createWatchRule(t, "ゆるキャン")
	require.Empty(t, codes)
	other := owner.as(t, "other", auth.RoleAdmin)

	tests := []struct {
		name      string
		query     string
		variables map[string]any
	}{
		{name: "読む", query: watchRuleNode, variables: map[string]any{"id": created.ID}},
		{name: "変える", query: updateWatchRule, variables: map[string]any{"id": created.ID, varInput: watchRuleInput("乗っ取り", "乗っ取り", false)}},
		{name: "無効にする", query: setWatchRuleOn, variables: map[string]any{"id": created.ID, "enabled": false}},
		{name: "消す", query: deleteWatchRule, variables: map[string]any{"id": created.ID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got map[string]any
			assert.Equal(t, []string{codeNotFound}, other.query(t, tt.query, tt.variables, &got))
		})
	}
	assert.Empty(t, other.watchRules(t))
}

// TestCreateWatchRuleRejects は、正しくない視聴ルールを BAD_USER_INPUT で、上限を超える視聴ルールを CONFLICT で拒むことを確かめる。
func TestCreateWatchRuleRejects(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	_, codes := f.createWatchRule(t, " ")
	assert.Equal(t, []string{codeBadUserInput}, codes, "blank name")
	var got map[string]any
	codes = f.query(t, createWatchRule, map[string]any{varInput: watchRuleInput("誤った正規表現", "(", true)}, &got)
	assert.Equal(t, []string{codeBadUserInput}, codes, "invalid pattern")

	// 上限まで作っておく。
	for range watch.MaxRulesPerUser {
		_, codes := f.createWatchRule(t, "ゆるキャン")
		require.Empty(t, codes)
	}
	_, codes = f.createWatchRule(t, "上限を超える")
	assert.Equal(t, []string{codeConflict}, codes)
}
