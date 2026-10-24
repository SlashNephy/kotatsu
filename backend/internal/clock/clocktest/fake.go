// Package clocktest は、テストで時刻を操作するための clock.Clock の実装を持つ。
package clocktest

import (
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/clock"
)

// Fake は、Advance や Set で進めない限り止まっている時計。複数の goroutine から使ってよい。
type Fake struct {
	mu  sync.Mutex
	now time.Time
	// elapsed は、作ってから Advance で進めた時間の合計。Stopwatch はこれで測る。
	elapsed time.Duration
}

var _ clock.Clock = new(Fake)

// NewFake は now で止まっている時計を返す。
func NewFake(now time.Time) *Fake {
	return &Fake{mu: sync.Mutex{}, now: now.UTC(), elapsed: 0}
}

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.now
}

// Advance は時計を d だけ進める。Stopwatch の経過時間も d だけ進む。
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.now = f.now.Add(d)
	f.elapsed += d
}

// Set は時計を now に合わせる。壁時計が飛んだ (NTP が時計を合わせた) ときと同じく、Stopwatch の経過時間は変えない。
func (f *Fake) Set(now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.now = now.UTC()
}

// StartStopwatch は、Advance で進めた時間だけを経過時間とする Stopwatch を返す。
func (f *Fake) StartStopwatch() clock.Stopwatch {
	f.mu.Lock()
	defer f.mu.Unlock()

	return &fakeStopwatch{clock: f, start: f.elapsed}
}

type fakeStopwatch struct {
	clock *Fake
	start time.Duration
}

var _ clock.Stopwatch = new(fakeStopwatch)

func (s *fakeStopwatch) Elapsed() time.Duration {
	s.clock.mu.Lock()
	defer s.clock.mu.Unlock()

	return s.clock.elapsed - s.start
}
