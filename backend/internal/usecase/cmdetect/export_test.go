package cmdetect

import (
	"io"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// NewDetectWorkerWith は、FFmpeg の代わりに newDetector の Detector を使う DetectWorker を返す。
func NewDetectWorkerWith(db Database, store *kago.Store, clk clock.Clock, newDetector func() (Detector, error)) *DetectWorker {
	return newDetectWorker(db, store, clk, newDetector)
}

// Signals は、テストから組み立てる手掛かり。時刻は秒で、Silences は無音の区間の [始まり, 終わり] の並び。
type Signals struct {
	Silences     [][2]float64
	Scenes       []float64
	AudioChanges []float64
	Duration     float64
}

func spansOf(pairs [][2]float64) []span {
	spans := make([]span, 0, len(pairs))
	for _, p := range pairs {
		spans = append(spans, span{start: p[0], end: p[1]})
	}

	return spans
}

func pairsOf(spans []span) [][2]float64 {
	pairs := make([][2]float64, 0, len(spans))
	for _, s := range spans {
		pairs = append(pairs, [2]float64{s.start, s.end})
	}

	return pairs
}

// FindSections は、テストから findSections を呼ぶ。区間は [始まり, 終わり] の並び。
func FindSections(s *Signals) [][2]float64 {
	return pairsOf(findSections(&signals{silences: spansOf(s.Silences), scenes: s.Scenes, audioChanges: s.AudioChanges, duration: s.Duration}))
}

// ParseStdout は、テストから parseStdout を呼び、シーンが切り替わった時刻を返す。
func ParseStdout(r io.Reader) []float64 {
	return parseStdout(r).scenes
}

// ParseStderr は、テストから parseStderr を呼び、無音の区間と、残したログの行を返す。
func ParseStderr(r io.Reader) ([][2]float64, []string) {
	log := parseStderr(r)

	return pairsOf(log.silences), log.tail
}

// NewSignals は、テストから newSignals を呼ぶ。
func NewSignals(cues *ts.Cues, scenes []float64, silences [][2]float64) (*Signals, error) {
	sig, err := newSignals(cues, scenes, spansOf(silences))
	if err != nil {
		return nil, err
	}

	return &Signals{Silences: pairsOf(sig.silences), Scenes: sig.scenes, AudioChanges: sig.audioChanges, Duration: sig.duration}, nil
}
