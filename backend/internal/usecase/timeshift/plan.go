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

// Busy は、予定の時間が今から先回りの時間までと重なる予約が使うチューナーと、その予約のチャンネル。予約ごとに 1 つ持つ。
type Busy struct {
	Tuner       int
	ChannelType string
	Channel     string
}

// Multiplex は、サービスのあるチャンネル種別とチャンネルの組。チューナーサーバーは、この単位で番組表を取る。
type Multiplex struct {
	ChannelType string
	Channel     string
}

type channelKey struct {
	channelType string
	channel     string
}

// Plan は、タイムシフトで録るサービスの ID を、channels の順に返す。
//
// busy のチューナーは使えない。busy のチューナーのうち、先回りの時間を通して 1 つのチャンネルに留まるもの (別のチャンネルの予約が続かないもの) と同じチャンネルは、
// 予約の録画と同じストリームなので、チューナーを使わずに録る。
// ほかのチャンネルは、channels の順に、受信できる空きチューナーを 1 本ずつ割り当てる。
// 同じチャンネルのサービスどうしは 1 本を共有する。チューナーが足りないチャンネルは録らない。
//
// チューナーサーバーの番組表の取得 (優先度 -1) は、タイムシフトのチューナーを奪えない (設計書 5.7)。
// そこで、multiplexes のうち、予約にもタイムシフトにも録られないチャンネルが残る種別では、
// その種別を受信できる最後の空きチューナーをタイムシフトに使わない。
// 録っているチャンネルの番組表は、チューナーサーバーが同じチャンネルのチューナーに相乗りして取れる。
// 種別ごとに受信できる空きチューナーが 1 本残ればよく、いくつもの種別を受信できるチューナーは、どの種別にも数える。
// 受信できる空きチューナーが初めからない種別は、残しようがないので見ない。
func Plan(channels []*Channel, multiplexes []*Multiplex, tuners []*assign.Tuner, busy []*Busy) []int64 {
	// チューナーごとに、先回りの時間に予約が使うチャンネルを集める。
	// 予約が続いて別のチャンネルに替わるチューナーは、どのチャンネルにも留まらないので、タイムシフトと共有しない。
	busyChannels := map[int]map[channelKey]bool{}
	for _, b := range busy {
		if busyChannels[b.Tuner] == nil {
			busyChannels[b.Tuner] = map[channelKey]bool{}
		}
		busyChannels[b.Tuner][channelKey{channelType: b.ChannelType, channel: b.Channel}] = true
	}
	taken := map[int]bool{}
	covered := map[channelKey]bool{}
	for tuner, keys := range busyChannels {
		taken[tuner] = true
		if len(keys) == 1 {
			for key := range keys {
				covered[key] = true
			}
		}
	}

	// 種別ごとに、予約にもタイムシフトにも録られず、番組表の取得にチューナーが要るチャンネルの数を数える。
	// タイムシフトで録るチャンネルは、multiplexes になくても数える。
	uncovered := map[string]int{}
	counted := map[channelKey]bool{}
	count := func(key channelKey) {
		if covered[key] || counted[key] {
			return
		}
		counted[key] = true
		uncovered[key.channelType]++
	}
	for _, m := range multiplexes {
		count(channelKey{channelType: m.ChannelType, channel: m.Channel})
	}
	for _, c := range channels {
		count(channelKey{channelType: c.ChannelType, channel: c.Channel})
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
			i := slices.IndexFunc(free, func(tuner *assign.Tuner) bool {
				return slices.Contains(tuner.Types, c.ChannelType) && leavesEPGTuner(free, tuner, c.ChannelType, uncovered)
			})
			if i < 0 {
				continue
			}
			free = slices.Delete(free, i, i+1)
			covered[key] = true
			uncovered[c.ChannelType]--
		}
		recorded = append(recorded, c.ServiceID)
	}

	return recorded
}

// leavesEPGTuner は、channelType のチャンネルを録るのに tuner を使っても、番組表の取得にチューナーが要る種別ごとに、
// その種別を受信できる空きチューナーがほかに残るかどうかを返す。
func leavesEPGTuner(free []*assign.Tuner, tuner *assign.Tuner, channelType string, uncovered map[string]int) bool {
	for _, t := range tuner.Types {
		need := uncovered[t]
		if t == channelType {
			// 録るチャンネルの番組表は、チューナーサーバーが録るチューナーに相乗りして取れる。
			need--
		}
		if need <= 0 {
			continue
		}
		other := slices.ContainsFunc(free, func(f *assign.Tuner) bool {
			return f.Index != tuner.Index && slices.Contains(f.Types, t)
		})
		if !other {
			return false
		}
	}

	return true
}
