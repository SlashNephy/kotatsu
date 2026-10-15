// Package assign は、予約をチューナーに割り当てる (設計書 4.5)。
//
// 割り当ては、予約の集合、チューナーの構成、現在時刻だけから決まる純粋な関数で、DB には依存しない。
// neko は、予約や EPG やチューナーの構成が変わるたびに、割り当てを毎回すべて計算し直す。
package assign

import (
	"cmp"
	"slices"
	"time"

	"github.com/google/uuid"
)

// State は、予約の割り当ての結果 (設計書 4.5)。
type State string

const (
	// StateOK は、全編を録画できる見込みであること。
	StateOK State = "ok"
	// StatePartial は、本編の一部が欠ける見込みであること。
	StatePartial State = "partial"
	// StateConflict は、チューナーが足りないこと。録画は優先度 1 で試み、チューナーが空いていれば録る。
	StateConflict State = "conflict"
)

// Reservation は、割り当てる予約。
type Reservation struct {
	ID uuid.UUID
	// ChannelType と Channel は、受信するチャンネル。同じチャンネルの予約どうしは 1 本のチューナーを共有する。
	ChannelType string
	Channel     string
	// StartAt と EndAt は本編の時刻。本編は削らない。
	StartAt time.Time
	EndAt   time.Time
	// StartMargin と EndMargin は、本編の前後に足して録画する時間。隣の録画と重なるときは削る。
	StartMargin time.Duration
	EndMargin   time.Duration
	// Priority は予約の優先度。高い予約から順に割り当てる。
	Priority int
	// Recording は、録画がすでに始まっているかどうか。始まっている予約は、Tuner のチューナーから動かさない。
	Recording bool
	// Tuner は、録画が始まっている予約が使っているチューナー。始まっていなければ無視する。
	Tuner int
}

func (r *Reservation) channel() channelKey {
	return channelKey{channelType: r.ChannelType, channel: r.Channel}
}

func (r *Reservation) fullStart() time.Time { return r.StartAt.Add(-r.StartMargin) }
func (r *Reservation) fullEnd() time.Time   { return r.EndAt.Add(r.EndMargin) }

// Tuner は、割り当てに使えるチューナー。
type Tuner struct {
	Index int
	// Types は、受信できるチャンネル種別。
	Types []string
}

// Assignment は、予約 1 件の割り当ての結果。
type Assignment struct {
	ReservationID uuid.UUID
	State         State
	// Tuner は、割り当てたチューナー。競合した予約では -1。
	Tuner int
	// PlannedStart と PlannedEnd は、隣の録画と重なるマージンを削った後の、録画する予定の時刻。
	// 競合した予約では、マージンを含めた元の時刻。
	PlannedStart time.Time
	PlannedEnd   time.Time
}

// NoTuner は、チューナーを割り当てられなかったことを表す Assignment.Tuner の値。
const NoTuner = -1

type channelKey struct {
	channelType string
	channel     string
}

// demand は、1 本のチューナーを共有する予約のまとまり。同じチャンネルで時間の重なる予約をまとめる。
type demand struct {
	channel      channelKey
	reservations []*Reservation
	end          time.Time
	// recording は、録画が始まった予約を含むかどうか。含むなら、その予約のチューナーから動かさない。
	recording bool
	tuner     int
}

// key は、並べ替えのキーにする予約 (優先度が最も高く、開始が最も早く、ID が最も小さいもの)。
func (d *demand) key() *Reservation {
	return slices.MinFunc(d.reservations, compareReservations)
}

func compareReservations(a, b *Reservation) int {
	return cmp.Or(
		cmp.Compare(b.Priority, a.Priority),
		a.StartAt.Compare(b.StartAt),
		slices.Compare(a.ID[:], b.ID[:]),
	)
}

// placed は、チューナーに置いた予約。
type placed struct {
	reservation *Reservation
	// start と end は、録画する予定の時刻。マージンや本編を削ると縮む。
	start, end time.Time
	// rank は、置いた順。小さいほど優先度が高い。
	rank int
}

// hardStart と hardEnd は、他の予約に譲れない区間。録画が始まった予約は、すでに録った開始マージンを含めて、録画の始まりから本編の終わりまでになる。
// 終了マージンは、録画の前と同じく、後の録画に譲る。録画が始まったときに戻すと、後の録画の冒頭が欠ける (#520)。
func (p *placed) hardStart() time.Time {
	if p.reservation.Recording {
		return p.start
	}

	return p.reservation.StartAt
}

func (p *placed) hardEnd() time.Time {
	return p.reservation.EndAt
}

// Assign は、予約をチューナーに割り当てる。同じ入力からは、常に同じ結果を返す。
//
//  1. 各予約を、マージンを含めた時間区間に展開し、同じチャンネルで重なる区間を、1 本のチューナーを共有する需要にまとめる。
//  2. 需要を「優先度、開始時刻、予約 ID」の順に並べる。ただし、録画が始まった需要は、そのチューナーに先に置いて動かさない。
//  3. 先頭から順に、その種別を受信できるチューナーへ割り当てる。候補が複数あるときは、削られる録画時間が最も少ないチューナーを選ぶ。
//     本編を削らずに済むチューナーがあればそれを選び、なければ本編の削られる時間が最も少ないものを選ぶ (一部のみ)。
//     本編をすべて削られるチューナーしかなければ、競合とする。
//  4. 同じチューナーで別のチャンネルの予約が隣り合い、マージンどうしが重なるときは、前の録画の終了マージンを先に削り、
//     次に後の録画の開始マージンを削る。マージンだけで重なるなら、本編は削らない。
//
// 戻り値は、予約と同じ順に並べる。
func Assign(reservations []*Reservation, tuners []*Tuner) []*Assignment {
	demands := group(reservations)
	slices.SortStableFunc(demands, func(a, b *demand) int {
		// 録画が始まった需要を先に置く。
		if a.recording != b.recording {
			if a.recording {
				return -1
			}

			return 1
		}

		return compareReservations(a.key(), b.key())
	})

	byTuner := map[int][]*placed{}
	results := map[uuid.UUID]*Assignment{}
	for rank, d := range demands {
		tuner, ok := choose(d, tuners, byTuner)
		if !ok {
			for _, r := range d.reservations {
				results[r.ID] = &Assignment{ReservationID: r.ID, State: StateConflict, Tuner: NoTuner, PlannedStart: r.fullStart(), PlannedEnd: r.fullEnd()}
			}

			continue
		}
		for _, r := range d.reservations {
			byTuner[tuner] = append(byTuner[tuner], &placed{reservation: r, start: r.fullStart(), end: r.fullEnd(), rank: rank})
		}
	}

	for tuner, list := range byTuner {
		trim(list)
		for _, p := range list {
			r := p.reservation
			state := StateOK
			switch {
			case !p.start.Before(p.end):
				// 先に置いた予約に、録画する時間をすべて取られた。
				results[r.ID] = &Assignment{ReservationID: r.ID, State: StateConflict, Tuner: NoTuner, PlannedStart: r.fullStart(), PlannedEnd: r.fullEnd()}

				continue
			case p.start.After(r.StartAt) || p.end.Before(r.EndAt):
				state = StatePartial
			}
			results[r.ID] = &Assignment{ReservationID: r.ID, State: state, Tuner: tuner, PlannedStart: p.start, PlannedEnd: p.end}
		}
	}

	assignments := make([]*Assignment, 0, len(reservations))
	for _, r := range reservations {
		assignments = append(assignments, results[r.ID])
	}

	return assignments
}

// group は、同じチャンネルで時間の重なる予約を、需要にまとめる。
func group(reservations []*Reservation) []*demand {
	sorted := slices.Clone(reservations)
	slices.SortFunc(sorted, func(a, b *Reservation) int {
		return cmp.Or(
			cmp.Compare(a.ChannelType, b.ChannelType),
			cmp.Compare(a.Channel, b.Channel),
			a.fullStart().Compare(b.fullStart()),
			slices.Compare(a.ID[:], b.ID[:]),
		)
	})

	demands := []*demand{}
	var current *demand
	for _, r := range sorted {
		if current != nil && current.channel == r.channel() && r.fullStart().Before(current.end) {
			current.reservations = append(current.reservations, r)
			current.end = later(current.end, r.fullEnd())
		} else {
			current = &demand{channel: r.channel(), reservations: []*Reservation{r}, end: r.fullEnd(), recording: false, tuner: NoTuner}
			demands = append(demands, current)
		}
		if r.Recording {
			current.recording = true
			current.tuner = r.Tuner
		}
	}

	return demands
}

// choose は、需要を置くチューナーを選ぶ。置けるチューナーがなければ false を返す。
func choose(d *demand, tuners []*Tuner, byTuner map[int][]*placed) (int, bool) {
	if d.recording {
		return d.tuner, true
	}

	type candidate struct {
		tuner            int
		coreCut, padding time.Duration
	}
	var best *candidate
	for _, tuner := range tuners {
		if !slices.Contains(tuner.Types, d.channel.channelType) {
			continue
		}
		coreCut, padding, ok := lossOn(d, byTuner[tuner.Index])
		if !ok {
			continue
		}
		c := &candidate{tuner: tuner.Index, coreCut: coreCut, padding: padding}
		if best == nil || cmp.Or(cmp.Compare(c.coreCut, best.coreCut), cmp.Compare(c.padding, best.padding), cmp.Compare(c.tuner, best.tuner)) < 0 {
			best = c
		}
	}
	if best == nil {
		return NoTuner, false
	}

	return best.tuner, true
}

// lossOn は、需要をチューナーに置いたときに削られる本編の時間とマージンの時間を返す。
// 先に置いた予約は優先度が同じか高いので、本編が重なれば、この需要の本編を削る。
// どれかの予約の本編がすべて削られるなら、置けないので false を返す。
func lossOn(d *demand, existing []*placed) (time.Duration, time.Duration, bool) {
	// 別のチャンネルの予約が譲れない区間の和。同じ時間に同じチャンネルの予約がいくつあっても、二重には数えない。
	others := []*placed{}
	for _, p := range existing {
		// 同じチャンネルは、チューナーを共有するので削らない。
		if p.reservation.channel() != d.channel {
			others = append(others, p)
		}
	}
	hard := union(others)

	var coreCut, padding time.Duration
	for _, r := range d.reservations {
		var cut time.Duration
		for _, span := range hard {
			cut += overlap(r.StartAt, r.EndAt, span.start, span.end)
		}
		if cut >= r.EndAt.Sub(r.StartAt) {
			return 0, 0, false
		}
		coreCut += cut
		for _, p := range others {
			padding += marginLoss(r, p)
		}
	}

	return coreCut, padding, true
}

type span struct {
	start, end time.Time
}

// union は、予約が譲れない区間を、重なりをまとめた区間の列にする。
func union(list []*placed) []span {
	spans := make([]span, 0, len(list))
	for _, p := range list {
		spans = append(spans, span{start: p.hardStart(), end: p.hardEnd()})
	}
	slices.SortFunc(spans, func(a, b span) int { return a.start.Compare(b.start) })

	merged := make([]span, 0, len(spans))
	for _, s := range spans {
		if n := len(merged); n > 0 && !s.start.After(merged[n-1].end) {
			merged[n-1].end = later(merged[n-1].end, s.end)

			continue
		}
		merged = append(merged, s)
	}

	return merged
}

// overlap は、2 つの区間の重なる長さを返す。
func overlap(aStart, aEnd, bStart, bEnd time.Time) time.Duration {
	return max(0, earlier(aEnd, bEnd).Sub(later(aStart, bStart)))
}

// marginLoss は、予約 r と、先に置いた予約 p のマージンが重なるとき、r のマージンから削られる時間を返す。
// 前の録画の終了マージンを先に削り、足りなければ後の録画の開始マージンを削る。
func marginLoss(r *Reservation, p *placed) time.Duration {
	if r.StartAt.Before(p.reservation.StartAt) {
		// r が前。r の終了マージンを先に削る。
		return min(max(0, r.fullEnd().Sub(p.start)), r.EndMargin)
	}
	// r が後。前の p の終了マージンを先に削り、残りを r の開始マージンから削る。
	// 録画が始まった予約も、終了マージンは譲る (#520)。
	pTrimmable := max(0, p.end.Sub(later(p.reservation.EndAt, r.fullStart())))

	return min(max(0, p.end.Sub(r.fullStart())-pTrimmable), r.StartMargin)
}

// trim は、同じチューナーに置いた別のチャンネルの予約の区間が重ならないように削る。録画が始まった予約は、終了マージンだけを削る (#520)。
//
//  1. 本編どうしが重なるときは、後から置いた方 (優先度の低い方) の録画する時間を、相手の本編の手前か後ろまで削る。
//  2. マージンが重なるときは、前の録画の終了マージンを先に削り、次に後の録画の開始マージンを削る。
func trim(list []*placed) {
	slices.SortFunc(list, func(a, b *placed) int {
		return cmp.Or(a.reservation.StartAt.Compare(b.reservation.StartAt), slices.Compare(a.reservation.ID[:], b.reservation.ID[:]))
	})
	resolveCores(list)
	trimMargins(list)
}

// conflicting は、同じチューナーに置いた別のチャンネルの予約の、録画する時間が重なるかどうかを返す。
// 録画する時間がなくなった予約は、競合になるので数えない。
func conflicting(a, b *placed) bool {
	return a.reservation.channel() != b.reservation.channel() &&
		a.start.Before(a.end) && b.start.Before(b.end) &&
		a.start.Before(b.end) && b.start.Before(a.end)
}

// resolveCores は、本編どうしが重なる予約のうち、後から置いた方の録画する時間を、相手の本編の手前か後ろまで削る。
// 本編はマージンより優先するので、残った本編に重なる相手のマージンも削る。
func resolveCores(list []*placed) {
	for i, a := range list {
		for _, b := range list[i+1:] {
			if !conflicting(a, b) || overlap(a.hardStart(), a.hardEnd(), b.hardStart(), b.hardEnd()) == 0 {
				continue
			}
			loserIsB := b.rank > a.rank
			if b.reservation.Recording {
				loserIsB = false
			}
			if a.reservation.Recording {
				loserIsB = true
			}
			if loserIsB {
				b.start = later(b.start, a.hardEnd())
				// 録画が始まった予約も、終了マージンは、残った相手の本編に譲る (#520)。
				a.end = earlier(a.end, later(b.start, a.hardEnd()))
			} else {
				a.end = earlier(a.end, b.hardStart())
				if !b.reservation.Recording {
					b.start = later(b.start, earlier(a.end, b.hardStart()))
				}
			}
		}
	}
}

// trimMargins は、マージンが重なる予約の、前の録画の終了マージンを先に削り、次に後の録画の開始マージンを削る。
func trimMargins(list []*placed) {
	// 前と後は、本編の順で決める。本編を削った予約は本編の始まりが後ろにずれるので、削った後の本編の始まりで並べ直す。
	coreStart := func(p *placed) time.Time { return later(p.reservation.StartAt, p.start) }
	slices.SortFunc(list, func(a, b *placed) int {
		return cmp.Or(coreStart(a).Compare(coreStart(b)), slices.Compare(a.reservation.ID[:], b.reservation.ID[:]))
	})
	for i, a := range list {
		for _, b := range list[i+1:] {
			if !conflicting(a, b) {
				continue
			}
			// 録画が始まった予約も、終了マージンは削る (#520)。
			a.end = later(earlier(a.reservation.EndAt, a.end), b.start)
			if b.start.Before(a.end) && !b.reservation.Recording {
				b.start = earlier(later(b.reservation.StartAt, b.start), a.end)
			}
		}
	}
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}

	return b
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}

	return b
}
