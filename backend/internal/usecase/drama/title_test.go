package drama_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/drama"
)

// テストで何度も使う作品名。
const (
	titleMoPapa   = "もうパパ!"
	titleKyumei   = "救命病棟24時"
	titleHagure2  = "はぐれ刑事純情派2"
	titleBanshaku = "晩酌の流儀5〜秋冬篇〜"
	titleSukeban  = "スケバン刑事II"
)

func TestWorkTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "hash episode and tagline", input: "もうパパ！　＃７　【心底ムカついています】脚本・吉田恵里香×原菜乃華 堤真一🈑🈖", want: titleMoPapa},
		{name: "tagline before the episode", input: "救命病棟２４時【江口洋介×松嶋菜々子！　プライドと情熱の果てに】　＃０６🈑🈖", want: titleKyumei},
		{name: "zero padded hash", input: "ハイド＆シーク　０秒前の彼女#07「死んだはずの恋人の謎が明らかに！」🈖🈑🈓", want: "ハイド&シーク 0秒前の彼女"},
		{name: "parenthesized episode", input: "豊臣兄弟！（４４）ある家臣の死🈖🈑", want: "豊臣兄弟!"},
		{name: "parenthesized episode and subtitle", input: "刑事フォイル（６）「兵役拒否」（後編）🈔🈑", want: "刑事フォイル"},
		{name: "slot in corner brackets and week", input: "【連続テレビ小説】ブラッサム　第７週「あばく、書く」🈑🈞", want: "ブラッサム"},
		{name: "slot and kanji episode", input: "中国ドラマ「Ｌ＆Ｐ」　第３５話「運命を変える」", want: "L&P"},
		{name: "retro slot", input: "懐ドラ「はぐれ刑事純情派２」　「安浦刑事 老警察犬を特訓す」🈞", want: titleHagure2},
		{name: "morning period drama slot", input: "おはよう！時代劇　暴れん坊将軍１２　＃４🈑🈞", want: "暴れん坊将軍12"},
		{name: "roman numeral and star", input: "スケバン刑事Ⅱ　少女鉄仮面伝説　★第２６話", want: "スケバン刑事II 少女鉄仮面伝説"},
		{name: "wave subtitle and kanji episode", input: banshakuEpisode7, want: titleBanshaku},
		{name: "enclosed new mark", input: "🈟TOKYO MER〜走る緊急救命室〜 #1🈑", want: "TOKYO MER〜走る緊急救命室〜"},
		{name: "mystery slot", input: "日曜ミステリー『神楽坂署生活安全課３迷刑事エイジとロク』舘ひろし、モト冬樹🈞🈑🈖", want: "神楽坂署生活安全課3迷刑事エイジとロク"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, drama.WorkTitle(tt.input))
		})
	}
}

func TestSearchQueries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  []string
	}{
		{name: "plain title", title: titleMoPapa, want: []string{titleMoPapa}},
		{name: "wave subtitle and sequel number", title: titleBanshaku, want: []string{titleBanshaku, "晩酌の流儀5", "晩酌の流儀"}},
		{name: "sequel number", title: titleHagure2, want: []string{titleHagure2, "はぐれ刑事純情派"}},
		{name: "roman numeral", title: titleSukeban, want: []string{titleSukeban, "スケバン刑事"}},
		{name: "number that is part of the title", title: titleKyumei, want: []string{titleKyumei}},
		{name: "too short", title: "X", want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, drama.SearchQueries(tt.title))
		})
	}
}

func TestHeadQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  string
	}{
		{name: "subtitle after a space", title: "南田南弁護士は天才肌 嘘をつくのは悪いことです!", want: "南田南弁護士は天才肌"},
		{name: "no space", title: titleMoPapa, want: ""},
		{name: "head too short", title: "X 何か", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, drama.HeadQuery(tt.title))
		})
	}
}

func TestTitleKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b string
	}{
		{name: "spaces and wave dashes", a: "TOKYO MER 〜走る緊急救命室〜", b: "TOKYO MER〜走る緊急救命室〜"},
		{name: "fullwidth ampersand", a: "ハイド＆シーク 0秒前の彼女", b: "ハイド&シーク　０秒前の彼女"},
		{name: "roman numeral", a: "スケバン刑事Ⅱ", b: titleSukeban},
		{name: "fullwidth exclamation", a: "もうパパ！", b: titleMoPapa},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, drama.TitleKey(tt.a), drama.TitleKey(tt.b))
		})
	}
}

func TestExcluded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "promotion", input: "【ドラマ】夜ドラ「青の花　器の森」ＰＲ　（第４週）", want: true},
		{name: "rerun promotion", input: "連続テレビ小説「ひよっこ」再放送ＰＲ", want: true},
		{name: "highlights", input: "救命病棟２４時　あす夜９時！６話見どころ▼江口洋介×松嶋菜々子！キャリアと葛藤🈑", want: true},
		{name: "digest", input: "もうパパ！ダイジェスト", want: true},
		{name: "word containing PR", input: "PRIDE　＃３", want: false},
		{name: "episode", input: "もうパパ！　＃７", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, drama.Excluded(tt.input))
		})
	}
}

func TestEpisodeNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  int32
	}{
		{name: "hash", input: "もうパパ！　＃７", want: 7},
		{name: "zero padded hash", input: "救命病棟２４時　＃０６🈑🈖", want: 6},
		{name: "kanji", input: "晩酌の流儀５〜秋冬篇〜第７話「ブーリンチー」", want: 7},
		{name: "parenthesized", input: "ゴーストライター２（１０）「時間旅行のゴースト　その３」", want: 10},
		{name: "year is not an episode", input: "名作ドラマ（１９７１）", want: 0},
		{name: "week is not an episode", input: "【連続テレビ小説】ブラッサム　第７週「あばく、書く」", want: 0},
		{name: "no episode", input: "ライターズ！", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, drama.EpisodeNumber(tt.input))
		})
	}
}
