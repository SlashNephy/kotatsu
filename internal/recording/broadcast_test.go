package recording_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/recording"
	"github.com/SlashNephy/kotatsu/internal/ts"
)

// TestBroadcastTracker は、EIT[p/f] の放送中と次の番組から、録画を止める時刻と、放送されなかったことを決めることを確かめる (#389)。
// 予約した番組の eventId は 100、予定の終わりは 12:30 (マージン込み)、後ろのマージンは 15 秒、延ばす単位は 5 分、上限は 15:30。
func TestBroadcastTracker(t *testing.T) {
	t.Parallel()

	at := func(hour, minute int) time.Time { return time.Date(2026, 10, 15, hour, minute, 0, 0, time.UTC) }
	pf := func(present, following *uint16) *ts.PresentFollowing {
		return &ts.PresentFollowing{Present: present, Following: following, PresentSeen: true, FollowingSeen: true}
	}
	ours, other, next := new(uint16(100)), new(uint16(200)), new(uint16(300))
	type step struct {
		now *time.Time
		pf  *ts.PresentFollowing
	}
	planned, limit := at(12, 30), at(15, 30)
	const margin, extension = 15 * time.Second, 5 * time.Minute
	tests := []struct {
		name             string
		steps            []step
		now              time.Time
		wantEnd          time.Time
		wantNotBroadcast bool
		wantStarted      bool
		wantEnded        bool
	}{
		{
			name: "EIT[p/f] を読めなければ、EPG の予定のまま",
			now:  at(12, 30), wantEnd: planned,
		},
		{
			name:  "放送中のあいだは、予定どおり",
			steps: []step{{now: new(at(12, 0)), pf: pf(ours, next)}},
			now:   at(12, 10), wantEnd: planned, wantStarted: true,
		},
		{
			name:  "早く終わったら、後ろのマージンを録って止める",
			steps: []step{{now: new(at(12, 0)), pf: pf(ours, next)}, {now: new(at(12, 20)), pf: pf(next, nil)}},
			now:   at(12, 20), wantEnd: at(12, 20).Add(margin), wantStarted: true, wantEnded: true,
		},
		{
			name:  "予定の終わりを過ぎても放送中なら、延ばす (延長)",
			steps: []step{{now: new(at(12, 0)), pf: pf(ours, next)}},
			now:   at(12, 31), wantEnd: at(12, 36), wantStarted: true,
		},
		{
			name:  "延ばすのは上限まで",
			steps: []step{{now: new(at(12, 0)), pf: pf(ours, next)}},
			now:   at(15, 28), wantEnd: limit, wantStarted: true,
		},
		{
			name:  "予定の開始を過ぎても次の番組なら、待つ (繰り下げ)",
			steps: []step{{now: new(at(12, 0)), pf: pf(other, ours)}},
			now:   at(12, 30), wantEnd: at(12, 35),
		},
		{
			name:  "予定の終わりまで放送中にも次の番組にも出てこなければ、放送されなかった",
			steps: []step{{now: new(at(12, 0)), pf: pf(other, next)}},
			now:   at(12, 30), wantEnd: planned, wantNotBroadcast: true,
		},
		{
			name:  "予定の終わりの前なら、まだ放送されなかったとはしない",
			steps: []step{{now: new(at(12, 0)), pf: pf(other, next)}},
			now:   at(12, 29), wantEnd: planned,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tracker := recording.NewBroadcastTracker(*ours)
			var started, ended bool
			for _, s := range tt.steps {
				st, en := tracker.Update(s.pf, *s.now)
				started, ended = started || st, ended || en
			}
			end, notBroadcast := tracker.End(planned, tt.now, margin, extension, limit)
			assert.Equal(t, tt.wantEnd, end)
			assert.Equal(t, tt.wantNotBroadcast, notBroadcast)
			assert.Equal(t, tt.wantStarted, started)
			assert.Equal(t, tt.wantEnded, ended)
		})
	}
}
