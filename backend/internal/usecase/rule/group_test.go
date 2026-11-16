package rule_test

import (
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

// groupNames は、グループの名前を並びの順に返す。
func groupNames(groups []*rule.Group) []string {
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.Name)
	}

	return names
}

// TestCreateGroupValidatesName は、グループの名前の前後の空白を除き、空の名前、長すぎる名前、ほかのグループと同じ名前を拒むことを確かめる。
func TestCreateGroupValidatesName(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	actor := newUser(t, pool)
	clk := clocktest.NewFake(testNow())
	rules := rule.NewRules(clk, audit.NewRecorder(clk))
	_, err := rules.CreateGroup(t.Context(), pool, actor, "アニメ")
	require.NoError(t, err)

	tests := []struct {
		name     string
		input    string
		wantName string
		wantErr  error
	}{
		{name: "前後の空白を除く", input: "  ドラマ　", wantName: "ドラマ", wantErr: nil},
		{name: "100 文字まで", input: strings.Repeat("あ", 100), wantName: strings.Repeat("あ", 100), wantErr: nil},
		{name: "空白だけの名前", input: " 　", wantName: "", wantErr: rule.ErrInvalidGroup},
		{name: "101 文字", input: strings.Repeat("あ", 101), wantName: "", wantErr: rule.ErrInvalidGroup},
		{name: "ほかのグループと同じ名前", input: " アニメ ", wantName: "", wantErr: rule.ErrGroupNameTaken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			created, err := rules.CreateGroup(t.Context(), pool, actor, tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, created.Name)
		})
	}
}

// TestGroupsAreOrderedAndAudited は、グループを作った順に並べ、名前を変え、並べ直せて、そのたびに監査ログが残ることを確かめる。
func TestGroupsAreOrderedAndAudited(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	actor := newUser(t, pool)
	clk := clocktest.NewFake(testNow())
	rules := rule.NewRules(clk, audit.NewRecorder(clk))
	anime, err := rules.CreateGroup(t.Context(), pool, actor, "アニメ")
	require.NoError(t, err)
	drama, err := rules.CreateGroup(t.Context(), pool, actor, "ドラマ")
	require.NoError(t, err)
	news, err := rules.CreateGroup(t.Context(), pool, actor, "ニュース")
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1, 2}, []int{anime.Position, drama.Position, news.Position})

	renamed, err := rules.RenameGroup(t.Context(), pool, actor, drama.ID, "連続ドラマ")
	require.NoError(t, err)
	assert.Equal(t, "連続ドラマ", renamed.Name)
	_, err = rules.RenameGroup(t.Context(), pool, actor, drama.ID, "アニメ")
	require.ErrorIs(t, err, rule.ErrGroupNameTaken)
	_, err = rules.RenameGroup(t.Context(), pool, actor, uuid.New(), "どこにもない")
	require.ErrorIs(t, err, rule.ErrGroupNotFound)

	reordered, err := rules.ReorderGroups(t.Context(), pool, actor, []uuid.UUID{news.ID, anime.ID, drama.ID})
	require.NoError(t, err)
	assert.Equal(t, []string{"ニュース", "アニメ", "連続ドラマ"}, groupNames(reordered))
	listed, err := rules.ListGroups(t.Context(), pool)
	require.NoError(t, err)
	assert.Equal(t, reordered, listed)

	// 並べ直した後に足したグループは、最後に並ぶ。
	_, err = rules.CreateGroup(t.Context(), pool, actor, "スポーツ")
	require.NoError(t, err)
	listed, err = rules.ListGroups(t.Context(), pool)
	require.NoError(t, err)
	assert.Equal(t, []string{"ニュース", "アニメ", "連続ドラマ", "スポーツ"}, groupNames(listed))

	for _, action := range []string{"rule_group.create", "rule_group.rename", "rule_group.reorder"} {
		var n int
		require.NoError(t, pool.QueryRow(t.Context(),
			"SELECT count(*) FROM audit_events WHERE action = $1 AND target_type = 'rule_group' AND actor_user_id = $2",
			action, actor.UserID).Scan(&n))
		assert.Positive(t, n, action)
	}
}

// TestReorderGroupsRequiresEveryGroup は、並べ直すときに、すべてのグループを 1 回ずつ渡さなければ拒み、並びを変えないことを確かめる。
func TestReorderGroupsRequiresEveryGroup(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	actor := newUser(t, pool)
	clk := clocktest.NewFake(testNow())
	rules := rule.NewRules(clk, audit.NewRecorder(clk))
	first, err := rules.CreateGroup(t.Context(), pool, actor, "1 番目")
	require.NoError(t, err)
	second, err := rules.CreateGroup(t.Context(), pool, actor, "2 番目")
	require.NoError(t, err)

	tests := []struct {
		name string
		ids  []uuid.UUID
	}{
		{name: "一部だけ", ids: []uuid.UUID{second.ID}},
		{name: "同じグループを 2 回", ids: []uuid.UUID{second.ID, second.ID}},
		{name: "知らないグループ", ids: []uuid.UUID{second.ID, uuid.New()}},
		{name: "多すぎる", ids: []uuid.UUID{second.ID, first.ID, first.ID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := rules.ReorderGroups(t.Context(), pool, actor, tt.ids)
			require.ErrorIs(t, err, rule.ErrInvalidGroup)
			// 拒んだ並べ替えは、並びを変えない。
			listed, err := rules.ListGroups(t.Context(), pool)
			require.NoError(t, err)
			assert.Equal(t, []string{"1 番目", "2 番目"}, groupNames(listed))
		})
	}
}

// TestUpdateKeepsOrChangesGroup は、ルールを変えるときに、グループを残すか、変えるか、外すかを選べることを確かめる。
// API でグループを省いた変更 (KeepGroup) が、黙ってグループを外さないことを確かめる (#1493)。
func TestUpdateKeepsOrChangesGroup(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	actor := newUser(t, pool)
	clk := clocktest.NewFake(testNow())
	rules := rule.NewRules(clk, audit.NewRecorder(clk))
	anime, err := rules.CreateGroup(t.Context(), pool, actor, "アニメ")
	require.NoError(t, err)
	drama, err := rules.CreateGroup(t.Context(), pool, actor, "ドラマ")
	require.NoError(t, err)
	missing := uuid.New()

	tests := []struct {
		name      string
		groupID   *uuid.UUID
		keepGroup bool
		want      *uuid.UUID
		wantErr   error
	}{
		{name: "省いたら残す", groupID: nil, keepGroup: true, want: &anime.ID, wantErr: nil},
		{name: "省いたら、渡した値は見ない", groupID: &drama.ID, keepGroup: true, want: &anime.ID, wantErr: nil},
		{name: "ほかのグループに変える", groupID: &drama.ID, keepGroup: false, want: &drama.ID, wantErr: nil},
		{name: "null ならグループから外す", groupID: nil, keepGroup: false, want: nil, wantErr: nil},
		{name: "ないグループ", groupID: &missing, keepGroup: false, want: nil, wantErr: rule.ErrGroupNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := validInput()
			in.GroupID = &anime.ID
			created, err := rules.Create(t.Context(), pool, actor, in)
			require.NoError(t, err)
			require.Equal(t, &anime.ID, created.GroupID)

			in = validInput()
			in.GroupID, in.KeepGroup = tt.groupID, tt.keepGroup
			updated, err := rules.Update(t.Context(), pool, actor, created.ID, in)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, updated.GroupID)
		})
	}

	// 作るときに、ないグループを渡すと拒む。
	in := validInput()
	in.GroupID = &missing
	_, err = rules.Create(t.Context(), pool, actor, in)
	require.ErrorIs(t, err, rule.ErrGroupNotFound)
}

// TestSetGroupMovesRulesWithoutTouchingUpdatedAt は、ルールをまとめてグループに移し、見つからないルールを飛ばして数え、
// ルールの変えた日を変えないこと、ルールごとに監査ログを残すことを確かめる。
func TestSetGroupMovesRulesWithoutTouchingUpdatedAt(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	actor := newUser(t, pool)
	clk := clocktest.NewFake(testNow())
	rules := rule.NewRules(clk, audit.NewRecorder(clk))
	anime, err := rules.CreateGroup(t.Context(), pool, actor, "アニメ")
	require.NoError(t, err)
	first, err := rules.Create(t.Context(), pool, actor, validInput())
	require.NoError(t, err)
	second, err := rules.Create(t.Context(), pool, actor, validInput())
	require.NoError(t, err)
	clk.Advance(time.Hour)

	result, err := rules.SetGroup(t.Context(), pool, actor, []uuid.UUID{first.ID, second.ID, uuid.New()}, &anime.ID)
	require.NoError(t, err)
	assert.Equal(t, &rule.SetGroupResult{Processed: 2, Skipped: 1}, result)
	for _, ruleID := range []uuid.UUID{first.ID, second.ID} {
		moved, err := rules.Get(t.Context(), pool, ruleID)
		require.NoError(t, err)
		assert.Equal(t, &anime.ID, moved.GroupID)
		assert.Equal(t, first.UpdatedAt, moved.UpdatedAt)
	}
	var n int
	require.NoError(t, pool.QueryRow(t.Context(),
		"SELECT count(*) FROM audit_events WHERE action = 'rule.set_group' AND summary->>'group' = 'アニメ'").Scan(&n))
	assert.Equal(t, 2, n)

	result, err = rules.SetGroup(t.Context(), pool, actor, []uuid.UUID{first.ID}, nil)
	require.NoError(t, err)
	assert.Equal(t, &rule.SetGroupResult{Processed: 1, Skipped: 0}, result)
	moved, err := rules.Get(t.Context(), pool, first.ID)
	require.NoError(t, err)
	assert.Nil(t, moved.GroupID)

	missing := uuid.New()
	_, err = rules.SetGroup(t.Context(), pool, actor, []uuid.UUID{first.ID}, &missing)
	require.ErrorIs(t, err, rule.ErrGroupNotFound)
}

// TestDeleteGroupReturnsRulesToOthers は、グループを消しても中のルールは消さず、どのグループにも入らないルールに戻し、
// 戻したルールの数を監査ログに残すことを確かめる。
func TestDeleteGroupReturnsRulesToOthers(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	actor := newUser(t, pool)
	clk := clocktest.NewFake(testNow())
	rules := rule.NewRules(clk, audit.NewRecorder(clk))
	anime, err := rules.CreateGroup(t.Context(), pool, actor, "アニメ")
	require.NoError(t, err)
	in := validInput()
	in.GroupID = &anime.ID
	grouped, err := rules.Create(t.Context(), pool, actor, in)
	require.NoError(t, err)

	require.NoError(t, rules.DeleteGroup(t.Context(), pool, actor, anime.ID))
	kept, err := rules.Get(t.Context(), pool, grouped.ID)
	require.NoError(t, err)
	assert.Nil(t, kept.GroupID)
	_, err = rules.GetGroup(t.Context(), pool, anime.ID)
	require.ErrorIs(t, err, rule.ErrGroupNotFound)
	require.ErrorIs(t, rules.DeleteGroup(t.Context(), pool, actor, anime.ID), rule.ErrGroupNotFound)

	var count string
	require.NoError(t, pool.QueryRow(t.Context(),
		"SELECT summary->>'rules' FROM audit_events WHERE action = 'rule_group.delete' AND target_id = $1",
		anime.ID.String()).Scan(&count))
	assert.Equal(t, "1", count)
}
