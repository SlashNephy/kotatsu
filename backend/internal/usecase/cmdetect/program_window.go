package cmdetect

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

// Program は、録画した番組の、番組表の時刻 (#1506)。録画の前後のマージンには前後の番組が入るので、
// 番組の尺に収まるウィンドウを置き、ウィンドウの外のロゴを前後の番組のものとして破棄するのに使う。
type Program struct {
	// Start は、番組が始まるはずの時刻を、ファイルの始まりからの時間で表したもの。録画を番組より遅れて始めれば負になる。
	Start time.Duration
	// Duration は、番組の尺 (番組表の終わり − 始まり)。
	Duration time.Duration
	// Fixed なら、ウィンドウを探さずに、Start から Duration の長さで置く。セグメントが分かれた録画で、録画全体で置いたウィンドウを
	// セグメントごとに渡すのに使う (ProgramWindows)。ウィンドウはファイルの外にはみ出してよい。
	Fixed bool
}

const (
	// windowEdgeExclusion は、ウィンドウの置き場所の候補から除く、ファイルの始まりと終わりからの幅 (端の除外幅)。
	// ファイルの始まりで始まるロゴの区間は、前の番組から続くロゴかもしれず、そこにウィンドウの先頭を合わせる理由にならない。
	windowEdgeExclusion = 2 * time.Second
	// windowShiftWeight は、ウィンドウの置き場所が、番組が始まるはずの時刻からずれた 1 秒ごとに引く、ロゴの秒数 (ウィンドウのずれの重み)。
	// 先頭のロゴを短くしか捉えられなかった録画で、末尾に入った次の番組のロゴにウィンドウが引っ張られないようにする。
	// 大きく遅れた放送は、ずらした分だけロゴが多く入るので、この減点を上回る。
	windowShiftWeight = 0.5
	// windowTie は、最高点からこの差の内の候補を同点とみなす幅 (秒)。同点なら、番組が始まるはずの時刻に近いものを採る。
	windowTie = 1.0
	// windowSnapDistance は、ウィンドウの端を無音の境目に寄せる距離 (#1506 の「ウィンドウの端を寄せる距離」)。番組表の始まりと実際の始まりのずれより
	// 広くする。開発用の環境の 5 本の録画では、実際に放送中になった日時は、番組表の始まりより 0.5〜2.6 秒遅れていた。
	windowSnapDistance = 3.0
	// windowSnapNoCut は、シーンの切り替わりも音声の構成の変化もない無音の境目を、この秒だけ遠いとみなす量 (「切れ目のない無音の割増し」)。
	// 切れ目のある境目のほうが、番組の切り替わりとして確かだからである。
	windowSnapNoCut = 1.0
)

// programWindow は、番組の尺のウィンドウを、ロゴの出ていた区間 intervals に合わせて置いて返す (#1506 の手順 1)。
// times はコマごとの時刻 (秒)、fps はコマの速さ、program は番組の時刻 (ファイルの始まりから)。
// 番組の尺が録画より短いときだけ置き、置けなければ ok を false にする。ロゴから候補が 1 つも作れないときも、番組の先頭を削らないよう置かない。
// program.Fixed なら、探さずに、program の時刻どおりに置く。ウィンドウの端を無音の境目に寄せるのは、置いた後に snapWindow で行う。
func programWindow(intervals []logo.Interval, times []float64, fps float64, program *Program) (window, bool) {
	none := window{start: 0, end: 0}
	if program == nil || program.Duration <= 0 || len(times) == 0 {
		return none, false
	}
	// 番組が始まるはずの時刻を、最初のコマからの秒にする。ファイルの始まりから最初のコマまでの空きを差し引く。
	expected := program.Start.Seconds() - times[0]
	length := program.Duration.Seconds()
	if program.Fixed {
		return window{start: expected, end: expected + length}, true
	}
	shown := make([]span, 0, len(intervals))
	for _, iv := range intervals {
		shown = append(shown, span{start: frameTime(times, fps, iv.Start), end: frameTime(times, fps, iv.End)})
	}
	pos, ok := placeWindow(shown, []span{{start: 0, end: frameTime(times, fps, len(times))}}, length, expected)
	if !ok {
		return none, false
	}

	return window{start: pos, end: pos + length}, true
}

// frameTime は、frame 番目のコマの、最初のコマからの秒を返す。最後のコマの後ろは、最後のコマの 1 コマ分 (速さ fps) 後とする。
func frameTime(times []float64, fps float64, frame int) float64 {
	if frame < len(times) {
		return times[frame] - times[0]
	}

	return times[len(times)-1] - times[0] + 1/fps
}

// placeWindow は、長さ length のウィンドウの始まりを、ロゴの出ていた区間 shown (始まりの順で、重ならないもの) に合わせて選ぶ (#1506 の手順 1)。
// files は、ファイルの映像のある範囲 (始まりの順)。時刻は、shown、files、番組が始まるはずの時刻 expected で、同じ基準の秒にする。
// ウィンドウは最初のファイルの始まりから最後のファイルの終わりまでに収める。ロゴの区間の始まりにウィンドウの先頭を合わせる位置と、
// 終わりにウィンドウの尻を合わせる位置を候補にし、ファイルの始まりと終わりから windowEdgeExclusion の内の端は候補にしない。
// ファイルの始まりで始まるロゴは、前の番組か、前のファイルから続くロゴかもしれないからである。
// 収まらないか、候補が 1 つもなければ、ok を false にする。
func placeWindow(shown, files []span, length, expected float64) (float64, bool) {
	if len(files) == 0 {
		return 0, false
	}
	first := files[0].start
	last := first
	for _, f := range files {
		last = max(last, f.end)
	}
	if length >= last-first {
		return 0, false
	}
	edge := windowEdgeExclusion.Seconds()
	nearStart := func(at float64) bool {
		return slices.ContainsFunc(files, func(f span) bool { return at >= f.start && at <= f.start+edge })
	}
	nearEnd := func(at float64) bool {
		return slices.ContainsFunc(files, func(f span) bool { return at <= f.end && at >= f.end-edge })
	}
	candidates := []float64{}
	for _, s := range shown {
		if !nearStart(s.start) && s.start >= first && s.start <= last-length {
			candidates = append(candidates, s.start)
		}
		if !nearEnd(s.end) && s.end <= last && s.end-length >= first {
			candidates = append(candidates, s.end-length)
		}
	}
	if len(candidates) == 0 {
		return 0, false
	}
	score := func(pos float64) float64 {
		var total float64
		for _, s := range shown {
			total += max(0, min(s.end, pos+length)-max(s.start, pos))
		}

		return total - windowShiftWeight*math.Abs(pos-expected)
	}
	best := math.Inf(-1)
	for _, pos := range candidates {
		best = max(best, score(pos))
	}
	chosen := math.NaN()
	for _, pos := range candidates {
		if score(pos) >= best-windowTie && (math.IsNaN(chosen) || math.Abs(pos-expected) < math.Abs(chosen-expected)) {
			chosen = pos
		}
	}

	return chosen, true
}

// snapWindow は、ウィンドウ w の両端を、windowSnapDistance 以内で最も近い無音の境目に寄せて返す (#1506 の手順 1)。
// 切れ目のない境目は、windowSnapNoCut だけ遠いとみなす。寄せた結果、ウィンドウの始まりが終わりより後になる境目は使わない。
// bounds の時刻は、今の見分け方の基準 (コマの時刻に offset を足したもの)。duration は、その基準での録画の長さ。
// combineSections と同じく、録画の端 (0 と duration) にある境目は、無音の境目ではないので使わない。
// origin は最初のコマの時刻。ウィンドウは最初のコマからの秒。
func snapWindow(w window, bounds []boundary, duration, origin, offset float64) window {
	start := snapEdge(w.start, bounds, duration, origin, offset, func(at float64) bool { return at >= 0 && at < w.end })
	end := snapEdge(w.end, bounds, duration, origin, offset, func(at float64) bool { return at > start })

	return window{start: start, end: end}
}

// snapEdge は、ウィンドウの端 edge (最初のコマからの秒) を、windowSnapDistance 以内で最も近い無音の境目に寄せて返す。
// valid が false を返す境目と、録画の端の境目は使わない。寄せる先がなければ edge のまま返す。引数は snapWindow と同じ。
func snapEdge(edge float64, bounds []boundary, duration, origin, offset float64, valid func(float64) bool) float64 {
	best, bestCost := edge, math.Inf(1)
	for _, b := range bounds {
		if b.at <= 0 || b.at >= duration {
			continue
		}
		at := b.at - offset - origin
		distance := math.Abs(at - edge)
		if distance > windowSnapDistance || !valid(at) {
			continue
		}
		cost := distance
		if !b.cut {
			cost += windowSnapNoCut
		}
		if cost < bestCost {
			best, bestCost = at, cost
		}
	}

	return best
}

// window は、番組のウィンドウ。始まりと終わりは、最初のコマからの秒。
type window struct {
	start, end float64
}

// clipIntervals は、ロゴの出ていた区間を、ウィンドウ [start, end) (最初のコマからの秒) に収める。ウィンドウの外のロゴは、前後の番組のものとして破棄する。
func clipIntervals(intervals []logo.Interval, times []float64, start, end float64) []logo.Interval {
	origin := times[0]
	// first は、ウィンドウの内に入る最初のコマ、last は、ウィンドウの外に出る最初のコマ。
	first, last := len(times), len(times)
	for i, t := range times {
		if first == len(times) && t-origin >= start {
			first = i
		}
		if t-origin >= end {
			last = i

			break
		}
	}
	out := make([]logo.Interval, 0, len(intervals))
	for _, iv := range intervals {
		s, e := max(iv.Start, first), min(iv.End, last)
		if e > s {
			out = append(out, logo.Interval{Start: s, End: e})
		}
	}

	return out
}

const (
	// minCMSection は、ほかの CM に続かない区間を CM とみなす最短 (#1506 の「破棄する短い区間」)。CM は 15 秒の倍数なので、
	// 許すずれ (1 秒) を引いた長さに届かない区間は、本当の CM になりえない。本番のアニメの録画では、明るい場面で局ロゴが見えにくくなり、
	// 本編の中に 2〜11 秒の区間が 14 個出たが、どれも本編だった。
	minCMSection = 14 * time.Second
	// cmAdjacency は、短い区間が、ほかの CM に続くとみなす隙間。CM に続く短い区間は、提供や局の告知かもしれないので残す。
	cmAdjacency = 2 * time.Second
)

// dropShortSections は、CM の区間のうち、minCMSection より短く、ファイルの端 (0 と duration) に接しておらず、
// ほかの残す区間から cmAdjacency の内にもないものを破棄する (#1506)。端の区間は、前後の番組の CM が途中から入るので、長さを問わない。
func dropShortSections(sections []Section, duration time.Duration) []Section {
	keep := make([]bool, len(sections))
	for i, s := range sections {
		keep[i] = s.End-s.Start >= minCMSection || s.Start <= 0 || s.End >= duration
	}
	// 残す区間に続く短い区間も残す。続いた区間にさらに続くものもあるので、増えなくなるまで繰り返す。
	for changed := true; changed; {
		changed = false
		for i := range sections {
			if keep[i] {
				continue
			}
			for j := range sections {
				if keep[j] && gap(sections[i], sections[j]) <= cmAdjacency {
					keep[i], changed = true, true

					break
				}
			}
		}
	}
	out := make([]Section, 0, len(sections))
	for i, s := range sections {
		if keep[i] {
			out = append(out, s)
		}
	}

	return out
}

// gap は、2 つの区間の隙間を返す。重なっていれば 0。
func gap(a, b Section) time.Duration {
	return max(b.Start-a.End, a.Start-b.End, 0)
}

// programEdgeWindow は、番組が始まるはずの時刻と終わるはずの時刻から、前後の番組との境目とみなす無音の境目を探す幅 (秒)。
const programEdgeWindow = 3.0

// programEdges は、今の見分け方の区間 sections (時刻は、今の見分け方の基準の秒) に、録画の前後のマージンに入った前後の番組を、
// CM として足す (#1506)。番組が始まるはずの時刻に最も近い、programEdgeWindow の内の無音の境目より前と、終わるはずの時刻に
// 最も近い境目より後を CM にする。番組の時刻は、ファイルの始まりからの時間なので、offset (局ロゴの見分け方の時刻の差) で、
// 今の見分け方の基準に直す。境目が見つからない端は、そのままにする。
func programEdges(sections []Section, bounds []boundary, duration float64, program *Program, offset float64) []Section {
	nearest := func(at float64) (float64, bool) {
		best, found := 0.0, false
		for _, b := range bounds {
			if b.at <= 0 || b.at >= duration || math.Abs(b.at-at) > programEdgeWindow {
				continue
			}
			if !found || math.Abs(b.at-at) < math.Abs(best-at) {
				best, found = b.at, true
			}
		}

		return best, found
	}
	start := program.Start.Seconds() + offset
	spans := make([]span, 0, len(sections)+2)
	if b, ok := nearest(start); ok {
		spans = append(spans, span{start: 0, end: b})
	}
	for _, s := range sections {
		spans = append(spans, span{start: s.Start.Seconds(), end: s.End.Seconds()})
	}
	if b, ok := nearest(start + program.Duration.Seconds()); ok {
		spans = append(spans, span{start: b, end: duration})
	}
	slices.SortFunc(spans, func(a, b span) int { return cmp.Compare(a.start, b.start) })
	merged := mergeAdjacent(spans)
	out := make([]Section, 0, len(merged))
	for _, s := range merged {
		out = append(out, Section{Start: seconds(s.start), End: seconds(s.end)})
	}

	return out
}
