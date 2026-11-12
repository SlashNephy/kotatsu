package episode_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain/episode"
)

func TestSame(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b *episode.Source
		want bool
	}{
		{
			name: "one station writes the subtitle",
			a:    &episode.Source{Name: ep3Subtitle}, b: &episode.Source{Name: ep3},
			want: true,
		},
		{
			name: "different episodes",
			a:    &episode.Source{Name: ep3}, b: &episode.Source{Name: ep4},
			want: false,
		},
		{
			name: "one station writes the arc",
			a:    &episode.Source{Name: harborEp3}, b: &episode.Source{Name: ep3},
			want: true,
		},
		{
			name: "different arcs",
			a:    &episode.Source{Name: harborEp3}, b: &episode.Source{Name: work + " 星の丘編 #3"},
			want: false,
		},
		{
			// 期を書かない番組名は 1 期の回かもしれないので、同じとみない。
			name: "one station writes the season",
			a:    &episode.Source{Name: work + " 第2期 #3"}, b: &episode.Source{Name: ep3},
			want: false,
		},
		{
			name: "a frame name before the work",
			a:    &episode.Source{Name: "アニメ・" + work + " #3「霧笛」"}, b: &episode.Source{Name: ep3Subtitle},
			want: true,
		},
		{
			name: "only the summaries match",
			a:    &episode.Source{Name: ep3, Description: longSummary},
			b:    &episode.Source{Name: remasterEp3, Description: longSummary},
			want: true,
		},
		{
			name: "different series without a matching summary",
			a:    &episode.Source{Name: ep3}, b: &episode.Source{Name: remasterEp3},
			want: false,
		},
		{
			// 曜日ごとの枠は同じ番組紹介を持つので、シリーズ名が違えば、概要が一致しても同じ回とみない。
			name: "weekday slots with the same summary",
			a:    &episode.Source{Name: work + " 月曜 #4", Description: longSummary},
			b:    &episode.Source{Name: work + " 火曜 #4", Description: longSummary},
			want: false,
		},
		{
			// 期の数字が違う別シリーズが、同じ番組紹介を持つことがある。
			name: "series with different numbers and the same summary",
			a:    &episode.Source{Name: work + "2 #4", Description: longSummary},
			b:    &episode.Source{Name: work + "2リマスター3 #4", Description: longSummary},
			want: false,
		},
		{
			name: "conflicting subtitles",
			a:    &episode.Source{Name: ep3Subtitle}, b: &episode.Source{Name: work + " #3「潮騒」"},
			want: false,
		},
		{
			name: "an uncertain subtitle conflicts",
			a:    &episode.Source{Name: ep3Subtitle}, b: &episode.Source{Name: ep3Guest},
			want: true,
		},
		{
			name: "the core is in the other series",
			a:    &episode.Source{Name: ep3Subtitle}, b: &episode.Source{Name: "夜の" + work + "劇場 #3「霧笛」"},
			want: true,
		},
		{
			name: "a short core is not relaxed",
			a:    &episode.Source{Name: "灯台 #3「霧笛」"}, b: &episode.Source{Name: "夜の灯台劇場 #3「霧笛」"},
			want: false,
		},
		{
			name: "anime episodes decide first",
			a:    &episode.Source{Name: ep3, Anime: &episode.Anime{TID: 100, Count: 3}},
			b:    &episode.Source{Name: work, Anime: &episode.Anime{TID: 100, Count: 3}},
			want: true,
		},
		{
			name: "different anime works",
			a:    &episode.Source{Name: ep3, Anime: &episode.Anime{TID: 100, Count: 3}},
			b:    &episode.Source{Name: ep3, Anime: &episode.Anime{TID: 200, Count: 3}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a, b := episode.Parse(tt.a), episode.Parse(tt.b)
			assert.Equal(t, tt.want, episode.Same(a, b))
			assert.Equal(t, tt.want, episode.Same(b, a))
		})
	}
}

func TestSuppressWeeklySummaries(t *testing.T) {
	t.Parallel()

	// 話数の数字だけを差し替える紹介文。
	weekly := func(n string) string {
		return work + "は、霧の島を舞台にした冒険の物語。今週は第" + n + "話をお届けします。毎週金曜の夜にお楽しみに。"
	}
	tests := []struct {
		name    string
		sources []*episode.Source
		want    []bool
	}{
		{
			name: "the same summary for different episodes",
			sources: []*episode.Source{
				{Name: ep3, Description: weekly("3")},
				{Name: ep4, Description: weekly("4")},
				{Name: remasterEp3, Description: weekly("3")},
			},
			want: []bool{false, false, false},
		},
		{
			name: "the same summary for the same episode",
			sources: []*episode.Source{
				{Name: ep3, Description: weekly("3")},
				{Name: remasterEp3, Description: weekly("3")},
			},
			want: []bool{true, true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			episodes := make([]*episode.Episode, 0, len(tt.sources))
			for _, src := range tt.sources {
				episodes = append(episodes, episode.Parse(src))
			}
			episode.SuppressWeeklySummaries(episodes)
			got := make([]bool, 0, len(episodes))
			for _, ep := range episodes {
				got = append(got, ep.Fingerprint != "")
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
