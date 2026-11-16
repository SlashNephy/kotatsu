package timeshift_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/usecase/assign"
	"github.com/SlashNephy/kotatsu/internal/usecase/timeshift"
)

const (
	bs01 = "BS01"
	bs03 = "BS03"
	gr1  = "GR_1"
)

// grChannels は、地上波の n 個のチャンネルを返す。サービス ID は 1 から、チャンネルは GR_1 から振る。
func grChannels(n int) []*timeshift.Channel {
	channels := make([]*timeshift.Channel, 0, n)
	for i := 1; i <= n; i++ {
		channels = append(channels, &timeshift.Channel{ServiceID: int64(i), ChannelType: "GR", Channel: fmt.Sprintf("GR_%d", i)})
	}

	return channels
}

// grMultiplexes は、地上波の n 個のチャンネル (GR_1 から) を、番組表を取るチャンネルとして返す。
func grMultiplexes(n int) []*timeshift.Multiplex {
	multiplexes := make([]*timeshift.Multiplex, 0, n)
	for i := 1; i <= n; i++ {
		multiplexes = append(multiplexes, &timeshift.Multiplex{ChannelType: "GR", Channel: fmt.Sprintf("GR_%d", i)})
	}

	return multiplexes
}

// idleNeed は、names のチューナーのうち、idle 本を空けておく EPGCaptureNeed を返す。
func idleNeed(idle int, names ...string) *tunerserver.EPGCaptureNeed {
	return &tunerserver.EPGCaptureNeed{TunerNames: names, Idle: idle}
}

// tuners は、types を受信できるチューナーを index が 0 から始まる n 本返す。
func tuners(n int, types ...string) []*assign.Tuner {
	list := make([]*assign.Tuner, 0, n)
	for i := range n {
		list = append(list, &assign.Tuner{Index: i, Types: types, Blocks: []assign.Block{}})
	}

	return list
}

func TestPlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		channels    []*timeshift.Channel
		watched     []*timeshift.Channel
		multiplexes []*timeshift.Multiplex
		tuners      []*assign.Tuner
		busy        []*timeshift.Busy
		want        []int64
	}{
		{
			name:        "視聴しているサービスは、空きチューナーがなくても録り、設定されたチャンネルのチューナーを使わない",
			channels:    grChannels(1),
			watched:     []*timeshift.Channel{{ServiceID: 10, ChannelType: "BS", Channel: bs01}},
			multiplexes: append(grMultiplexes(1), &timeshift.Multiplex{ChannelType: "BS", Channel: bs01}),
			tuners:      tuners(1, "GR"),
			busy:        []*timeshift.Busy{},
			want:        []int64{1, 10},
		},
		{
			name:     "設定されたチャンネルを視聴していれば、チューナーを使わずに録り、結果に 2 度出さない",
			channels: grChannels(2),
			watched:  []*timeshift.Channel{{ServiceID: 1, ChannelType: "GR", Channel: gr1}},
			// GR_1 は視聴のチューナーに相乗りするので、1 本のチューナーを GR_2 に使える。
			multiplexes: grMultiplexes(2),
			tuners:      tuners(1, "GR"),
			busy:        []*timeshift.Busy{},
			want:        []int64{1, 2},
		},
		{
			name:        "チューナーがすべて空いていれば、8 つとも録る",
			channels:    grChannels(8),
			multiplexes: grMultiplexes(8),
			tuners:      tuners(8, "GR"),
			busy:        []*timeshift.Busy{},
			want:        []int64{1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name:        "予約が 1 本使えば、番組表の取得にも 1 本残すので、並びの最後の 2 チャンネルを録らない",
			channels:    grChannels(8),
			multiplexes: append(grMultiplexes(8), &timeshift.Multiplex{ChannelType: "BS", Channel: bs01}),
			tuners:      append(tuners(8, "GR"), &assign.Tuner{Index: 8, Types: []string{"BS"}, Blocks: []assign.Block{}}),
			busy:        []*timeshift.Busy{{Tuner: 0, ChannelType: "BS", Channel: bs01}},
			want:        []int64{1, 2, 3, 4, 5, 6},
		},
		{
			name:        "予約と同じチャンネルは、チューナーを使わずに録り、ほかも全部録る",
			channels:    grChannels(8),
			multiplexes: grMultiplexes(8),
			tuners:      tuners(8, "GR"),
			busy:        []*timeshift.Busy{{Tuner: 0, ChannelType: "GR", Channel: "GR_8"}},
			want:        []int64{1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name:        "予約と同じチャンネルの別のサービスも、予約のチューナーを共有する",
			channels:    []*timeshift.Channel{{ServiceID: 2, ChannelType: "GR", Channel: gr1}},
			multiplexes: grMultiplexes(1),
			tuners:      tuners(1, "GR"),
			busy:        []*timeshift.Busy{{Tuner: 0, ServiceID: 1, ChannelType: "GR", Channel: gr1}},
			want:        []int64{2},
		},
		{
			name: "同じチャンネルの 2 サービスは、チューナー 1 本を共有する",
			channels: []*timeshift.Channel{
				{ServiceID: 1, ChannelType: "GR", Channel: gr1},
				{ServiceID: 2, ChannelType: "GR", Channel: gr1},
			},
			multiplexes: grMultiplexes(1),
			tuners:      tuners(1, "GR"),
			busy:        []*timeshift.Busy{},
			want:        []int64{1, 2},
		},
		{
			name:        "1 本のチューナーで続く予約のどちらのチャンネルも、チューナーを使わずには録らない",
			channels:    grChannels(2),
			multiplexes: grMultiplexes(3),
			tuners:      tuners(3, "GR"),
			busy: []*timeshift.Busy{
				{Tuner: 0, ChannelType: "GR", Channel: gr1},
				{Tuner: 0, ChannelType: "GR", Channel: "GR_2"},
			},
			want: []int64{1},
		},
		{
			name:        "1 本のチューナーで続く予約のチャンネルは、空きチューナーがなければ録らない",
			channels:    grChannels(2),
			multiplexes: grMultiplexes(2),
			tuners:      tuners(1, "GR"),
			busy: []*timeshift.Busy{
				{Tuner: 0, ChannelType: "GR", Channel: gr1},
				{Tuner: 0, ChannelType: "GR", Channel: "GR_2"},
			},
			want: []int64{},
		},
		{
			name:        "同じチャンネルの予約が続くチューナーは、そのチャンネルのタイムシフトと共有する",
			channels:    grChannels(1),
			multiplexes: grMultiplexes(1),
			tuners:      tuners(1, "GR"),
			busy: []*timeshift.Busy{
				{Tuner: 0, ChannelType: "GR", Channel: gr1},
				{Tuner: 0, ChannelType: "GR", Channel: gr1},
			},
			want: []int64{1},
		},
		{
			name:        "予約が 1 本だけのチューナーは、そのチャンネルのタイムシフトと共有する",
			channels:    grChannels(1),
			multiplexes: grMultiplexes(1),
			tuners:      tuners(1, "GR"),
			busy:        []*timeshift.Busy{{Tuner: 0, ChannelType: "GR", Channel: gr1}},
			want:        []int64{1},
		},
		{
			name:        "チューナーの種別が合わなければ録らない",
			channels:    grChannels(2),
			multiplexes: grMultiplexes(2),
			tuners:      tuners(2, "BS"),
			busy:        []*timeshift.Busy{},
			want:        []int64{},
		},
		{
			name:        "予約の割り振りの時間に入ったチューナーは使わず、残る 2 本のうち 1 本を番組表の取得に残す",
			channels:    grChannels(3),
			multiplexes: grMultiplexes(3),
			tuners:      tuners(3, "GR"),
			busy:        []*timeshift.Busy{{Tuner: 1, ChannelType: "BS", Channel: bs01}},
			want:        []int64{1},
		},
		{
			name: "受信できる種別が少ないチューナーを先に使い、多いチューナーを残す",
			channels: []*timeshift.Channel{
				{ServiceID: 1, ChannelType: "BS", Channel: bs01},
				{ServiceID: 2, ChannelType: "GR", Channel: gr1},
			},
			multiplexes: []*timeshift.Multiplex{{ChannelType: "BS", Channel: bs01}, {ChannelType: "GR", Channel: gr1}},
			tuners:      []*assign.Tuner{{Index: 0, Types: []string{"GR", "BS"}, Blocks: []assign.Block{}}, {Index: 1, Types: []string{"BS"}, Blocks: []assign.Block{}}},
			busy:        []*timeshift.Busy{},
			want:        []int64{1, 2},
		},
		{
			name:        "タイムシフトで録らないチャンネルが残れば、番組表の取得にチューナーを 1 本残す",
			channels:    grChannels(2),
			multiplexes: grMultiplexes(3),
			tuners:      tuners(2, "GR"),
			busy:        []*timeshift.Busy{},
			want:        []int64{1},
		},
		{
			name:        "すべてのチャンネルをタイムシフトで録れば、番組表の取得にチューナーを残さない",
			channels:    grChannels(2),
			multiplexes: grMultiplexes(2),
			tuners:      tuners(2, "GR"),
			busy:        []*timeshift.Busy{},
			want:        []int64{1, 2},
		},
		{
			name:        "予約が録るチャンネルの番組表は予約のストリームで取れるので、そのためにチューナーを残さない",
			channels:    grChannels(2),
			multiplexes: grMultiplexes(3),
			tuners:      tuners(3, "GR"),
			busy:        []*timeshift.Busy{{Tuner: 2, ChannelType: "GR", Channel: "GR_3"}},
			want:        []int64{1, 2},
		},
		{
			name: "いくつもの種別を受信できる最後のチューナーは、ほかの種別の番組表の取得に残す",
			channels: []*timeshift.Channel{
				{ServiceID: 1, ChannelType: "BS", Channel: bs01},
				{ServiceID: 2, ChannelType: "BS", Channel: bs03},
			},
			multiplexes: []*timeshift.Multiplex{
				{ChannelType: "BS", Channel: bs01},
				{ChannelType: "BS", Channel: bs03},
				{ChannelType: "GR", Channel: gr1},
			},
			tuners: []*assign.Tuner{{Index: 0, Types: []string{"GR", "BS"}, Blocks: []assign.Block{}}, {Index: 1, Types: []string{"BS"}, Blocks: []assign.Block{}}},
			busy:   []*timeshift.Busy{},
			want:   []int64{1},
		},
		{
			name:        "受信できるチューナーがない種別のためには、チューナーを残さない",
			channels:    grChannels(1),
			multiplexes: append(grMultiplexes(1), &timeshift.Multiplex{ChannelType: "SKY", Channel: "SKY_1"}),
			tuners:      tuners(1, "GR"),
			busy:        []*timeshift.Busy{},
			want:        []int64{1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, timeshift.Plan(tt.channels, tt.watched, tt.multiplexes, tt.tuners, tt.busy, nil))
		})
	}
}

// TestPlanEDCB は、EDCB の数え方 (#1856) を確かめる。相乗りの単位はサービスで、EPG の取得のためには、種別と録られるチャンネルを問わずに、
// 空けておくチューナー (IdleNeeds) を満たすように残す。
func TestPlanEDCB(t *testing.T) {
	t.Parallel()

	// GR_1 の 2 つのサービス。
	main := &timeshift.Channel{ServiceID: 1, ChannelType: "GR", Channel: gr1}
	sub := &timeshift.Channel{ServiceID: 2, ChannelType: "GR", Channel: gr1}
	tests := []struct {
		name     string
		channels []*timeshift.Channel
		watched  []*timeshift.Channel
		tuners   []*assign.Tuner
		busy     []*timeshift.Busy
		// names は、チューナーの BonDriver。nil なら、すべてのチューナーが bon。
		names map[int]string
		// needs は、空けておくチューナー。nil なら、bon に 1 本 (EpgTimerSrv.ini を読めないときや、EPGCount が 1 のとき)。
		needs []*tunerserver.EPGCaptureNeed
		// others は、tuners のほかにある、Kotatsu が使わない空きチューナーの名前。
		others []string
		want   []int64
	}{
		{
			name:     "予約と同じサービスは、チューナーを使わずに録る",
			channels: []*timeshift.Channel{main},
			tuners:   tuners(1, "GR"),
			busy:     []*timeshift.Busy{{Tuner: 0, ServiceID: 1, ChannelType: "GR", Channel: gr1}},
			want:     []int64{1},
		},
		{
			name:     "予約と同じチャンネルの別のサービスは、空きチューナーを使う",
			channels: []*timeshift.Channel{sub},
			tuners:   tuners(3, "GR"),
			busy:     []*timeshift.Busy{{Tuner: 0, ServiceID: 1, ChannelType: "GR", Channel: gr1}},
			want:     []int64{2},
		},
		{
			name:     "予約と同じチャンネルの別のサービスは、最後の空きチューナーしかなければ録らない",
			channels: []*timeshift.Channel{sub},
			tuners:   tuners(2, "GR"),
			busy:     []*timeshift.Busy{{Tuner: 0, ServiceID: 1, ChannelType: "GR", Channel: gr1}},
			want:     []int64{},
		},
		{
			name:     "同じチャンネルの 2 サービスは、別のチューナーを使う",
			channels: []*timeshift.Channel{main, sub},
			tuners:   tuners(3, "GR"),
			want:     []int64{1, 2},
		},
		{
			name:     "同じチャンネルの 2 サービスは、チューナーが 1 本分しかなければ先のサービスだけを録る",
			channels: []*timeshift.Channel{main, sub},
			tuners:   tuners(2, "GR"),
			want:     []int64{1},
		},
		{
			name:     "視聴しているサービスと同じチャンネルの別のサービスは、空きチューナーを使う",
			channels: []*timeshift.Channel{sub},
			watched:  []*timeshift.Channel{main},
			tuners:   tuners(2, "GR"),
			want:     []int64{2, 1},
		},
		{
			name:     "視聴しているサービスと同じチャンネルの別のサービスは、最後の空きチューナーしかなければ録らない",
			channels: []*timeshift.Channel{sub},
			watched:  []*timeshift.Channel{main},
			tuners:   tuners(1, "GR"),
			want:     []int64{1},
		},
		{
			name:     "1 本のチューナーで同じチャンネルの別のサービスの予約が続けば、どちらのサービスとも共有しない",
			channels: []*timeshift.Channel{main},
			tuners:   tuners(1, "GR"),
			busy: []*timeshift.Busy{
				{Tuner: 0, ServiceID: 1, ChannelType: "GR", Channel: gr1},
				{Tuner: 0, ServiceID: 2, ChannelType: "GR", Channel: gr1},
			},
			want: []int64{},
		},
		{
			name:     "すべてのチャンネルをタイムシフトで録っても、空きチューナーを 1 本残す",
			channels: grChannels(2),
			tuners:   tuners(2, "GR"),
			want:     []int64{1},
		},
		{
			name:     "BonDriver ごとに 1 本を残す",
			channels: grChannels(3),
			tuners:   tuners(4, "GR"),
			names:    map[int]string{0: "a", 1: "a", 2: "b", 3: "b"},
			needs:    []*tunerserver.EPGCaptureNeed{idleNeed(1, "a"), idleNeed(1, "b")},
			want:     []int64{1, 2},
		},
		{
			// EDCB の EPG の取得は、空きチューナーの足りない BonDriver が 1 つでもあれば、その回の取得をしない。
			name:     "ほかの BonDriver のチューナーが空いていても、BonDriver の最後の 1 本は使わない",
			channels: grChannels(2),
			tuners:   tuners(3, "GR"),
			names:    map[int]string{0: "a", 1: "b", 2: "b"},
			needs:    []*tunerserver.EPGCaptureNeed{idleNeed(1, "a"), idleNeed(1, "b")},
			want:     []int64{1},
		},
		{
			// EPG の取得の時刻の前後でなければ、空けておかない。
			name:     "空けておくチューナーがなければ、すべて使う",
			channels: grChannels(2),
			tuners:   tuners(2, "GR"),
			needs:    []*tunerserver.EPGCaptureNeed{},
			want:     []int64{1, 2},
		},
		{
			name:     "EPGCount が 2 なら、2 本を残す",
			channels: grChannels(3),
			tuners:   tuners(4, "GR"),
			needs:    []*tunerserver.EPGCaptureNeed{idleNeed(2, "bon")},
			want:     []int64{1, 2},
		},
		{
			// EPGCount が 0 で、BonDriver のすべてのチューナーで取得するときに、予約が 1 本を使えば、空けても取得されない。
			name:     "使う前から満たせないなら、空けずに使う",
			channels: grChannels(2),
			tuners:   tuners(3, "GR"),
			busy:     []*timeshift.Busy{{Tuner: 2, ServiceID: 10, ChannelType: "BS", Channel: bs01}},
			needs:    []*tunerserver.EPGCaptureNeed{idleNeed(3, "bon")},
			want:     []int64{1, 2},
		},
		{
			name:     "BonDriver のすべてのチューナーで取得するなら、すべて空いているうちは使わない",
			channels: grChannels(2),
			tuners:   tuners(3, "GR"),
			needs:    []*tunerserver.EPGCaptureNeed{idleNeed(3, "bon")},
			want:     []int64{},
		},
		{
			// NGEpgCapTime が 0 なら、取得に使う BonDriver のどれかに 1 本空いていればよい。Kotatsu が使わない BonDriver も数える。
			name:     "Kotatsu が使わない空きチューナーで足りれば、すべて使う",
			channels: grChannels(2),
			tuners:   tuners(2, "GR"),
			needs:    []*tunerserver.EPGCaptureNeed{idleNeed(1, "bon", "other")},
			others:   []string{"other"},
			want:     []int64{1, 2},
		},
		{
			// EDCB の EPG の取得は、どの種別のチューナーでも、使っていないものが 1 本あればよい。
			name:     "番組表の取得に残すのは、種別を問わず 1 本",
			channels: []*timeshift.Channel{{ServiceID: 1, ChannelType: "BS", Channel: bs01}, {ServiceID: 2, ChannelType: "GR", Channel: gr1}},
			tuners:   []*assign.Tuner{{Index: 0, Types: []string{"BS"}, Blocks: []assign.Block{}}, {Index: 1, Types: []string{"GR"}, Blocks: []assign.Block{}}},
			want:     []int64{1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// 番組表を取るチャンネルは、EDCB の数え方では見ない。録らないチャンネルを並べても結果は変わらない。
			names, needs, others := tt.names, tt.needs, tt.others
			if names == nil {
				names = map[int]string{}
				for _, tuner := range tt.tuners {
					names[tuner.Index] = "bon"
				}
			}
			if needs == nil {
				needs = []*tunerserver.EPGCaptureNeed{idleNeed(1, "bon")}
			}
			if others == nil {
				others = []string{}
			}
			opts := &timeshift.PlanOptions{ShareByService: true, IdleNeeds: needs, TunerNames: names, OtherIdleTuners: others}
			assert.Equal(t, tt.want, timeshift.Plan(tt.channels, tt.watched, grMultiplexes(8), tt.tuners, tt.busy, opts))
		})
	}
}
