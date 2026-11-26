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
	// combineMaxCM は、録画の端に接しない CM の区間の 1 本の長さの上限 (CM の最長)。これを超える CM は、長い CM として扱う。
	combineMaxCM = 60.0
	// combineMaxLongCM は、録画の端に接しない長い CM (通販の CM など) の 1 本の長さの上限 (長い CM の最長、#1958)。
	// BS の局は、間に無音のない 90〜180 秒の通販の CM を流す。
	combineMaxLongCM = 180.0
	// combineLongTolerance は、長い CM の長さが 15 秒の倍数からずれてよい幅 (長い CM の許すずれ、#1958)。
	// 局がロゴを消したオープニング (90 秒ほど) を長い CM と取り違えにくいよう、CM の許すずれより狭くする。
	// 2026 年 11 月の本番の録画では、通販の CM は、1080i で 15 秒の倍数から 0.12 秒以内、BS4K で 0.4 秒ずれていた。
	combineLongTolerance = 0.5
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
	// combineSponsorLookback は、提供の始まりの前にロゴを探す幅 (#2168)。番組の終わりの絵がロゴを消してから無音に入ると、
	// 本編のロゴの消え際から提供の始まりまでが 3〜4 秒空く (実録画で 4.1 秒)。CM の先頭に提供の点を付けないよう、15 秒の CM 1 本よりずっと短くする。
	combineSponsorLookback = 5.0
	// combineMidSponsorMax は、CM のあいだの提供 (CM の並びの途中に出す、ロゴのない提供の表示) とみなす長さの上限 (#2168)。
	// 下限は combineSponsorMin。提供は 1 枚の 10 秒なので、15 秒の CM と取り違えない長さにする。
	combineMidSponsorMax = 12.0
	// combineMidSponsorAfter は、CM のあいだの提供の前に、ロゴが見えてはならない幅 (#2168)。15 秒の CM 2 本分。
	// 実録画の CM のあいだの提供は、どれも本編の終わりから 60 秒の CM の後に出ていた。本編の直後の提供 2 枚 (15.5 秒) の後の
	// 案内や番宣を、CM のあいだの提供と取り違えないようにする。
	combineMidSponsorAfter = 2 * cmUnit
	// combineMidSponsorTail は、CM のあいだの提供を認めない、録画の終わりからの幅 (#2168)。次の番組の直前の CM の並びには、
	// 局の 10 秒の番宣が入る。本番の録画では、提供は録画の終わりの 170 秒以上前に、ロゴのない 10 秒の番宣は 6〜50 秒前にあった。
	combineMidSponsorTail = 120.0
	// combinePromoTolerance は、ロゴを出したままの番宣とみなす長さの、15 秒の倍数からのずれの上限 (#2168)。
	// 番宣は無音から無音までが 15 秒の倍数にほぼちょうど合う (実録画で 0.05 秒以内)。本編の区間をたまたま番宣にしないよう、CM の許すずれよりずっと狭くする。
	combinePromoTolerance = 0.1
	// combinePromoMax は、ロゴを出したままの番宣の長さの上限 (#2168)。局の番宣は 15 秒か 30 秒で、
	// 本番の録画では、60 秒のアバンや本編の区間が 60 秒の倍数にちょうど合っていた。
	combinePromoMax = 30.0
	// combinePromoProgram は、ロゴを出したままの番宣の隣に求める本編の長さの下限 (#2168)。番組の終わりの次回予告やエンディングも、
	// ロゴを出したまま 15 秒の倍数で CM の隣に来るが、その隣は提供や次回予告の短い本編か、別の CM になる。
	combinePromoProgram = 60.0
	// combinePromoBonus は、ロゴを出したままの番宣に足す点 (#2168)。ロゴの見えた本編の点 (1 秒に番組の重み) を上回る分。
	combinePromoBonus = 5.0
	// combineHeadGrace は、録画の先頭の CM が、番組の始まりを過ぎてよい幅 (#2168)。番組の始まりは、番組の時刻の始まりと
	// ウィンドウの始まりの遅いほう (LogoDetector.evaluate)。放送開始を見つけるまでの遅れ (開発用の環境で 0.5〜2.6 秒) と、
	// 番組の始まりの後の無音の境目より広くする。
	combineHeadGrace = 10.0
	// combineHeadMismatchWeight は、番組の始まりの近くで始まる、15 秒の倍数にまったく合わない CM に、
	// 許すロゴの途切れを超えた 1 秒ごとに足して引く点 (#2168)。ロゴなしの重みより大きくし、ロゴのない本編より点を下げる。
	// 番組の始まりの直後のロゴのない所は、ロゴを消したオープニングか枠の先頭のジングルなので、15 秒の倍数に合わなければ本編に残す。
	// 録画の途中では、合わない CM (分かれた CM や通販) が本番の録画にあったので、この減点を掛けない。
	combineHeadMismatchWeight = 2.0
)

// combineSections は、無音の境目の候補 bounds (boundaries の結果) と、局ロゴが出ていた区間 logo から、本編と CM の並びのうち、
// 点数の最も高いものを選び、CM の区間を返す (#1506 の手順 3〜5 の骨格)。時刻は、今の見分け方の基準 (ファイルの最初の映像からの秒) で、
// duration は、その基準での録画の長さ。
//
// 境目の候補は、無音 (とシーンの切り替わり、音声の構成の変化) の境目と、録画の始まりと終わり。ロゴの端は候補にしない。
// 本編の区間は、ロゴが見えた秒を足し、ロゴがなかった秒を差し引く。CM の区間は、15 秒の倍数への合い方で足し、ロゴが見えた秒を大きく差し引く。
// 録画の端に接する CM は、前後の番組の CM が途中から入るので、長さを問わず、ロゴが見えた秒だけを差し引く。
// CM の最長を超える CM (通販の CM など) は、長い CM の最長まで、15 秒の倍数にほぼちょうど合うときだけ CM とみなす (#1958)。
// 録画の端の近くの CM は、端まで延ばす (#1958)。
// 本編の直後の 10 秒前後の区間で、次が CM か録画の終わりのものは、提供として本編の側に数え、加点する。
// 5 秒か 10 秒の区間は、告知として、長さが合うほど足す。本編の直後の告知の後に、CM を挟まずに本編へ戻るときは、大きく引く。
// CM の並びの途中のロゴのない 10 秒前後は、CM のあいだの提供として本編の側に数える。ロゴを出したまま 15 秒か 30 秒にほぼちょうど合う区間は、
// CM の並びと長い本編のあいだにあれば、局の番宣として CM にする (#2168)。
// 番組の始まりの時刻 head (分からなければ 0) が分かれば、録画の先頭の CM は番組の始まりの近くで終え、その近くの 15 秒の倍数に合わない
// ロゴのない所は本編に残す (#2168)。
// 本編と CM の並びは、境目の候補を順にたどる動的計画法で選ぶ。境目の減点の規則は、録画で比較して良くならなかったので入れない (#1506)。
func combineSections(bounds []boundary, duration float64, logo []span, head float64) []span {
	points := make([]float64, 0, len(bounds)+2)
	points = append(points, 0)
	for _, b := range bounds {
		if b.at > 0 && b.at < duration {
			points = append(points, b.at)
		}
	}
	points = append(points, duration)
	sc := &combineScorer{duration: duration, logo: logo, head: head}
	table := sc.solve(points)

	return extendToEdges(mergeAdjacent(table.trace(points)), duration)
}

// extendToEdges は、録画の始まりか終わりから edgeExtension の内にある CM の区間を、その端まで延ばす (今の見分け方と同じ規則、#1958)。
// 録画の前後のマージンには、前後の番組が入る。番組のウィンドウを置けない録画 (マージンが短い録画など) では、次の番組の先頭のロゴが見え、
// 長い CM (#1958) の後の数秒を、本編として残してしまうためである。
func extendToEdges(sections []span, duration float64) []span {
	if len(sections) == 0 {
		return sections
	}
	if sections[0].start <= edgeExtension {
		sections[0].start = 0
	}
	if last := len(sections) - 1; duration-sections[last].end <= edgeExtension {
		sections[last].end = duration
	}

	return sections
}

// 動的計画法の状態。直前の区間が、本編、CM、提供、本編の直後の告知、CM のあいだの提供、長い本編、ロゴを出したままの番宣、
// 録画の先頭の CM のどれで終わったか。
const (
	stateProgram = iota
	stateCM
	stateSponsor
	stateNotice
	stateMidSponsor
	// stateLongProgram は、combinePromoProgram 以上の本編で終わった状態。本編のほかの状態と同じ点で、番宣の隣に置けるかだけを分ける。
	stateLongProgram
	// statePromoHead は、長い本編の直後の番宣で終わった状態。次は CM か番宣にしなければならない。
	statePromoHead
	// statePromoTail は、CM の直後の番宣で終わった状態。次は長い本編か番宣にしなければならない。
	statePromoTail
	// stateHeadCM は、録画の先頭の CM で終わった状態。前の番組の CM で、CM の並びとは違うので、番宣を続けない。
	stateHeadCM
	stateCount
)

// combineScorer は、区間に点数を付ける。head は、番組の始まりの時刻 (分からなければ 0)。
type combineScorer struct {
	duration float64
	logo     []span
	head     float64
}

// shown は、[a, b) のうち、ロゴが見えた秒を返す。
func (sc *combineScorer) shown(a, b float64) float64 {
	var total float64
	for _, l := range sc.logo {
		total += max(0, min(l.end, b)-max(l.start, a))
	}

	return total
}

// program は、[a, b) を本編とみなした点。ロゴが見えた秒を足し、ロゴがなかった秒を差し引く。
func (sc *combineScorer) program(a, b float64) float64 {
	s := sc.shown(a, b)

	return combineProgramWeight*s - combineNoLogoWeight*(b-a-s)
}

// cm は、[a, b) を CM とみなした点。CM になりえなければ ok を false にする。
// 録画の端に接する CM は、前後の番組の CM が途中から入るので、長さを問わず、ロゴが見えた秒だけを差し引く。
// ただし、番組の始まりが分かれば、録画の先頭の CM は、番組の始まりから combineHeadGrace の内に終える (#2168)。
// 番組の始まりの後に CM が続くなら、ふつうの CM の規則で数える。
func (sc *combineScorer) cm(a, b float64) (float64, bool) {
	s := sc.shown(a, b)
	if a <= 0 && sc.head > 0 && b > sc.head+combineHeadGrace && b < sc.duration {
		return 0, false
	}
	if a <= 0 || b >= sc.duration {
		return -combineLogoPenalty * s, true
	}
	length := b - a
	if length > combineMaxLongCM+combineLongTolerance {
		return 0, false
	}
	f := cmFit(length)
	if length > combineMaxCM+combineTolerance {
		// 長い CM は、15 秒の倍数にほぼちょうど合うものだけを CM にする。ロゴの減点は、ふつうの CM と同じに掛ける。
		if f = longCMFit(length); f == 0 {
			return 0, false
		}
	}

	// 番組の始まりの近くで始まる、15 秒の倍数にまったく合わない CM は、ロゴのない本編より点を下げる (#2168)。
	mismatch := combineMismatch * (1 - f)
	if f == 0 && sc.head > 0 && a <= sc.head+combineHeadGrace {
		mismatch += combineHeadMismatchWeight * max(0, length-combineLogoGap)
	}

	return combineCMWeight*length*f - mismatch - combineLogoPenalty*s, true
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
	// 提供は本編の直後に出すので、録画の先頭に接する区間は提供にしない (前の番組の CM が途中から入る所)。
	if length := b - a; a <= 0 || length < combineSponsorMin || length > combineSponsorMax {
		return 0, false
	}
	if sc.shown(a, b) > combineLogoGap || sc.shown(a-combineSponsorLookback, a) == 0 {
		return 0, false
	}

	return sc.program(a, b) + combineSponsorBonus, true
}

// midSponsor は、[a, b) を CM のあいだの提供とみなした点 (#2168)。提供の長さ (1 枚) でないか、ロゴが許すロゴの途切れより長く見えれば、
// ok を false にする。アニメの枠では、番組の終わりの CM の並びの途中に、ロゴを消した提供を 10 秒出す。
// 局の 10 秒の番宣とは、ロゴでは見分けられない。本番の録画では、番宣は次の番組の直前 (録画の終わりの近く) にあったので、そこでは認めない。
func (sc *combineScorer) midSponsor(a, b float64) (float64, bool) {
	if length := b - a; a <= 0 || b > sc.duration-combineMidSponsorTail || length < combineSponsorMin || length > combineMidSponsorMax {
		return 0, false
	}
	if sc.shown(a, b) > combineLogoGap || sc.shown(a-combineMidSponsorAfter, a) > 0 {
		return 0, false
	}

	return sc.program(a, b) + combineSponsorBonus, true
}

// promo は、[a, b) を、局ロゴを出したままの番宣 (ほかの番組の宣伝) とみなした点 (#2168)。15 秒か 30 秒にほぼちょうど合い、
// ロゴが許すロゴの途切れより長く見える区間だけを番宣にし、ほかは ok を false にする。ロゴの減点は掛けない。
// 番宣は CM の並びの端に置き、隣は長い本編にする (並べ方は relax で縛る)。
func (sc *combineScorer) promo(a, b float64) (float64, bool) {
	length := b - a
	if a <= 0 || b >= sc.duration || length < cmUnit-combinePromoTolerance || length > combinePromoMax+combinePromoTolerance {
		return 0, false
	}
	if math.Abs(length-math.Round(length/cmUnit)*cmUnit) > combinePromoTolerance || sc.shown(a, b) <= combineLogoGap {
		return 0, false
	}

	return combineCMWeight*length + combinePromoBonus, true
}

// combineTable は、動的計画法のテーブル。best[j][k] は、j 番目の候補で、直前の区間が k で終わる並びの最高点。
// from[j][k] と prev[j][k] は、その区間の始まりの候補と、その候補で終わっていた区間の状態。
type combineTable struct {
	best [][stateCount]float64
	from [][stateCount]int
	prev [][stateCount]int
}

// solve は、境目の候補 points を順にたどり、テーブルを作る。
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

// relax は、i 番目から j 番目の候補までの区間を、本編、提供、告知、CM、CM のあいだの提供、番宣のそれぞれとみなして、テーブルを更新する。
// 提供の後は、CM か録画の終わりしか続けられない。提供と本編の直後の告知の前は、本編でなければならない。
// 本編の直後の告知から、CM を挟まずに本編へ戻るときは、大きく引く。CM の後の告知だけを、CM の続きとして数える。
// CM のあいだの提供は、CM の後に置き、次も CM にする。ロゴを出したままの番宣は、長い本編と CM の並びのあいだに置く (#2168)。
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

	base, prev := best(returning, stateProgram, stateLongProgram, stateCM, stateHeadCM, stateNotice)
	update(stateProgram, base, prev, sc.program(a, b))
	// 長い本編は、ほかの本編と同じ点で、番宣の後にも続けられる。CM の直後の番宣の後は、長い本編にしなければならない。
	if b-a >= combinePromoProgram {
		base, prev := best(returning, stateProgram, stateLongProgram, stateCM, stateHeadCM, stateNotice, statePromoTail)
		update(stateLongProgram, base, prev, sc.program(a, b))
	}
	if v, ok := sc.promo(a, b); ok {
		// 長い本編の直後の番宣は、次を CM にする。CM の直後の番宣は、次を長い本編にする。番宣が 2 本続いてもよい。
		// 録画の先頭の CM (前の番組の分) の直後は、番組の先頭の本編 (アバンなど) なので、番宣にしない。
		base, prev := best(none, stateLongProgram, statePromoHead)
		update(statePromoHead, base, prev, v)
		base, prev = best(none, stateCM, statePromoTail)
		update(statePromoTail, base, prev, v)
	}
	if v, ok := sc.sponsor(a, b); ok {
		base, prev := best(none, stateProgram, stateLongProgram)
		update(stateSponsor, base, prev, v)
	}
	if v, ok := sc.midSponsor(a, b); ok {
		base, prev := best(none, stateCM, stateHeadCM)
		update(stateMidSponsor, base, prev, v)
	}
	if v, ok := sc.notice(a, b); ok {
		base, prev := best(none, stateProgram, stateLongProgram)
		update(stateNotice, base, prev, v)
		// CM と提供の後の告知だけを、CM の続きとして数える。本編の直後の告知から告知で CM へ移ると、
		// 告知から本編へ戻るときの減点を避けられてしまうため。提供の後に、見逃し配信の案内などを出してから CM に入る並びがある。
		// ただし、提供と告知を合わせて 15 秒の倍数に合うときは、提供の後に続けない。本当の CM の途中に境目の候補があると、
		// CM の先頭を提供に、残りを告知に取れてしまうため。
		from := []int{stateCM, stateHeadCM}
		if sp := t.from[i][stateSponsor]; !math.IsInf(t.best[i][stateSponsor], -1) && cmFit(b-points[sp]) == 0 {
			from = append(from, stateSponsor)
		}
		base, prev = best(none, from...)
		update(stateCM, base, prev, v)
	}
	// 端に接しない CM は、長い CM の最長より遠くの候補からは始められない。
	if a > 0 && b < sc.duration && b-a > combineMaxLongCM+combineLongTolerance {
		return
	}
	if v, ok := sc.cm(a, b); ok {
		base, prev := best(none, stateProgram, stateLongProgram, stateCM, stateHeadCM, stateSponsor, stateNotice, stateMidSponsor, statePromoHead)
		k := stateCM
		if a <= 0 {
			k = stateHeadCM
		}
		update(k, base, prev, v)
	}
}

// trace は、末尾から遡り、CM の区間 (告知と番宣を含む) を始まりの順に返す。録画の終わりは、本編、CM、提供のどれで終わってもよい。
// 本編の直後の告知で終わる並びは、録画の終わりなら CM と同じに扱う。番宣と CM のあいだの提供では終われない。
func (t *combineTable) trace(points []float64) []span {
	j := len(points) - 1
	k := stateProgram
	for _, c := range []int{stateCM, stateHeadCM, stateSponsor, stateNotice, stateLongProgram} {
		if t.best[j][c] > t.best[j][k] {
			k = c
		}
	}
	sections := []span{}
	for j > 0 {
		i := t.from[j][k]
		if k == stateCM || k == stateHeadCM || k == stateNotice || k == statePromoHead || k == statePromoTail {
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

// longCMFit は、CM の最長を超える長さ length の、15 秒の倍数への合い方を、0〜1 で返す (#1958)。
// 最寄りの倍数からのずれが長い CM の許すずれ以上か、長い CM の最長を超えれば 0。
func longCMFit(length float64) float64 {
	if length > combineMaxLongCM+combineLongTolerance {
		return 0
	}
	diff := math.Abs(length - math.Round(length/cmUnit)*cmUnit)
	if diff >= combineLongTolerance {
		return 0
	}

	return 1 - diff/combineLongTolerance
}
