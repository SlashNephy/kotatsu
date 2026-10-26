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
		name     string
		channels []*timeshift.Channel
		tuners   []*assign.Tuner
		busy     []*timeshift.Busy
		want     []int64
	}{
		{
			name:     "チューナーがすべて空いていれば、8 つとも録る",
			channels: grChannels(8),
			tuners:   tuners(8, "GR"),
			busy:     []*timeshift.Busy{},
			want:     []int64{1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name:     "予約が 1 本使っていれば、並びの最後の 1 チャンネルを録らない",
			channels: grChannels(8),
			tuners:   append(tuners(8, "GR"), &assign.Tuner{Index: 8, Types: []string{"BS"}}),
			busy:     []*timeshift.Busy{{Tuner: 0, ChannelType: "BS", Channel: bs01}},
			want:     []int64{1, 2, 3, 4, 5, 6, 7},
		},
		{
			name:     "予約と同じチャンネルは、チューナーを使わずに録り、ほかも全部録る",
			channels: grChannels(8),
			tuners:   tuners(8, "GR"),
			busy:     []*timeshift.Busy{{Tuner: 0, ChannelType: "GR", Channel: "GR_8"}},
			want:     []int64{1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name: "同じチャンネルの 2 サービスは、チューナー 1 本を共有する",
			channels: []*timeshift.Channel{
				{ServiceID: 1, ChannelType: "GR", Channel: gr1},
				{ServiceID: 2, ChannelType: "GR", Channel: gr1},
			},
			tuners: tuners(1, "GR"),
			busy:   []*timeshift.Busy{},
			want:   []int64{1, 2},
		},
		{
			name:     "チューナーの種別が合わなければ録らない",
			channels: grChannels(2),
			tuners:   tuners(2, "BS"),
			busy:     []*timeshift.Busy{},
			want:     []int64{},
		},
		{
			name:     "予約の割り振りの時間に入ったチューナーは、タイムシフトから外す",
			channels: grChannels(2),
			tuners:   tuners(2, "GR"),
			busy:     []*timeshift.Busy{{Tuner: 1, ChannelType: "BS", Channel: bs01}},
			want:     []int64{1},
		},
		{
			name: "受信できる種別が少ないチューナーを先に使い、多いチューナーを残す",
			channels: []*timeshift.Channel{
				{ServiceID: 1, ChannelType: "BS", Channel: bs01},
				{ServiceID: 2, ChannelType: "GR", Channel: gr1},
			},
			tuners: []*assign.Tuner{{Index: 0, Types: []string{"GR", "BS"}}, {Index: 1, Types: []string{"BS"}}},
			busy:   []*timeshift.Busy{},
			want:   []int64{1, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, timeshift.Plan(tt.channels, tt.tuners, tt.busy))
		})
	}
}
