package recording

import (
	"bufio"
	"io"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/ts"
)

// flushInterval は、書き込みのバッファを、たまっていなくても書き出す間隔 (#525)。
// ビットレートが低いと、バッファが埋まるまで数分かかり、録画ファイルの更新時刻が遅れる。
// 遅れると、落ちた mikan のものとみなして早すぎる時刻で閉じ、実際にはない途切れを記録してしまう。
const flushInterval = time.Second

// qualityCounts は、録画の品質の集計 (ドロップ、エラー、スクランブル)。
type qualityCounts struct {
	Drops     int64
	Errors    int64
	Scrambled int64
}

func countsOf(stats *ts.Stats) qualityCounts {
	counts := qualityCounts{Drops: drops(stats), Errors: 0, Scrambled: 0}
	for _, pid := range stats.PIDs {
		counts.Errors += pid.Errors
		counts.Scrambled += pid.Scrambled
	}

	return counts
}

func (c qualityCounts) minus(o qualityCounts) qualityCounts {
	return qualityCounts{Drops: c.Drops - o.Drops, Errors: c.Errors - o.Errors, Scrambled: c.Scrambled - o.Scrambled}
}

// qualitySample は、ある時刻までに数えた、セグメントの品質の集計。
type qualitySample struct {
	At     time.Time
	Counts qualityCounts
}

// countsAfter は、end に書き終えたセグメントの最後の集計 final のうち、since より後に数えた分を、控えた集計 samples から求める。
// since より前に控えたものがなければ、セグメントの全体を since より後とみなす。書き終えた後なら、重なっていない。
func countsAfter(samples []qualitySample, since, end time.Time, final qualityCounts) qualityCounts {
	if !since.Before(end) {
		return qualityCounts{Drops: 0, Errors: 0, Scrambled: 0}
	}
	var before *qualitySample
	for i := range samples {
		if samples[i].At.After(since) {
			break
		}
		before = &samples[i]
	}
	if before == nil {
		return final
	}

	return final.minus(before.Counts)
}

// qualitySamples は、セグメントを書いている間に、品質の集計を一定の間隔で控える。
// 引き継ぎで重なった区間の分を、古い mikan の集計から差し引くのに使う (#525)。
type qualitySamples struct {
	mu      sync.Mutex
	samples []qualitySample
}

func (s *qualitySamples) add(at time.Time, counts qualityCounts) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = append(s.samples, qualitySample{At: at, Counts: counts})
}

func (s *qualitySamples) list() []qualitySample {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]qualitySample{}, s.samples...)
}

var _ io.Writer = new(flushingWriter)

// flushingWriter は、書き込みのバッファを、flushInterval ごとに書き出す。書き込みが途絶えても、flushIfStale で書き出す。
type flushingWriter struct {
	mu     sync.Mutex
	buffer *bufio.Writer
	last   time.Time
}

func (w *flushingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buffer.Write(p)
	if err != nil {
		return n, err
	}

	return n, w.flushIfStaleLocked()
}

// flushIfStale は、前に書き出してから flushInterval が過ぎていれば、バッファを書き出す。
func (w *flushingWriter) flushIfStale() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.flushIfStaleLocked()
}

func (w *flushingWriter) flushIfStaleLocked() error {
	now := time.Now()
	if now.Sub(w.last) < flushInterval || w.buffer.Buffered() == 0 {
		return nil
	}
	w.last = now

	return w.buffer.Flush()
}
