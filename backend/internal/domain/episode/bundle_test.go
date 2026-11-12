package episode_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain/episode"
)

// base は、テストの放送の時刻の基準。
func base() time.Time {
	return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
}

// broadcast は、base から startHour 時間後に 30 分の放送を作る。
type broadcast struct {
	name, description string
	serviceID         int64
	startHour         int
	captioned         bool
	satellite         bool
	rejected          bool
}

func (b broadcast) build() *episode.Broadcast {
	start := base().Add(time.Duration(b.startHour) * time.Hour)

	return &episode.Broadcast{
		Episode:   episode.Parse(&episode.Source{Name: b.name, Description: b.description}),
		ServiceID: b.serviceID, EventID: 1, StartAt: start, EndAt: start.Add(30 * time.Minute),
		Captioned: b.captioned, Satellite: b.satellite, Rejected: b.rejected,
	}
}

// outcome は、放送 1 本の扱いを、テストで読みやすい形にしたもの。instead は、代わりに録る放送の添字 (外さなければ -1)。
type outcome struct {
	recorded bool
	instead  int
}

func TestResolve(t *testing.T) {
	t.Parallel()

	keep := outcome{recorded: false, instead: -1}

	tests := []struct {
		name       string
		broadcasts []broadcast
		recorded   []string
		want       []outcome
	}{
		{
			name: "the earliest broadcast across stations",
			broadcasts: []broadcast{
				{name: ep3Subtitle, serviceID: 1, startHour: 24},
				{name: ep3, serviceID: 2, startHour: 0},
				{name: ep4, serviceID: 2, startHour: 1},
			},
			want: []outcome{{instead: 1}, keep, keep},
		},
		{
			// 書き方の違う 3 局 (編と副題、編だけ、どちらも無し) は、どの 2 つも互いに同じ回と判定されるので、1 つにまとめる。
			name: "three stations writing differently",
			broadcasts: []broadcast{
				{name: work + " 霧の港編 #3「霧笛」", serviceID: 1, startHour: 0},
				{name: harborEp3, serviceID: 2, startHour: 24},
				{name: ep3, serviceID: 3, startHour: 48},
			},
			want: []outcome{keep, {instead: 0}, {instead: 0}},
		},
		{
			// 編を書かない回は、2 つの違う編のどちらとも同じ回と判定されるが、2 つの編どうしは違う回なので、どれもまとめない。
			name: "a group that is not mutually the same is not bundled",
			broadcasts: []broadcast{
				{name: ep3, serviceID: 1, startHour: 0},
				{name: harborEp3, serviceID: 2, startHour: 24},
				{name: work + " 星の丘編 #3", serviceID: 3, startHour: 48},
			},
			want: []outcome{keep, keep, keep},
		},
		{
			name: "an episode already recorded",
			broadcasts: []broadcast{
				{name: ep3, serviceID: 1, startHour: 0},
				{name: ep3Subtitle, serviceID: 2, startHour: 24},
				{name: ep4, serviceID: 2, startHour: 25},
			},
			recorded: []string{ep3Subtitle},
			want:     []outcome{{recorded: true, instead: -1}, {recorded: true, instead: -1}, keep},
		},
		{
			// 話数を載せない局の同時放送は、作品名が一致する回の束に入れる。
			name: "a simulcast without an episode joins the bundle",
			broadcasts: []broadcast{
				{name: work, serviceID: 1, startHour: 0},
				{name: ep3, serviceID: 2, startHour: 0, satellite: true},
			},
			want: []outcome{{instead: 1}, keep},
		},
		{
			name: "a simulcast that joined the bundle is also recorded",
			broadcasts: []broadcast{
				{name: work, serviceID: 1, startHour: 0},
				{name: ep3, serviceID: 2, startHour: 0},
			},
			recorded: []string{ep3},
			want:     []outcome{{recorded: true, instead: -1}, {recorded: true, instead: -1}},
		},
		{
			name: "a special does not join the bundle",
			broadcasts: []broadcast{
				{name: work, description: "特別編をお届けします", serviceID: 1, startHour: 0},
				{name: ep3, serviceID: 2, startHour: 0},
			},
			want: []outcome{keep, keep},
		},
		{
			name: "a simulcast does not join a special",
			broadcasts: []broadcast{
				{name: work, serviceID: 1, startHour: 0},
				{name: ep3, description: "スペシャルとしてお届けします", serviceID: 2, startHour: 0},
			},
			want: []outcome{keep, keep},
		},
		{
			name: "unknown episodes at the same time",
			broadcasts: []broadcast{
				{name: news, serviceID: 1, startHour: 0},
				{name: "[字]夜のニュース", serviceID: 2, startHour: 0, captioned: true},
			},
			want: []outcome{{instead: 1}, keep},
		},
		{
			// 時刻をまたいで束ねると、毎日のニュースが初回しか録れなくなる。
			name: "unknown episodes at different times",
			broadcasts: []broadcast{
				{name: news, serviceID: 1, startHour: 0},
				{name: news, serviceID: 1, startHour: 24},
			},
			want: []outcome{keep, keep},
		},
		{
			name: "unknown episodes are not compared with the recorded",
			broadcasts: []broadcast{
				{name: news, serviceID: 1, startHour: 0},
			},
			recorded: []string{news},
			want:     []outcome{keep},
		},
		{
			name: "a captioned broadcast overlapping the earliest",
			broadcasts: []broadcast{
				{name: ep3, serviceID: 1, startHour: 0},
				{name: ep3, serviceID: 2, startHour: 0, captioned: true},
				{name: ep3, serviceID: 3, startHour: 24, captioned: true, satellite: true},
			},
			want: []outcome{{instead: 1}, keep, {instead: 1}},
		},
		{
			name: "a satellite broadcast overlapping the earliest",
			broadcasts: []broadcast{
				{name: ep3, serviceID: 1, startHour: 0},
				{name: ep3, serviceID: 2, startHour: 0, satellite: true},
			},
			want: []outcome{{instead: 1}, keep},
		},
		{
			name: "the smaller service at the same time",
			broadcasts: []broadcast{
				{name: ep3, serviceID: 2, startHour: 0},
				{name: ep3, serviceID: 1, startHour: 0},
			},
			want: []outcome{{instead: 1}, keep},
		},
		{
			// 競合で録れない見込みの放送も残し、次の放送をもう 1 本選ぶ。
			name: "a rejected broadcast keeps the next one",
			broadcasts: []broadcast{
				{name: ep3, serviceID: 1, startHour: 0, rejected: true},
				{name: ep3, serviceID: 2, startHour: 24},
				{name: ep3, serviceID: 3, startHour: 48},
			},
			want: []outcome{keep, keep, {instead: 1}},
		},
		{
			name: "every broadcast is rejected",
			broadcasts: []broadcast{
				{name: ep3, serviceID: 1, startHour: 0, rejected: true},
				{name: ep3, serviceID: 2, startHour: 24, rejected: true},
			},
			want: []outcome{keep, keep},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			broadcasts := make([]*episode.Broadcast, 0, len(tt.broadcasts))
			for _, b := range tt.broadcasts {
				broadcasts = append(broadcasts, b.build())
			}
			recorded := make([]*episode.Recorded, 0, len(tt.recorded))
			for _, name := range tt.recorded {
				recorded = append(recorded, &episode.Recorded{Episode: episode.Parse(&episode.Source{Name: name}), ServiceID: 9, StartAt: base().Add(-time.Hour)})
			}

			got := []outcome{}
			for _, d := range episode.Resolve(broadcasts, recorded) {
				o := outcome{recorded: d.Recorded != nil, instead: -1}
				for i, b := range broadcasts {
					if d.Instead == b {
						o.instead = i
					}
				}
				got = append(got, o)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
