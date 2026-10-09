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
}

var _ clock.Clock = new(Fake)

// NewFake は now で止まっている時計を返す。
func NewFake(now time.Time) *Fake {
	return &Fake{mu: sync.Mutex{}, now: now.UTC()}
}

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.now
}

// Advance は時計を d だけ進める。
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.now = f.now.Add(d)
}
