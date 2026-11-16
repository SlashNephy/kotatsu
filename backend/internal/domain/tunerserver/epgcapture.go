package tunerserver

import (
	"context"
	"slices"
	"strings"
	"time"
)

// EPGCapture は、チューナーサーバー自身の EPG の取得 (EDCB) のために、ある時刻に空けておくチューナーを決める設定 (#1856、#1873)。
// EDCB の EpgTimerSrv は、EPG の取得の時刻に、取得に使う BonDriver ごとに決めた数の空きチューナーがなければ、その回の取得をしない
// (ReserveManager.cpp の CheckEpgCap、GetEpgCapTunerList)。
type EPGCapture struct {
	// Known は、取得の時刻と、取得に使うチューナーの数が分かるかどうか (EDCB では EpgTimerSrv.ini を読めたか)。
	// 偽なら、ほかの項目は使わない。
	Known bool
	// Near は、その時刻が、取得の時刻の前後 (空けておく時間帯) にあるかどうか。
	Near bool
	// AnyBonDriver は、取得に使う BonDriver のどれかに空きチューナーが 1 本あれば取得するかどうか (EDCB の NGEpgCapTime が 0)。
	// 偽なら、取得に使う BonDriver ごとに、BonDrivers の数だけ空きチューナーが要る。
	AnyBonDriver bool
	// BonDrivers は、BonDriver (Tuner の Name を小文字にしたもの) ごとの、取得に使うチューナーの数。
	// 載っていない BonDriver は DefaultBonDriver に従う。
	BonDrivers map[string]EPGCaptureBonDriver
	// DefaultBonDriver は、BonDrivers に載っていない BonDriver の、取得に使うチューナーの数。
	DefaultBonDriver EPGCaptureBonDriver
}

// EPGCaptureBonDriver は、1 つの BonDriver の、EPG の取得に使うチューナーの数。
type EPGCaptureBonDriver struct {
	// Disabled は、この BonDriver を EPG の取得に使わないかどうか (EDCB の GetEpg が 0)。
	Disabled bool
	// Tuners は、取得に使うチューナーの数。0 なら、その BonDriver のすべてのチューナー (EDCB の EPGCount の 0)。
	Tuners int
}

// EPGCaptureNeed は、空けておくチューナー。TunerNames のチューナーのうち、Idle 本が空いていなければならない。
type EPGCaptureNeed struct {
	// TunerNames は、まとまりに入るチューナーの名前 (小文字にしたもの)。
	TunerNames []string
	Idle       int
}

// Contains は、名前が name のチューナーがまとまりに入るかを返す。
func (n *EPGCaptureNeed) Contains(name string) bool {
	return slices.Contains(n.TunerNames, strings.ToLower(name))
}

// Needs は、tunerNames (チューナーサーバーのすべてのチューナーの名前。チューナーごとに 1 つ) に対して、空けておくチューナーを返す。
//   - Known が偽なら、取得の時刻も数も分からないので、BonDriver ごとに 1 本を、時刻を問わずに空けておく。
//   - Near が偽なら、空けておかない。
//   - AnyBonDriver なら、取得に使う BonDriver のチューナーのうち 1 本を空けておく。
//   - それ以外は、取得に使う BonDriver ごとに、その数 (0 なら BonDriver のすべてのチューナー) を空けておく。
func (c *EPGCapture) Needs(tunerNames []string) []*EPGCaptureNeed {
	counts := map[string]int{}
	order := []string{}
	for _, name := range tunerNames {
		name = strings.ToLower(name)
		if counts[name] == 0 {
			order = append(order, name)
		}
		counts[name]++
	}
	needs := []*EPGCaptureNeed{}
	if !c.Known {
		for _, name := range order {
			needs = append(needs, &EPGCaptureNeed{TunerNames: []string{name}, Idle: 1})
		}

		return needs
	}
	if !c.Near {
		return needs
	}
	capturing := []string{}
	for _, name := range order {
		bonDriver, ok := c.BonDrivers[name]
		if !ok {
			bonDriver = c.DefaultBonDriver
		}
		if bonDriver.Disabled {
			continue
		}
		capturing = append(capturing, name)
		if c.AnyBonDriver {
			continue
		}
		// 数がチューナーより多い BonDriver は、EpgTimerSrv が取得をしないので、空けても取得されない (満たせない数のまま返す)。
		idle := bonDriver.Tuners
		if idle <= 0 {
			idle = counts[name]
		}
		needs = append(needs, &EPGCaptureNeed{TunerNames: []string{name}, Idle: idle})
	}
	if c.AnyBonDriver && len(capturing) > 0 {
		needs = append(needs, &EPGCaptureNeed{TunerNames: capturing, Idle: 1})
	}

	return needs
}

// EPGCaptureReporter は、自分で EPG を取得し、そのためにチューナーを空けておく必要のあるチューナーサーバー (EDCB) (#1856)。
// Mirakurun 互換のサーバーは、EPG の取得をタイムシフトと同じ優先度の受け手として扱うので満たさない。
type EPGCaptureReporter interface {
	EPGCapture(ctx context.Context, at time.Time) (*EPGCapture, error)
}
