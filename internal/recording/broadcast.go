package recording

import (
	"time"

	"github.com/SlashNephy/kotatsu/internal/ts"
)

// broadcastTracker は、録画中の TS の EIT[p/f] から、予約した番組が実際に放送されているかを追う (設計書 5.1、#389)。
// 番組表 (EIT[schedule]) に時間の重なった番組があっても、EIT[p/f] の放送中の番組は 1 つだけなので、実際に放送されている番組が分かる。
// 番組は eventId で見分ける。eventId が同じなら、番組名が変わっても同じ番組として追う。
type broadcastTracker struct {
	eventID uint16
	// observed は、サービスの EIT[p/f] を一度でも読んだかどうか。読めない TS (データの流れないサービスなど) では、何も判断しない。
	observed bool
	// startedAt と endedAt は、予約した番組が放送中になった時刻と、放送中でなくなった時刻。
	startedAt *time.Time
	endedAt   *time.Time
	// following は、最後に読んだ次の番組が、予約した番組だったかどうか。
	following bool
	// present は、最後に読んだ放送中の番組の eventId。
	present *uint16
}

// broadcastChange は、update で変わったこと。
type broadcastChange struct {
	// started と ended は、予約した番組が放送中になったか、放送中でなくなったか。
	started bool
	ended   bool
}

func newBroadcastTracker(eventID uint16) *broadcastTracker {
	return &broadcastTracker{eventID: eventID, observed: false, startedAt: nil, endedAt: nil, following: false, present: nil}
}

// update は、今読んだ EIT[p/f] pf (nil ならまだ読んでいない) で、放送の状態を進める。
func (b *broadcastTracker) update(pf *ts.PresentFollowing, now time.Time) broadcastChange {
	change := broadcastChange{started: false, ended: false}
	if pf == nil || !pf.PresentSeen {
		return change
	}
	b.observed = true
	b.present = pf.Present
	b.following = pf.FollowingSeen && pf.Following != nil && *pf.Following == b.eventID
	onAir := pf.Present != nil && *pf.Present == b.eventID
	switch {
	case onAir && b.startedAt == nil:
		b.startedAt = new(now)
		change.started = true
	case !onAir && b.startedAt != nil && b.endedAt == nil:
		b.endedAt = new(now)
		change.ended = true
	}

	return change
}

// leftAir は、予約した番組が放送中でなくなったことを検知したかどうか。b が nil (EIT[p/f] で追わない録画) なら false。
func (b *broadcastTracker) leftAir() bool {
	return b != nil && b.endedAt != nil
}

// end は、EPG から決めた録画の終わり planned を、放送の状態で決め直す。notBroadcast は、予約した番組が放送されなかったかどうか。
//   - 放送が終わったら (放送中が別の番組に切り替わったら)、後ろのマージン margin を録って止める
//   - 予定の終わりを過ぎても、予約した番組が放送中か、次の番組 (繰り下げ) なら、step ずつ延ばす。延ばすのは limit まで
//   - 次の番組のまま limit に来たら、放送されなかったとして、limit で止める (#640)
//   - 予定の終わりまでに、放送中にも次の番組にも出てこなければ、放送されなかったとして、予定の終わりで止める
func (b *broadcastTracker) end(planned, now time.Time, margin, step time.Duration, limit time.Time) (time.Time, bool) {
	if !b.observed {
		return planned, false
	}
	if b.endedAt != nil {
		return b.endedAt.Add(margin), false
	}
	onAir := b.startedAt != nil
	if (onAir || b.following) && !now.Before(planned) {
		return minTime(now.Add(step), limit), !onAir && !now.Before(limit)
	}

	return planned, !onAir && !now.Before(planned)
}
