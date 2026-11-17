package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

func TestWorkTitle(t *testing.T) {
	t.Parallel()

	const work = "作品名"

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain title with episode", in: "豊臣兄弟! (7)", want: "豊臣兄弟!"},
		{name: "enclosed marks and episode", in: "🈑豊臣兄弟!#7「サブタイトル」🈞", want: "豊臣兄弟!"},
		{name: "episode in kanji", in: "作品名 第七話", want: work},
		{name: "slot name with quoted title", in: "土曜ドラマ「作品名」 第3話", want: work},
		{name: "slot name with full-width number", in: "ドラマ２４「作品名」 第4話", want: work},
		{name: "slot name with space", in: "日曜劇場 作品名 #5", want: work},
		{name: "drama prefix", in: "ドラマ「作品名」 第2話", want: work},
		{name: "morning drama", in: "連続テレビ小説 作品名 (12)", want: work},
		{name: "movie slot", in: "金曜ロードショー 映画のタイトル", want: "映画のタイトル"},
		// 話数のない番組名でも、枠の名前の後のかぎ括弧を作品名とする。
		{name: "movie with quoted title", in: "映画「はんてんの約束」", want: "はんてんの約束"},
		{name: "slot name and quote without episode", in: "土曜ドラマ「作品名」", want: work},
		{name: "numbered day-of-week slot", in: "木ドラ24「作品名」 第1話", want: work},
		{name: "WOWOW serial drama", in: "連続ドラマW「作品名」 第2話", want: work},
		{name: "serial drama", in: "連続ドラマ「縁側の午後」 第12話", want: "縁側の午後"},
		{name: "leading mark before slot name", in: "【新】土曜ドラマ「作品名」#1", want: work},
		{name: "quoted title with suffix", in: "金曜ドラマ「作品名」SP", want: work},
		// 海外ドラマや時代劇の再放送の枠も除く (#1651)。
		{name: "foreign drama slot", in: "中国ドラマ「作品名」　第３５話「サブタイトル」", want: work},
		{name: "retro drama slot", in: "懐ドラ「作品名」　「サブタイトル」🈞", want: work},
		{name: "period drama slot with space", in: "おはよう！時代劇　作品名　＃４🈑🈞", want: work},
		{name: "mystery slot", in: "日曜ミステリー『作品名』出演者🈞", want: work},
		// 枠の名前に似た作品名は、後ろに空白もかぎ括弧もないので削らない。
		{name: "title starting like a slot name", in: "ドラマチックな恋 #1", want: "ドラマチックな恋"},
		// 作品名の後ろや途中のすみ付き括弧は、外部のサービスで探すのに要らないので除く。
		{name: "trailing lenticular brackets", in: "救命病棟２４時【最終章へ…】　＃０６", want: "救命病棟24時"},
		{name: "lenticular brackets in the middle", in: "作品名【SP】完結編 #6", want: "作品名 完結編"},
		{name: "consecutive lenticular brackets", in: "作品名【前編】 【SP】完結編 #6", want: "作品名 完結編"},
		{name: "lenticular brackets in a quoted title", in: "土曜ドラマ「作品名【SP】」 #1", want: work},
		// RuleKeyword が話数とみなさない書き方の話数とサブタイトルも除く (#1929)。
		{name: "spaced number after an anime prefix", in: "アニメ　天は赤い河のほとり　２０　亀裂", want: "天は赤い河のほとり"},
		{name: "anime prefix after another mark", in: "【新】アニメ　天は赤い河のほとり　２０　亀裂", want: "天は赤い河のほとり"},
		{name: "spaced number with double spaces", in: "アニメ　作品名　　１５　サブタイトル", want: work},
		{name: "lowercase word, number and period", in: "ホテル・インヒューマンズ　request　20.🈑", want: "ホテル・インヒューマンズ"},
		{name: "lowercase word, number and period before a subtitle", in: "ホテル・インヒューマンズ request 20. サブタイトル", want: "ホテル・インヒューマンズ"},
		{name: "lowercase season, number and period", in: "作品名 season 2.", want: "作品名 season 2."},
		{name: "lowercase word glued to a number", in: "🈑アニメA・メダリスト　score08「西の強豪（前）」", want: "アニメA・メダリスト"},
		{name: "two lowercase-word episodes", in: "アニメA・メダリスト score04、score05", want: "アニメA・メダリスト"},
		{name: "operation number", in: "🈑アニメ　夜桜さんちの大作戦 第2期　作戦44　姉弟奪還作戦開始", want: "夜桜さんちの大作戦 第2期"},
		{name: "number and period before a subtitle", in: "BS11ガンダムアワー 機動戦士ガンダムSEED DESTINY 21. PHASE-21", want: "BS11ガンダムアワー 機動戦士ガンダムSEED DESTINY"},
		{name: "number and period at the end", in: "作品名 Season 2.", want: "作品名 Season 2."},
		{name: "episode without 第", in: "ギャビーのドールハウス 130話 ネコちゃんロボット(前編)", want: "ギャビーのドールハウス"},
		{name: "episode without 第 keeps a trailing hyphen of the title", in: "BLEACH 千年血戦篇-禍進譚- 49話", want: "BLEACH 千年血戦篇-禍進譚-"},
		{name: "episode range", in: "作品名 第1~6話", want: work},
		{name: "episode range without 第", in: "無脳魔女 ブレインレス・ウィッチ season1 10話〜12話", want: "無脳魔女 ブレインレス・ウィッチ season1"},
		{name: "episode without 第 before the total", in: "韓ドラ 女王の家 16話（全74話）", want: "韓ドラ 女王の家"},
		// 作品名に入る数は削らない。
		{name: "number glued to the title", in: "機動戦士ガンダム00 #5", want: "機動戦士ガンダム00"},
		{name: "number after an English word", in: "ULTRAMAN 2 #3", want: "ULTRAMAN 2"},
		{name: "season after an English word", in: "作品名 Season 3 #50", want: "作品名 Season 3"},
		{name: "capitalized season glued to a number", in: "傷だらけ聖女より報復をこめて Season2", want: "傷だらけ聖女より報復をこめて Season2"},
		{name: "capitalized part glued to a number", in: "ルパン三世 PART2", want: "ルパン三世 PART2"},
		{name: "lowercase season glued to a number", in: "相棒 season12", want: "相棒 season12"},
		{name: "sequel number glued to the title before a subtitle", in: "さすらい署長 風間昭平15 塩屋岬いわき殺人事件", want: "さすらい署長 風間昭平15 塩屋岬いわき殺人事件"},
		// アニメの前置きがなければ、空白で区切った数は続編の番号とみなす。
		{name: "spaced sequel number without an anime prefix", in: "山村美紗サスペンス 名探偵キャサリン 12 坂本龍馬殺人事件", want: "山村美紗サスペンス 名探偵キャサリン 12 坂本龍馬殺人事件"},
		{name: "spaced number at the end", in: "アニメ　タイトル　4", want: "タイトル 4"},
		{name: "total episodes", in: "作品名 全13話一挙放送", want: "作品名 全13話一挙放送"},
		{name: "episode followed by a particle", in: "作品名 1話からおさらい", want: "作品名 1話からおさらい"},
		// すみ付き括弧だけの番組名は、そのまま返す。
		{name: "lenticular brackets only", in: "【特集】", want: "【特集】"},
		// 枠の名前だけなら、そのまま返す。
		{name: "slot name only", in: "土曜ドラマ", want: "土曜ドラマ"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, domain.WorkTitle(tt.in), tt.name)
	}
}
