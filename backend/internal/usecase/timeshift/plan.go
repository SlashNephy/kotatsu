// Package timeshift は、タイムシフトの割り振りを扱う。
//
// neko の Scheduler が、タイムシフトで録るチャンネルのうち、チューナーを使えるものの割り振り (timeshift_assignments) を書く。
// チューナーは予約の録画が先に使うので、予約の割り振りの時間に入ったチューナーのタイムシフトは外す。
package timeshift

import (
	"slices"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/usecase/assign"
)

// Channel は、タイムシフトで録るサービス。
type Channel struct {
	ServiceID   int64
	ChannelType string
	Channel     string
}

// Busy は、予定の時間が今から先回りの時間までと重なる予約が使うチューナーと、その予約のサービス。予約ごとに 1 つ持つ。
type Busy struct {
	Tuner       int
	ServiceID   int64
	ChannelType string
	Channel     string
}

// Multiplex は、サービスのあるチャンネル種別とチャンネルの組。チューナーサーバーは、この単位で番組表を取る。
type Multiplex struct {
	ChannelType string
	Channel     string
}

// PlanOptions は、Plan の数え方。ゼロ値は、Mirakurun 互換のチューナーサーバーの数え方。
type PlanOptions struct {
	// ShareByService は、相乗りの単位をチャンネルでなくサービスにするかどうか。EDCB で使う (#1856)。
	// EDCB の TS はサービスで絞られるので、同じチャンネルの別のサービスは別のチューナーになる。
	ShareByService bool
	// IdleNeeds は、チューナーサーバー自身の EPG の取得 (EDCB) のために空けておくチューナー (#1856)。
	// nil なら、Mirakurun 互換のサーバーと同じく、録られないチャンネルの種別ごとに 1 本を残す (leavesEPGTuner)。
	// nil でなければ、録られるチャンネルと種別を問わず、これを満たすように残す (keepsIdleTuners)。空なら残さない。
	// EDCB の EPG の取得は、録っているチャンネルに相乗りせず、使っていないチューナーで、取得するチャンネルをすべて回る。
	IdleNeeds []*tunerserver.EPGCaptureNeed
	// TunerNames は、tuners の番号から、チューナーの名前 (EDCB では BonDriver) への対応。IdleNeeds と照合する。
	TunerNames map[int]string
	// OtherIdleTuners は、tuners のほかにある、使っていないチューナー (Kotatsu が使わないもの) の名前。IdleNeeds で数える。
	OtherIdleTuners []string
}

// channelKey は、チャンネル種別とチャンネルの組。相乗りの単位に使うときは、ShareByService ならサービスも持つ。
type channelKey struct {
	channelType string
	channel     string
	service     int64
}

// shareKey は、相乗りの単位を返す。ShareByService でなければ、サービスを持たない。
func (o *PlanOptions) shareKey(serviceID int64, channelType, channel string) channelKey {
	if !o.ShareByService {
		serviceID = 0
	}

	return channelKey{channelType: channelType, channel: channel, service: serviceID}
}

// multiplexKey は、番組表を取る単位 (チャンネル) を返す。
func multiplexKey(channelType, channel string) channelKey {
	return channelKey{channelType: channelType, channel: channel, service: 0}
}

// Plan は、タイムシフトで録るサービスの ID を、channels の順に返す。opts が nil なら、ゼロ値の PlanOptions とみなす。
//
// 相乗りの単位は、チャンネル (opts.ShareByService ならサービス) である。
// busy のチューナーは使えない。busy のチューナーのうち、先回りの時間を通して 1 つの単位に留まるもの (別の単位の予約が続かないもの) と同じ単位は、
// 予約の録画と同じストリームなので、チューナーを使わずに録る。
// ほかの単位には、channels の順に、受信できる空きチューナーを 1 本ずつ割り当てる。
// 同じ単位のサービスどうしは 1 本を共有する。チューナーが足りない単位は録らない。
//
// チューナーサーバーの番組表の取得 (優先度 -1) は、タイムシフトのチューナーを奪えない (設計書 5.7)。
// そこで、multiplexes のうち、予約にもタイムシフトにも録られないチャンネルが残る種別では、
// その種別を受信できる最後の空きチューナーをタイムシフトに使わない。
// 録っているチャンネルの番組表は、チューナーサーバーが同じチャンネルのチューナーに相乗りして取れる。
// 種別ごとに受信できる空きチューナーが 1 本残ればよく、いくつもの種別を受信できるチューナーは、どの種別にも数える。
// 受信できる空きチューナーが初めからない種別は、残しようがないので見ない。
// opts.IdleNeeds が nil でなければ、代わりに、録られるチャンネルと種別を問わず、それを満たすように空きチューナーを残す (keepsIdleTuners)。
//
// watched は、ライブ視聴しているサービス (#1408)。視聴のチューナーに相乗りするので、空きチューナーを使わずに録り、結果の後ろに足す。
// neko は視聴のチューナーを知らないが、空きチューナーを当てると、設定されたチャンネルのチューナーや、番組表の取得に残すチューナーを、視聴が奪ってしまう。
func Plan(channels, watched []*Channel, multiplexes []*Multiplex, tuners []*assign.Tuner, busy []*Busy, opts *PlanOptions) []int64 {
	if opts == nil {
		opts = &PlanOptions{ShareByService: false, IdleNeeds: nil, TunerNames: map[int]string{}, OtherIdleTuners: []string{}}
	}

	taken, shared, covered := coverage(busy, watched, opts)
	uncovered := countUncovered(multiplexes, channels, covered)
	free := freeTuners(tuners, taken)

	recorded := make([]int64, 0, len(channels)+len(watched))
	for _, c := range channels {
		key := opts.shareKey(c.ServiceID, c.ChannelType, c.Channel)
		if !shared[key] {
			i := slices.IndexFunc(free, func(tuner *assign.Tuner) bool {
				if !slices.Contains(tuner.Types, c.ChannelType) {
					return false
				}
				if opts.IdleNeeds != nil {
					return keepsIdleTuners(free, tuner, opts)
				}

				return leavesEPGTuner(free, tuner, c.ChannelType, uncovered)
			})
			if i < 0 {
				continue
			}
			free = slices.Delete(free, i, i+1)
			shared[key] = true
			if multiplex := multiplexKey(c.ChannelType, c.Channel); !covered[multiplex] {
				covered[multiplex] = true
				uncovered[c.ChannelType]--
			}
		}
		recorded = append(recorded, c.ServiceID)
	}
	for _, c := range watched {
		if !slices.Contains(recorded, c.ServiceID) {
			recorded = append(recorded, c.ServiceID)
		}
	}

	return recorded
}

// coverage は、予約が使うチューナー (taken) と、チューナーを使わずに相乗りできる単位 (shared) と、
// 予約か視聴に録られていて、番組表の取得にチューナーが要らないチャンネル (covered) を返す。
func coverage(busy []*Busy, watched []*Channel, opts *PlanOptions) (map[int]bool, map[channelKey]bool, map[channelKey]bool) {
	// チューナーごとに、先回りの時間に予約が使う単位を集める。
	// 予約が続いて別の単位に替わるチューナーは、どの単位にも留まらないので、タイムシフトと共有しない。
	busyKeys := map[int]map[channelKey]bool{}
	for _, b := range busy {
		if busyKeys[b.Tuner] == nil {
			busyKeys[b.Tuner] = map[channelKey]bool{}
		}
		busyKeys[b.Tuner][opts.shareKey(b.ServiceID, b.ChannelType, b.Channel)] = true
	}
	taken := map[int]bool{}
	shared := map[channelKey]bool{}
	covered := map[channelKey]bool{}
	for tuner, keys := range busyKeys {
		taken[tuner] = true
		if len(keys) == 1 {
			for key := range keys {
				shared[key] = true
				covered[multiplexKey(key.channelType, key.channel)] = true
			}
		}
	}
	for _, c := range watched {
		shared[opts.shareKey(c.ServiceID, c.ChannelType, c.Channel)] = true
		covered[multiplexKey(c.ChannelType, c.Channel)] = true
	}

	return taken, shared, covered
}

// countUncovered は、種別ごとに、予約にもタイムシフトにも録られず、番組表の取得にチューナーが要るチャンネルの数を返す。
// タイムシフトで録るチャンネルは、multiplexes になくても数える。
func countUncovered(multiplexes []*Multiplex, channels []*Channel, covered map[channelKey]bool) map[string]int {
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
		count(multiplexKey(m.ChannelType, m.Channel))
	}
	for _, c := range channels {
		count(multiplexKey(c.ChannelType, c.Channel))
	}

	return uncovered
}

// freeTuners は、taken でないチューナーを、受信できる種別の少ない順に返す。
// 受信できる種別の少ないチューナーから使い、いろいろな種別を受信できるチューナーを残すためである。
func freeTuners(tuners []*assign.Tuner, taken map[int]bool) []*assign.Tuner {
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

	return free
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

// keepsIdleTuners は、tuner をタイムシフトに使っても、opts.IdleNeeds の満たせていたものが、満たせなくなることがないかを返す。
// 空きチューナーは、free と opts.OtherIdleTuners を数える。tuner を使う前から満たせないものは、tuner を空けても EPG の取得はされないので見ない。
func keepsIdleTuners(free []*assign.Tuner, tuner *assign.Tuner, opts *PlanOptions) bool {
	for _, need := range opts.IdleNeeds {
		if !need.Contains(opts.TunerNames[tuner.Index]) {
			continue
		}
		idle := 0
		for _, f := range free {
			if need.Contains(opts.TunerNames[f.Index]) {
				idle++
			}
		}
		for _, name := range opts.OtherIdleTuners {
			if need.Contains(name) {
				idle++
			}
		}
		if idle >= need.Idle && idle-1 < need.Idle {
			return false
		}
	}

	return true
}
