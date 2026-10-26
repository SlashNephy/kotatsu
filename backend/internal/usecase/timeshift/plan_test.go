package timeshift_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

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

// tuners は、types を受信できるチューナーを index が 0 から始まる n 本返す。
func tuners(n int, types ...string) []*assign.Tuner {
	list := make([]*assign.Tuner, 0, n)
	for i := range n {
		list = append(list, &assign.Tuner{Index: i, Types: types})
	}

	return list
}

func TestPlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		channels    []*timeshift.Channel
		multiplexes []*timeshift.Multiplex
		tuners      []*assign.Tuner
		busy        []*timeshift.Busy
		want        []int64
	}{
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
			tuners:      append(tuners(8, "GR"), &assign.Tuner{Index: 8, Types: []string{"BS"}}),
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
			tuners:      []*assign.Tuner{{Index: 0, Types: []string{"GR", "BS"}}, {Index: 1, Types: []string{"BS"}}},
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
			tuners: []*assign.Tuner{{Index: 0, Types: []string{"GR", "BS"}}, {Index: 1, Types: []string{"BS"}}},
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

			assert.Equal(t, tt.want, timeshift.Plan(tt.channels, tt.multiplexes, tt.tuners, tt.busy))
		})
	}
}
