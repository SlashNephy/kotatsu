package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

func TestRuleKeyword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "hash episode with a subtitle", in: "ガラスの仮面　＃５「ふたりの美登利」🈞", want: "ガラスの仮面"},
		{name: "kanji episode after an enclosed mark", in: "サンダーバード🈥［ＨＤ完全版］　第２５話「魅惑のメロディー」", want: "サンダーバード"},
		{name: "kanji numeral episode", in: "FX戦士くるみちゃん　第六話", want: "FX戦士くるみちゃん"},
		{name: "kanji episode", in: "名探偵コナン　第１１５０話「消えた探偵」", want: "名探偵コナン"},
		{name: "episode without a space", in: "MASTERキートン#7-8", want: "MASTERキートン"},
		{name: "two episodes", in: "TIGER & BUNNY 2 #03・#04", want: "TIGER & BUNNY 2"},
		{name: "chapter", in: "ＢａｔｔｌｅＳｐｉｒｉｔｓ［Ｒｅ］絶界の空　CHAPTER:5 「THE MIRROR」", want: "BattleSpirits[Re]絶界の空"},
		{name: "a bracket that is part of the title", in: "Battle Spirits [Re] 絶界の空　＃5「THE MIRROR」", want: "Battle Spirits [Re] 絶界の空"},
		{name: "a parenthesized mark", in: "アドベンチャー・タイム #101(二)", want: "アドベンチャー・タイム"},
		{name: "an annotation in angle brackets", in: "アドベンチャー・タイム：サイドクエスト ＜日本初＞ #20(二)", want: "アドベンチャー・タイム:サイドクエスト"},
		{name: "a parenthesized year is kept", in: "ゲゲゲの鬼太郎(1971) #30", want: "ゲゲゲの鬼太郎(1971)"},
		{name: "a leading decoration", in: "【50周年記念】プロレスの星 アステカイザー #4", want: "プロレスの星 アステカイザー"},
		{name: "a TV anime prefix with a quoted title", in: "TVアニメ「バンドリ！ ゆめ∞みた」 #13 🈡", want: "バンドリ! ゆめ∞みた"},
		{name: "a TV anime prefix with a space", in: "アニメ　Ｂａｔｔｌｅ　Ｓｐｉｒｉｔｓ　［Ｒｅ］　絶界の空", want: "Battle Spirits [Re] 絶界の空"},
		{name: "a quoted title followed by the season", in: "『アオのハコ』Season2　#5🈑", want: "『アオのハコ』Season2"},
		{name: "a season before the episode", in: "BEASTARS 第1期：#4　君は聖杯までふやかして(日)", want: "BEASTARS 第1期"},
		{name: "a subtitle without an episode", in: "ＳＰＹ×ＦＡＭＩＬＹ「合否の行方」", want: "SPY×FAMILY"},
		{name: "a trailing mark only", in: "おねがいアイプリ　おねがいかなえチャンネル！🈑", want: "おねがいアイプリ おねがいかなえチャンネル!"},
		{name: "a leading mark", in: "[新]葬送のフリーレン　#29", want: "葬送のフリーレン"},
		{name: "no episode", in: "サイコダイバー 魔性菩薩", want: "サイコダイバー 魔性菩薩"},
		{name: "episodes separated by a comma", in: "BANANA FISH #17,18", want: "BANANA FISH"},
		{name: "a quoted title before the episode", in: "連続ドラマ「縁側の午後」　第３話「再会」", want: "連続ドラマ「縁側の午後」"},
		{name: "a subtitle before a mark", in: "ドラマ「みかん箱の手紙」 第2話🈑", want: "ドラマ「みかん箱の手紙」"},
		{name: "an episode in parentheses", in: "🆞【連続テレビ小説】ブラッサム（２６）第６週「ふたり、北へ」🈖🈑🈞", want: "ブラッサム"},
		{name: "a separator before the subtitle", in: "ＧＴＯ▽ストーカー教師です", want: "GTO"},
		{name: "a scheduling mark", in: "🅊まだ結婚できない男【連日】#10🈡", want: "まだ結婚できない男"},
		{name: "a cast guide", in: "🅊🈑ヤメ検の女２　出演：賀来千香子", want: "ヤメ検の女2"},
		{name: "a title enclosed in wave dashes", in: "🅊ＳＡＫＵＲＡ〜事件を聞く女〜【一挙】#1-2", want: "SAKURA〜事件を聞く女〜"},
		{name: "a slot label before a TV anime prefix", in: "【新】華◆アニメ「漢化日記」＃１🈑　▼声の出演：チャオ・フェイフェイ", want: "漢化日記"},
		{name: "a diamond after the episode", in: "FBI: 特別捜査班 シーズン5 #10🈔◆一挙放送", want: "FBI: 特別捜査班 シーズン5"},
		{name: "a numbered installment", in: "ドキュメント７２時間　第５０回", want: "ドキュメント72時間"},
		{name: "a quoted title before a numbered installment", in: "連続ドラマ「縁側の午後」　第１回「朝の光」", want: "連続ドラマ「縁側の午後」"},
		{name: "an episode with a colon", in: "Sample Show Episode: 5", want: "Sample Show"},
		{name: "only enclosed marks", in: "🈑🈞", want: ""},
		{name: "falls back to the whole name", in: "「名探偵コナン」", want: "「名探偵コナン」"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, domain.RuleKeyword(tt.in))
		})
	}
}
