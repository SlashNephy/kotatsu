package stream

import (
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/ts"
)

// segment は、録画のセグメントと、その索引。
type segment struct {
	id uuid.UUID
	// poolID と path は、セグメントのファイルの、プール ID とプール内の相対パス。
	poolID string
	path   string
	// startedAt は、セグメントの始まり。録画の始まりからの時間は、最初のセグメントとの差で求める。
	startedAt time.Time
	// open は、セグメントを書いている途中かどうか。
	open bool
	// index は、セグメントの索引。索引がなければ空で、そのセグメントには始まりからしか入れない。
	index []ts.IndexEntry
}

// piece は、セグメントのうち、配信するバイトの範囲。
type piece struct {
	segment *segment
	// from は、配信を始めるバイトの位置。
	from int64
	// to は、配信を終えるバイトの位置 (含まない)。-1 なら、ファイルの終わりまで配信する。
	to int64
}

// plan は、始まりの順に並べた segments を、録画の始まりから start の位置から配信する範囲の並びにする。
//
// 引き継ぎで重なった区間は、後のセグメントで配信する (設計書 5.4)。
// 後のセグメントの最初の I フレームの PTS を P とすると、前のセグメントは、PTS が P 以上の最初の I フレームの手前で切り、
// 後のセグメントは、最初の I フレームから配信する。前のセグメントに P 以上の I フレームがなければ、前のセグメントは終わりまで配信する。
//
// start は、最も近い手前の I フレームに丸める。時間は、サムネイルのコマの時間と同じく、セグメントの始まりの差と、
// セグメントの中の PTS の差を足して求める。start より手前に I フレームがなければ、最初から配信する。
func plan(segments []*segment, start time.Duration) ([]*piece, time.Duration) {
	pieces := trim(segments)
	if len(pieces) == 0 {
		return pieces, 0
	}
	first, from, at := seek(pieces, segments[0].startedAt, start)
	pieces = pieces[first:]
	pieces[0].from = from

	return pieces, at
}

// keyframes は、trim した範囲の I フレームの、録画の始まりからの時間を、順に返す。索引のないセグメントは、その始まりだけを含める。
func keyframes(segments []*segment) []time.Duration {
	times := []time.Duration{}
	pieces := trim(segments)
	if len(pieces) == 0 {
		return times
	}
	origin := segments[0].startedAt
	for _, p := range pieces {
		base := p.segment.startedAt.Sub(origin)
		if len(p.segment.index) == 0 {
			times = append(times, base)

			continue
		}
		head := p.segment.index[0].PTS
		for _, entry := range p.segment.index {
			if entry.Offset >= p.from && (p.to < 0 || entry.Offset < p.to) {
				times = append(times, base+time.Duration(ts.PTSDiff(head, entry.PTS))*time.Second/ts.PTSClock)
			}
		}
	}

	return times
}

// trim は、セグメントごとに配信する範囲を決め、引き継ぎで重なった区間を切り詰める。
func trim(segments []*segment) []*piece {
	pieces := make([]*piece, 0, len(segments))
	for i, seg := range segments {
		p := &piece{segment: seg, from: 0, to: -1}
		if i > 0 && len(seg.index) > 0 && len(segments[i-1].index) > 0 {
			prev := pieces[i-1]
			first := seg.index[0]
			for _, entry := range segments[i-1].index {
				if entry.Offset >= prev.from && ts.PTSDiff(first.PTS, entry.PTS) >= 0 {
					prev.to = entry.Offset

					break
				}
			}
			p.from = first.Offset
		}
		pieces = append(pieces, p)
	}

	return pieces
}

// seek は、録画の始まり origin から start の位置に最も近い手前の I フレームを探し、その範囲の添え字と、バイト位置と、時間を返す。
// 配信しない範囲 (切り詰めた区間) の I フレームは使わない。索引のないセグメントには、始まりからしか入れない。
func seek(pieces []*piece, origin time.Time, start time.Duration) (int, int64, time.Duration) {
	best, bestOffset := 0, pieces[0].from
	var bestAt time.Duration
	found := false
	consider := func(i int, offset int64, at time.Duration) {
		if at <= start && (!found || at >= bestAt) {
			best, bestOffset, bestAt, found = i, offset, at, true
		}
	}
	for i, p := range pieces {
		base := p.segment.startedAt.Sub(origin)
		if len(p.segment.index) == 0 {
			consider(i, p.from, base)

			continue
		}
		head := p.segment.index[0].PTS
		for _, entry := range p.segment.index {
			if entry.Offset >= p.from && (p.to < 0 || entry.Offset < p.to) {
				consider(i, entry.Offset, base+time.Duration(ts.PTSDiff(head, entry.PTS))*time.Second/ts.PTSClock)
			}
		}
	}

	return best, bestOffset, bestAt
}
