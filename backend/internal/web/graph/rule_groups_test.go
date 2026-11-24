package graph_test

import (
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	createRuleGroup   = `mutation($name: String!) { createRuleGroup(name: $name) { id name position } }`
	renameRuleGroup   = `mutation($id: ID!, $name: String!) { renameRuleGroup(id: $id, name: $name) { id name } }`
	reorderRuleGroups = `mutation($ids: [ID!]!) { reorderRuleGroups(ids: $ids) { name position } }`
	deleteRuleGroup   = `mutation($id: ID!) { deleteRuleGroup(id: $id) }`
	setRulesGroup     = `mutation($ids: [ID!]!, $groupId: ID) { setRulesGroup(ids: $ids, groupId: $groupId) { processed skipped } }`
	ruleWithGroup     = `query($id: ID!) { node(id: $id) { ... on Rule { group { id name } } } }`
	listRuleGroups    = `{ ruleGroups { id name position } }`
	varGroupID        = "groupId"
	dramaLabel        = "ドラマ"
)

// groupedRuleInput は、グループを渡すルールの内容。GroupID が nil なら null を送る。
type groupedRuleInput struct {
	ruleInput

	GroupID *string `json:"groupId"`
}

type ruleGroupResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

// createGroup は、グループを作り、その ID を返す。
func (f *fixture) createGroup(t *testing.T, name string) string {
	t.Helper()

	var created struct {
		CreateRuleGroup ruleGroupResult `json:"createRuleGroup"`
	}
	require.Empty(t, f.query(t, createRuleGroup, map[string]any{varName: name}, &created))

	return created.CreateRuleGroup.ID
}

// createTestRule は、テスト番組のルールを作り、その ID を返す。input を省くと、グループに入れない。
func (f *fixture) createTestRule(t *testing.T, input any) string {
	t.Helper()

	var created struct {
		CreateRule ruleResult `json:"createRule"`
	}
	require.Empty(t, f.query(t, createRule, map[string]any{varInput: input}, &created))

	return created.CreateRule.ID
}

// groupOf は、ルールが入るグループの名前を返す。どのグループにも入らなければ空を返す。
func (f *fixture) groupOf(t *testing.T, ruleID string) string {
	t.Helper()

	var got struct {
		Node struct {
			Group *ruleGroupResult `json:"group"`
		} `json:"node"`
	}
	require.Empty(t, f.query(t, ruleWithGroup, map[string]any{"id": ruleID}, &got))
	if got.Node.Group == nil {
		return ""
	}

	return got.Node.Group.Name
}

// TestUpdateRuleGroup は、updateRule で groupId を省くとグループを残し、null を送るとグループから外すことを確認する (#1493)。
func TestUpdateRuleGroup(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)
	anime := f.createGroup(t, animeLabel)
	drama := f.createGroup(t, dramaLabel)

	tests := []struct {
		name  string
		input any
		want  string
	}{
		{name: "groupId を省くと残す", input: testProgramRule(services), want: animeLabel},
		{name: "ほかのグループに変える", input: groupedRuleInput{ruleInput: testProgramRule(services), GroupID: &drama}, want: dramaLabel},
		{name: "null ならグループから外す", input: groupedRuleInput{ruleInput: testProgramRule(services), GroupID: nil}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ruleID := f.createTestRule(t, groupedRuleInput{ruleInput: testProgramRule(services), GroupID: &anime})
			require.Equal(t, animeLabel, f.groupOf(t, ruleID))

			require.Empty(t, f.query(t, updateRule, map[string]any{"id": ruleID, varInput: tt.input}, nil))
			assert.Equal(t, tt.want, f.groupOf(t, ruleID))
		})
	}
}

// TestRuleGroups は、グループを作り、名前を変え、並べ直し、node で引き、消せることを確認する。
// グループを消すと、中のルールはどのグループにも入らないルールに戻る。
func TestRuleGroups(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)
	anime := f.createGroup(t, animeLabel)
	drama := f.createGroup(t, dramaLabel)
	ruleID := f.createTestRule(t, groupedRuleInput{ruleInput: testProgramRule(services), GroupID: &anime})

	var renamed struct {
		RenameRuleGroup ruleGroupResult `json:"renameRuleGroup"`
	}
	require.Empty(t, f.query(t, renameRuleGroup, map[string]any{"id": drama, varName: " 連続ドラマ "}, &renamed))
	assert.Equal(t, "連続ドラマ", renamed.RenameRuleGroup.Name)

	var reordered struct {
		ReorderRuleGroups []ruleGroupResult `json:"reorderRuleGroups"`
	}
	require.Empty(t, f.query(t, reorderRuleGroups, map[string]any{varIDs: []string{drama, anime}}, &reordered))
	assert.Equal(t, []ruleGroupResult{{ID: "", Name: "連続ドラマ", Position: 0}, {ID: "", Name: animeLabel, Position: 1}}, reordered.ReorderRuleGroups)

	var listed struct {
		RuleGroups []ruleGroupResult `json:"ruleGroups"`
	}
	require.Empty(t, f.query(t, listRuleGroups, nil, &listed))
	assert.Equal(t, []ruleGroupResult{{ID: drama, Name: "連続ドラマ", Position: 0}, {ID: anime, Name: animeLabel, Position: 1}}, listed.RuleGroups)

	// グループは、node でも引ける。
	var node struct {
		Node ruleGroupResult `json:"node"`
	}
	require.Empty(t, f.query(t, `query($id: ID!) { node(id: $id) { ... on RuleGroup { id name position } } }`, map[string]any{"id": anime}, &node))
	assert.Equal(t, ruleGroupResult{ID: anime, Name: animeLabel, Position: 1}, node.Node)

	require.Empty(t, f.query(t, deleteRuleGroup, map[string]any{"id": anime}, nil))
	assert.Empty(t, f.groupOf(t, ruleID))
}

// TestSetRulesGroup は、ルールをまとめてグループに移し、見つからないルールを飛ばして数えることを確認する。
func TestSetRulesGroup(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)
	anime := f.createGroup(t, animeLabel)
	// ids は、作った 2 件のルールの ID。移すときに、見つからないルールを 1 件足す。
	ids := make([]string, 0, 3)
	ids = append(ids, f.createTestRule(t, testProgramRule(services)), f.createTestRule(t, testProgramRule(services)))

	var moved struct {
		SetRulesGroup struct {
			Processed int `json:"processed"`
			Skipped   int `json:"skipped"`
		} `json:"setRulesGroup"`
	}
	require.Empty(t, f.query(t, setRulesGroup, map[string]any{varIDs: append(ids, graph.GlobalID("Rule", uuid.New().String())), varGroupID: anime}, &moved))
	assert.Equal(t, 2, moved.SetRulesGroup.Processed)
	assert.Equal(t, 1, moved.SetRulesGroup.Skipped)
	for _, ruleID := range ids {
		assert.Equal(t, animeLabel, f.groupOf(t, ruleID))
	}

	require.Empty(t, f.query(t, setRulesGroup, map[string]any{varIDs: ids[:1], varGroupID: nil}, &moved))
	assert.Empty(t, f.groupOf(t, ids[0]))
	assert.Equal(t, animeLabel, f.groupOf(t, ids[1]))
}

// TestRuleGroupErrors は、視聴者がグループを変えられず、読めることと、正しくない入力のエラーコードを確認する。
func TestRuleGroupErrors(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)
	anime := f.createGroup(t, animeLabel)
	ruleID := f.createTestRule(t, groupedRuleInput{ruleInput: testProgramRule(services), GroupID: &anime})
	viewer := f.as(t, "child", domain.RoleViewer)
	missing := graph.GlobalID("RuleGroup", uuid.New().String())

	tests := []struct {
		name      string
		f         *fixture
		query     string
		variables map[string]any
		want      string
	}{
		{name: "viewer cannot create a group", f: viewer, query: createRuleGroup, variables: map[string]any{varName: dramaLabel}, want: codeForbidden},
		{name: "viewer cannot rename a group", f: viewer, query: renameRuleGroup, variables: map[string]any{"id": anime, varName: dramaLabel}, want: codeForbidden},
		{name: "viewer cannot reorder groups", f: viewer, query: reorderRuleGroups, variables: map[string]any{varIDs: []string{anime}}, want: codeForbidden},
		{name: "viewer cannot delete a group", f: viewer, query: deleteRuleGroup, variables: map[string]any{"id": anime}, want: codeForbidden},
		{name: "viewer cannot move rules", f: viewer, query: setRulesGroup, variables: map[string]any{varIDs: []string{ruleID}, varGroupID: nil}, want: codeForbidden},
		{name: "same group name", f: f, query: createRuleGroup, variables: map[string]any{varName: animeLabel}, want: codeBadUserInput},
		{name: "blank group name", f: f, query: createRuleGroup, variables: map[string]any{varName: "  "}, want: codeBadUserInput},
		{name: "rename missing group", f: f, query: renameRuleGroup, variables: map[string]any{"id": missing, varName: dramaLabel}, want: codeNotFound},
		{name: "reorder partially", f: f, query: reorderRuleGroups, variables: map[string]any{varIDs: []string{}}, want: codeBadUserInput},
		{name: "delete missing group", f: f, query: deleteRuleGroup, variables: map[string]any{"id": missing}, want: codeNotFound},
		{name: "move to missing group", f: f, query: setRulesGroup, variables: map[string]any{varIDs: []string{ruleID}, varGroupID: missing}, want: codeNotFound},
		{name: "group ID of another type", f: f, query: setRulesGroup, variables: map[string]any{varIDs: []string{ruleID}, varGroupID: ruleID}, want: codeBadUserInput},
		{
			name: "update with missing group", f: f, query: updateRule,
			variables: map[string]any{"id": ruleID, varInput: groupedRuleInput{ruleInput: testProgramRule(services), GroupID: &missing}}, want: codeNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []string{tt.want}, tt.f.query(t, tt.query, tt.variables, nil))
		})
	}

	t.Run("viewer can read", func(t *testing.T) {
		t.Parallel()

		var listed struct {
			RuleGroups []ruleGroupResult `json:"ruleGroups"`
		}
		require.Empty(t, viewer.query(t, listRuleGroups, nil, &listed))
		assert.Equal(t, []ruleGroupResult{{ID: anime, Name: animeLabel, Position: 0}}, listed.RuleGroups)
		assert.Equal(t, animeLabel, viewer.groupOf(t, ruleID))
	})
}
