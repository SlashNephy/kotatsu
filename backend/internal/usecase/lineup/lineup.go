// Package lineup は、放送の NIT から局の顔ぶれの変化を見つけ、チューナーサーバーのスキャンし直しを促す (#1519)。
//
// Kotatsu はチャンネルの設定を書き換えない。チャンネルの持ち主は、チューナーサーバーだからである。
package lineup

import (
	"fmt"
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

// serviceTypeTV は、テレビのサービスの service_type (ARIB STD-B10)。比較するのは、テレビのサービスだけである。
const serviceTypeTV = 0x01

// 中継器の周波数の割り当て (ARIB STD-B20)。番号から周波数を出し、NIT の周波数と比較する。
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

// uhdTransponderBS7 と uhdTransponderBS17 は、4K・8K (ISDB-S3) の中継器。構成から除く。
const (
	uhdTransponderBS7  = 7
	uhdTransponderBS17 = 17
)

// Transponder は、kind の中継器の番号を、周波数 (kHz) から求める。テーブルに当てはまらない周波数と、
// 4K・8K の中継器 (BS-7、BS-17) なら false を返す。
func Transponder(kind Kind, frequencyKHz int) (int, bool) {
	mhz := float64(frequencyKHz) / 1000
	var first, spacing float64
	var start, maxNumber int
	switch kind {
	case KindBS:
		first, spacing, start, maxNumber = bsFirstMHz, bsSpacingMHz, 1, bsMaxNumber
	case KindCS:
		first, spacing, start, maxNumber = csFirstMHz, csSpacingMHz, 2, csMaxNumber
	default:
		return 0, false
	}
	// BS は奇数、CS は偶数の番号を使う。
	const step = 2
	index := int(math.Round((mhz - first) / spacing))
	if index < 0 {
		return 0, false
	}
	number := start + index*step
	if number > maxNumber || math.Abs(mhz-(first+float64(index)*spacing)) > FrequencyToleranceMHz {
		return 0, false
	}
	if kind == KindBS && (number == uhdTransponderBS7 || number == uhdTransponderBS17) {
		return 0, false
	}

	return number, true
}

// Stream は、NIT[actual] で読んだ、1 つのトランスポートストリーム。
type Stream struct {
	TransportStreamID uint16 `json:"transportStreamId"`
	// FrequencyKHz は、衛星分配システム記述子の周波数。
	FrequencyKHz int `json:"frequencyKHz"`
	// Services は、サービスリスト記述子に並ぶサービスの、service_id と service_type。
	Services []Service `json:"services"`
}

// Service は、サービスの service_id と service_type。
type Service struct {
	ServiceID   uint16 `json:"serviceId"`
	ServiceType uint8  `json:"serviceType"`
	// Name は、SDT のサービス記述子のサービス名。NIT から読んだ衛星のサービスと、前に書いた構成では空 (#2034)。
	Name string `json:"name,omitempty"`
}

// Observation は、ある時点で読んだ、1 つのネットワークの NIT[actual] の構成。
type Observation struct {
	Kind      Kind
	NetworkID uint16
	Version   uint8
	Streams   []Stream
	// ObservedAt は、放送から読んだ時刻。零値なら、比較した時刻を使う。DB に書いたものを読み直して比較するときは、
	// 読み直した時刻ではなく、放送から読んだ時刻で、間隔をあけて 2 回読んだかを決める。
	ObservedAt time.Time
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
	// AddedServiceIDs と RemovedServiceIDs は、Added と Removed のサービスの service_id の昇順 (#2034)。
	AddedServiceIDs   []uint16
	RemovedServiceIDs []uint16
}

// DefaultRecheckInterval は、同じ version の食い違いを 2 回目と数えるまでに、あける時間。
// 1 回の読み取りの周回で、別の中継器を続けて読む時間より長くする。短すぎると、混ざった古い情報を 2 回読んで信じる。
const DefaultRecheckInterval = 30 * time.Minute

// key は、比較する単位 (種別とネットワーク)。
type key struct {
	kind      Kind
	networkID uint16
}

// pending は、初めて見た食い違いと、その構成の指紋と時刻。
type pending struct {
	signature string
	seenAt    time.Time
}

// Comparer は、放送の NIT[actual] とチューナーサーバーの一覧を比較し、食い違いを通知するかを決める。
//
//   - version_number を (種別、network_id) ごとに「確かめ済み」として保存し、同じ version の NIT が来たら比較するのを省く。
//   - 食い違いを初めて見たら、その version と時刻を保存するだけで、通知しない。同じ version の食い違いを、
//     間隔 (interval) 以上あけてもう一度読んだら、通知する。放送に古い構成の情報が混ざることがあるからである。
//     間隔より前に読んだものは、2 回目に数えない。version が変わったら、数え直す。
//   - 読めたトランスポートストリームが 1 つもない構成は使わない。読み違いで、全部が消えたと判断しないためである。
//
// 比較するのは 1 つずつ順に行う。2 つの経路から同時に来ても、二重に通知しないためである。
type Comparer struct {
	clock    clock.Clock
	interval time.Duration
	mu       sync.Mutex
	// checked は、確認済みの構成の指紋。指紋は、version と、比較するテレビのサービスの集合から作る。
	// 番組のないサービス (#2034) が変わると、version が同じでも比較するサービスが変わるので、比較し直す。
	checked map[key]string
	pending map[key]pending
}

// NewComparer は、Comparer を返す。interval が 0 以下なら DefaultRecheckInterval を使う。
func NewComparer(clk clock.Clock, interval time.Duration) *Comparer {
	if interval <= 0 {
		interval = DefaultRecheckInterval
	}

	return &Comparer{clock: clk, interval: interval, mu: sync.Mutex{}, checked: map[key]string{}, pending: map[key]pending{}}
}

// Observe は、読んだ構成 obs を、チューナーサーバーのテレビのサービス known と比較し、通知する食い違いがあれば返す。
// known は、obs と同じ種別のサービスだけでよい (ほかのネットワークのものは、無視する)。
func (c *Comparer) Observe(obs *Observation, known []Known) (*Change, bool) {
	if obs == nil {
		return nil, false
	}
	broadcast := tvServices(obs)
	if len(broadcast) == 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	k := key{kind: obs.Kind, networkID: obs.NetworkID}
	sig := signatureOf(obs.Version, broadcast)
	if signature, ok := c.checked[k]; ok && signature == sig {
		return nil, false
	}
	change := compare(obs, broadcast, known)
	if change.Added == 0 && change.Removed == 0 {
		c.checked[k] = sig
		delete(c.pending, k)

		return nil, false
	}
	now := obs.ObservedAt
	if now.IsZero() {
		now = c.clock.Now()
	}
	first, ok := c.pending[k]
	if !ok || first.signature != sig {
		c.pending[k] = pending{signature: sig, seenAt: now}

		return nil, false
	}
	if now.Sub(first.seenAt) < c.interval {
		return nil, false
	}
	c.checked[k] = sig
	delete(c.pending, k)

	return change, true
}

// Unconfirm は、Observe が通知すると決めた obs を、通知できなかったときに、まだ通知していないことに戻す。
// 同じ version を次に読んだら、間隔を待たずに、もう一度知らせると決める。2 回読んで信じたことは変わらないからである。
func (c *Comparer) Unconfirm(obs *Observation) {
	if obs == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	k := key{kind: obs.Kind, networkID: obs.NetworkID}
	sig := signatureOf(obs.Version, tvServices(obs))
	if signature, ok := c.checked[k]; !ok || signature != sig {
		return
	}
	delete(c.checked, k)
	// 1 回目を、間隔より十分前に読んだことにする。
	c.pending[k] = pending{signature: sig, seenAt: time.Time{}}
}

// Forget は、チューナーサーバーの一覧が変わったとき (スキャンし直したときなど) に、確認済みの version を忘れ、比較し直す。
func (c *Comparer) Forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.checked)
	clear(c.pending)
}

// tvServices は、obs の、テーブルに当てはまる中継器のテレビのサービスの service_id を返す。
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

// signatureOf は、version と、比較するテレビのサービスの集合 broadcast から、構成の指紋を作る。
func signatureOf(version uint8, broadcast map[uint16]bool) string {
	ids := make([]int, 0, len(broadcast))
	for id := range broadcast {
		ids = append(ids, int(id))
	}
	slices.Sort(ids)

	return fmt.Sprint(version, ids)
}

// WithoutIdle は、obs から、番組のないサービス idle (#2034) のうち、チューナーサーバーの known にないものを除いた構成を返す。
// obs は変えない。NIT に載っていても、放送していないサービスは、チューナーサーバーが登録しないので、新しい局と数えない。
// known にあるサービスは除かない。除くと、登録済みのサービスに番組のない間、終わった局と数えてしまうからである。
func WithoutIdle(obs *Observation, idle map[Known]bool, known []Known) *Observation {
	if obs == nil || len(idle) == 0 {
		return obs
	}
	registered := map[Known]bool{}
	for _, service := range known {
		registered[service] = true
	}
	out := *obs
	out.Streams = make([]Stream, 0, len(obs.Streams))
	for _, stream := range obs.Streams {
		services := make([]Service, 0, len(stream.Services))
		for _, service := range stream.Services {
			k := Known{NetworkID: obs.NetworkID, ServiceID: service.ServiceID}
			if !idle[k] || registered[k] {
				services = append(services, service)
			}
		}
		stream.Services = services
		out.Streams = append(out.Streams, stream)
	}

	return &out
}

// compare は、放送のテレビのサービス broadcast と、チューナーサーバーの known を比較する。
func compare(obs *Observation, broadcast map[uint16]bool, known []Known) *Change {
	registered := map[uint16]bool{}
	for _, service := range known {
		if service.NetworkID == obs.NetworkID {
			registered[service.ServiceID] = true
		}
	}
	change := &Change{
		Kind: obs.Kind, NetworkID: obs.NetworkID, Added: 0, Removed: 0, AddedServiceIDs: []uint16{}, RemovedServiceIDs: []uint16{},
	}
	for id := range broadcast {
		if !registered[id] {
			change.AddedServiceIDs = append(change.AddedServiceIDs, id)
		}
	}
	for id := range registered {
		if !broadcast[id] {
			change.RemovedServiceIDs = append(change.RemovedServiceIDs, id)
		}
	}
	slices.Sort(change.AddedServiceIDs)
	slices.Sort(change.RemovedServiceIDs)
	change.Added, change.Removed = len(change.AddedServiceIDs), len(change.RemovedServiceIDs)

	return change
}
