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
			// 割り込まれたのか早く終わったのかは見分けられないので、予定の終わりまでは録る (#636)。
			name:  "予定の終わりより早く放送中でなくなっても、予定の終わりまで録る",
			steps: []step{{now: new(at(12, 0)), pf: pf(ours, next)}, {now: new(at(12, 20)), pf: pf(next, nil)}},
			now:   at(12, 20), wantEnd: planned, wantStarted: true, wantEnded: true,
		},
		{
			name: "延長して予定の終わりの後に終わったら、後ろのマージンを録って止める",
			steps: []step{
				{now: new(at(12, 0)), pf: pf(ours, next)},
				{now: new(at(12, 40)), pf: pf(next, nil)},
			},
			now: at(12, 40), wantEnd: at(12, 40).Add(margin), wantStarted: true, wantEnded: true,
		},
		{
			name: "割り込まれても、次の番組が予約した番組なら、終わったとはしない",
			steps: []step{
				{now: new(at(12, 0)), pf: pf(ours, next)},
				{now: new(at(12, 10)), pf: pf(other, ours)},
			},
			now: at(12, 15), wantEnd: planned, wantStarted: true,
		},
		{
			name: "割り込みの間に、次の番組が予約した番組でなくなったら、そのときに終わったとし、予定の終わりまで録る",
			steps: []step{
				{now: new(at(12, 0)), pf: pf(ours, next)},
				{now: new(at(12, 10)), pf: pf(other, ours)},
				{now: new(at(12, 12)), pf: pf(other, next)},
			},
			now: at(12, 12), wantEnd: planned, wantStarted: true, wantEnded: true,
		},
		{
			name: "割り込みの後、後ろのマージンを録り終える前に予約した番組に戻ったら、録り続ける",
			steps: []step{
				{now: new(at(12, 0)), pf: pf(ours, next)},
				{now: new(at(12, 10)), pf: pf(other, next)},
				{now: new(at(12, 10).Add(10 * time.Second)), pf: pf(ours, next)},
			},
			now: at(12, 11), wantEnd: planned, wantStarted: true, wantEnded: true,
		},
		{
			// 番組の切り替わりなどで、放送中のセクションが一瞬だけ空で届くことがある (#637)。
			name: "延長中に放送中が一瞬だけ空になっても、終わったとはせず延ばし続ける",
			steps: []step{
				{now: new(at(12, 0)), pf: pf(ours, next)},
				{now: new(at(12, 40)), pf: pf(nil, next)},
				{now: new(at(12, 40).Add(5 * time.Second)), pf: pf(ours, next)},
			},
			now: at(12, 41), wantEnd: at(12, 46), wantStarted: true,
		},
		{
			name: "延長中に放送中が空のままなら、保留する間は延ばし続ける",
			steps: []step{
				{now: new(at(12, 0)), pf: pf(ours, next)},
				{now: new(at(12, 40)), pf: pf(nil, nil)},
				{now: new(at(12, 40).Add(50 * time.Second)), pf: pf(nil, nil)},
			},
			now: at(12, 40).Add(50 * time.Second), wantEnd: at(12, 45).Add(50 * time.Second), wantStarted: true,
		},
		{
			// 放送の休止などで空のまま続くときは、空になった時刻に終わったとみなし、後ろのマージンを録って止める。
			name: "延長中に放送中が 1 分を超えて空のままなら、空になった時刻に終わったとする",
			steps: []step{
				{now: new(at(12, 0)), pf: pf(ours, next)},
				{now: new(at(12, 40)), pf: pf(nil, nil)},
				{now: new(at(12, 41)), pf: pf(nil, nil)},
			},
			now: at(12, 41), wantEnd: at(12, 40).Add(margin), wantStarted: true, wantEnded: true,
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
			name:  "次の番組のまま待ち続けても、延ばすのは上限まで",
			steps: []step{{now: new(at(12, 0)), pf: pf(other, ours)}},
			now:   at(15, 28), wantEnd: limit,
		},
		{
			name:  "次の番組のまま上限に来たら、上限で止め、放送されなかったとする",
			steps: []step{{now: new(at(12, 0)), pf: pf(other, ours)}},
			now:   limit, wantEnd: limit, wantNotBroadcast: true,
		},
		{
			name:  "延長して放送中のまま上限に来ても、放送されなかったとはしない",
			steps: []step{{now: new(at(12, 0)), pf: pf(ours, next)}},
			now:   limit, wantEnd: limit, wantStarted: true,
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
