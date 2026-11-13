package transcode

import (
	"math"
	"slices"
	"time"
)

//go:generate go tool go-enum --values --nocomments

// Cadence は、録画の映像の窓を、フィールドどうしの差から分類したもの (#1509、設計書 6.1)。
//
// ENUM(
// unknown // 動いている秒が足りないか、FFmpeg が失敗して、測れなかったこと。
// 24p // 24p を 3:2 プルダウンで載せた映像 (アニメやフィルム)。5 フィールドに 1 回、同じフィールドが繰り返される。
// 30p // 上下のフィールドが同じ時刻の絵である 30p の映像。
// 60i // フィールドごとに時刻の違う 60i の映像 (実写)。
// )
type Cadence string

const (
	// topPerSecond は、上フィールドの差の 1 秒あたりの数。
	topPerSecond = 30
	// fieldsPerSecond は、上下を交互に並べたフィールドの差の 1 秒あたりの数。
	fieldsPerSecond = 60
	// pulldownCycle は、3:2 プルダウンで、上フィールドが繰り返される周期 (上フィールドの数)。
	pulldownCycle = 5
)

// CadenceParams は、窓の取り方と、窓を分類する閾値 (#1509、設計書の付録 A)。
type CadenceParams struct {
	// Window は、1 つの窓で測る時間。
	Window time.Duration
	// Windows は、録画全体に散らして置く窓の数。
	Windows int
	// MotionThreshold は、動いている秒とする、その 1 秒の上フィールドの差の平均の下限。静止や黒の秒は数えない。
	MotionThreshold float64
	// MinSeconds は、窓を測れたとするのに要る、動いている秒の数。
	MinSeconds int
	// RepThreshold は、24p 系と、30p・60i を分ける繰り返し度 (rep) の値。
	RepThreshold float64
	// WRThreshold は、30p と 60i を分ける上下の揃い度 (wr) の値。
	WRThreshold float64
}

// DefaultCadenceParams は、窓の取り方と閾値の既定値を返す。合成の映像 (testsrc2 から作ったテレシネ、30p、60i、黒) で測った値で決めてあり、
// 本番の録画ではまだ測っていない (設計書の付録 A)。
func DefaultCadenceParams() *CadenceParams {
	return &CadenceParams{
		Window:          20 * time.Second,
		Windows:         9,
		MotionThreshold: 1,
		MinSeconds:      5,
		RepThreshold:    0.5,
		WRThreshold:     0.6,
	}
}

// FieldDiffs は、1 つの窓の、フィールドどうしの輝度の差 (signalstats の YDIF)。どちらも、前がないので意味のない先頭の値を捨ててある。
type FieldDiffs struct {
	// Top は、上フィールドだけを並べ、前の上フィールドとの差を並べたもの (1 秒に 30 個)。
	Top []float64
	// Fields は、上下のフィールドを交互に並べ、隣との差を並べたもの (1 秒に 60 個)。偶数番目が同じコマの上と下の差、
	// 奇数番目がコマをまたぐ差になる。
	Fields []float64
}

// WindowResult は、1 つの窓を測った結果。
type WindowResult struct {
	// Rep は、繰り返し度 (動いている秒ごとの値の中央値)。RepSeconds は、それを数えた秒の数。
	Rep        float64
	RepSeconds int
	// WR は、上下の揃い度 (動いている秒ごとの値の中央値)。WRSeconds は、それを数えた秒の数。
	WR        float64
	WRSeconds int
	Cadence   Cadence
}

// Classify は、窓のフィールドどうしの差 d から、rep と wr を求め、窓を分類する。
func (p *CadenceParams) Classify(d *FieldDiffs) *WindowResult {
	moving := p.movingSeconds(d.Top)
	reps := repetitions(d.Top, moving)
	ratios := weaveRatios(d.Fields, moving)
	result := &WindowResult{Rep: median(reps), RepSeconds: len(reps), WR: median(ratios), WRSeconds: len(ratios), Cadence: CadenceUnknown}
	switch {
	case result.RepSeconds < p.MinSeconds:
	case result.Rep < p.RepThreshold:
		result.Cadence = Cadence24P
	case result.WRSeconds < p.MinSeconds:
	case result.WR < p.WRThreshold:
		result.Cadence = Cadence30P
	default:
		result.Cadence = Cadence60I
	}

	return result
}

// movingSeconds は、上フィールドの差 top を 1 秒ずつに区切り、その 1 秒の差の平均が動きの閾値以上の秒を返す。静止や黒の秒は数えない。
// 動いているかは、上フィールドの差 (前の上フィールドとの、時刻の違う絵の差) だけで決め、上下を交互に並べた差にも同じ秒を使う。
// 上下を交互に並べた差は、上と下で行がずれるので、静止した絵でも細かい模様の分だけ大きく出るため。
func (p *CadenceParams) movingSeconds(top []float64) []bool {
	moving := make([]bool, len(top)/topPerSecond)
	for s := range moving {
		moving[s] = mean(top[s*topPerSecond:(s+1)*topPerSecond]) >= p.MotionThreshold
	}

	return moving
}

// repetitions は、上フィールドの差 top を 1 秒ずつに区切り、動いている秒ごとの繰り返し度を返す。
// 差を、添字 mod 5 の 5 つの位相に分けて合計し、最小の位相の合計を、残り 4 つの合計の平均で割る。3:2 テレシネでは、
// 同じ上フィールドが 5 回ごとに 1 回繰り返されるので、0 に近くなる。60i と 30p は 1 に近くなる。
// 位相は窓の中の通しの添字で決めるので、秒の区切りとプルダウンの周期がずれていても、繰り返す差は同じ位相に集まる。
func repetitions(top []float64, moving []bool) []float64 {
	reps := []float64{}
	for s, ok := range moving {
		if !ok {
			continue
		}
		var phases [pulldownCycle]float64
		for i := s * topPerSecond; i < (s+1)*topPerSecond; i++ {
			phases[i%pulldownCycle] += top[i]
		}
		lowest := slices.Min(phases[:])
		var total float64
		for _, sum := range phases {
			total += sum
		}
		if others := (total - lowest) / (pulldownCycle - 1); others > 0 {
			reps = append(reps, lowest/others)
		}
	}

	return reps
}

// weaveRatios は、上下を交互に並べたフィールドの差 fields を 1 秒ずつに区切り、動いている秒ごとの上下の揃い度を返す。
// 偶数番目 (同じコマの上と下の差) の平均と、奇数番目 (コマをまたぐ差) の平均の、小さいほうを大きいほうで割る。
// 30p は上下が同じ時刻の絵なので低く、60i とテレシネは 1 に近くなる。
func weaveRatios(fields []float64, moving []bool) []float64 {
	ratios := []float64{}
	for s, ok := range moving {
		if !ok || (s+1)*fieldsPerSecond > len(fields) {
			continue
		}
		var within, across float64
		for i := s * fieldsPerSecond; i < (s+1)*fieldsPerSecond; i++ {
			if i%2 == 0 {
				within += fields[i]
			} else {
				across += fields[i]
			}
		}
		if larger := math.Max(within, across); larger > 0 {
			ratios = append(ratios, math.Min(within, across)/larger)
		}
	}

	return ratios
}

// FieldOutput は、窓の分類の多数決から、フィールドごとに 1 コマにして 60 コマで出すかどうかを返す。
// 60i の窓の数が、24p 系と 30p の窓の数の合計以上なら 60 コマにする。測れた窓が 1 つもないときも 60 コマにする。
// 本当に 60i の素材を 30 コマにすると動きが壊れるが、逆の誤りなら無駄が出るだけなので、迷ったら 60 コマにする。
func FieldOutput(results []*WindowResult) bool {
	var interlaced, progressive int
	for _, r := range results {
		switch r.Cadence {
		case Cadence60I:
			interlaced++
		case Cadence24P, Cadence30P:
			progressive++
		case CadenceUnknown:
		}
	}

	return interlaced >= progressive
}

// WindowPositions は、長さ duration の録画に、長さ window の窓を n 個置く位置を返す。(duration − window) を n + 1 等分した
// 内側の点に置き、録画全体に散らす。冒頭と末尾には寄せない。尺が窓より短く位置が重なるときは、同じ位置を 2 回測らない。
// 位置は秒に丸める。
func WindowPositions(duration, window time.Duration, n int) []time.Duration {
	span := max(duration-window, 0)
	positions := []time.Duration{}
	for i := 1; i <= n; i++ {
		at := (span * time.Duration(i) / time.Duration(n+1)).Round(time.Second)
		if len(positions) == 0 || positions[len(positions)-1] != at {
			positions = append(positions, at)
		}
	}

	return positions
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}

	return sum / float64(len(values))
}

// median は、values の中央値を返す。空なら 0 を返す。
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := slices.Sorted(slices.Values(values))
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}

	return sorted[mid]
}
