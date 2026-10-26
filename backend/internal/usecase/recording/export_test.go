package recording

import (
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// BroadcastTracker は、テストから broadcastTracker を使うために公開する。
type BroadcastTracker struct {
	tracker *broadcastTracker
}

// NewBroadcastTracker は、テストから newBroadcastTracker を呼ぶために公開する。
func NewBroadcastTracker(eventID uint16) *BroadcastTracker {
	return &BroadcastTracker{tracker: newBroadcastTracker(eventID)}
}

// Update は、テストから broadcastTracker.update を呼ぶために公開する。放送中になったか、放送中でなくなったかを返す。
func (b *BroadcastTracker) Update(pf *ts.PresentFollowing, now time.Time) (bool, bool) {
	change := b.tracker.update(pf, now)

	return change.started, change.ended
}

// End は、テストから broadcastTracker.end を呼ぶために公開する。
func (b *BroadcastTracker) End(planned, now time.Time, margin, step time.Duration, limit time.Time) (time.Time, bool) {
	return b.tracker.end(planned, now, margin, step, limit)
}

// QualityCounts は、テストから qualityCounts を使うために公開する。
type QualityCounts = qualityCounts

// QualitySample は、テストから qualitySample を使うために公開する。
type QualitySample = qualitySample

// CountsAfter は、テストから countsAfter を呼ぶために公開する。
func CountsAfter(samples []QualitySample, since, end time.Time, final QualityCounts) QualityCounts {
	return countsAfter(samples, since, end, final)
}

// Preroll は、テストから preroll を使うために公開する。
type Preroll struct {
	preroll *preroll
}

// NewPreroll は、テストから newPreroll を呼ぶために公開する。
func NewPreroll(window time.Duration) *Preroll {
	return &Preroll{preroll: newPreroll(window)}
}

// Add は、テストから preroll.add を呼ぶために公開する。
func (p *Preroll) Add(at time.Time, b []byte) {
	p.preroll.add(at, b)
}

// Bytes と StartedAt は、テストから preroll.bytes と preroll.startedAt を呼ぶために公開する。
func (p *Preroll) Bytes() []byte {
	return p.preroll.bytes()
}

func (p *Preroll) StartedAt(now time.Time) time.Time {
	return p.preroll.startedAt(now)
}
