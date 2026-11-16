package edcb_test

import (
	"encoding/binary"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
)

// jstNovember は、2026 年 11 月の日本時間の時刻を返す。2026-11-16 は月曜。
func jstNovember(day, hour, minute, second int) time.Time {
	return time.Date(2026, time.November, day, hour, minute, second, 0, time.FixedZone("JST", 9*60*60))
}

// emptyFileCopy2 は、EpgTimerSrv が EpgTimerSrv.ini を返さないとき (互換動作を有効にしていない) の FILE_COPY2 の応答。
// ファイルの名前だけがあり、中身は空になる。
func emptyFileCopy2() []byte {
	name := utf16.Encode([]rune("EpgTimerSrv.ini"))
	str := binary.LittleEndian.AppendUint32(nil, dword(4+2*len(name)+2))
	for _, u := range name {
		str = binary.LittleEndian.AppendUint16(str, u)
	}
	str = binary.LittleEndian.AppendUint16(str, 0)
	// FILE_DATA: 名前、大きさ (0)、予約の DWORD (0)、中身 (なし)。
	file := append(append([]byte{}, str...), make([]byte, 8)...)
	file = append(binary.LittleEndian.AppendUint32(nil, dword(4+len(file))), file...)
	items := append(binary.LittleEndian.AppendUint32(nil, 1), file...)
	vector := append(binary.LittleEndian.AppendUint32(nil, dword(4+len(items))), items...)

	return append(binary.LittleEndian.AppendUint16(nil, 5), vector...)
}

// TestTunerServerTunerOccupancies は、実際の EDCB の予約 (個別のマージン 30 秒と 20 秒の予約と、マージンを指定しない予約) から、
// EDCB の予約がチューナーを使う時間帯を作ることを確かめる (#1536)。
func TestTunerServerTunerOccupancies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ini  []byte
		// defaultStart と defaultEnd は、マージンを指定しない予約に使うマージン。
		defaultStart, defaultEnd time.Duration
	}{
		// EpgTimerSrv.ini を読めれば、[SET] の StartMargin (10 秒) と EndMargin (5 秒) を使う。
		{name: "with EpgTimerSrv.ini", ini: nil, defaultStart: 10 * time.Second, defaultEnd: 5 * time.Second},
		// 読めなければ、EDCB の既定 (5 秒と 2 秒) を使う。
		{name: "without EpgTimerSrv.ini", ini: emptyFileCopy2(), defaultStart: 5 * time.Second, defaultEnd: 2 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ini := tt.ini
			if ini == nil {
				ini = fixture(t, "file_copy2_epgtimersrv_ini.bin")
			}
			s := newFakeServer(t, map[uint32]reply{
				1016: {status: 1, payload: fixture(t, "enum_tuner_reserve_reserved.bin")},
				2011: {status: 1, payload: fixture(t, "enum_reserve2_margin.bin")},
				2060: {status: 1, payload: ini},
			})
			got, err := edcb.NewTunerServer(s.client(t), nil).TunerOccupancies(t.Context())
			require.NoError(t, err)
			assert.Equal(t, []*tunerserver.TunerOccupancy{
				// 13:30 から 5 分、マージンは 30 秒と 20 秒。
				{
					TunerIndex: 1,
					Start:      jstNovember(16, 13, 30, 0).Add(-30*time.Second - edcb.OccupancyLead),
					End:        jstNovember(16, 13, 35, 0).Add(20*time.Second + edcb.OccupancyTrail),
				},
				// 13:40 から 2 分、マージンは既定。時間が重ならないので、EpgTimerSrv は同じチューナーに割り当てた。
				{
					TunerIndex: 1,
					Start:      jstNovember(16, 13, 40, 0).Add(-tt.defaultStart - edcb.OccupancyLead),
					End:        jstNovember(16, 13, 42, 0).Add(tt.defaultEnd + edcb.OccupancyTrail),
				},
			}, got)
		})
	}
}

// TestOccupancies は、チューナーが足りない予約と無効にした予約を除き、負のマージンでも始まりと終わりを越えないことを確かめる。
func TestOccupancies(t *testing.T) {
	t.Parallel()

	start := jstNovember(16, 20, 0, 0)
	settings := &edcb.TimerSettings{
		Loaded: true, EPGCaptureTimes: []edcb.EPGCaptureTime{}, StartMargin: 0, EndMargin: 0,
		EPGCaptureAnyBonDriver: false, EPGCaptureBonDrivers: map[string]tunerserver.EPGCaptureBonDriver{},
	}
	reserve := func(id uint32, mode uint8, useMargin bool, startMargin, endMargin time.Duration) *edcb.Reserve {
		r := new(edcb.Reserve)
		r.ONID, r.TSID, r.SID, r.EventID = 1, 1, 1, 0xFFFF
		r.Start, r.Duration, r.ID = start, time.Hour, id
		r.RecSetting = edcb.RecSetting{RecMode: mode, Priority: 2, UseMargin: useMargin, StartMargin: startMargin, EndMargin: endMargin}

		return r
	}
	occupancy := func(tuner int, from, to time.Time) *tunerserver.TunerOccupancy {
		return &tunerserver.TunerOccupancy{TunerIndex: tuner, Start: from.Add(-edcb.OccupancyLead), End: to.Add(edcb.OccupancyTrail)}
	}
	tests := []struct {
		name     string
		assigned []*edcb.TunerReserve
		reserves []*edcb.Reserve
		want     []*tunerserver.TunerOccupancy
	}{
		{
			name:     "view-mode reservations hold a tuner too",
			assigned: []*edcb.TunerReserve{{TunerID: 0x10001, TunerName: "a", ReserveIDs: []uint32{1}}},
			reserves: []*edcb.Reserve{reserve(1, 4, false, 0, 0)},
			want:     []*tunerserver.TunerOccupancy{occupancy(0x10001, start, start.Add(time.Hour))},
		},
		{
			name: "reservations without a tuner and disabled ones are skipped",
			assigned: []*edcb.TunerReserve{
				{TunerID: 0x10001, TunerName: "a", ReserveIDs: []uint32{1}},
				{TunerID: edcb.UnassignedTunerID, TunerName: "チューナー不足", ReserveIDs: []uint32{2}},
			},
			reserves: []*edcb.Reserve{reserve(1, 6, false, 0, 0), reserve(2, 1, false, 0, 0)},
			want:     []*tunerserver.TunerOccupancy{},
		},
		{
			// 負の開始マージンは本編を削る。終わりを越えて削らない。
			name:     "negative margins do not cross the other end",
			assigned: []*edcb.TunerReserve{{TunerID: 0x10001, TunerName: "a", ReserveIDs: []uint32{1}}},
			reserves: []*edcb.Reserve{reserve(1, 1, true, -2*time.Hour, -2*time.Hour)},
			want:     []*tunerserver.TunerOccupancy{occupancy(0x10001, start.Add(time.Hour), start.Add(time.Hour))},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, edcb.Occupancies(tt.assigned, tt.reserves, settings))
		})
	}
}

func TestParseTimerSettings(t *testing.T) {
	t.Parallel()

	daily := func(hour, minute int) edcb.EPGCaptureTime {
		return edcb.EPGCaptureTime{Weekday: time.Sunday, Daily: true, Minute: hour*60 + minute}
	}
	weekly := func(weekday time.Weekday, hour, minute int) edcb.EPGCaptureTime {
		return edcb.EPGCaptureTime{Weekday: weekday, Daily: false, Minute: hour*60 + minute}
	}
	utf16LE := func(text string) []byte {
		b := []byte{0xFF, 0xFE}
		for _, u := range utf16.Encode([]rune(text)) {
			b = binary.LittleEndian.AppendUint16(b, u)
		}

		return b
	}
	noBonDrivers := map[string]tunerserver.EPGCaptureBonDriver{}
	tests := []struct {
		name string
		data []byte
		want *edcb.TimerSettings
	}{
		{
			// [EPG_CAP] の Count がなければ、EpgTimerSrv は毎日 23:00 に取得する。マージンも EDCB の既定にする。
			// NGEpgCapTime の既定は 20 分なので、BonDriver ごとに空きチューナーが要る。
			name: "defaults",
			data: []byte("[SET]\nEnableTCPSrv=1\n"),
			want: &edcb.TimerSettings{
				Loaded: true, EPGCaptureTimes: []edcb.EPGCaptureTime{daily(23, 0)}, StartMargin: 5 * time.Second, EndMargin: 2 * time.Second,
				EPGCaptureAnyBonDriver: false, EPGCaptureBonDrivers: noBonDrivers,
			},
		},
		{
			// BonDriver の節の GetEpg と EPGCount を読む。名前は小文字にする。
			name: "bon drivers",
			data: []byte("[SET]\nNGEpgCapTime=0\n[BonDriver_PT-T.dll]\nCount=4\nGetEpg=1\nEPGCount=2\n[BonDriver_PT-S.dll]\nCount=4\nGetEpg=0\n" +
				"[BonDriver_Mirakurun.dll]\nCount=2\n[TVTEST]\nNum=1\n0=BonDriver_PT-T.dll\n"),
			want: &edcb.TimerSettings{
				Loaded: true, EPGCaptureTimes: []edcb.EPGCaptureTime{daily(23, 0)}, StartMargin: 5 * time.Second, EndMargin: 2 * time.Second,
				EPGCaptureAnyBonDriver: true, EPGCaptureBonDrivers: map[string]tunerserver.EPGCaptureBonDriver{
					"bondriver_pt-t.dll":      {Disabled: false, Tuners: 2},
					"bondriver_pt-s.dll":      {Disabled: true, Tuners: 0},
					"bondriver_mirakurun.dll": {Disabled: false, Tuners: 0},
				},
			},
		},
		{
			// 無効にした時刻は除く。曜日は w1 が月曜、w7 が日曜。
			name: "selected times with weekdays",
			data: []byte("[SET]\r\nStartMargin=10\r\nEndMargin=-3\r\n[EPG_CAP]\r\nCount=4\r\n0=4:30\r\n0Select=1\r\n1=12:00\r\n1Select=0\r\n" +
				"2=6:15w1\r\n2Select=1\r\n3=23:59w7\r\n3Select=1\r\n"),
			want: &edcb.TimerSettings{
				Loaded:          true,
				EPGCaptureTimes: []edcb.EPGCaptureTime{daily(4, 30), weekly(time.Monday, 6, 15), weekly(time.Sunday, 23, 59)},
				StartMargin:     10 * time.Second, EndMargin: -3 * time.Second,
				EPGCaptureAnyBonDriver: false, EPGCaptureBonDrivers: noBonDrivers,
			},
		},
		{
			// Windows の EDCB は、UTF-16 (BOM 付き) で書く。Count が 0 なら取得しない。
			name: "utf-16 without capture times",
			data: utf16LE("[SET]\r\nStartMargin=7\r\n[EPG_CAP]\r\nCount=0\r\n"),
			want: &edcb.TimerSettings{
				Loaded: true, EPGCaptureTimes: []edcb.EPGCaptureTime{}, StartMargin: 7 * time.Second, EndMargin: 2 * time.Second,
				EPGCaptureAnyBonDriver: false, EPGCaptureBonDrivers: noBonDrivers,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, edcb.ParseTimerSettings(tt.data))
		})
	}
}

// TestKeepsTunerForEPG は、EPG の取得の前後と取得中に、ライブ視聴とタイムシフトに新しいチューナーを使わせないことを確かめる
// (#1536、#1873)。EpgTimerSrv は、新しい NetworkTV を、[TVTEST] に並べた BonDriver のチューナーのうち、ID の大きいものから試す。
func TestKeepsTunerForEPG(t *testing.T) {
	t.Parallel()

	// 2026-11-16 は月曜。毎日 23:00 と、月曜の 4:30 に取得する。
	times := []edcb.EPGCaptureTime{
		{Weekday: time.Sunday, Daily: true, Minute: 23 * 60},
		{Weekday: time.Monday, Daily: false, Minute: 4*60 + 30},
	}
	const (
		terrestrial = "BonDriver_T.dll"
		satellite   = "BonDriver_S.dll"
		// terrestrialKey は、EpgTimerSrv.ini の節の名前 (小文字にしたもの)。
		terrestrialKey = "bondriver_t.dll"
	)
	settings := func(anyBonDriver bool, bonDrivers map[string]tunerserver.EPGCaptureBonDriver) *edcb.TimerSettings {
		return &edcb.TimerSettings{
			Loaded: true, EPGCaptureTimes: times, StartMargin: 0, EndMargin: 0, EPGCaptureAnyBonDriver: anyBonDriver, EPGCaptureBonDrivers: bonDrivers,
		}
	}
	// epgCount1 は、どの BonDriver も 1 本で取得する設定。
	epgCount1 := settings(false, map[string]tunerserver.EPGCaptureBonDriver{
		terrestrialKey: {Disabled: false, Tuners: 1}, "bondriver_s.dll": {Disabled: false, Tuners: 1},
	})
	// tuner は、ID が id のチューナー。running は使っているかどうか。
	tuner := func(id uint32, name string, usable, running bool) *edcb.EPGCaptureTuner {
		return &edcb.EPGCaptureTuner{ID: id, Name: name, Usable: usable, Running: running, CapturingEPG: false}
	}
	// oneBusy は、地上波の BonDriver の 2 本のうち、1 本を使っているチューナー。
	oneBusy := []*edcb.EPGCaptureTuner{tuner(1, terrestrial, true, true), tuner(2, terrestrial, true, false)}
	tests := []struct {
		name     string
		now      time.Time
		settings *edcb.TimerSettings
		tuners   []*edcb.EPGCaptureTuner
		want     bool
	}{
		{name: "far from capture times", now: jstNovember(16, 13, 0, 0), settings: epgCount1, tuners: oneBusy, want: false},
		{
			// 取得の 5 分前から、取得に要る最後の空きチューナーを使わせない。
			name: "last free tuner before a daily capture", now: jstNovember(16, 22, 55, 0), settings: epgCount1, tuners: oneBusy, want: true,
		},
		{name: "just before the lead", now: jstNovember(16, 22, 54, 59), settings: epgCount1, tuners: oneBusy, want: false},
		{
			// 空きチューナーが 2 本あれば、1 本を使っても取得に 1 本残る。
			name: "two free tuners before a capture", now: jstNovember(16, 22, 58, 0), settings: epgCount1,
			tuners: append(oneBusy, tuner(3, terrestrial, true, false)), want: false,
		},
		{
			// 取得の時刻の 2 分後までは、取得が始まる前の判定に間に合わせる。
			name: "within the trail after the capture time", now: jstNovember(16, 23, 1, 59), settings: epgCount1, tuners: oneBusy, want: true,
		},
		{name: "after the trail", now: jstNovember(16, 23, 2, 0), settings: epgCount1, tuners: oneBusy, want: false},
		{
			// 曜日を指定した取得は、その曜日だけ。2026-11-17 は火曜。
			name: "weekly capture on its weekday", now: jstNovember(16, 4, 27, 0), settings: epgCount1,
			tuners: []*edcb.EPGCaptureTuner{tuner(1, terrestrial, true, false)}, want: true,
		},
		{
			name: "weekly capture on another weekday", now: jstNovember(17, 4, 27, 0), settings: epgCount1,
			tuners: []*edcb.EPGCaptureTuner{tuner(1, terrestrial, true, false)}, want: false,
		},
		{
			// 新しい NetworkTV が取得中のチューナーを選ぶなら、取得を取り消すので、時刻に依らず開かない。
			name: "a new NetworkTV would cancel the capture", now: jstNovember(16, 13, 0, 0), settings: epgCount1,
			tuners: []*edcb.EPGCaptureTuner{
				tuner(1, terrestrial, true, true), {ID: 2, Name: terrestrial, Usable: true, Running: true, CapturingEPG: true},
			},
			want: true,
		},
		{
			// 取得中のチューナーより ID の大きい空きチューナーが選ばれるなら、取得は続く。
			name: "a new NetworkTV picks a free tuner while capturing", now: jstNovember(16, 13, 0, 0), settings: epgCount1,
			tuners: []*edcb.EPGCaptureTuner{
				{ID: 1, Name: terrestrial, Usable: true, Running: true, CapturingEPG: true}, tuner(2, terrestrial, true, false),
			},
			want: false,
		},
		{
			// EpgTimerSrv.ini を読めず、取得の時刻が分からなければ、取得中だけを避ける。
			name: "unknown capture times", now: jstNovember(16, 22, 58, 0),
			settings: &edcb.TimerSettings{
				Loaded: false, EPGCaptureTimes: []edcb.EPGCaptureTime{}, StartMargin: 0, EndMargin: 0,
				EPGCaptureAnyBonDriver: false, EPGCaptureBonDrivers: map[string]tunerserver.EPGCaptureBonDriver{},
			},
			tuners: oneBusy, want: false,
		},
		{
			// ほかの BonDriver のチューナーが空いていても、地上波の BonDriver の取得に要る 1 本は代わりにならない。
			name: "a free tuner of another bon driver does not help", now: jstNovember(16, 22, 58, 0), settings: epgCount1,
			tuners: append(oneBusy, tuner(0, satellite, false, false)), want: true,
		},
		{
			// EPGCount が 2 なら、2 本の空きが要る。3 本空いていれば、1 本を使っても 2 本残る。
			name: "two tuners for the capture", now: jstNovember(16, 22, 58, 0),
			settings: settings(false, map[string]tunerserver.EPGCaptureBonDriver{terrestrialKey: {Disabled: false, Tuners: 2}}),
			tuners: []*edcb.EPGCaptureTuner{
				tuner(1, terrestrial, true, false), tuner(2, terrestrial, true, false), tuner(3, terrestrial, true, false),
			},
			want: false,
		},
		{
			name: "two tuners for the capture with only two free", now: jstNovember(16, 22, 58, 0),
			settings: settings(false, map[string]tunerserver.EPGCaptureBonDriver{terrestrialKey: {Disabled: false, Tuners: 2}}),
			tuners:   append(oneBusy, tuner(3, terrestrial, true, false)), want: true,
		},
		{
			// EPGCount が 0 (既定) なら、BonDriver のすべてのチューナーで取得する。1 本でも使っていれば取得されないので、空けても意味がない。
			name: "all tuners for the capture while one is busy", now: jstNovember(16, 22, 58, 0), settings: settings(false, map[string]tunerserver.EPGCaptureBonDriver{}),
			tuners: oneBusy, want: false,
		},
		{
			name: "all tuners for the capture while all are free", now: jstNovember(16, 22, 58, 0), settings: settings(false, map[string]tunerserver.EPGCaptureBonDriver{}),
			tuners: []*edcb.EPGCaptureTuner{tuner(1, terrestrial, true, false), tuner(2, terrestrial, true, false)}, want: true,
		},
		{
			// GetEpg が 0 の BonDriver は、取得に使わない。
			name: "bon driver without the capture", now: jstNovember(16, 22, 58, 0),
			settings: settings(false, map[string]tunerserver.EPGCaptureBonDriver{terrestrialKey: {Disabled: true, Tuners: 0}}),
			tuners:   oneBusy, want: false,
		},
		{
			// NGEpgCapTime が 0 なら、取得に使う BonDriver のどれかに 1 本空いていればよい。
			name: "any bon driver with a free tuner", now: jstNovember(16, 22, 58, 0), settings: settings(true, map[string]tunerserver.EPGCaptureBonDriver{}),
			tuners: append(oneBusy, tuner(0, satellite, false, false)), want: false,
		},
		{
			name: "any bon driver without another free tuner", now: jstNovember(16, 22, 58, 0), settings: settings(true, map[string]tunerserver.EPGCaptureBonDriver{}),
			tuners: oneBusy, want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			state := &edcb.EPGCaptureState{Capture: edcb.CaptureOf(tt.settings, tt.now), Tuners: tt.tuners}
			assert.Equal(t, tt.want, edcb.KeepsTunerForEPG(state))
		})
	}
}

// TestTunerServerKeepsTunerForEPG は、ライブ視聴とタイムシフトが、EPG の取得の前に最後の空きチューナーで NetworkTV を開かず、
// 録画と、開いている NetworkTV に加わる受け手は開けることを、偽の EpgTimerSrv で確かめる (#1536)。
func TestTunerServerKeepsTunerForEPG(t *testing.T) {
	t.Parallel()

	recording := &tunerserver.StreamOptions{Decode: false, Priority: 2}
	live := &tunerserver.StreamOptions{Decode: false, Priority: 0}
	timeshift := &tunerserver.StreamOptions{Decode: false, Priority: -1}
	tests := []struct {
		name string
		now  time.Time
		opts *tunerserver.StreamOptions
		want error
	}{
		// 実際の EDCB の EpgTimerSrv.ini は、毎日 23:00 に取得する。チューナーは 2 本で、1 本は使っている。
		{name: "live before the capture", now: jstNovember(16, 22, 57, 0), opts: live, want: tunerserver.ErrTunerUnavailable},
		{name: "timeshift before the capture", now: jstNovember(16, 22, 57, 0), opts: timeshift, want: tunerserver.ErrTunerUnavailable},
		{name: "recording before the capture", now: jstNovember(16, 22, 57, 0), opts: recording, want: nil},
		{name: "live far from the capture", now: jstNovember(16, 13, 0, 0), opts: live, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := streamServer(t, make([]byte, 188), 0)
			s.replies[2060] = reply{status: 1, payload: fixture(t, "file_copy2_epgtimersrv_ini.bin")}
			s.replies[1016] = reply{status: 1, payload: fixture(t, "enum_tuner_reserve.bin")}
			s.replies[1066] = reply{status: 1, payload: fixture(t, "enum_tuner_process_open.bin")}
			ids := &fakeIDs{}
			server := edcb.NewTunerServer(s.client(t), &edcb.TunerServerOptions{NetworkTVIDs: ids, Clock: clocktest.NewFake(tt.now)})
			stream, err := server.OpenServiceStream(t.Context(), nhkSougou, tt.opts)
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
				assert.NotContains(t, commands(s.received()), uint32(1073), "does not open a NetworkTV")

				return
			}
			require.NoError(t, err)
			require.NoError(t, stream.Close())
		})
	}
}

// capturingProcesses は、tunerIDs のチューナーが EPG を取得している最中の、ENUM_TUNER_PROCESS の応答を作る。
func capturingProcesses(tunerIDs ...uint32) []byte {
	items := binary.LittleEndian.AppendUint32(nil, dword(len(tunerIDs)))
	for _, tunerID := range tunerIDs {
		item := binary.LittleEndian.AppendUint32(nil, tunerID)
		item = binary.LittleEndian.AppendUint32(item, 100)  // プロセス ID
		item = append(item, make([]byte, 8+8+4+4+4+4+4)...) // ドロップ、スクランブル、信号、空間、チャンネル、ONID、TSID
		item = append(item, 0, 1, 0, 0)                     // 録画中でなく、EPG の取得中
		items = binary.LittleEndian.AppendUint32(items, dword(4+len(item)))
		items = append(items, item...)
	}

	return append(binary.LittleEndian.AppendUint32(nil, dword(4+len(items))), items...)
}

// TestTunerServerKeepsTunerWhileCapturing は、EPG を取得している最中に、新しい NetworkTV が取得中のチューナーを選ぶなら、
// ライブ視聴に開かず、空いているチューナーを選ぶなら開くことを確かめる (#1536)。EpgTimerSrv は、ID の大きいチューナーから試す。
// すでに開いている同じサービスの NetworkTV には、取得中でも加われる。
func TestTunerServerKeepsTunerWhileCapturing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// processes は、ENUM_TUNER_PROCESS の応答。テストのチューナーは 1 と 2。
		processes []byte
		want      error
	}{
		// チューナー 2 で取得している。新しい NetworkTV は先にチューナー 2 を試すので、取得を取り消してしまう。
		{name: "capturing on the tuner tried first", processes: capturingProcesses(2), want: tunerserver.ErrTunerUnavailable},
		// チューナー 1 で取得している。新しい NetworkTV は、空いているチューナー 2 で開くので、取得は続く。
		{name: "capturing on the tuner tried last", processes: capturingProcesses(1), want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, server, _, feed := feedServer(t, 0, 0)
			s.mu.Lock()
			s.replies[2060] = reply{status: 1, payload: emptyFileCopy2()}
			s.replies[1016] = reply{status: 1, payload: fixture(t, "enum_tuner_reserve.bin")}
			s.replies[1066] = reply{status: 1, payload: tt.processes}
			s.mu.Unlock()

			live, err := server.OpenServiceStream(t.Context(), nhkSougou, &tunerserver.StreamOptions{Decode: false, Priority: 0})
			if tt.want == nil {
				require.NoError(t, err)
				require.NoError(t, live.Close())

				return
			}
			require.ErrorIs(t, err, tt.want)
			assert.Equal(t, 0, countCommand(s, 1073))

			// 録画は開ける。
			first, err := server.OpenServiceStream(t.Context(), nhkSougou, &tunerserver.StreamOptions{Decode: false, Priority: 2})
			require.NoError(t, err)
			defer first.Close()
			feed <- make([]byte, 188)

			// ライブ視聴は、開いている NetworkTV に加わるだけなら、チューナーを増やさないので受け取れる。
			second, err := server.OpenServiceStream(t.Context(), nhkSougou, &tunerserver.StreamOptions{Decode: false, Priority: 0})
			require.NoError(t, err)
			require.NoError(t, second.Close())
			assert.Equal(t, 1, countCommand(s, 1073))
		})
	}
}

// TestTunerServerEPGCapture は、EpgTimerSrv.ini から、EPG の取得のために空けておくチューナーを決める設定を作ることを確かめる (#1856)。
// 読み直せないときは、前に読んだ設定を使い続け、一度も読めていなければ、エラーにする。
func TestTunerServerEPGCapture(t *testing.T) {
	t.Parallel()

	// FILE_COPY2 の応答の種類。
	const (
		ini   = "ini"
		empty = "empty"
		drop  = "drop"
	)
	tests := []struct {
		name string
		// first と later は、最初と、読み直すとき (EpgTimerSrv.ini を読み直す間隔の後) の FILE_COPY2 の応答の種類。
		first, later string
		wantErr      bool
		wantKnown    bool
		wantNear     bool
	}{
		{name: "readable", first: ini, later: ini, wantErr: false, wantKnown: true, wantNear: true},
		{name: "keeps the last settings while unreachable", first: ini, later: drop, wantErr: false, wantKnown: true, wantNear: true},
		{name: "not returned without the compatible mode", first: empty, later: empty, wantErr: false, wantKnown: false, wantNear: false},
		{name: "never readable", first: drop, later: drop, wantErr: true, wantKnown: false, wantNear: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			replyOf := func(kind string) reply {
				switch kind {
				case ini:
					return reply{status: 1, payload: fixture(t, "file_copy2_epgtimersrv_ini.bin")}
				case empty:
					return reply{status: 1, payload: emptyFileCopy2()}
				default:
					return reply{status: 0, drop: true}
				}
			}
			s := newFakeServer(t, map[uint32]reply{2060: replyOf(tt.later)})
			s.queued[2060] = []reply{replyOf(tt.first)}
			// 実際の EDCB の EpgTimerSrv.ini は、毎日 23:00 に取得する。
			clk := clocktest.NewFake(jstNovember(16, 22, 40, 0))
			server := edcb.NewTunerServer(s.client(t), &edcb.TunerServerOptions{Clock: clk})
			_, _ = server.EPGCapture(t.Context(), clk.Now())

			clk.Set(jstNovember(16, 22, 58, 0))
			capture, err := server.EPGCapture(t.Context(), clk.Now())
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantKnown, capture.Known)
			assert.Equal(t, tt.wantNear, capture.Near)
			assert.Len(t, s.received(), 2, "reads EpgTimerSrv.ini again after the interval")
		})
	}
}
