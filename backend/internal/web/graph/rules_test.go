package graph_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	ruleFields = `id name memo enabled validFrom validUntil avoidDuplicates duplicateWindowDays followEventRelay
		conditions { keywords { text regexp caseSensitive fields } keywordMatch channelTypes services { name } genres { lv1 lv2 } timeRanges { weekdays startMinute durationMinutes } freeOnly }
		settings { priority startMarginSeconds poolId }
		createdBy { username } updatedBy { username }`
	createRule  = `mutation($input: RuleInput!) { createRule(input: $input) { ` + ruleFields + ` } }`
	updateRule  = `mutation($id: ID!, $input: RuleInput!) { updateRule(id: $id, input: $input) { ` + ruleFields + ` } }`
	setEnabled  = `mutation($id: ID!, $enabled: Boolean!) { setRuleEnabled(id: $id, enabled: $enabled) { ` + ruleFields + ` } }`
	deleteRule  = `mutation($id: ID!) { deleteRule(id: $id) }`
	previewRule = `query($input: RuleInput!, $first: Int!, $after: String) {
		previewRule(input: $input, first: $first, after: $after) { edges { node { name startAt } } pageInfo { hasNextPage endCursor } }
	}`
)

type settingsInput struct {
	Priority *int `json:"priority,omitempty"`
}

type timeRangeInput struct {
	Weekdays        []int `json:"weekdays"`
	StartMinute     int   `json:"startMinute"`
	DurationMinutes int   `json:"durationMinutes"`
}

type ruleConditionsInput struct {
	conditionsInput

	TimeRanges []timeRangeInput `json:"timeRanges,omitempty"`
}

type ruleInput struct {
	Name                string              `json:"name"`
	Memo                string              `json:"memo,omitempty"`
	Enabled             *bool               `json:"enabled,omitempty"`
	Conditions          ruleConditionsInput `json:"conditions"`
	Settings            *settingsInput      `json:"settings,omitempty"`
	ValidFrom           string              `json:"validFrom,omitempty"`
	ValidUntil          string              `json:"validUntil,omitempty"`
	AvoidDuplicates     bool                `json:"avoidDuplicates"`
	DuplicateWindowDays *int                `json:"duplicateWindowDays,omitempty"`
}

type username struct {
	Username string `json:"username"`
}

type ruleResult struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Memo       string     `json:"memo"`
	Enabled    bool       `json:"enabled"`
	ValidFrom  *time.Time `json:"validFrom"`
	ValidUntil *time.Time `json:"validUntil"`
	Conditions struct {
		Keywords []struct {
			Text   string   `json:"text"`
			Fields []string `json:"fields"`
		} `json:"keywords"`
		KeywordMatch string   `json:"keywordMatch"`
		ChannelTypes []string `json:"channelTypes"`
		Services     []struct {
			Name string `json:"name"`
		} `json:"services"`
		TimeRanges []timeRangeInput `json:"timeRanges"`
	} `json:"conditions"`
	Settings struct {
		Priority *int `json:"priority"`
	} `json:"settings"`
	DuplicateWindowDays *int      `json:"duplicateWindowDays"`
	CreatedBy           *username `json:"createdBy"`
	UpdatedBy           *username `json:"updatedBy"`
}

type previewResult struct {
	PreviewRule struct {
		Edges []struct {
			Node struct {
				Name string `json:"name"`
			} `json:"node"`
		} `json:"edges"`
		PageInfo struct {
			HasNextPage bool    `json:"hasNextPage"`
			EndCursor   *string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"previewRule"`
}

func (r *previewResult) names() []string {
	names := make([]string, 0, len(r.PreviewRule.Edges))
	for _, edge := range r.PreviewRule.Edges {
		names = append(names, edge.Node.Name)
	}

	return names
}

// testProgramRule は、こたつテスト1 のテスト番組に一致するルールの内容を返す。
func testProgramRule(services map[string]string) ruleInput {
	return ruleInput{
		Name: "テスト番組",
		Conditions: ruleConditionsInput{
			conditionsInput: conditionsInput{
				Keywords: []keywordInput{{Text: "テスト番組", Regexp: false, Fields: []string{fieldName}}},
				Services: []string{services["こたつテスト1"]},
			},
		},
	}
}

func ruleVariables(id string, input ruleInput) map[string]any {
	variables := map[string]any{varInput: input}
	if id != "" {
		variables["id"] = id
	}

	return variables
}

// TestRules は、ルールを作り、取得し、変え、消せることと、その操作が操作者とともに監査ログに残ることを確かめる。
func TestRules(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)

	input := testProgramRule(services)
	input.Memo = "毎日"
	input.Settings = &settingsInput{Priority: new(3)}
	input.ValidFrom, input.ValidUntil = noon, "2026-12-31T15:00:00Z"
	input.DuplicateWindowDays = new(7)
	input.Conditions.TimeRanges = []timeRangeInput{{Weekdays: []int{6}, StartMinute: 1200, DurationMinutes: 120}}
	input.Conditions.KeywordMatch = keywordMatchAny
	var created struct {
		CreateRule ruleResult `json:"createRule"`
	}
	require.Empty(t, f.query(t, createRule, ruleVariables("", input), &created))
	got := created.CreateRule
	assert.Equal(t, "テスト番組", got.Name)
	assert.Equal(t, "毎日", got.Memo)
	assert.True(t, got.Enabled)
	assert.Equal(t, []string{fieldName}, got.Conditions.Keywords[0].Fields)
	assert.Equal(t, "ANY", got.Conditions.KeywordMatch)
	assert.Equal(t, "こたつテスト1", got.Conditions.Services[0].Name)
	assert.Equal(t, []timeRangeInput{{Weekdays: []int{6}, StartMinute: 1200, DurationMinutes: 120}}, got.Conditions.TimeRanges)
	assert.Equal(t, new(3), got.Settings.Priority)
	assert.Equal(t, new(7), got.DuplicateWindowDays)
	assert.Equal(t, &username{Username: testUsername}, got.CreatedBy)

	// 一覧と node で、同じルールを引ける。
	var listed struct {
		Rules []ruleResult `json:"rules"`
	}
	require.Empty(t, f.query(t, `{ rules { `+ruleFields+` } }`, nil, &listed))
	assert.Equal(t, []ruleResult{got}, listed.Rules)
	var node struct {
		Node ruleResult `json:"node"`
	}
	require.Empty(t, f.query(t, `query($id: ID!) { node(id: $id) { ... on Rule { `+ruleFields+` } } }`, map[string]any{"id": got.ID}, &node))
	assert.Equal(t, got, node.Node)

	// 別のメンバーが変える。ルールは家族で共有する (設計書 9.7)。
	other := f.as(t, "other", auth.RoleMember)
	var updated struct {
		UpdateRule ruleResult `json:"updateRule"`
	}
	input.Name, input.Settings = "テスト番組 (変更)", nil
	require.Empty(t, other.query(t, updateRule, ruleVariables(got.ID, input), &updated))
	assert.Equal(t, "テスト番組 (変更)", updated.UpdateRule.Name)
	assert.Nil(t, updated.UpdateRule.Settings.Priority)
	assert.Equal(t, &username{Username: testUsername}, updated.UpdateRule.CreatedBy)
	assert.Equal(t, &username{Username: "other"}, updated.UpdateRule.UpdatedBy)

	var disabled struct {
		SetRuleEnabled ruleResult `json:"setRuleEnabled"`
	}
	require.Empty(t, f.query(t, setEnabled, map[string]any{"id": got.ID, varEnabled: false}, &disabled))
	assert.False(t, disabled.SetRuleEnabled.Enabled)
	require.Empty(t, f.query(t, setEnabled, map[string]any{"id": got.ID, varEnabled: true}, &disabled))
	assert.True(t, disabled.SetRuleEnabled.Enabled)

	var deleted struct {
		DeleteRule string `json:"deleteRule"`
	}
	require.Empty(t, f.query(t, deleteRule, map[string]any{"id": got.ID}, &deleted))
	assert.Equal(t, got.ID, deleted.DeleteRule)
	require.Empty(t, f.query(t, `{ rules { id } }`, nil, &listed))
	assert.Empty(t, listed.Rules)

	var actions []string
	rows, err := f.pool.Query(t.Context(), `SELECT action FROM audit_events WHERE target_type = 'rule' AND user_agent = $1 ORDER BY occurred_at, action`, testUserAgent)
	require.NoError(t, err)
	for rows.Next() {
		var action string
		require.NoError(t, rows.Scan(&action))
		actions = append(actions, action)
	}
	require.NoError(t, rows.Err())
	assert.ElementsMatch(t, []string{"rule.create", "rule.update", "rule.disable", "rule.enable", "rule.delete"}, actions)
}

func TestRuleErrors(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)
	var created struct {
		CreateRule ruleResult `json:"createRule"`
	}
	require.Empty(t, f.query(t, createRule, ruleVariables("", testProgramRule(services)), &created))
	ruleID := created.CreateRule.ID
	viewer := f.as(t, "child", auth.RoleViewer)

	invalid := func(modify func(*ruleInput)) map[string]any {
		input := testProgramRule(services)
		modify(&input)

		return ruleVariables("", input)
	}
	invalidPreview := func(modify func(*ruleInput)) map[string]any {
		variables := invalid(modify)
		variables[varFirst] = 1

		return variables
	}
	tests := []struct {
		name      string
		f         *fixture
		query     string
		variables map[string]any
		want      string
	}{
		{name: "viewer cannot create", f: viewer, query: createRule, variables: ruleVariables("", testProgramRule(services)), want: codeForbidden},
		{name: "viewer cannot update", f: viewer, query: updateRule, variables: ruleVariables(ruleID, testProgramRule(services)), want: codeForbidden},
		{name: "viewer cannot disable", f: viewer, query: setEnabled, variables: map[string]any{"id": ruleID, varEnabled: false}, want: codeForbidden},
		{name: "viewer cannot delete", f: viewer, query: deleteRule, variables: map[string]any{"id": ruleID}, want: codeForbidden},
		{name: "blank name", f: f, query: createRule, variables: invalid(func(in *ruleInput) { in.Name = " " }), want: codeBadUserInput},
		{name: "invalid regular expression", f: f, query: createRule, variables: invalid(func(in *ruleInput) {
			in.Conditions.Keywords = []keywordInput{{Text: "(", Regexp: true, Fields: []string{fieldName}}}
		}), want: codeBadUserInput},
		{name: "invalid duplicate window", f: f, query: createRule, variables: invalid(func(in *ruleInput) { in.DuplicateWindowDays = new(0) }), want: codeBadUserInput},
		{name: "invalid priority", f: f, query: createRule, variables: invalid(func(in *ruleInput) { in.Settings = &settingsInput{Priority: new(11)} }), want: codeBadUserInput},
		{name: "empty valid period", f: f, query: createRule, variables: invalid(func(in *ruleInput) { in.ValidFrom, in.ValidUntil = noon, noon }), want: codeBadUserInput},
		{name: "preview without programs to return", f: f, query: previewRule, variables: map[string]any{varInput: testProgramRule(services), varFirst: 0}, want: codeBadUserInput},
		{name: "preview with too many programs to return", f: f, query: previewRule, variables: map[string]any{varInput: testProgramRule(services), varFirst: 101}, want: codeBadUserInput},
		{name: "preview with an empty valid period", f: f, query: previewRule, variables: invalidPreview(func(in *ruleInput) { in.ValidFrom, in.ValidUntil = noon, noon }), want: codeBadUserInput},
		{name: "invalid rule ID", f: f, query: deleteRule, variables: map[string]any{"id": "invalid"}, want: codeBadUserInput},
		{
			name: "unknown rule", f: f, query: setEnabled,
			variables: map[string]any{"id": "UnVsZTowMWExMjI4MC0wN2IwLTc1NzYtYWQwMC0xMWYzYjM3MGVkY2E", varEnabled: true}, want: codeNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []string{tt.want}, tt.f.query(t, tt.query, tt.variables, nil))
		})
	}

	// 視聴者も、ルールの一覧とプレビューは見られる。
	t.Run("viewer can read", func(t *testing.T) {
		t.Parallel()

		var listed struct {
			Rules []ruleResult `json:"rules"`
		}
		require.Empty(t, viewer.query(t, `{ rules { id } }`, nil, &listed))
		assert.Len(t, listed.Rules, 1)
		var preview previewResult
		require.Empty(t, viewer.query(t, previewRule, map[string]any{varInput: testProgramRule(services), varFirst: 1}, &preview))
		assert.Len(t, preview.PreviewRule.Edges, 1)
	})
}

// TestPreviewRule は、プレビューが、ルールの照合と同じく、放送が始まった番組、終了時刻が未定の番組、有効期間の外の番組を含めないことを確かめる。
func TestPreviewRule(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)
	// 14:30 の番組 (eventId 8) の終了時刻を未定にする。
	_, err := f.pool.Exec(t.Context(), `UPDATE programs SET end_undecided = true
		WHERE event_id = 8 AND service_id = (SELECT id FROM services WHERE name = 'こたつテスト1')`)
	require.NoError(t, err)

	tests := []struct {
		name   string
		modify func(*ruleInput)
		first  int
		want   []string
	}{
		{
			// 今は 12:10 なので、12:00 に始まった番組 (eventId 3) は含めない。
			name:   "programs after now",
			modify: func(*ruleInput) {},
			first:  3,
			want:   []string{programName(4), programName(5), programName(6)},
		},
		{
			name:   "programs in the valid period",
			modify: func(in *ruleInput) { in.ValidFrom, in.ValidUntil = onePM, "2026-10-10T14:00:00Z" },
			first:  10,
			want:   []string{programName(5), programName(6)},
		},
		{
			name:   "programs whose end is undecided",
			modify: func(in *ruleInput) { in.ValidFrom, in.ValidUntil = "2026-10-10T14:00:00Z", "2026-10-10T15:30:00Z" },
			first:  10,
			want:   []string{programName(7), programName(9)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := testProgramRule(services)
			tt.modify(&input)
			var got previewResult
			require.Empty(t, f.query(t, previewRule, map[string]any{varInput: input, varFirst: tt.first}, &got))
			assert.Equal(t, tt.want, got.names())
		})
	}

	// プレビューはルールを保存しない。
	var listed struct {
		Rules []ruleResult `json:"rules"`
	}
	require.Empty(t, f.query(t, `{ rules { id } }`, nil, &listed))
	assert.Empty(t, listed.Rules)
}

// TestRuleWarnings は、詳細まで探す正規表現のうち、前段で絞り込めないキーワードを、警告として返すことを確かめる (#904)。
func TestRuleWarnings(t *testing.T) {
	t.Parallel()

	const fieldExtended = "EXTENDED"
	f := newFixture(t, nil)
	f.syncEPG(t)
	input := testProgramRule(f.serviceIDs(t))
	extended := []string{fieldName, fieldExtended}
	input.Conditions.Keywords = []keywordInput{
		{Text: "みかん紀行", Regexp: true, Fields: extended},
		{Text: "[0-9]+", Regexp: true, Fields: extended},
	}
	input.Conditions.ExcludeKeywords = []keywordInput{{Text: "[0-9]+", Regexp: true, Fields: []string{fieldName}}}
	var got struct {
		RuleWarnings []struct {
			Kind    string `json:"kind"`
			Exclude bool   `json:"exclude"`
			Index   int    `json:"index"`
		} `json:"ruleWarnings"`
	}
	require.Empty(t, f.query(t, `query($input: RuleInput!) { ruleWarnings(input: $input) { kind exclude index } }`,
		map[string]any{varInput: input}, &got))
	require.Len(t, got.RuleWarnings, 1)
	assert.Equal(t, "UNFILTERABLE_REGEXP", got.RuleWarnings[0].Kind)
	assert.False(t, got.RuleWarnings[0].Exclude)
	assert.Equal(t, 1, got.RuleWarnings[0].Index)
}

// TestPreviewRulePages は、プレビューを続きから読むと、同じ番組を 2 回返さずに読み進められることを確かめる。
func TestPreviewRulePages(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	input := testProgramRule(f.serviceIDs(t))
	input.ValidUntil = "2026-10-10T14:30:00Z"

	got := []string{}
	variables := map[string]any{varInput: input, varFirst: 2}
	for {
		var page previewResult
		require.Empty(t, f.query(t, previewRule, variables, &page))
		got = append(got, page.names()...)
		if !page.PreviewRule.PageInfo.HasNextPage {
			break
		}
		variables["after"] = *page.PreviewRule.PageInfo.EndCursor
	}
	assert.Equal(t, []string{programName(4), programName(5), programName(6), programName(7)}, got)
}

// TestBulkRules は、ルールをまとめて有効、無効にし、まとめて消せることと、見つからないルールは飛ばして数えることを確かめる (#471)。
func TestBulkRules(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)
	ids := make([]string, 0, 3)
	for range 2 {
		var created struct {
			CreateRule ruleResult `json:"createRule"`
		}
		require.Empty(t, f.query(t, createRule, ruleVariables("", testProgramRule(services)), &created))
		ids = append(ids, created.CreateRule.ID)
	}
	ids = append(ids, graph.GlobalID("Rule", uuid.New().String()))
	type payload struct {
		Processed int `json:"processed"`
		Skipped   int `json:"skipped"`
	}
	const (
		setEnabled = `mutation($ids: [ID!]!, $enabled: Boolean!) { setRulesEnabled(ids: $ids, enabled: $enabled) { processed skipped } }`
		deleteMany = `mutation($ids: [ID!]!) { deleteRules(ids: $ids) { processed skipped } }`
	)
	enabled := func() []bool {
		var got struct {
			Rules []struct {
				Enabled bool `json:"enabled"`
			} `json:"rules"`
		}
		require.Empty(t, f.query(t, `{ rules { enabled } }`, nil, &got))
		flags := make([]bool, 0, len(got.Rules))
		for _, r := range got.Rules {
			flags = append(flags, r.Enabled)
		}

		return flags
	}

	assert.Equal(t, []string{codeForbidden}, f.as(t, "viewer", auth.RoleViewer).query(t, deleteMany, map[string]any{varIDs: ids}, nil))

	var disabled struct {
		SetRulesEnabled payload `json:"setRulesEnabled"`
	}
	require.Empty(t, f.query(t, setEnabled, map[string]any{varIDs: ids, varEnabled: false}, &disabled))
	assert.Equal(t, payload{Processed: 2, Skipped: 1}, disabled.SetRulesEnabled)
	assert.Equal(t, []bool{false, false}, enabled())

	var deleted struct {
		DeleteRules payload `json:"deleteRules"`
	}
	require.Empty(t, f.query(t, deleteMany, map[string]any{varIDs: ids}, &deleted))
	assert.Equal(t, payload{Processed: 2, Skipped: 1}, deleted.DeleteRules)
	assert.Empty(t, enabled())
	var audited int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM audit_events WHERE action = 'rule.delete'").Scan(&audited))
	assert.Equal(t, 2, audited, "each deletion is audited")
}
