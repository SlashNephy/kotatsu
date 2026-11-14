// Package lineup は、放送の NIT から局の顔ぶれの変化を見つけ、チューナーサーバーのスキャンし直しを促す (#1519)。
//
// Kotatsu はチャンネルの設定を書き換えない。チャンネルの持ち主は、チューナーサーバーだからである。
package lineup

import (
	"math"
	"slices"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// Kind は、衛星の種別。
type Kind string

const (
	KindBS Kind = "BS"
	KindCS Kind = "CS"
)

// serviceTypeTV は、テレビのサービスの service_type (ARIB STD-B10)。比べるのは、テレビのサービスだけである。
const serviceTypeTV = 0x01

// 中継器の周波数の割り当て (ARIB STD-B20)。番号から周波数を出し、NIT の周波数と比べる。
const (
	bsFirstMHz   = 11_727.48
	bsSpacingMHz = 38.36
	bsMaxNumber  = 23
	csFirstMHz   = 12_291.0
	csSpacingMHz = 40.0
	csMaxNumber  = 24
	// FrequencyToleranceMHz は、計算した中継器の周波数と、NIT の周波数の差を、同じ中継器とみなす幅。
	// NIT の BCD の単位 (10 kHz) より十分大きく、中継器の間隔 (38.36 MHz、40 MHz) の半分より十分小さくする。
	FrequencyToleranceMHz = 2.0
)

// uhdTransponders は、4K・8K (ISDB-S3) の中継器。構成から除く。
func uhdTransponders() []int {
	return []int{7, 17}
}

// Transponder は、kind の中継器の番号を、周波数 (kHz) から求める。表に当てはまらない周波数と、
// 4K・8K の中継器 (BS-7、BS-17) なら false を返す。
func Transponder(kind Kind, frequencyKHz int) (int, bool) {
	mhz := float64(frequencyKHz) / 1000
	first, spacing, start, step, maxNumber := bsFirstMHz, bsSpacingMHz, 1, 2, bsMaxNumber
	if kind == KindCS {
		first, spacing, start, step, maxNumber = csFirstMHz, csSpacingMHz, 2, 2, csMaxNumber
	}
	index := int(math.Round((mhz - first) / spacing))
	if index < 0 {
		return 0, false
	}
	number := start + index*step
	if number > maxNumber || math.Abs(mhz-(first+float64(index)*spacing)) > FrequencyToleranceMHz {
		return 0, false
	}
	if kind == KindBS && slices.Contains(uhdTransponders(), number) {
		return 0, false
	}

	return number, true
}

// Stream は、NIT[actual] で読んだ、1 つのトランスポートストリーム。
type Stream struct {
	TransportStreamID uint16
	// FrequencyKHz は、衛星分配システム記述子の周波数。
	FrequencyKHz int
	// Services は、サービスリスト記述子に並ぶサービスの、service_id と service_type。
	Services []Service
}

// Service は、サービスの service_id と service_type。
type Service struct {
	ServiceID   uint16
	ServiceType uint8
}

// Observation は、ある時点で読んだ、1 つのネットワークの NIT[actual] の構成。
type Observation struct {
	Kind      Kind
	NetworkID uint16
	Version   uint8
	Streams   []Stream
}

// Known は、チューナーサーバーが返すテレビのサービス。
type Known struct {
	NetworkID uint16
	ServiceID uint16
}

// Change は、放送とチューナーサーバーの、テレビのサービスの食い違い。
type Change struct {
	Kind      Kind
	NetworkID uint16
	// Added は、放送にあって、チューナーサーバーにないサービスの数。Removed は、その逆。
	Added   int
	Removed int
}

// DefaultRecheckInterval は、同じ version の食い違いを 2 回目と数えるまでに、あける時間。
// 1 回の読み取りの周回で、別の中継器を続けて読む時間より長くする。短すぎると、混ざった古い情報を 2 回読んで信じる。
const DefaultRecheckInterval = 30 * time.Minute

// key は、比べる単位 (種別とネットワーク)。
type key struct {
	kind      Kind
	networkID uint16
}

// pending は、初めて見た食い違いと、その version と時刻。
type pending struct {
	version uint8
	seenAt  time.Time
}

// Comparer は、放送の NIT[actual] とチューナーサーバーの一覧を比べ、食い違いを知らせるかを決める。
//
//   - version_number を (種別、network_id) ごとに「確かめ済み」として覚え、同じ version の NIT が来たら比べるのを省く。
//   - 食い違いを初めて見たら、その version と時刻を覚えるだけで、知らせない。同じ version の食い違いを、
//     間隔 (interval) 以上あけてもう一度読んだら、知らせる。放送に古い構成の情報が混ざることがあるからである。
//     間隔より前に読んだものは、2 回目に数えない。version が変わったら、数え直す。
//   - 読めたトランスポートストリームが 1 つもない構成は使わない。読み違いで、全部が消えたと判断しないためである。
//
// 比べるのは 1 つずつ順に行う。2 つの経路から同時に来ても、二重に知らせないためである。
type Comparer struct {
	clock    clock.Clock
	interval time.Duration
	mu       sync.Mutex
	checked  map[key]uint8
	pending  map[key]pending
}

// NewComparer は、Comparer を返す。interval が 0 以下なら DefaultRecheckInterval を使う。
func NewComparer(clk clock.Clock, interval time.Duration) *Comparer {
	if interval <= 0 {
		interval = DefaultRecheckInterval
	}

	return &Comparer{clock: clk, interval: interval, mu: sync.Mutex{}, checked: map[key]uint8{}, pending: map[key]pending{}}
}

// Observe は、読んだ構成 obs を、チューナーサーバーのテレビのサービス known と比べ、知らせる食い違いがあれば返す。
// known は、obs と同じ種別のサービスだけでよい (ほかのネットワークのものは、無視する)。
func (c *Comparer) Observe(obs *Observation, known []Known) (*Change, bool) {
	broadcast := tvServices(obs)
	if len(broadcast) == 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	k := key{kind: obs.Kind, networkID: obs.NetworkID}
	if version, ok := c.checked[k]; ok && version == obs.Version {
		return nil, false
	}
	change := compare(obs, broadcast, known)
	if change.Added == 0 && change.Removed == 0 {
		c.checked[k] = obs.Version
		delete(c.pending, k)

		return nil, false
	}
	now := c.clock.Now()
	first, ok := c.pending[k]
	if !ok || first.version != obs.Version {
		c.pending[k] = pending{version: obs.Version, seenAt: now}

		return nil, false
	}
	if now.Sub(first.seenAt) < c.interval {
		return nil, false
	}
	c.checked[k] = obs.Version
	delete(c.pending, k)

	return change, true
}

// Forget は、チューナーサーバーの一覧が変わったとき (スキャンし直したときなど) に、確かめ済みの version を忘れ、比べ直す。
func (c *Comparer) Forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.checked)
	clear(c.pending)
}

// tvServices は、obs の、表に当てはまる中継器のテレビのサービスの service_id を返す。
func tvServices(obs *Observation) map[uint16]bool {
	services := map[uint16]bool{}
	for _, stream := range obs.Streams {
		if _, ok := Transponder(obs.Kind, stream.FrequencyKHz); !ok {
			continue
		}
		for _, service := range stream.Services {
			if service.ServiceType == serviceTypeTV {
				services[service.ServiceID] = true
			}
		}
	}

	return services
}

// compare は、放送のテレビのサービス broadcast と、チューナーサーバーの known を比べる。
func compare(obs *Observation, broadcast map[uint16]bool, known []Known) *Change {
	registered := map[uint16]bool{}
	for _, service := range known {
		if service.NetworkID == obs.NetworkID {
			registered[service.ServiceID] = true
		}
	}
	change := &Change{Kind: obs.Kind, NetworkID: obs.NetworkID, Added: 0, Removed: 0}
	for id := range broadcast {
		if !registered[id] {
			change.Added++
		}
	}
	for id := range registered {
		if !broadcast[id] {
			change.Removed++
		}
	}

	return change
}
