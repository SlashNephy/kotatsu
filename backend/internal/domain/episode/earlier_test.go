package episode_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain/episode"
)

// aired は、base から hours 時間後に始まる、番組名 name の回を返す。
func aired(name string, hours int) *episode.Aired {
	return &episode.Aired{
		Episode: episode.Parse(&episode.Source{Name: name}),
		StartAt: base().Add(time.Duration(hours) * time.Hour),
	}
}

// TestLaterEpisode は、前の回を録らない (#1499) ときに、放送より前に始まり、シリーズ名が完全に一致し、話数が大きい相手の
// 一番大きい話数を返すことを確かめる。
func TestLaterEpisode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		target    *episode.Aired
		opponents []*episode.Aired
		want      int
	}{
		{
			name:      "録画済みの 4 話より前の、一挙放送の 1 話を外す",
			target:    aired("こたつ探偵 #1", 2),
			opponents: []*episode.Aired{aired("こたつ探偵 #4", -24*7)},
			want:      4,
		},
		{
			name:      "4 話より前に流れる一挙放送の 3 話は、これからの 4 話では外さない",
			target:    aired("こたつ探偵 #3", 2),
			opponents: []*episode.Aired{aired("こたつ探偵 #4", 24)},
			want:      0,
		},
		{
			name:      "翌週の 6 話があるだけでは、今週の 5 話を外さない",
			target:    aired("こたつ探偵 #5", 2),
			opponents: []*episode.Aired{aired("こたつ探偵 #6", 2+24*7)},
			want:      0,
		},
		{
			name:      "今週の 6 話の後に流れる 5 話は外す",
			target:    aired("こたつ探偵 #5", 2+24*3),
			opponents: []*episode.Aired{aired("こたつ探偵 #6", 2)},
			want:      6,
		},
		{
			name:      "同じ時刻に始まる相手とは比べない",
			target:    aired("こたつ探偵 #5", 2),
			opponents: []*episode.Aired{aired("こたつ探偵 #6", 2)},
			want:      0,
		},
		{
			name:      "期の違うシリーズ名は外さない",
			target:    aired("こたつ探偵 第2期 #3", 2),
			opponents: []*episode.Aired{aired("こたつ探偵 #12", -24*90)},
			want:      0,
		},
		{
			name:      "編の違うシリーズ名は外さない",
			target:    aired("こたつ探偵 #3", 2),
			opponents: []*episode.Aired{aired("こたつ探偵 雪国編 #5", -24)},
			want:      0,
		},
		{
			name:      "全角と半角の違いは、シリーズ名の一致に効かない",
			target:    aired("ＫＯＴＡＴＳＵ探偵 ＃３", 2),
			opponents: []*episode.Aired{aired("KOTATSU探偵 #4", -24)},
			want:      4,
		},
		{
			name:      "話数の読めない放送は外さない",
			target:    aired("こたつ探偵", 2),
			opponents: []*episode.Aired{aired("こたつ探偵 #4", -24)},
			want:      0,
		},
		{
			name:      "話数の読めない相手とは比べない",
			target:    aired("こたつ探偵 #3", 2),
			opponents: []*episode.Aired{aired("こたつ探偵 「最後の謎」", -24)},
			want:      0,
		},
		{
			name:      "話数が同じか小さい相手では外さない",
			target:    aired("こたつ探偵 #3", 2),
			opponents: []*episode.Aired{aired("こたつ探偵 #3", -24), aired("こたつ探偵 #2", -48)},
			want:      0,
		},
		{
			name:   "理由には一番大きい話数を添える",
			target: aired("こたつ探偵 #3", 2),
			opponents: []*episode.Aired{
				aired("こたつ探偵 #5", -48), aired("こたつ探偵 #7", -24), aired("こたつ探偵 #9", 24),
			},
			want: 7,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, episode.LaterEpisode(tt.target, tt.opponents))
		})
	}
}

// TestSeriesOf は、番組名だけからシリーズ名を読むことを確かめる。録画履歴をシリーズ名で引くのに使う。
func TestSeriesOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  string
	}{
		{name: "飾りの印と話数と副題を除く", title: "[新]こたつ探偵 #1「消えた蜜柑」", want: "こたつ探偵"},
		{name: "期を含む", title: "こたつ探偵 第2期 #3", want: "こたつ探偵第2期"},
		{name: "半角に寄せる", title: "ＫＯＴＡＴＳＵ探偵 ＃３", want: "kotatsu探偵"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, episode.SeriesOf(tt.title))
		})
	}
}
