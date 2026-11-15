package cmdetect

import (
	"math"
	"slices"
)

// 局ロゴと無音を組み合わせて CM の区間を選ぶ規則の値 (#1506)。時間はすべて秒、重みは秒あたりの点。
const (
	// combineProgramWeight は、本編の区間で、ロゴが見えた 1 秒ごとに足す点 (番組の重み)。
	combineProgramWeight = 1.0
	// combineNoLogoWeight は、本編の区間で、ロゴがなかった 1 秒ごとに引く点 (ロゴなしの重み)。
	combineNoLogoWeight = 1.0
	// combineCMWeight は、CM の区間で、15 秒の倍数に合った 1 秒ごとに足す点 (CM の重み)。
	combineCMWeight = 1.0
	// combineLogoGap は、本編の側に残す、ロゴの途切れの長さ (許すロゴの途切れ)。アイキャッチのように、ロゴが短く消える所。
	combineLogoGap = 3.0
	// combineMismatch は、尺の合わない CM に引く点 (尺違いの減点)。許すロゴの途切れの長さの本編と釣り合わせる。
	combineMismatch = combineLogoGap * combineNoLogoWeight
	// combineLogoPenalty は、CM の区間で、ロゴが見えた 1 秒ごとに引く点 (ロゴの減点)。番組の重みと CM の重みの和より大きくする。
	combineLogoPenalty = 3.0
	// combineTolerance は、CM の長さが 15 秒の倍数からずれてよい幅 (許すずれ)。
	combineTolerance = 1.0
	// combineMaxCM は、録画の端に接しない CM の区間の 1 本の長さの上限 (CM の最長)。
	combineMaxCM = 60.0
	// combineSponsor は、提供 (番組の終わりに出す、スポンサーの表示) の長さ。局ロゴを消して出すことが多い。
	combineSponsor = 10.0
	// combineSponsorBonus は、本編の直後の提供で、次が CM か録画の終わりのときに足す点 (提供の加点)。
	// 10 秒の CM とみなした点 (尺違いの減点だけ) を、ロゴのない 10 秒の本編の点が上回るようにする。
	combineSponsorBonus = 10.0
)

// combineSections は、無音の境目の候補 bounds (boundaries の結果) と、局ロゴが出ていた区間 logo から、本編と CM の並びのうち、
// 点数の最も高いものを選び、CM の区間を返す (#1506 の手順 3〜5 の骨格)。時刻は、今の見分け方の基準 (ファイルの最初の映像からの秒) で、
// duration は、その基準での録画の長さ。
//
// 境目の候補は、無音 (とシーンの切り替わり、音声の構成の変化) の境目と、録画の始まりと終わり。ロゴの端は候補にしない。
// 本編の区間は、ロゴが見えた秒を足し、ロゴがなかった秒を引く。CM の区間は、15 秒の倍数への合い方で足し、ロゴが見えた秒を大きく引く。
// 録画の端に接する CM は、前後の番組の CM が途中から入るので、長さを問わず、ロゴが見えた秒だけを引く。
// 本編の直後の 10 秒前後の区間で、次が CM か録画の終わりのものは、提供として本編の側に数え、加点する。
// 本編と CM の並びは、境目の候補を順にたどる動的計画法で選ぶ。告知、番組の頭の切れ目などの細かい規則は、まだ入れていない。
func combineSections(bounds []boundary, duration float64, logo []span) []span {
	points := make([]float64, 0, len(bounds)+2)
	points = append(points, 0)
	for _, b := range bounds {
		if b.at > 0 && b.at < duration {
			points = append(points, b.at)
		}
	}
	points = append(points, duration)
	sc := &combineScorer{duration: duration, logo: logo}
	table := sc.solve(points)

	return mergeAdjacent(table.trace(points))
}

// 動的計画法の状態。直前の区間が、本編、CM、提供のどれで終わったか。
const (
	stateProgram = iota
	stateCM
	stateSponsor
	stateCount
)

// combineScorer は、区間に点数を付ける。
type combineScorer struct {
	duration float64
	logo     []span
}

// shown は、[a, b) のうち、ロゴが見えた秒を返す。
func (sc *combineScorer) shown(a, b float64) float64 {
	var total float64
	for _, l := range sc.logo {
		total += max(0, min(l.end, b)-max(l.start, a))
	}

	return total
}

// program は、[a, b) を本編とみなした点。ロゴが見えた秒を足し、ロゴがなかった秒を引く。
func (sc *combineScorer) program(a, b float64) float64 {
	s := sc.shown(a, b)

	return combineProgramWeight*s - combineNoLogoWeight*(b-a-s)
}

// cm は、[a, b) を CM とみなした点。CM になりえなければ ok を false にする。
// 録画の端に接する CM は、前後の番組の CM が途中から入るので、長さを問わず、ロゴが見えた秒だけを引く。
func (sc *combineScorer) cm(a, b float64) (float64, bool) {
	s := sc.shown(a, b)
	if a <= 0 || b >= sc.duration {
		return -combineLogoPenalty * s, true
	}
	length := b - a
	if length > combineMaxCM+combineTolerance {
		return 0, false
	}
	f := cmFit(length)

	return combineCMWeight*length*f - combineMismatch*(1-f) - combineLogoPenalty*s, true
}

// sponsor は、[a, b) を提供とみなした点。10 秒前後でなければ ok を false にする。
func (sc *combineScorer) sponsor(a, b float64) (float64, bool) {
	if math.Abs(b-a-combineSponsor) > combineTolerance {
		return 0, false
	}

	return sc.program(a, b) + combineSponsorBonus, true
}

// combineTable は、動的計画法の表。best[j][k] は、j 番目の候補で、直前の区間が k で終わる並びの最高点、
// from[j][k] は、その区間の始まりの候補。
type combineTable struct {
	best [][stateCount]float64
	from [][stateCount]int
}

// solve は、境目の候補 points を順にたどり、表を作る。提供の後は、CM か録画の終わりしか続けられない。提供の前は、本編でなければならない。
func (sc *combineScorer) solve(points []float64) *combineTable {
	n := len(points)
	t := &combineTable{best: make([][stateCount]float64, n), from: make([][stateCount]int, n)}
	for j := range n {
		t.best[j] = [stateCount]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	}
	t.best[0] = [stateCount]float64{0, 0, math.Inf(-1)}
	for j := 1; j < n; j++ {
		for i := j - 1; i >= 0; i-- {
			sc.relax(t, points, i, j)
		}
	}

	return t
}

// relax は、i 番目から j 番目の候補までの区間を、本編、提供、CM のそれぞれとみなして、表を更新する。
func (sc *combineScorer) relax(t *combineTable, points []float64, i, j int) {
	anyState := max(t.best[i][stateProgram], t.best[i][stateCM], t.best[i][stateSponsor])
	if math.IsInf(anyState, -1) {
		return
	}
	a, b := points[i], points[j]
	update := func(k int, v float64) {
		if v > t.best[j][k] {
			t.best[j][k], t.from[j][k] = v, i
		}
	}
	update(stateProgram, max(t.best[i][stateProgram], t.best[i][stateCM])+sc.program(a, b))
	if v, ok := sc.sponsor(a, b); ok {
		update(stateSponsor, t.best[i][stateProgram]+v)
	}
	// 端に接しない CM は、CM の最長より遠くの候補からは始められない。
	if a > 0 && b < sc.duration && b-a > combineMaxCM+combineTolerance {
		return
	}
	if v, ok := sc.cm(a, b); ok {
		update(stateCM, anyState+v)
	}
}

// trace は、末尾から遡り、CM の区間を始まりの順に返す。区間ごとに、その始まりの候補で最高点だった状態から来たとみなす。
// 提供の始まりは本編、本編の始まりは提供でないものに限る。
func (t *combineTable) trace(points []float64) []span {
	argmax := func(i int, allowed ...int) int {
		k := allowed[0]
		for _, c := range allowed[1:] {
			if t.best[i][c] > t.best[i][k] {
				k = c
			}
		}

		return k
	}
	sections := []span{}
	j := len(points) - 1
	k := argmax(j, stateProgram, stateCM, stateSponsor)
	for j > 0 {
		i := t.from[j][k]
		if k == stateCM {
			sections = append(sections, span{start: points[i], end: points[j]})
		}
		switch k {
		case stateSponsor:
			k = stateProgram
		case stateProgram:
			k = argmax(i, stateProgram, stateCM)
		default:
			k = argmax(i, stateProgram, stateCM, stateSponsor)
		}
		j = i
	}
	slices.Reverse(sections)

	return sections
}

// mergeAdjacent は、隣り合う CM の区間を 1 つにつなぐ。
func mergeAdjacent(sections []span) []span {
	merged := make([]span, 0, len(sections))
	for _, s := range sections {
		if m := len(merged); m > 0 && s.start <= merged[m-1].end {
			merged[m-1].end = max(merged[m-1].end, s.end)

			continue
		}
		merged = append(merged, s)
	}

	return merged
}

// cmFit は、長さ length の、15 秒の倍数への合い方を、0〜1 で返す。最寄りの倍数からのずれが許すずれ以上か、15 秒から CM の最長の外なら 0。
func cmFit(length float64) float64 {
	if length < cmUnit-combineTolerance || length > combineMaxCM+combineTolerance {
		return 0
	}
	diff := math.Abs(length - math.Round(length/cmUnit)*cmUnit)
	if diff >= combineTolerance {
		return 0
	}

	return 1 - diff/combineTolerance
}
