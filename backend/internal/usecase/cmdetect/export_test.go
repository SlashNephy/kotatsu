package cmdetect

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ffprogress"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

// NewDetectWorkerWith は、FFmpeg の代わりに newDetector の Detector を使う DetectWorker を返す。
func NewDetectWorkerWith(db Database, store *kago.Store, clk clock.Clock, newDetector func() (Detector, error)) *DetectWorker {
	return newDetectWorker(db, store, clk, nil, slog.New(slog.DiscardHandler), func(context.Context) (Detector, error) { return newDetector() })
}

// NewDetectWorkerWithHook は、NewDetectWorkerWith に、見分け終えたことを受け取る hook を足したもの。
func NewDetectWorkerWithHook(db Database, store *kago.Store, clk clock.Clock, hook DetectedHook, newDetector func() (Detector, error)) *DetectWorker {
	return newDetectWorker(db, store, clk, hook, slog.New(slog.DiscardHandler), func(context.Context) (Detector, error) { return newDetector() })
}

// NewLogoDetectorWith は、ロゴを使えないときの見分け方を fallback にした LogoDetector を返す。
func NewLogoDetectorWith(fallback Detector) (*LogoDetector, error) {
	d, err := NewLogoDetector(nil)
	if err != nil {
		return nil, err
	}
	d.fallback = fallback

	return d, nil
}

// ComplementSections は complementSections。
func ComplementSections(intervals []logo.Interval, times []float64, fps float64) []Section {
	return complementSections(intervals, times, fps)
}

// CMRatio は cmRatio。
func CMRatio(sections, missing []Section, duration time.Duration) float64 {
	return cmRatio(sections, missing, duration)
}

// MissingSections は missingSections。
func MissingSections(times []float64, fps float64) []Section {
	return missingSections(times, fps)
}

// FrameRate は frameRate。
func FrameRate(s string) float64 {
	return frameRate(s)
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
	log := parseStderr(r, nil)

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

// ParseFrameLog は、parseFrameLog で読んだコマの時刻と、-progress のまとまりの数を返す。
func ParseFrameLog(stderr string) ([]float64, int) {
	blocks := 0
	log := parseFrameLog(strings.NewReader(stderr), func(*ffprogress.Block) { blocks++ })

	return log.times, blocks
}

// TrimMissing は trimMissing。
func TrimMissing(sections, missing []Section) []Section {
	return trimMissing(sections, missing)
}

// EdgeMissing は edgeMissing。
func EdgeMissing(times []float64, fps float64, duration time.Duration) time.Duration {
	return edgeMissing(times, &videoInfo{width: 0, height: 0, fps: fps, duration: duration, start: 0})
}

// LogoDetail は、ロゴの結果を捨てた理由の詳細を、既定のパラメーターで返す。
func LogoDetail(corner logo.Corner, box logo.Box, measures []logo.Measure, intervals []logo.Interval, times []float64, fps float64) string {
	r := &logoResult{
		sections: []Section{}, missing: []Section{}, duration: 0, edgeMissing: 0,
		corner: corner, box: box, measures: measures, intervals: intervals, times: times, fps: fps,
	}

	return r.detail(logo.DefaultParams())
}

// ProgramWindow は programWindow。
func ProgramWindow(intervals []logo.Interval, times []float64, fps float64, program *Program) (float64, float64, bool) {
	w, ok := programWindow(intervals, times, fps, program)

	return w.start, w.end, ok
}

// ClipIntervals は clipIntervals。
func ClipIntervals(intervals []logo.Interval, times []float64, start, end float64) []logo.Interval {
	return clipIntervals(intervals, times, start, end)
}

// DropShortSections は dropShortSections。
func DropShortSections(sections []Section, duration time.Duration) []Section {
	return dropShortSections(sections, duration)
}

// CombineSections は、境目の候補の時刻 (秒) と、録画の長さ、ロゴの出ていた区間から、combineSections の CM の区間を秒の組で返す。
func CombineSections(bounds []float64, duration float64, shown [][2]float64) [][2]float64 {
	bs := make([]boundary, 0, len(bounds))
	for _, at := range bounds {
		bs = append(bs, boundary{at: at, cut: true})
	}
	logoSpans := make([]span, 0, len(shown))
	for _, s := range shown {
		logoSpans = append(logoSpans, span{start: s[0], end: s[1]})
	}
	sections := combineSections(bs, duration, logoSpans)
	out := make([][2]float64, 0, len(sections))
	for _, s := range sections {
		out = append(out, [2]float64{s.start, s.end})
	}

	return out
}

// SetMaterialBounds は、材料に、今の見分け方の境目の候補 (切り替わりのあるもの) と、その基準での長さと、時刻の差を入れる。
func SetMaterialBounds(m *Material, ats []float64, duration, offset float64) {
	m.bounds = make([]boundary, 0, len(ats))
	for _, at := range ats {
		m.bounds = append(m.bounds, boundary{at: at, cut: true})
	}
	m.boundsDuration, m.offset = duration, offset
}

// MaterialBounds は、材料の境目の候補の時刻と、その基準での長さと、時刻の差を返す。候補がなければ nil。
func MaterialBounds(m *Material) ([]float64, float64, float64) {
	if m.bounds == nil {
		return nil, m.boundsDuration, m.offset
	}
	ats := make([]float64, 0, len(m.bounds))
	for _, b := range m.bounds {
		ats = append(ats, b.at)
	}

	return ats, m.boundsDuration, m.offset
}
