package logo

import (
	"math"
	"slices"
	"time"
)

// Interval は、ロゴが出ていた区間。Start と End はコマの番号で、End は含まない。
type Interval struct {
	Start int
	End   int
}

// frames は、d に当たるコマの数を返す。
func frames(d time.Duration, fps float64) int {
	return int(math.Round(d.Seconds() * fps))
}

// Intervals は、コマごとの測り measures から、ロゴが出ていた区間を返す (#1505 の E と F)。fps は、コマの速さ。
//
// 半透明の白いロゴは、白っぽい背景では点数が 0 近くまで落ちる。それを「消えた」と読むと、本編の明るい場面のたびに区間が切れるので、
// 判断できない背景のコマでは、直前の判断を持ち越す。どの背景が判断できないかは、録画ごとに映像から 2 段で決める。
// 1 段目は明るさの段ごとに、ロゴの出ているコマの近くで点数の上位が届くかを見る。2 段目は、1 回目の区間の中で、
// 明るさと肌理の格子ごとに、続いているとみなす点数に届かないコマの割合を見る。
func Intervals(measures []Measure, fps float64, p *Params) []Interval {
	if len(measures) == 0 {
		return []Interval{}
	}
	judge := levelJudgement(measures, fps, p)
	flatten(judge, measures, p)
	first := extract(measures, judge, fps, p)
	judge = gridJudgement(measures, judge, first, fps, p)
	flatten(judge, measures, p)

	return extract(measures, judge, fps, p)
}

// brightnessLevel は、m の明るさの段を返す。
func brightnessLevel(m Measure, p *Params) int {
	return min(int(m.Brightness/256*float64(p.BrightnessLevels)), p.BrightnessLevels-1)
}

// textureLevel は、m の肌理の段を返す。
func textureLevel(m Measure, p *Params) int {
	for i, edge := range p.TextureEdges {
		if m.Texture < edge {
			return i
		}
	}

	return len(p.TextureEdges)
}

// levelJudgement は、1 段目の判断を返す。点数が出たとみなす点数以上のコマの近くのコマだけで、明るさの段ごとに、
// 段の分位の点数が出たとみなす点数に届くかを見る。CM の間しか出ない背景まで数えると、その段が判断できない段にされ、
// CM の頭まで判断が持ち越されるからである。コマの足りない段は、確かめようがないので判断できるとする。
func levelJudgement(measures []Measure, fps float64, p *Params) []bool {
	reach := frames(p.NearWindow, fps)
	near := make([]bool, len(measures))
	// last は、直前 (後ろからなら直後) の、出たとみなす点数以上のコマ。まだなければ -1。
	last := -1
	for i, m := range measures {
		if m.Score >= p.AppearScore {
			last = i
		}
		if last >= 0 && i-last <= reach {
			near[i] = true
		}
	}
	last = -1
	for i := len(measures) - 1; i >= 0; i-- {
		if measures[i].Score >= p.AppearScore {
			last = i
		}
		if last >= 0 && last-i <= reach {
			near[i] = true
		}
	}
	scores := make([][]float64, p.BrightnessLevels)
	for i, m := range measures {
		if near[i] {
			l := brightnessLevel(m, p)
			scores[l] = append(scores[l], m.Score)
		}
	}
	minFrames := frames(p.LevelMinDuration, fps)
	levels := make([]bool, p.BrightnessLevels)
	for l, s := range scores {
		if len(s) < minFrames {
			levels[l] = true

			continue
		}
		slices.Sort(s)
		levels[l] = quantile(s, p.LevelQuantile) >= p.AppearScore
	}
	judge := make([]bool, len(measures))
	for i, m := range measures {
		judge[i] = levels[brightnessLevel(m, p)]
	}

	return judge
}

// gridJudgement は、2 段目の判断を返す。1 回目の区間 intervals の中のコマを、明るさと肌理の格子ごとに数え、
// 続いているとみなす点数に届かないコマの割合が許容割合以下の格子だけを判断できるとする。コマの足りない格子は、1 段目の判断 prev のままにする。
func gridJudgement(measures []Measure, prev []bool, intervals []Interval, fps float64, p *Params) []bool {
	cells := p.BrightnessLevels * (len(p.TextureEdges) + 1)
	cell := func(m Measure) int { return brightnessLevel(m, p)*(len(p.TextureEdges)+1) + textureLevel(m, p) }
	count := make([]int, cells)
	miss := make([]int, cells)
	for _, iv := range intervals {
		for i := iv.Start; i < iv.End; i++ {
			c := cell(measures[i])
			count[c]++
			if measures[i].Score < p.ContinueScore {
				miss[c]++
			}
		}
	}
	minFrames := frames(p.LevelMinDuration, fps)
	judge := make([]bool, len(measures))
	for i, m := range measures {
		c := cell(m)
		if count[c] < minFrames {
			judge[i] = prev[i]

			continue
		}
		judge[i] = float64(miss[c])/float64(count[c]) <= p.GridMissRatio
	}

	return judge
}

// flatten は、のっぺりしたコマ (肌理がのっぺりの肌理未満で、白飛びでないもの) を、いつも判断できるとする。
// ロゴが乗っていれば必ず縁が立つからである。番組の切れ目の真っ黒を判断できない段にすると、番組の終わりのロゴが黒い間ずっと持ち越される。
func flatten(judge []bool, measures []Measure, p *Params) {
	for i, m := range measures {
		if m.Texture < p.FlatTexture && m.Brightness < p.WhiteBrightness {
			judge[i] = true
		}
	}
}

// extract は、判断 judge のもとで、区間を出す (#1505 の F)。
func extract(measures []Measure, judge []bool, fps float64, p *Params) []Interval {
	values := windowValues(measures, judge, fps, p)
	// ヒステリシス。不明の間は前の状態を持ち越す。
	on := make([]bool, len(measures))
	state := false
	for i, v := range values {
		if !math.IsNaN(v) {
			if !state && v >= p.AppearScore {
				state = true
			} else if state && v < p.ContinueScore {
				state = false
			}
		}
		on[i] = state
	}
	intervals := runs(on)
	intervals = pullBack(intervals, measures, fps, p)
	intervals = fill(intervals, frames(p.FillGap, fps))

	out := []Interval{}
	minLength := frames(p.MinInterval, fps)
	for _, iv := range intervals {
		if iv.End-iv.Start >= minLength {
			out = append(out, iv)
		}
	}

	return out
}

// windowValues は、コマごとに、前後の窓の判断できるコマの中で、窓の順位番目に高い点数を返す。足りなければ NaN (不明)。
// 最高点でなく下の順位を見るのは、溶けかけたロゴでも、少し暗い所が横切るたびに点数が戻るからである。
func windowValues(measures []Measure, judge []bool, fps float64, p *Params) []float64 {
	half := frames(p.WindowHalf, fps)
	need := max(p.WindowRank, int(math.Ceil(float64(2*half+1)*p.WindowMinRatio)))
	values := make([]float64, len(measures))
	window := make([]float64, 0, 2*half+1)
	for i := range measures {
		window = window[:0]
		for j := max(i-half, 0); j <= min(i+half, len(measures)-1); j++ {
			if judge[j] {
				window = append(window, measures[j].Score)
			}
		}
		if len(window) < need {
			values[i] = math.NaN()

			continue
		}
		slices.Sort(window)
		values[i] = window[len(window)-p.WindowRank]
	}

	return values
}

// runs は、on が続く所を区間にして返す。
func runs(on []bool) []Interval {
	out := []Interval{}
	for i := 0; i < len(on); {
		if !on[i] {
			i++

			continue
		}
		start := i
		for i < len(on) && on[i] {
			i++
		}
		out = append(out, Interval{Start: start, End: i})
	}

	return out
}

// pullBack は、窓で外に膨らんだ境目を、点数を中央値でならした値で戻す。始まりは、ならした値が中間値未満の間、後ろへずらし、
// 終わりは前へずらす。どちらも戻しの上限まで。始まりをずらし切っても中間値に届かず、上限にも達しなかった区間は捨てる。
func pullBack(intervals []Interval, measures []Measure, fps float64, p *Params) []Interval {
	smoothed := median(measures, p.SmoothFrames)
	mid := (p.AppearScore + p.ContinueScore) / 2
	limit := frames(p.PullbackMax, fps)
	out := []Interval{}
	for _, iv := range intervals {
		start := iv.Start
		for start < iv.End && start-iv.Start < limit && smoothed[start] < mid {
			start++
		}
		if start == iv.End {
			continue
		}
		end := iv.End
		for end > start && iv.End-end < limit && smoothed[end-1] < mid {
			end--
		}
		if start >= end {
			continue
		}
		out = append(out, Interval{Start: start, End: end})
	}

	return out
}

// median は、点数を前後 width コマの中央値でならした値を返す。
func median(measures []Measure, width int) []float64 {
	half := width / 2
	out := make([]float64, len(measures))
	window := make([]float64, 0, width+1)
	for i := range measures {
		window = window[:0]
		for j := max(i-half, 0); j <= min(i+half, len(measures)-1); j++ {
			window = append(window, measures[j].Score)
		}
		slices.Sort(window)
		out[i] = window[len(window)/2]
	}

	return out
}

// fill は、gap コマ未満の途切れを埋める。
func fill(intervals []Interval, gap int) []Interval {
	out := []Interval{}
	for _, iv := range intervals {
		if n := len(out); n > 0 && iv.Start-out[n-1].End < gap {
			out[n-1].End = iv.End

			continue
		}
		out = append(out, iv)
	}

	return out
}
