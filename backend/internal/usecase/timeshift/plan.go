// Package timeshift は、タイムシフトの割り振りを扱う。
//
// neko の Scheduler が、タイムシフトで録るチャンネルのうち、チューナーを使えるものの割り振り (timeshift_assignments) を書く。
// チューナーは予約の録画が先に使うので、予約の割り振りの時間に入ったチューナーのタイムシフトは外す。
package timeshift

import (
	"slices"

	"github.com/SlashNephy/kotatsu/internal/usecase/assign"
)

// Channel は、タイムシフトで録るサービス。
type Channel struct {
	ServiceID   int64
	ChannelType string
	Channel     string
}

// Busy は、予約の録画が使うチューナーと、その予約のチャンネル。
type Busy struct {
	Tuner       int
	ChannelType string
	Channel     string
}

type channelKey struct {
	channelType string
	channel     string
}

// Plan は、タイムシフトで録るサービスの ID を、channels の順に返す。
//
// busy のチューナーは使えない。busy と同じチャンネルは、予約の録画と同じストリームなので、チューナーを使わずに録る。
// ほかのチャンネルは、channels の順に、受信できる空きチューナーを 1 本ずつ割り当てる。
// 同じチャンネルのサービスどうしは 1 本を共有する。チューナーが足りないチャンネルは録らない。
func Plan(channels []*Channel, tuners []*assign.Tuner, busy []*Busy) []int64 {
	taken := map[int]bool{}
	covered := map[channelKey]bool{}
	for _, b := range busy {
		taken[b.Tuner] = true
		covered[channelKey{channelType: b.ChannelType, channel: b.Channel}] = true
	}

	// 受信できる種別の少ないチューナーから使い、いろいろな種別を受信できるチューナーを残す。
	free := make([]*assign.Tuner, 0, len(tuners))
	for _, tuner := range tuners {
		if !taken[tuner.Index] {
			free = append(free, tuner)
		}
	}
	slices.SortStableFunc(free, func(a, b *assign.Tuner) int {
		if len(a.Types) != len(b.Types) {
			return len(a.Types) - len(b.Types)
		}

		return a.Index - b.Index
	})

	recorded := make([]int64, 0, len(channels))
	for _, c := range channels {
		key := channelKey{channelType: c.ChannelType, channel: c.Channel}
		if !covered[key] {
			i := slices.IndexFunc(free, func(tuner *assign.Tuner) bool { return slices.Contains(tuner.Types, c.ChannelType) })
			if i < 0 {
				continue
			}
			free = slices.Delete(free, i, i+1)
			covered[key] = true
		}
		recorded = append(recorded, c.ServiceID)
	}

	return recorded
}
