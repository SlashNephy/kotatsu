package graph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
)

// lighthouse は、ルールのキーワードのテストで読む作品名。
const lighthouse = "灯台の地図"

// stormEpisode は、副題の付いた回の番組名。stormKeyword は、その番組名をほかの回と突き合わせずに読んだキーワード。
const (
	stormEpisode = "灯台の地図　嵐の夜　＃０５"
	stormKeyword = "灯台の地図 嵐の夜"
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
		{name: "ほかの回は別の副題", target: stormEpisode, others: []string{"灯台の地図　霧の朝　＃０６"}, want: lighthouse},
		{name: "ほかの回に副題がない", target: stormEpisode, others: []string{"灯台の地図　＃０６"}, want: lighthouse},
		{name: "副題に空白が入る", target: "灯台の地図　嵐の夜　その二　＃０５", others: []string{"灯台の地図　霧の朝　＃０６"}, want: lighthouse},
		{name: "ほかの回の頭に印がある", target: stormEpisode, others: []string{"[新]灯台の地図　霧の朝　＃０１"}, want: lighthouse},
		{name: "どの回にも枠の名前が付く", target: "連続ドラマＷ　灯台の地図　嵐の夜　＃０５", others: []string{"連続ドラマＷ　灯台の地図　霧の朝　＃０６"}, want: lighthouse},
		{name: "作品名の後ろが区切りの記号で続く", target: "灯台の地図：　嵐の夜　＃０５", others: []string{"灯台の地図：　霧の朝　＃０６"}, want: lighthouse},
		{name: "区切りの記号の前後に空白がある", target: "灯台の地図　－　嵐の夜　＃０５", others: []string{"灯台の地図　－　霧の朝　＃０６"}, want: lighthouse},
		{name: "作品名に続く文字が区切りでなければ、別の作品とみなす", target: stormEpisode, others: []string{"灯台の地図帳　霧の朝　＃０６"}, want: stormKeyword},
		{name: "長く一致する部分を作品名にする", target: "灯台の地図　嵐の夜　副題Ａ　＃０５", others: []string{"灯台の地図　嵐の夜　副題Ｂ　＃０６"}, want: stormKeyword},
		{name: "ほかの回がなければ、今の読み方のまま", target: stormEpisode, others: []string{}, want: stormKeyword},
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
