package edcb_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
)

// jstTime は、日本時間の時刻を返す。
func jstTime(year int, month time.Month, day, hour, minute, second int) time.Time {
	return time.Date(year, month, day, hour, minute, second, 0, time.FixedZone("JST", 9*60*60))
}

// TestRecordsFromRealEDCB は、実際の EDCB (e6e3c61、Linux) の応答を読む (#1503)。
// キーワード予約と予約は、CtrlCmd で足したもの。録画済みは、足した時刻予約を EDCB が録ったもの。
func TestRecordsFromRealEDCB(t *testing.T) {
	t.Parallel()

	s := newFakeServer(t, map[uint32]reply{
		2131: {status: 1, payload: fixture(t, "enum_auto_add2.bin"), stream: nil, hold: false, drop: false},
		2011: {status: 1, payload: fixture(t, "enum_reserve2.bin"), stream: nil, hold: false, drop: false},
		2020: {status: 1, payload: fixture(t, "enum_recinfo_basic2.bin"), stream: nil, hold: false, drop: false},
	})
	client := s.client(t)

	autoAdds, err := client.AutoAdds(t.Context())
	require.NoError(t, err)
	require.Len(t, autoAdds, 1)
	assert.Equal(t, &edcb.AutoAdd{
		ID: autoAdds[0].ID,
		Search: edcb.SearchKey{
			AndKey: "^!{999}ニュース", NotKey: ":note:メモ 天気", Regexp: false, TitleOnly: true,
			Contents: []edcb.ContentData{{Level1: 0, Level2: 0xFF, User1: 0xFF, User2: 0xFF}, {Level1: 7, Level2: 0, User1: 0xFF, User2: 0xFF}},
			Dates:    []edcb.SearchDate{{StartDayOfWeek: 1, StartHour: 19, StartMinute: 0, EndDayOfWeek: 1, EndHour: 22, EndMinute: 59}},
			Services: []uint64{32736<<32 | 32736<<16 | 1024},
			Fuzzy:    false, NotContents: false, NotDates: false, FreeCA: 1, CheckRecorded: true,
		},
		RecSetting: edcb.RecSetting{RecMode: 5, Priority: 2},
	}, autoAdds[0])

	reserves, err := client.Reserves(t.Context())
	require.NoError(t, err)
	require.Len(t, reserves, 1)
	assert.Equal(t, "EDCB 移行 先の予約", reserves[0].Title)
	assert.Equal(t, "手動の予約", reserves[0].Comment)
	assert.Equal(t, edcb.ServiceKey{ONID: 32736, TSID: 32736, SID: 1024}, reserves[0].ServiceKey)
	assert.Equal(t, uint16(0xFFFF), reserves[0].EventID)
	assert.Equal(t, 30*time.Minute, reserves[0].Duration)
	assert.Equal(t, edcb.RecSetting{RecMode: 1, Priority: 2}, reserves[0].RecSetting)

	recorded, err := client.RecordedFiles(t.Context())
	require.NoError(t, err)
	require.Len(t, recorded, 1)
	f := recorded[0]
	assert.Equal(t, "EDCB 移行 e2e", f.Title)
	assert.Equal(t, edcb.ServiceKey{ONID: 32736, TSID: 32736, SID: 1024}, f.ServiceKey)
	assert.Equal(t, uint32(1), f.Status)
	assert.Equal(t, jstTime(2026, 11, 14, 1, 35, 7), f.Start)
	assert.Equal(t, 60*time.Second, f.Duration)
}
