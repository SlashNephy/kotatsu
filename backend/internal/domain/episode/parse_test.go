package episode_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain/episode"
)

// テストで繰り返し使う番組名。
const (
	work        = "灯台の地図"
	ep3         = work + " #3"
	ep3Subtitle = work + " #3「霧笛」"
	ep3Guest    = work + " #3 ▽ゲスト登場"
	ep4         = work + " #4"
	harborEp3   = work + " 霧の港編 #3"
	remasterEp3 = work + "リマスター #3"
	news        = "夜のニュース"
)

// longSummary は、概要の指紋に使える長さの概要。
const longSummary = "主人公の少年は、古い灯台で見つけた地図を手がかりに、仲間と一緒に島の奥へ向かう。そこで待っていたのは、思いもよらない再会だった。"

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  *episode.Source
		want *episode.Episode
	}{
		{
			name: "a season, an episode, a subtitle and trailing frames",
			src:  &episode.Source{Name: "🈟アニメ・" + work + "　第２期　＃３「始まりの朝」【アニメの時間】"},
			want: &episode.Episode{
				Series: work + "第2期", Work: work, Arc: "第2期", HasSeason: true, Core: work,
				Number: 3, Subtitle: "始まりの朝", Key: "アニメ・" + work + "第2期#3「始まりの朝」【アニメの時間】",
			},
		},
		{
			name: "a leading frame name",
			src:  &episode.Source{Name: "＜深夜アニメ枠＞" + work + "　＃５"},
			want: &episode.Episode{Series: work, Work: work, Core: work, Number: 5, Key: "<深夜アニメ枠>" + work + "#5"},
		},
		{
			name: "a quoted work followed by an arc",
			src:  &episode.Source{Name: "『" + work + "』霧の港編　第３話"},
			want: &episode.Episode{
				Series: work + "霧の港編", Work: work, Arc: "霧の港編", Core: work, Number: 3,
				Key: "『" + work + "』霧の港編第3話",
			},
		},
		{
			name: "a trailing arc",
			src:  &episode.Source{Name: work + " 霧の港編 #4"},
			want: &episode.Episode{Series: work + "霧の港編", Work: work, Arc: "霧の港編", Core: work, Number: 4, Key: work + "霧の港編#4"},
		},
		{
			name: "a recap is not an arc",
			src:  &episode.Source{Name: work + " 総集編"},
			want: &episode.Episode{Series: work + "総集編", Work: work + "総集編", Core: work, Key: work + "総集編", Special: true},
		},
		{
			name: "a subtitle without an episode",
			src:  &episode.Source{Name: work + "「始まりの朝」"},
			want: &episode.Episode{Series: work, Work: work, Core: work, Subtitle: "始まりの朝", Key: work + "「始まりの朝」"},
		},
		{
			name: "too many digits are not an episode",
			src:  &episode.Source{Name: work + " #12345"},
			want: &episode.Episode{Series: work + "#12345", Work: work + "#12345", Core: "#12345", Key: work + "#12345"},
		},
		{
			name: "an English episode",
			src:  &episode.Source{Name: work + " file.7"},
			want: &episode.Episode{Series: work, Work: work, Core: work, Number: 7, Key: work + "file.7"},
		},
		{
			name: "an English word followed by a year is not an episode",
			src:  &episode.Source{Name: work + " case2026年"},
			want: &episode.Episode{Series: work + "case2026年", Work: work + "case2026年", Core: "case2026年", Key: work + "case2026年"},
		},
		{
			name: "the series descriptor comes first",
			src:  &episode.Source{Name: work, SeriesEpisode: new(int32(7))},
			want: &episode.Episode{Series: work, Work: work, Core: work, Number: 7, Key: work},
		},
		{
			name: "a kanji season",
			src:  &episode.Source{Name: work + " 第二期"},
			want: &episode.Episode{Series: work + "第2期", Work: work, Arc: "第2期", HasSeason: true, Core: work, Key: work + "第二期"},
		},
		{
			name: "a season followed by other words is kept in the name",
			src:  &episode.Source{Name: work + " 第2期 特別版"},
			want: &episode.Episode{Series: work + "第2期特別版", Work: work + "第2期特別版", Core: work, Key: work + "第2期特別版", Special: true},
		},
		{
			name: "a quoted subtitle in the summary",
			src:  &episode.Source{Name: work, Description: "第十八話「決戦の灯」　嵐の夜、灯台に明かりがともる。"},
			want: &episode.Episode{Series: work, Work: work, Core: work, Number: 18, Subtitle: "決戦の灯", Key: work},
		},
		{
			name: "kanji digits without a positional",
			src:  &episode.Source{Name: work, Description: "第一〇話「霧笛」"},
			want: &episode.Episode{Series: work, Work: work, Core: work, Number: 10, Subtitle: "霧笛", Key: work},
		},
		{
			name: "a plain episode at the start of a line in the summary",
			src:  &episode.Source{Name: work, Description: "【＃４】 雨の日の約束▽出演者"},
			want: &episode.Episode{Series: work, Work: work, Core: work, Number: 4, Subtitle: "雨の日の約束", UncertainSubtitle: true, Key: work},
		},
		{
			name: "an episode in running text is not read",
			src:  &episode.Source{Name: work, Description: "今年#1ヒットの曲が流れる。第3話から登場した仲間も。"},
			want: &episode.Episode{Series: work, Work: work, Core: work, Key: work},
		},
		{
			name: "the previous episode is not read",
			src:  &episode.Source{Name: work, Description: "前回の#3「霧笛」では、少年が島へ渡った。"},
			want: &episode.Episode{Series: work, Work: work, Core: work, Key: work},
		},
		{
			name: "an uncertain subtitle is replaced by the summary",
			src:  &episode.Source{Name: ep3Guest, Description: "#3「霧笛」"},
			want: &episode.Episode{Series: work, Work: work, Core: work, Number: 3, Subtitle: "霧笛", UncertainSubtitle: true, Key: work + "#3▽ゲスト登場"},
		},
		{
			name: "a summary with another episode does not replace the subtitle",
			src:  &episode.Source{Name: ep3Guest, Description: "#4「霧笛」"},
			want: &episode.Episode{Series: work, Work: work, Core: work, Number: 3, Subtitle: "▽ゲスト登場", UncertainSubtitle: true, Key: work + "#3▽ゲスト登場"},
		},
		{
			name: "the extended text is read when the summary is empty",
			src:  &episode.Source{Name: work, Extended: "番組内容\n#6「潮騒」\n"},
			want: &episode.Episode{Series: work, Work: work, Core: work, Number: 6, Subtitle: "潮騒", Key: work},
		},
		{
			name: "a long summary is a fingerprint",
			src:  &episode.Source{Name: work + " #2", Description: longSummary},
			want: &episode.Episode{
				Series: work, Work: work, Core: work, Number: 2, Key: work + "#2",
				Fingerprint: "主人公の少年は、古い灯台で見つけた地図を手がかりに、仲間と一緒に島の奥へ向かう。そこで待っていたのは、思いもよらない再会だった。",
			},
		},
		{
			name: "SP as a word is a special",
			src:  &episode.Source{Name: work + " SP"},
			want: &episode.Episode{Series: work + "sp", Work: work + "sp", Core: work, Key: work + "sp", Special: true},
		},
		{
			name: "SP inside a word is not a special",
			src:  &episode.Source{Name: work + " SPY"},
			want: &episode.Episode{Series: work + "spy", Work: work + "spy", Core: work, Key: work + "spy"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, episode.Parse(tt.src))
		})
	}
}

func TestCaptioned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "an enclosed mark", in: work + " #3🈑", want: true},
		{name: "a bracketed mark", in: work + " #3【字】", want: true},
		{name: "a full-width bracket", in: work + " #3［字］", want: true},
		{name: "no mark", in: ep3, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, episode.Captioned(tt.in))
		})
	}
}
