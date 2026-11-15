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
	// combineSponsorMin と combineSponsorMax は、提供 (番組の終わりに出す、スポンサーの表示。局ロゴを消して出すことが多い) とみなす長さの幅。提供が 2 枚続くと、15 秒を少し超える (実録画で 15.5 秒)。
	// 本当の CM は、無音から無音までがほぼちょうど 15 秒 (実録画で 15.0〜15.2 秒) なので、本編の直後でも、
	// 15 秒にぴったり合う区間は、CM の点が提供の点を上回る。
	combineSponsorMin = 8.0
	combineSponsorMax = 16.0
	// combineSponsorBonus は、本編の直後の提供で、次が CM か録画の終わりのときに足す点 (提供の加点)。
	// 本編の直後の 10 秒は、提供とも告知とも読めるが、番組の終わりに提供を出してから CM に入る並びが多いので、提供を先に採る。
	// ロゴのない 10 秒の本編の点 (-10) に足して、10 秒の告知の点 (10) を上回るようにする。
	combineSponsorBonus = 25.0
	// combineNoticeWeight は、告知 (局の短い番宣。5 秒か 10 秒) で、長さの合った 1 秒ごとに足す点 (告知の重み)。
	combineNoticeWeight = 1.0
	// combineNoticeReturn は、本編の直後の告知の後に、CM を挟まずに本編へ戻るときに引く点。告知の重み × 10 秒と尺違いの減点の和で、
	// 告知だけで本編へ戻る所を、告知としては認めず、尺の合わない CM と同じ扱いにする。
	combineNoticeReturn = combineNoticeWeight*10 + combineMismatch
)

// combineSections は、無音の境目の候補 bounds (boundaries の結果) と、局ロゴが出ていた区間 logo から、本編と CM の並びのうち、
// 点数の最も高いものを選び、CM の区間を返す (#1506 の手順 3〜5 の骨格)。時刻は、今の見分け方の基準 (ファイルの最初の映像からの秒) で、
// duration は、その基準での録画の長さ。
//
// 境目の候補は、無音 (とシーンの切り替わり、音声の構成の変化) の境目と、録画の始まりと終わり。ロゴの端は候補にしない。
// 本編の区間は、ロゴが見えた秒を足し、ロゴがなかった秒を引く。CM の区間は、15 秒の倍数への合い方で足し、ロゴが見えた秒を大きく引く。
// 録画の端に接する CM は、前後の番組の CM が途中から入るので、長さを問わず、ロゴが見えた秒だけを引く。
// 本編の直後の 10 秒前後の区間で、次が CM か録画の終わりのものは、提供として本編の側に数え、加点する。
// 5 秒か 10 秒の区間は、告知として、長さが合うほど足す。本編の直後の告知の後に、CM を挟まずに本編へ戻るときは、大きく引く。
// 本編と CM の並びは、境目の候補を順にたどる動的計画法で選ぶ。番組の頭の切れ目、ロゴの点数の変化などの細かい規則は、まだ入れていない。
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

// 動的計画法の状態。直前の区間が、本編、CM、提供、本編の直後の告知のどれで終わったか。
const (
	stateProgram = iota
	stateCM
	stateSponsor
	stateNotice
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

// notice は、[a, b) を告知とみなした点。5 秒か 10 秒に合わなければ ok を false にする。録画の端に接する区間は、CM として扱う。
func (sc *combineScorer) notice(a, b float64) (float64, bool) {
	if a <= 0 || b >= sc.duration {
		return 0, false
	}
	length := b - a
	g := max(1-math.Abs(length-5)/combineTolerance, 1-math.Abs(length-10)/combineTolerance)
	if g <= 0 {
		return 0, false
	}

	return combineNoticeWeight*length*g - combineMismatch*(1-g) - combineLogoPenalty*sc.shown(a, b), true
}

// sponsor は、[a, b) を提供とみなした点。提供の長さの幅 (1 枚か 2 枚) でなければ ok を false にする。
// 提供は、ロゴの出ていた本編の直後に、ロゴを消して出す。区間の中でロゴが許すロゴの途切れより長く見えるもの (ロゴの出た本編の終わり) と、
// 始まりの前の許すロゴの途切れの内にロゴが見えないもの (ロゴのない所の後の CM) は、提供にしない。
func (sc *combineScorer) sponsor(a, b float64) (float64, bool) {
	// 提供は本編の直後に出すので、録画の頭に接する区間は提供にしない (前の番組の CM が途中から入る所)。
	if length := b - a; a <= 0 || length < combineSponsorMin || length > combineSponsorMax {
		return 0, false
	}
	if sc.shown(a, b) > combineLogoGap || sc.shown(a-combineLogoGap, a) == 0 {
		return 0, false
	}

	return sc.program(a, b) + combineSponsorBonus, true
}

// combineTable は、動的計画法の表。best[j][k] は、j 番目の候補で、直前の区間が k で終わる並びの最高点。
// from[j][k] と prev[j][k] は、その区間の始まりの候補と、その候補で終わっていた区間の状態。
type combineTable struct {
	best [][stateCount]float64
	from [][stateCount]int
	prev [][stateCount]int
}

// solve は、境目の候補 points を順にたどり、表を作る。
func (sc *combineScorer) solve(points []float64) *combineTable {
	n := len(points)
	t := &combineTable{best: make([][stateCount]float64, n), from: make([][stateCount]int, n), prev: make([][stateCount]int, n)}
	for j := range n {
		for k := range stateCount {
			t.best[j][k] = math.Inf(-1)
		}
	}
	t.best[0][stateProgram], t.best[0][stateCM] = 0, 0
	for j := 1; j < n; j++ {
		for i := j - 1; i >= 0; i-- {
			sc.relax(t, points, i, j)
		}
	}

	return t
}

// relax は、i 番目から j 番目の候補までの区間を、本編、提供、告知、CM のそれぞれとみなして、表を更新する。
// 提供の後は、CM か録画の終わりしか続けられない。提供と本編の直後の告知の前は、本編でなければならない。
// 本編の直後の告知から、CM を挟まずに本編へ戻るときは、大きく引く。CM の後の告知だけを、CM の続きとして数える。
func (sc *combineScorer) relax(t *combineTable, points []float64, i, j int) {
	a, b := points[i], points[j]
	// best は、i 番目の候補で終わる状態のうち、from に含まれるものの最高点と、その状態。penalty は、状態ごとに引く点。
	best := func(penalty [stateCount]float64, from ...int) (float64, int) {
		v, k := math.Inf(-1), -1
		for _, c := range from {
			if x := t.best[i][c] - penalty[c]; x > v {
				v, k = x, c
			}
		}

		return v, k
	}
	update := func(k int, base float64, prev int, score float64) {
		if prev < 0 || math.IsInf(base, -1) {
			return
		}
		if v := base + score; v > t.best[j][k] {
			t.best[j][k], t.from[j][k], t.prev[j][k] = v, i, prev
		}
	}
	none := [stateCount]float64{}
	returning := [stateCount]float64{}
	returning[stateNotice] = combineNoticeReturn

	base, prev := best(returning, stateProgram, stateCM, stateNotice)
	update(stateProgram, base, prev, sc.program(a, b))
	if v, ok := sc.sponsor(a, b); ok {
		base, prev := best(none, stateProgram)
		update(stateSponsor, base, prev, v)
	}
	if v, ok := sc.notice(a, b); ok {
		base, prev := best(none, stateProgram)
		update(stateNotice, base, prev, v)
		// CM と提供の後の告知だけを、CM の続きとして数える。本編の直後の告知から告知で CM へ移ると、
		// 告知から本編へ戻るときの減点を避けられてしまうため。提供の後に、見逃し配信の案内などを出してから CM に入る並びがある。
		// ただし、提供と告知を合わせて 15 秒の倍数に合うときは、提供の後に続けない。本当の CM の途中に境目の候補があると、
		// CM の頭を提供に、残りを告知に取れてしまうため。
		from := []int{stateCM}
		if sp := t.from[i][stateSponsor]; !math.IsInf(t.best[i][stateSponsor], -1) && cmFit(b-points[sp]) == 0 {
			from = append(from, stateSponsor)
		}
		base, prev = best(none, from...)
		update(stateCM, base, prev, v)
	}
	// 端に接しない CM は、CM の最長より遠くの候補からは始められない。
	if a > 0 && b < sc.duration && b-a > combineMaxCM+combineTolerance {
		return
	}
	if v, ok := sc.cm(a, b); ok {
		base, prev := best(none, stateProgram, stateCM, stateSponsor, stateNotice)
		update(stateCM, base, prev, v)
	}
}

// trace は、末尾から遡り、CM の区間 (告知を含む) を始まりの順に返す。録画の終わりは、本編、CM、提供のどれで終わってもよい。
// 本編の直後の告知で終わる並びは、録画の終わりなら CM と同じに扱う。
func (t *combineTable) trace(points []float64) []span {
	j := len(points) - 1
	k := stateProgram
	for _, c := range []int{stateCM, stateSponsor, stateNotice} {
		if t.best[j][c] > t.best[j][k] {
			k = c
		}
	}
	sections := []span{}
	for j > 0 {
		i := t.from[j][k]
		if k == stateCM || k == stateNotice {
			sections = append(sections, span{start: points[i], end: points[j]})
		}
		j, k = i, t.prev[j][k]
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
