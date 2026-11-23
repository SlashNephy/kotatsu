package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
)

// engawa、lighthouse、doyouDrama は、ルールのキーワードのテストで読む作品名と枠の名前。
const (
	engawa     = "縁側の午後"
	lighthouse = "灯台の地図"
	doyouDrama = "土曜ドラマ"
)

func TestRuleKeyword(t *testing.T) {
	t.Parallel()

	const work = "作品名"

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
		{name: "a quoted title after a slot before the episode", in: "連続ドラマ「縁側の午後」　第３話「再会」", want: engawa},
		{name: "a quoted title after a slot before a mark", in: "ドラマ「みかん箱の手紙」 第2話🈑", want: "みかん箱の手紙"},
		{name: "an episode in parentheses", in: "🆞【連続テレビ小説】ブラッサム（２６）第６週「ふたり、北へ」🈖🈑🈞", want: "ブラッサム"},
		{name: "a separator before the subtitle", in: "ＧＴＯ▽ストーカー教師です", want: "GTO"},
		{name: "a scheduling mark", in: "🅊まだ結婚できない男【連日】#10🈡", want: "まだ結婚できない男"},
		{name: "a cast guide", in: "🅊🈑ヤメ検の女２　出演：賀来千香子", want: "ヤメ検の女2"},
		{name: "a subtitle enclosed in wave dashes", in: "🅊ＳＡＫＵＲＡ〜事件を聞く女〜【一挙】#1-2", want: "SAKURA"},
		{name: "a slot label before a TV anime prefix", in: "【新】華◆アニメ「漢化日記」＃１🈑　▼声の出演：チャオ・フェイフェイ", want: "漢化日記"},
		{name: "a diamond after the episode", in: "FBI: 特別捜査班 シーズン5 #10🈔◆一挙放送", want: "FBI: 特別捜査班 シーズン5"},
		{name: "a numbered installment", in: "ドキュメント７２時間　第５０回", want: "ドキュメント72時間"},
		// #2041: 放送枠の名前を除き、かぎ括弧の中を作品名にする。括弧の中のダッシュで囲んだ副題も除く。
		{name: "a slot with a quoted title and a dashed subtitle", in: "木曜ドラマ「灯台の地図　ー霧笛の夜ー」第８話", want: lighthouse},
		{name: "a dashed subtitle without a slot", in: "灯台の地図　ー霧笛の夜ー　第８話", want: lighthouse},
		{name: "a dashed subtitle with a prolonged sound mark inside", in: "灯台の地図　ーヒーローの夜ー　第８話", want: lighthouse},
		{name: "a prolonged sound mark inside a word is not a subtitle", in: "ヒーローの灯台　第８話", want: "ヒーローの灯台"},
		{name: "a slot without quotes", in: "連続ドラマＷ　灯台の地図　＃１", want: lighthouse},
		{name: "a slot with a space before the quoted title", in: "木曜ドラマ　「灯台の地図」　第８話", want: lighthouse},
		{name: "a slot with a space before the double-quoted title", in: "木曜ドラマ　『灯台の地図』　第８話", want: lighthouse},
		{name: "a slot of period dramas", in: "おはよう！時代劇　灯台の地図１２　＃８", want: "灯台の地図12"},
		{name: "a subtitle in full-width tildes", in: "灯台の地図～霧笛の夜～　第９話", want: lighthouse},
		{name: "a long vowel inside a word is not a subtitle", in: "ゴーストライター２（１１）", want: "ゴーストライター2"},
		{name: "a slot alone is kept", in: "土曜ドラマ　＃３", want: doyouDrama},
		{name: "wave dashes followed by more of the title are kept", in: "２０２６　〜全英への道〜ミズノオープン　３日目", want: "2026 ~全英への道~ミズノオープン 3日目"},
		{name: "a one-character title keeps its wave dashes", in: "声〜あなたと読売テレビ〜🈖🈑", want: "声~あなたと読売テレビ~"},
		{name: "wave dashes inside quotes are kept", in: "🈑ブラブラ「男ブラめし〜謎のグルメ〜」　＃７", want: "ブラブラ「男ブラめし~謎のグルメ~」"},
		{name: "a quoted title after a slot before a numbered installment", in: "連続ドラマ「縁側の午後」　第１回「朝の光」", want: engawa},
		{name: "an episode with a colon", in: "Sample Show Episode: 5", want: "Sample Show"},
		{name: "only enclosed marks", in: "🈑🈞", want: ""},
		{name: "falls back to the whole name", in: "「名探偵コナン」", want: "「名探偵コナン」"},
		// 作品名が読めないときは、印を空白に置き換えず、印で区切った最初の部分にする (#1984)。
		{name: "falls back to the part before the mark", in: "<アニメイズム>🈟#3", want: "<アニメイズム>"},
		{name: "falls back without collapsing spaces", in: "<アニメ　　イズム>🈟#3", want: "<アニメ  イズム>"},
		// 表の囲み文字は、どれも作品名の終わりとする (#1984)。
		{name: "a trailing performer mark", in: "作品名🈦　#2", want: work},
		{name: "a trailing 120P mark", in: "作品名🆤", want: work},
		{name: "a trailing parental lock mark", in: "作品名⚿　#2", want: work},
		{name: "a trailing bracketed wide mark", in: "作品名[W][P]　#2", want: work},
		{name: "a trailing bracketed data mark", in: "作品名(d) #2", want: work},
		// 前編と後編は、番組名の一部とする。
		{name: "the first part is kept", in: "作品名[前]　#2", want: "作品名[前]"},
		// 囲み文字の中身は、大文字と小文字を区別する。
		{name: "an uppercase D is not a mark", in: "作品名(D) #2", want: "作品名(D)"},
		// 頭の印は、中身が表の語なら長さによらず除く (#1984)。
		{name: "a leading 120P mark", in: "[120P]作品名　#1", want: work},
		{name: "a leading Hi-Res mark", in: "[Hi-Res]作品名　#1", want: work},
		{name: "a leading second screen mark", in: "[2ndScr]作品名　#1", want: work},
		{name: "a leading lossless mark", in: "[Lossless]作品名　#1", want: work},
		{name: "a leading 22.2 mark", in: "[22.2]作品名　#1", want: work},
		{name: "a leading parenthesized mark", in: "(字)作品名　#1", want: work},
		{name: "a leading parenthesized scheduling mark", in: "【新】(日)作品名　#1", want: work},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.RuleKeyword(tt.in)
			assert.Equal(t, tt.want, got)
			if got == "" {
				return
			}
			assert.True(t, keywordMatches(t, got, tt.in), "the keyword %q matches the program", got)
		})
	}
}

// keywordMatches は、番組名のキーワード keyword だけのルールが、番組名 name の番組に一致するかを返す。
func keywordMatches(t *testing.T, keyword, name string) bool {
	t.Helper()
	matcher, err := match.Compile(&match.Conditions{
		Keywords:           []*match.Keyword{{Text: keyword, Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldName}}},
		KeywordMatch:       match.KeywordMatchAll,
		ExcludeKeywords:    []*match.Keyword{},
		ChannelTypes:       []string{},
		ServiceIDs:         []int64{},
		ExcludeServiceIDs:  []int64{},
		Genres:             []*match.Genre{},
		ExcludeGenres:      []*match.Genre{},
		TimeRanges:         []*match.TimeRange{},
		MinDurationSeconds: nil,
		MaxDurationSeconds: nil,
		FreeOnly:           false,
		Marks:              []string{},
		ExcludeMarks:       []string{},
	}, time.UTC)
	require.NoError(t, err)

	return matcher.Match(match.NewProgram(&match.ProgramInput{
		ServiceID: 1, SharedFrom: 0, ChannelType: "GR", Name: name, Description: "", Extended: map[string]string{}, Genres: []*match.Genre{},
		StartAt: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), Duration: 30 * time.Minute, IsFree: true,
	}))
}

// TestRuleKeywordPrefixes は、キーワードを空白で区切った語の先頭からの部分を、長い順に返し、1 文字のものは返さないことを確認する (#2041)。
func TestRuleKeywordPrefixes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		keyword string
		want    []string
	}{
		{name: "語が 3 つ", keyword: "灯台の地図 嵐の夜 その二", want: []string{"灯台の地図 嵐の夜", lighthouse}},
		{name: "語が 1 つ", keyword: lighthouse, want: []string{}},
		{name: "1 文字の語は候補にしない", keyword: "A 嵐の夜", want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, domain.RuleKeywordPrefixes(tt.keyword))
		})
	}
}

func TestHasEpisodeNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "hash", in: "灯台の地図　＃０５", want: true},
		{name: "kanji episode", in: "灯台の地図　第八話", want: true},
		{name: "parenthesized number", in: "ブラッサム（２６）", want: true},
		{name: "a parenthesized year is not an episode", in: "ゲゲゲの鬼太郎(1971)", want: false},
		{name: "no episode", in: "ＭＵＳＩＣ　ＳＴＡＴＩＯＮ", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, domain.HasEpisodeNumber(tt.in))
		})
	}
}
