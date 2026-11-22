package graph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
)

// TestProgramRuleKeyword は、番組からルールを作るときのキーワードから、同じサービスのほかの回と突き合わせて、回ごとに変わる副題を除くことを確かめる (#2041)。
func TestProgramRuleKeyword(t *testing.T) {
	t.Parallel()

	const target = 6
	tests := []struct {
		name   string
		target string
		others []string
		want   string
	}{
		{name: "ほかの回は別の副題", target: "灯台の地図　嵐の夜　＃０５", others: []string{"灯台の地図　霧の朝　＃０６"}, want: "灯台の地図"},
		{name: "ほかの回に副題がない", target: "灯台の地図　嵐の夜　＃０５", others: []string{"灯台の地図　＃０６"}, want: "灯台の地図"},
		{name: "副題に空白が入る", target: "灯台の地図　嵐の夜　その二　＃０５", others: []string{"灯台の地図　霧の朝　＃０６"}, want: "灯台の地図"},
		{name: "ほかの回がなければ、今の読み方のまま", target: "灯台の地図　嵐の夜　＃０５", others: []string{}, want: "灯台の地図 嵐の夜"},
		{name: "同じ名前の回だけなら、作品名に空白が入るとみなす", target: "ＭＵＳＩＣ　ＢＬＩＳＳ　＃２６", others: []string{"ＭＵＳＩＣ　ＢＬＩＳＳ　＃２７"}, want: "MUSIC BLISS"},
		{name: "話数のない別の番組とは突き合わせない", target: "ＭＵＳＩＣ　ＢＬＩＳＳ　＃２６", others: []string{"ＭＵＳＩＣ　ＳＴＡＴＩＯＮ"}, want: "MUSIC BLISS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			f.syncEPG(t)
			rename := func(eventID int, name string) {
				_, err := f.pool.Exec(t.Context(), `UPDATE programs SET name = $3, name_normalized = $4 WHERE service_id = $1 AND event_id = $2`,
					faketuner.ServiceItemID(grNetworkID, grServiceID), eventID, name, domain.Normalize(name))
				require.NoError(t, err)
			}
			rename(target, tt.target)
			for i, other := range tt.others {
				rename(target+1+i, other)
			}

			var got struct {
				Node struct {
					RuleKeyword string `json:"ruleKeyword"`
				} `json:"node"`
			}
			require.Empty(t, f.query(t, `query($id: ID!) { node(id: $id) { ... on Program { ruleKeyword } } }`, map[string]any{"id": programID(target)}, &got))
			assert.Equal(t, tt.want, got.Node.RuleKeyword)
		})
	}
}
