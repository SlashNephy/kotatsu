package ffprogress

import (
	"time"
)

const (
	// Cap は、progress=end が来るまでの割合の上限 (頭打ちの割合)。最後のコマの後にも、溜めたコマの書き出しと、
	// mux の後処理 (MP4 の索引を先頭に移す faststart) が残るので、早く 100% を出すと止まって見える。
	// 見込みのコマ数は、合成の録画 (2 分、29.97fps) で実際のコマ数と一致し (3596)、索引から求める尺のずれは最後の GOP の長さ
	// (0.5 秒ほど、30 分の録画で 0.03%) なので、見込みが実際より大きくても、バーは 99% に届く前にほとんど止まらない。
	Cap = 0.99
	// Window は、残り時間の計算に使う直近の時間 (残り時間の窓)。低い優先度で動かすので、ライブ視聴の変換が始まると速さが変わる。
	// 1 分なら、速さの変化に 1 分で追いつき、0.5 秒ごとのまとまりの揺れは 120 個で均される。
	Window = time.Minute
	// MinWindow は、残り時間を出すのに要る窓の幅 (残り時間の最短の窓)。-progress のまとまりの間隔 (0.5 秒) の 20 倍にし、
	// 始まってすぐの、溜めたコマを出す前の速さで、桁違いの残り時間を出さないようにする。
	MinWindow = 10 * time.Second
)

// Config は、割合と残り時間の決め方。
type Config struct {
	// Cap は、progress=end が来るまでの割合の上限。1 より小さくする。
	Cap float64
	// Window は、残り時間の計算に使う直近の時間。
	Window time.Duration
	// MinWindow は、残り時間を出すのに要る窓の幅。Window より短くする。
	MinWindow time.Duration
}

// DefaultConfig は、既定の Config を返す (設計書 付録 A)。
func DefaultConfig() *Config {
	return &Config{Cap: Cap, Window: Window, MinWindow: MinWindow}
}

// Estimate は、割合の分母。
type Estimate struct {
	// Frames は、見込みのコマ数。0 なら出せない (映像がない、尺やコマの速さが分からない)。
	Frames float64
	// Bytes は、入力の大きさ (バイト)。Frames が 0 のときに、読み位置の割合に使う。0 なら分からない。
	Bytes int64
}

// Snapshot は、ある時点の進み具合。
type Snapshot struct {
	// Ratio は、0 から 1 の割合。前の値より下がらない。
	Ratio float64
	// Remaining は、残り時間。nil なら不明。
	Remaining *time.Duration
}

// source は、割合の出どころ。
type source int

const (
	sourceNone source = iota
	sourceFrames
	sourceBytes
)

type sample struct {
	at    time.Time
	ratio float64
}

// Tracker は、-progress のまとまりから、割合と残り時間を求める。
//
// 出どころ (コマ数か読み位置) は、作ったときに 1 つに決め、途中で切り替えない。止まった数を動いている速さで割って、
// 桁違いに長い残り時間を出さないようにするためである。
type Tracker struct {
	cfg     *Config
	est     Estimate
	source  source
	ratio   float64
	samples []sample
}

// NewTracker は Tracker を返す。見込みのコマ数があればコマ数で、なければ入力の読み位置で割合を出す。どちらもなければ、0 のまま。
func NewTracker(cfg *Config, est Estimate) *Tracker {
	src := sourceNone
	switch {
	case est.Frames > 0:
		src = sourceFrames
	case est.Bytes > 0:
		src = sourceBytes
	}

	return &Tracker{cfg: cfg, est: est, source: src, ratio: 0, samples: []sample{}}
}

// Observe は、時刻 now に届いたまとまり b と、そのときまでに FFmpeg に流した入力のバイト数 read から、進み具合を返す。
// 割合は前の値より下げない。progress=end までは Cap で止め、progress=end で 1 にする。
// 分子 (コマ数か読み位置) が取れなければ、前の値を保つ。
func (t *Tracker) Observe(now time.Time, b *Block, read int64) Snapshot {
	if b.End {
		t.ratio = 1
	} else if ratio, ok := t.measure(b, read); ok {
		t.ratio = max(t.ratio, min(ratio, t.cfg.Cap))
	}
	t.samples = append(t.samples, sample{at: now, ratio: t.ratio})
	// 窓より古い記録を捨てる。
	cutoff := now.Add(-t.cfg.Window)
	drop := 0
	for drop < len(t.samples)-1 && t.samples[drop].at.Before(cutoff) {
		drop++
	}
	t.samples = t.samples[drop:]

	return Snapshot{Ratio: t.ratio, Remaining: t.remaining(now)}
}

// measure は、まとまりの分子を、見込みで割る。
func (t *Tracker) measure(b *Block, read int64) (float64, bool) {
	switch t.source {
	case sourceFrames:
		if b.Frame < 0 {
			return 0, false
		}

		return float64(b.Frame) / t.est.Frames, true
	case sourceBytes:
		if read < 0 {
			return 0, false
		}

		return float64(read) / float64(t.est.Bytes), true
	case sourceNone:
		return 0, false
	}

	return 0, false
}

// remaining は、窓の中の割合の増え方から、残り時間を求める。窓の幅が MinWindow に満たないか、割合が増えていなければ、不明 (nil) にする。
func (t *Tracker) remaining(now time.Time) *time.Duration {
	if t.ratio >= 1 {
		return new(time.Duration(0))
	}
	oldest := t.samples[0]
	width := now.Sub(oldest.at)
	gained := t.ratio - oldest.ratio
	if width < t.cfg.MinWindow || gained <= 0 {
		return nil
	}

	return new(time.Duration((1 - t.ratio) / gained * float64(width)))
}
