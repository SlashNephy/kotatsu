package cmdetect

import (
	"cmp"
	"math"
	"slices"
)

// CM の区間を見分ける規則の値 (設計書 付録 A)。時間はすべて秒。
const (
	// minSilence は、境目の候補にする無音の長さの下限。本編の台詞の間 (0.1〜0.2 秒) を拾わないようにする。
	minSilence = 0.3
	// sceneMargin は、無音の前後のうち、シーンの切り替わりを探す幅。I フレームだけを比べるので、間隔 (0.5 秒) ほどずれる。
	sceneMargin = 0.5
	// mergeWindow は、1 つの境目とみなす候補の間隔。CM の境目の前後に、短い無音が続けて入ることがあるため。
	mergeWindow = 1.0
	// cmUnit は、CM の長さの単位。日本の CM は 15 秒の倍数の長さで流れる。
	cmUnit = 15.0
	// maxUnits は、1 本の CM とみなす長さの上限 (cmUnit の倍数)。
	maxUnits = 4
	// tolerance は、CM の長さが cmUnit の倍数からずれてよい幅。
	tolerance = 0.5
	// minCMs は、CM の区間とみなす、続く CM の数の下限。1 本だけでは、本編の場面の切れ目と見分けにくい。
	minCMs = 2
	// edgeExtension は、録画の始まりか終わりから、この長さまでの間に CM の区間があれば、そこまで CM とみなす長さ。
	// 録画の前後のマージンには、前後の番組の CM が途中から入る。
	edgeExtension = cmUnit + tolerance
)

// span は、始まりと終わりのある区間 (秒)。
type span struct {
	start float64
	end   float64
}

// signals は、録画のファイルから読んだ手掛かり。時刻は、ファイルの最初の映像からの秒。
type signals struct {
	// silences は、無音の区間。
	silences []span
	// scenes は、シーンが切り替わった時刻。
	scenes []float64
	// audioChanges は、音声の構成が変わった時刻。
	audioChanges []float64
	// duration は、ファイルの長さ。
	duration float64
}

// boundary は、本編と CM、CM と CM の境目の候補。
type boundary struct {
	at float64
	// cut は、境目でシーンが切り替わったか、音声の構成が変わったかどうか。無音だけの境目より確か。
	cut bool
}

// findSections は、手掛かりから CM の区間を見分ける。
//
// デジタルの無音の区間の真ん中を境目の候補にし、間隔が 15 秒の倍数の候補が続くところを CM とみなす。
func findSections(sig *signals) []span {
	bounds := boundaries(sig)
	chains := findChains(bounds)
	sections := make([]span, 0, len(chains))
	for _, c := range chains {
		s := span{start: bounds[c.first].at, end: bounds[c.last].at}
		if s.start <= edgeExtension {
			s.start = 0
		}
		if sig.duration-s.end <= edgeExtension {
			s.end = sig.duration
		}
		sections = append(sections, s)
	}
	slices.SortFunc(sections, func(a, b span) int { return cmp.Compare(a.start, b.start) })

	return sections
}

// boundaries は、無音と音声の構成の変化から、境目の候補を時刻の順に返す。近い候補は 1 つにまとめる。
func boundaries(sig *signals) []boundary {
	candidates := make([]boundary, 0, len(sig.silences)+len(sig.audioChanges))
	for _, silence := range sig.silences {
		if silence.end-silence.start < minSilence {
			continue
		}
		// 境目の時刻は、無音の真ん中にする。CM の前後の無音はどれも同じ長さなので、I フレームの位置より正確にそろう。
		mid := (silence.start + silence.end) / 2
		cut := slices.ContainsFunc(sig.scenes, func(scene float64) bool {
			return scene >= silence.start-sceneMargin && scene <= silence.end+sceneMargin
		})
		candidates = append(candidates, boundary{at: mid, cut: cut})
	}
	for _, at := range sig.audioChanges {
		candidates = append(candidates, boundary{at: at, cut: true})
	}
	slices.SortFunc(candidates, func(a, b boundary) int { return cmp.Compare(a.at, b.at) })

	merged := make([]boundary, 0, len(candidates))
	for _, c := range candidates {
		if n := len(merged); n > 0 && c.at-merged[n-1].at < mergeWindow {
			// 切り替わりのある方を残す。どちらにもなければ、先の方を残す。
			if c.cut && !merged[n-1].cut {
				merged[n-1] = c
			}

			continue
		}
		merged = append(merged, c)
	}

	return merged
}

// chain は、CM が続く境目の並び。first と last は、boundaries の添字。
type chain struct {
	first int
	last  int
	// count は、並びの中の CM の数。
	count int
}

// findChains は、境目の候補から、CM が minCMs 本以上続く並びを、重ならないように選ぶ。長い並びから選ぶ。
func findChains(bounds []boundary) []chain {
	n := len(bounds)
	// length[j] は、境目 j で終わる最も長い並びの長さ (秒)。prev[j] はその 1 つ前の境目、count[j] は CM の数。
	length := make([]float64, n)
	prev := make([]int, n)
	count := make([]int, n)
	for j := range n {
		prev[j] = -1
		for i := j - 1; i >= 0; i-- {
			d := bounds[j].at - bounds[i].at
			if d > cmUnit*maxUnits+tolerance {
				break
			}
			if !isCMLength(d, bounds[i].cut && bounds[j].cut) {
				continue
			}
			// CM が minCMs 本以上続く並びを、それより少ない並びより先に選び、同じなら長い方を選ぶ。
			// 長さだけで選ぶと、離れた候補からの 1 本の長い間隔が、CM が続く並びを上書きして、区間ごと落とす。
			l, c := length[i]+d, count[i]+1
			if better(c, l, count[j], length[j]) {
				length[j], prev[j], count[j] = l, i, c
			}
		}
	}

	ends := make([]int, 0, n)
	for j := range n {
		if count[j] >= minCMs {
			ends = append(ends, j)
		}
	}
	slices.SortFunc(ends, func(a, b int) int { return cmp.Compare(length[b], length[a]) })
	used := make([]bool, n)
	chains := []chain{}
	for _, end := range ends {
		path := []int{}
		for k := end; k != -1; k = prev[k] {
			path = append(path, k)
		}
		slices.Reverse(path)
		path = trimShortSpots(bounds, path)
		if len(path)-1 < minCMs || slices.ContainsFunc(path, func(k int) bool { return used[k] }) ||
			chainOverlaps(chains, bounds, bounds[path[0]].at, bounds[path[len(path)-1]].at) {
			continue
		}
		for _, k := range path {
			used[k] = true
		}
		chains = append(chains, chain{first: path[0], last: path[len(path)-1], count: len(path) - 1})
	}

	return chains
}

// better は、CM の数が c で長さが l の並びが、cur 本で curLength 秒の並びより良いかどうかを返す。
func better(c int, l float64, cur int, curLength float64) bool {
	if enough, curEnough := c >= minCMs, cur >= minCMs; enough != curEnough {
		return enough
	}

	return l > curLength
}

// trimShortSpots は、並び path の両端から、15 秒の倍数でない間隔を除く。
// 短い間隔は、CM の間に挟まる番組の宣伝のほか、本編の終わりのアイキャッチや次回予告の題名でもあるので、本編との境目には使わない。
func trimShortSpots(bounds []boundary, path []int) []int {
	isShort := func(i, j int) bool { return !isCMLength(bounds[j].at-bounds[i].at, false) }
	for len(path) >= 2 && isShort(path[0], path[1]) {
		path = path[1:]
	}
	for len(path) >= 2 && isShort(path[len(path)-2], path[len(path)-1]) {
		path = path[:len(path)-1]
	}

	return path
}

// isCMLength は、d 秒が CM の長さとみなせるかどうかを返す。cut が true なら、両端でシーンが切り替わっているので、短い CM も認める。
func isCMLength(d float64, cut bool) bool {
	for k := 1; k <= maxUnits; k++ {
		if math.Abs(d-cmUnit*float64(k)) <= tolerance {
			return true
		}
	}
	// ほかの CM の間に挟まる、15 秒の倍数でない長さの CM (番組の宣伝など) は、本編と取り違えやすいので、
	// 両端でシーンが切り替わっているときだけ認める。
	if cut {
		for _, spot := range [...]float64{5, 10, 20} {
			if math.Abs(d-spot) <= tolerance {
				return true
			}
		}
	}

	return false
}

// chainOverlaps は、from から to までが、選んだ並びのどれかと重なるかどうかを返す。
func chainOverlaps(chains []chain, bounds []boundary, from, to float64) bool {
	for _, c := range chains {
		if from < bounds[c.last].at && bounds[c.first].at < to {
			return true
		}
	}

	return false
}
