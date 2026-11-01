package watch_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/usecase/watch"
)

func ruleInput(name string) *watch.RuleInput {
	return &watch.RuleInput{
		Name:    name,
		Enabled: true,
		Conditions: &match.Conditions{
			Keywords: []*match.Keyword{{Text: "テスト番組", Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldName}}},
		},
	}
}

// TestCreateRuleValidates は、視聴ルールの名前と条件を、録画のルールと同じく検証することを確かめる。
func TestCreateRuleValidates(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	tests := []struct {
		name    string
		modify  func(in *watch.RuleInput)
		wantErr bool
	}{
		{name: "正しい内容", modify: func(*watch.RuleInput) {}, wantErr: false},
		{name: "空の名前", modify: func(in *watch.RuleInput) { in.Name = " " }, wantErr: true},
		{name: "長すぎる名前", modify: func(in *watch.RuleInput) { in.Name = strings.Repeat("あ", 201) }, wantErr: true},
		{name: "条件がない", modify: func(in *watch.RuleInput) { in.Conditions = nil }, wantErr: true},
		{name: "正規表現の誤り", modify: func(in *watch.RuleInput) {
			in.Conditions.Keywords[0].Text, in.Conditions.Keywords[0].Regexp = "(", true
		}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := ruleInput("視聴ルール")
			tt.modify(in)
			_, err := watch.CreateRule(t.Context(), f.pool, f.clock, f.users["alice"], in)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreateRuleLimit は、1 ユーザーあたり MaxRulesPerUser 個までしか視聴ルールを作れず、上限はユーザーごとであることを確かめる。
func TestCreateRuleLimit(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	for i := range watch.MaxRulesPerUser {
		_, err := watch.CreateRule(t.Context(), f.pool, f.clock, f.users["alice"], ruleInput(fmt.Sprintf("ルール %d", i)))
		require.NoError(t, err)
	}
	_, err := watch.CreateRule(t.Context(), f.pool, f.clock, f.users["alice"], ruleInput("上限を超える"))
	require.ErrorIs(t, err, watch.ErrTooManyRules)
	_, err = watch.CreateRule(t.Context(), f.pool, f.clock, f.users["bob"], ruleInput("ほかのユーザー"))
	require.NoError(t, err)
}

// TestRulesAreOwnedByUser は、ほかのユーザーの視聴ルールは、見つからないものとして扱い、読むことも変えることもできないことを確かめる。
func TestRulesAreOwnedByUser(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	alice, bob := f.users["alice"], f.users["bob"]
	created, err := watch.CreateRule(t.Context(), f.pool, f.clock, alice, ruleInput("alice のルール"))
	require.NoError(t, err)

	tests := []struct {
		name string
		call func() error
	}{
		{name: "読む", call: func() error {
			_, err := watch.GetRule(t.Context(), f.pool, bob, created.ID)

			return err
		}},
		{name: "変える", call: func() error {
			_, err := watch.UpdateRule(t.Context(), f.pool, f.clock, bob, created.ID, ruleInput("乗っ取り"))

			return err
		}},
		{name: "無効にする", call: func() error {
			_, err := watch.SetRuleEnabled(t.Context(), f.pool, f.clock, bob, created.ID, false)

			return err
		}},
		{name: "消す", call: func() error { return watch.DeleteRule(t.Context(), f.pool, f.clock, bob, created.ID) }},
	}
	for _, tt := range tests {
		require.ErrorIs(t, tt.call(), watch.ErrRuleNotFound, tt.name)
	}

	rules, err := watch.ListRules(t.Context(), f.pool, bob)
	require.NoError(t, err)
	assert.Empty(t, rules)
	got, err := watch.GetRule(t.Context(), f.pool, alice, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created, got)
}
