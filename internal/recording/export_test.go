package recording

import (
	"time"

	"github.com/SlashNephy/kotatsu/internal/ts"
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
