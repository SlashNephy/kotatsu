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
	require.Empty(t, f.query(t, createRuleGroup, map[string]any{"name": name}, &created))

	return created.CreateRuleGroup.ID
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

// TestRuleGroups は、グループを作り、名前を変え、並べ直し、消せることと、ルールのグループを API で選べることを確かめる。
// updateRule で groupId を省くとグループを残し、null を送るとグループから外す (#1493)。
func TestRuleGroups(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)
	anime := f.createGroup(t, "アニメ")
	drama := f.createGroup(t, "ドラマ")

	var created struct {
		CreateRule ruleResult `json:"createRule"`
	}
	require.Empty(t, f.query(t, createRule, map[string]any{varInput: groupedRuleInput{ruleInput: testProgramRule(services), GroupID: &anime}}, &created))
	ruleID := created.CreateRule.ID
	assert.Equal(t, "アニメ", f.groupOf(t, ruleID))

	tests := []struct {
		name  string
		input any
		want  string
	}{
		{name: "groupId を省くと残す", input: testProgramRule(services), want: "アニメ"},
		{name: "ほかのグループに変える", input: groupedRuleInput{ruleInput: testProgramRule(services), GroupID: &drama}, want: "ドラマ"},
		{name: "null ならグループから外す", input: groupedRuleInput{ruleInput: testProgramRule(services), GroupID: nil}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Empty(t, f.query(t, setRulesGroup, map[string]any{varIDs: []string{ruleID}, "groupId": anime}, nil))
			require.Empty(t, f.query(t, updateRule, map[string]any{"id": ruleID, varInput: tt.input}, nil))
			assert.Equal(t, tt.want, f.groupOf(t, ruleID))
		})
	}

	var renamed struct {
		RenameRuleGroup ruleGroupResult `json:"renameRuleGroup"`
	}
	require.Empty(t, f.query(t, renameRuleGroup, map[string]any{"id": drama, "name": " 連続ドラマ "}, &renamed))
	assert.Equal(t, "連続ドラマ", renamed.RenameRuleGroup.Name)

	var reordered struct {
		ReorderRuleGroups []ruleGroupResult `json:"reorderRuleGroups"`
	}
	require.Empty(t, f.query(t, reorderRuleGroups, map[string]any{varIDs: []string{drama, anime}}, &reordered))
	assert.Equal(t, []ruleGroupResult{{ID: "", Name: "連続ドラマ", Position: 0}, {ID: "", Name: "アニメ", Position: 1}}, reordered.ReorderRuleGroups)

	var listed struct {
		RuleGroups []ruleGroupResult `json:"ruleGroups"`
	}
	require.Empty(t, f.query(t, `{ ruleGroups { id name position } }`, nil, &listed))
	assert.Equal(t, []ruleGroupResult{{ID: drama, Name: "連続ドラマ", Position: 0}, {ID: anime, Name: "アニメ", Position: 1}}, listed.RuleGroups)

	// グループを消すと、中のルールはどのグループにも入らないルールに戻る。
	require.Empty(t, f.query(t, setRulesGroup, map[string]any{varIDs: []string{ruleID}, "groupId": anime}, nil))
	require.Empty(t, f.query(t, deleteRuleGroup, map[string]any{"id": anime}, nil))
	assert.Empty(t, f.groupOf(t, ruleID))
}

// TestSetRulesGroup は、ルールをまとめてグループに移し、見つからないルールを飛ばして数えることを確かめる。
func TestSetRulesGroup(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)
	anime := f.createGroup(t, "アニメ")
	ids := make([]string, 0, 3)
	for range 2 {
		var created struct {
			CreateRule ruleResult `json:"createRule"`
		}
		require.Empty(t, f.query(t, createRule, ruleVariables("", testProgramRule(services)), &created))
		ids = append(ids, created.CreateRule.ID)
	}

	var moved struct {
		SetRulesGroup struct {
			Processed int `json:"processed"`
			Skipped   int `json:"skipped"`
		} `json:"setRulesGroup"`
	}
	require.Empty(t, f.query(t, setRulesGroup, map[string]any{varIDs: append(ids, graph.GlobalID("Rule", uuid.New().String())), "groupId": anime}, &moved))
	assert.Equal(t, 2, moved.SetRulesGroup.Processed)
	assert.Equal(t, 1, moved.SetRulesGroup.Skipped)
	for _, ruleID := range ids {
		assert.Equal(t, "アニメ", f.groupOf(t, ruleID))
	}

	require.Empty(t, f.query(t, setRulesGroup, map[string]any{varIDs: ids[:1], "groupId": nil}, &moved))
	assert.Empty(t, f.groupOf(t, ids[0]))
	assert.Equal(t, "アニメ", f.groupOf(t, ids[1]))
}

// TestRuleGroupErrors は、視聴者がグループを変えられないことと、正しくない入力のエラーコードを確かめる。
func TestRuleGroupErrors(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)
	anime := f.createGroup(t, "アニメ")
	var created struct {
		CreateRule ruleResult `json:"createRule"`
	}
	require.Empty(t, f.query(t, createRule, ruleVariables("", testProgramRule(services)), &created))
	ruleID := created.CreateRule.ID
	viewer := f.as(t, "child", domain.RoleViewer)
	missing := graph.GlobalID("RuleGroup", uuid.New().String())

	tests := []struct {
		name      string
		f         *fixture
		query     string
		variables map[string]any
		want      string
	}{
		{name: "viewer cannot create", f: viewer, query: createRuleGroup, variables: map[string]any{"name": "ドラマ"}, want: codeForbidden},
		{name: "viewer cannot rename", f: viewer, query: renameRuleGroup, variables: map[string]any{"id": anime, "name": "ドラマ"}, want: codeForbidden},
		{name: "viewer cannot reorder", f: viewer, query: reorderRuleGroups, variables: map[string]any{varIDs: []string{anime}}, want: codeForbidden},
		{name: "viewer cannot delete", f: viewer, query: deleteRuleGroup, variables: map[string]any{"id": anime}, want: codeForbidden},
		{name: "viewer cannot move rules", f: viewer, query: setRulesGroup, variables: map[string]any{varIDs: []string{ruleID}, "groupId": anime}, want: codeForbidden},
		{name: "same name", f: f, query: createRuleGroup, variables: map[string]any{"name": "アニメ"}, want: codeBadUserInput},
		{name: "blank name", f: f, query: createRuleGroup, variables: map[string]any{"name": "  "}, want: codeBadUserInput},
		{name: "rename missing group", f: f, query: renameRuleGroup, variables: map[string]any{"id": missing, "name": "ドラマ"}, want: codeNotFound},
		{name: "reorder partially", f: f, query: reorderRuleGroups, variables: map[string]any{varIDs: []string{}}, want: codeBadUserInput},
		{name: "delete missing group", f: f, query: deleteRuleGroup, variables: map[string]any{"id": missing}, want: codeNotFound},
		{name: "move to missing group", f: f, query: setRulesGroup, variables: map[string]any{varIDs: []string{ruleID}, "groupId": missing}, want: codeNotFound},
		{name: "group ID of another type", f: f, query: setRulesGroup, variables: map[string]any{varIDs: []string{ruleID}, "groupId": ruleID}, want: codeBadUserInput},
		{
			name: "update with missing group", f: f, query: updateRule,
			variables: map[string]any{"id": ruleID, varInput: groupedRuleInput{ruleInput: testProgramRule(services), GroupID: &missing}}, want: codeNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, []string{tt.want}, tt.f.query(t, tt.query, tt.variables, nil))
		})
	}

	t.Run("viewer can read", func(t *testing.T) {
		require.Empty(t, f.query(t, setRulesGroup, map[string]any{varIDs: []string{ruleID}, "groupId": anime}, nil))
		var listed struct {
			RuleGroups []ruleGroupResult `json:"ruleGroups"`
		}
		require.Empty(t, viewer.query(t, `{ ruleGroups { id name position } }`, nil, &listed))
		assert.Equal(t, []ruleGroupResult{{ID: anime, Name: "アニメ", Position: 0}}, listed.RuleGroups)
		assert.Equal(t, "アニメ", viewer.groupOf(t, ruleID))
	})
}
