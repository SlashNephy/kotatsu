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
	// startedAt は、セグメントの始まり。
	startedAt time.Time
	// endedAt は、セグメントを書き終えた時刻。書いている途中なら nil。
	endedAt *time.Time
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
// start は、最も近い手前の I フレームに丸める。時間は、途切れた区間を含めない再生の時間 (walk) で表す。
// start より手前に I フレームがなければ、最初から配信する。
func plan(segments []*segment, start time.Duration) ([]*piece, time.Duration) {
	pieces := trim(segments)
	if len(pieces) == 0 {
		return pieces, 0
	}
	first, from, at := seek(pieces, start)
	pieces = pieces[first:]
	pieces[0].from = from

	return pieces, at
}

// keyframes は、trim した範囲の I フレームの、録画の始まりからの再生の時間 (walk) を、順に返す。索引のないセグメントは、その始まりだけを含める。
func keyframes(segments []*segment) []time.Duration {
	times := []time.Duration{}
	walk(trim(segments), func(_ int, _ int64, at time.Duration, _ int64) {
		times = append(times, at)
	})

	return times
}

// Keyframe は、流す I フレームの、録画の始まりからの再生の時間と、PTS。
type Keyframe struct {
	At time.Duration
	// PTS は、放送局が付けた PTS (90 kHz)。索引のないセグメントの始まりでは、分からないので -1。
	PTS int64
}

// keyframePoints は、keyframes と同じ I フレームの、再生の時間と PTS を、順に返す。
func keyframePoints(segments []*segment) []Keyframe {
	points := []Keyframe{}
	walk(trim(segments), func(_ int, _ int64, at time.Duration, pts int64) {
		points = append(points, Keyframe{At: at, PTS: pts})
	})

	return points
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

// seek は、再生の時間で start の位置に最も近い手前の I フレームを探し、その範囲の添え字と、バイト位置と、時間を返す。
// 配信しない範囲 (切り詰めた区間) の I フレームは使わない。索引のないセグメントには、始まりからしか入れない。
func seek(pieces []*piece, start time.Duration) (int, int64, time.Duration) {
	best, bestOffset := 0, pieces[0].from
	var bestAt time.Duration
	walk(pieces, func(i int, offset int64, at time.Duration, _ int64) {
		// 時間は増えていくので、start までのうち最後のものが、最も近い手前の I フレーム。
		if at <= start {
			best, bestOffset, bestAt = i, offset, at
		}
	})

	return best, bestOffset, bestAt
}

// walk は、配信する I フレームを順にたどり、範囲の添え字と、バイト位置と、録画の始まりからの再生の時間と、PTS を fn に渡す。
// 索引のないセグメントの始まりは、PTS が分からないので -1 を渡す。
//
// 再生の時間は、途切れた区間を含めない (設計書 7.2)。録画の配信もトランスコードも、途切れた区間の前後をそのままつないで流すので、
// プレイヤーとエンコーダーの時間も、途切れた区間を含まないからである。
// 時間は、前の I フレームからの PTS の差を足して求める。差が ts.MaxKeyframeInterval を超えるか、PTS が戻ったら、間で途切れたので、
// 直前の I フレームの間隔だけ進める。セグメントをつないだファイルの中の途切れも、同じく詰める。
// 索引のないセグメントは、PTS が分からないので、前の I フレームの次の位置から始まり、書いていた時間だけ続くものとする。
func walk(pieces []*piece, fn func(i int, offset int64, at time.Duration, pts int64)) {
	// next は、PTS でつながらないときに、次の I フレームに付ける時間。
	var at, next time.Duration
	interval := ts.DefaultKeyframeInterval
	var prev int64
	chained := false
	for i, p := range pieces {
		seg := p.segment
		if len(seg.index) == 0 {
			at = next
			fn(i, p.from, at, -1)
			chained = false
			next = at + interval
			if seg.endedAt != nil {
				next = at + max(seg.endedAt.Sub(seg.startedAt), interval)
			}

			continue
		}
		for _, entry := range seg.index {
			if entry.Offset < p.from || (p.to >= 0 && entry.Offset >= p.to) {
				continue
			}
			t := next
			if chained {
				if d := time.Duration(ts.PTSDiff(prev, entry.PTS)) * time.Second / ts.PTSClock; d > 0 && d <= ts.MaxKeyframeInterval {
					interval, t = d, at+d
				}
			}
			at = t
			fn(i, entry.Offset, at, entry.PTS)
			prev, chained = entry.PTS, true
			next = at + interval
		}
	}
}
