package cmdetect

import (
	"math"
	"time"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

// Program は、録画した番組の、番組表の時刻 (#1506)。録画の前後のマージンには前後の番組が入るので、
// 番組の尺に収まる窓を置き、窓の外のロゴを前後の番組のものとして捨てるのに使う。
type Program struct {
	// Start は、番組が始まるはずの時刻を、ファイルの始まりからの時間で表したもの。録画を番組より遅れて始めれば負になる。
	Start time.Duration
	// Duration は、番組の尺 (番組表の終わり − 始まり)。
	Duration time.Duration
}

const (
	// windowEdgeExclusion は、窓の置き場所の候補から除く、ファイルの始まりと終わりからの幅 (端の除外幅)。
	// ファイルの始まりで始まるロゴの区間は、前の番組から続くロゴかもしれず、そこに窓の頭を合わせる理由にならない。
	windowEdgeExclusion = 2 * time.Second
	// windowShiftWeight は、窓の置き場所が、番組が始まるはずの時刻からずれた 1 秒ごとに引く、ロゴの秒数 (窓のずれの重み)。
	// 先頭のロゴを短くしか捉えられなかった録画で、末尾に入った次の番組のロゴに窓が引っ張られないようにする。
	// 大きく遅れた放送は、ずらした分だけロゴが多く入るので、この減点を上回る。
	windowShiftWeight = 0.5
	// windowTie は、最高点からこの差の内の候補を同点とみなす幅 (秒)。同点なら、番組が始まるはずの時刻に近いものを採る。
	windowTie = 1.0
)

// programWindow は、番組の尺の窓を、ロゴの出ていた区間 intervals に合わせて置いて返す (#1506 の手順 1)。
// times はコマごとの時刻 (秒)、fps はコマの速さ、program は番組の時刻 (ファイルの始まりから)。
// 番組の尺が録画より短いときだけ置き、置けなければ ok を false にする。ロゴから候補が 1 つも作れないときも、番組の頭を削らないよう置かない。
// 窓の端を無音の境目に寄せることは、無音の材料がまだないので行わない。
func programWindow(intervals []logo.Interval, times []float64, fps float64, program *Program) (window, bool) {
	none := window{start: 0, end: 0}
	if program == nil || program.Duration <= 0 || len(times) == 0 {
		return none, false
	}
	origin := times[0]
	at := func(frame int) float64 {
		if frame < len(times) {
			return times[frame] - origin
		}

		return times[len(times)-1] - origin + 1/fps
	}
	total := at(len(times))
	length := program.Duration.Seconds()
	if length >= total {
		return none, false
	}
	// 番組が始まるはずの時刻を、最初のコマからの秒にする。ファイルの始まりから最初のコマまでの空きを引く。
	expected := program.Start.Seconds() - origin
	edge := windowEdgeExclusion.Seconds()
	candidates := []float64{}
	for _, iv := range intervals {
		if s := at(iv.Start); s > edge && s <= total-length {
			candidates = append(candidates, s)
		}
		if e := at(iv.End); e < total-edge && e-length >= 0 {
			candidates = append(candidates, e-length)
		}
	}
	if len(candidates) == 0 {
		return none, false
	}
	score := func(pos float64) float64 {
		var shown float64
		for _, iv := range intervals {
			shown += max(0, min(at(iv.End), pos+length)-max(at(iv.Start), pos))
		}

		return shown - windowShiftWeight*math.Abs(pos-expected)
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

	return window{start: chosen, end: chosen + length}, true
}

// window は、番組の窓。始まりと終わりは、最初のコマからの秒。
type window struct {
	start, end float64
}

// clipIntervals は、ロゴの出ていた区間を、窓 [start, end) (最初のコマからの秒) に収める。窓の外のロゴは、前後の番組のものとして捨てる。
func clipIntervals(intervals []logo.Interval, times []float64, start, end float64) []logo.Interval {
	origin := times[0]
	// first は、窓の内に入る最初のコマ、last は、窓の外に出る最初のコマ。
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
