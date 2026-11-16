package edcb

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
)

const (
	// OccupancyLead は、EDCB の予約がチューナーを使い始めるのを、開始マージンを含めた予約の始まりより前に見込む時間 (#1536、初期値)。
	// EDCB は、予約の始まりの 20 秒前 (TunerBankCtrl.h の READY_MARGIN) に NetworkTV を閉じてチューナーを引き継ぎ、
	// チューナーが空いていれば、録画のアプリを起こす時間 (EpgTimerSrv.ini の RecAppWakeTime、既定 2 分) の前に開く。
	// 既定の 2 分に合わせる。
	OccupancyLead = 2 * time.Minute
	// OccupancyTrail は、EDCB の予約が、終了マージンを含めた予約の終わりの後もチューナーを使うと見込む時間 (#1536、初期値)。
	// 録画を閉じてチューナーを手放すまでの時間を見込む。
	OccupancyTrail = 30 * time.Second
	// EPGCaptureLead と EPGCaptureTrail は、EPG の取得の時刻の前後で、ライブ視聴とタイムシフトのために、
	// チューナーを 1 本空けておく時間 (#1536、初期値)。
	// EpgTimerSrv は、毎分 0 秒に取得の時刻かを確かめ、そのときに使っていないチューナーがなければ、その回の取得をしない
	// (ReserveManager.cpp の CheckEpgCap)。取得が始まった後は、ENUM_TUNER_PROCESS の取得中の印で分かるので、後ろは短くする。
	EPGCaptureLead  = 5 * time.Minute
	EPGCaptureTrail = 2 * time.Minute
)

// TunerOccupancies は、EDCB の予約がチューナーを使う時間帯を返す (#1536、設計書 4.5)。
//   - どの予約がどのチューナーを使うかは、ENUM_TUNER_RESERVE (EpgTimerSrv が予約に割り当てたチューナー) から読む。
//     チューナーが足りない予約を集めた行 (UnassignedTunerID) と、無効にした予約は除く。
//   - 時間帯は、予約の始まりと長さに、マージン (個別に指定していなければ EpgTimerSrv.ini の既定。読めなければ EDCB の既定の
//     開始 5 秒、終了 2 秒) を足し、さらに前に OccupancyLead、後ろに OccupancyTrail を足す。
//
// Kotatsu は EDCB の予約に優先度で勝てないので (EDCB に優先度はない)、割り当ては、この時間帯のチューナーを使わない。
func (s *TunerServer) TunerOccupancies(ctx context.Context) ([]*tunerserver.TunerOccupancy, error) {
	assigned, err := s.client.TunerReserves(ctx)
	if err != nil {
		return nil, err
	}
	reserves, err := s.client.Reserves(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.timerSettings(ctx)
	if err != nil {
		return nil, err
	}

	return Occupancies(assigned, reserves, settings), nil
}

var _ tunerserver.TunerOccupancyLister = new(TunerServer)

// Occupancies は、EpgTimerSrv が予約に割り当てたチューナーと予約から、予約がチューナーを使う時間帯を作る。
func Occupancies(assigned []*TunerReserve, reserves []*Reserve, settings *TimerSettings) []*tunerserver.TunerOccupancy {
	byID := make(map[uint32]*Reserve, len(reserves))
	for _, r := range reserves {
		byID[r.ID] = r
	}
	occupancies := []*tunerserver.TunerOccupancy{}
	for _, tuner := range assigned {
		if tuner.TunerID == UnassignedTunerID {
			continue
		}
		for _, id := range tuner.ReserveIDs {
			r := byID[id]
			if r == nil || r.RecSetting.Disabled() {
				continue
			}
			start, end := reserveSpan(r, settings)
			occupancies = append(occupancies, &tunerserver.TunerOccupancy{
				TunerIndex: int(tuner.TunerID), Start: start.Add(-OccupancyLead), End: end.Add(OccupancyTrail),
			})
		}
	}

	return occupancies
}

// reserveSpan は、予約のマージンを含めた始まりと終わりを返す (ReserveManager.cpp の CalcEntireReserveTime と同じ)。
func reserveSpan(r *Reserve, settings *TimerSettings) (time.Time, time.Time) {
	startMargin, endMargin := settings.StartMargin, settings.EndMargin
	if r.RecSetting.UseMargin {
		startMargin, endMargin = r.RecSetting.StartMargin, r.RecSetting.EndMargin
	}
	// 負のマージンで、始まりが終わりを、終わりが始まりを越えないようにする。
	startMargin = max(startMargin, -r.Duration)
	endMargin = max(endMargin, -r.Duration-min(startMargin, 0))

	return r.Start.Add(-startMargin), r.Start.Add(r.Duration + endMargin)
}

// errEPGCapture は、EPG の取得の前後でチューナーを空けておくので、ライブ視聴やタイムシフトに新しいチューナーを使わせないことを表す。
var errEPGCapture = fmt.Errorf("%w: keeping a tuner free for the EPG capture of EDCB", tunerserver.ErrTunerUnavailable)

// EPGCaptureState は、EPG の取得のためにチューナーを空けておくかを決めるのに使う、EDCB の今の状態。
type EPGCaptureState struct {
	// Now は今の時刻。
	Now time.Time
	// CaptureTimes は、EPG の取得の時刻。EpgTimerSrv.ini を読めなければ空。
	CaptureTimes []EPGCaptureTime
	// Tuners は、チューナーの数 (どの BonDriver も含む。EPG の取得は、[TVTEST] に並べていない BonDriver も使う)。
	Tuners int
	// Busy は、プロセスを起動しているチューナーの数。
	Busy int
	// CancelsCapture は、新しく開く NetworkTV が、EPG を取得している最中のチューナーを選び、取得を取り消すかどうか。
	CancelsCapture bool
}

// KeepsTunerForEPG は、ライブ視聴やタイムシフトのために、新しく NetworkTV を開いてはいけないかを返す (#1536)。
//   - 新しく開く NetworkTV が、EPG を取得している最中のチューナーを選ぶなら、開かない。NetworkTV は、取得中のチューナーを選ぶと
//     取得を取り消す (TunerBankCtrl.cpp の OpenNWTV)。どのチューナーを選ぶかは EpgTimerSrv が決める (cancelsCapture)。
//   - EPG の取得の時刻の EPGCaptureLead 前から EPGCaptureTrail 後までは、開いた後に使っていないチューナーが 1 本も残らないなら、開かない。
//
// 録画は空けない。EPG が古くなるより、録画を録り逃すほうが困るからである。
func KeepsTunerForEPG(state *EPGCaptureState) bool {
	if state.CancelsCapture {
		return true
	}
	if !nearEPGCapture(state.Now, state.CaptureTimes) {
		return false
	}

	return state.Tuners-state.Busy < 2
}

// nearEPGCapture は、now が、どれかの EPG の取得の時刻の EPGCaptureLead 前から EPGCaptureTrail 後までにあるかを返す。
func nearEPGCapture(now time.Time, times []EPGCaptureTime) bool {
	for _, t := range times {
		// 取得の時刻を過ぎた直後も含めるため、EPGCaptureTrail だけ前から次の取得の時刻を探す。
		next := t.next(now.Add(-EPGCaptureTrail))
		if !now.Before(next.Add(-EPGCaptureLead)) && now.Before(next.Add(EPGCaptureTrail)) {
			return true
		}
	}

	return false
}

// checkEPGCapture は、ライブ視聴やタイムシフトに新しく NetworkTV を開く前に、EPG の取得のためにチューナーを空けておくかを確かめる。
// 空けておくなら、チューナーが足りないことを表すエラーを返す。EDCB の状態を読めなければ、空けずに開く。
// EPG を守ることより、視聴を止めないことを先にする。EDCB に届かなければ、どのみち NetworkTV も開けない。
func (s *TunerServer) checkEPGCapture(ctx context.Context) error {
	state, err := s.epgCaptureState(ctx)
	if err != nil {
		return nil //nolint:nilerr // 読めなければ、空けずに開く (上のとおり)。
	}
	if KeepsTunerForEPG(state) {
		return errEPGCapture
	}

	return nil
}

// epgCaptureState は、EPG の取得のためにチューナーを空けておくかを決めるのに使う、EDCB の今の状態を読む。
func (s *TunerServer) epgCaptureState(ctx context.Context) (*EPGCaptureState, error) {
	settings, err := s.timerSettings(ctx)
	if err != nil {
		return nil, err
	}
	assigned, err := s.client.TunerReserves(ctx)
	if err != nil {
		return nil, err
	}
	processes, err := s.client.TunerProcesses(ctx)
	if err != nil {
		return nil, err
	}
	names := map[uint32]string{}
	for _, tuner := range assigned {
		if tuner.TunerID != UnassignedTunerID {
			names[tuner.TunerID] = tuner.TunerName
		}
	}
	running := map[uint32]*TunerProcess{}
	for _, process := range processes {
		if _, ok := names[process.TunerID]; ok {
			running[process.TunerID] = process
		}
	}
	usable := []uint32{}
	for id, name := range names {
		if s.usable(name) {
			usable = append(usable, id)
		}
	}

	return &EPGCaptureState{
		Now: s.now(), CaptureTimes: settings.EPGCaptureTimes, Tuners: len(names), Busy: len(running),
		CancelsCapture: cancelsCapture(usable, running),
	}, nil
}

// cancelsCapture は、新しく開く NetworkTV が、EPG を取得している最中のチューナーを選ぶかを返す。
// EpgTimerSrv は、[TVTEST] に並べた BonDriver のチューナーを、ID の大きい順に試し (EpgTimerSrvMain.cpp の NWTV_ID_SET_CH)、
// 空いているか EPG を取得しているチューナーで開く。録画や NetworkTV に使っているチューナーは飛ばす (ReserveManager.cpp の OpenNWTV)。
// EPG の取得は、空いているチューナーを ID の小さい順に使うので (GetEpgCapTunerList)、ふつうは ID の大きい空きチューナーが先に選ばれる。
func cancelsCapture(usable []uint32, running map[uint32]*TunerProcess) bool {
	slices.Sort(usable)
	for _, id := range slices.Backward(usable) {
		process, ok := running[id]
		if !ok {
			return false
		}
		if process.CapturingEPG {
			return true
		}
	}

	return false
}

// viewing は、ストリームが録画のものでない (ライブ視聴、kyusu、タイムシフト) かを返す。
// 録画は、優先度 1 以上で開く (設計書 1.6)。
func viewing(opts *tunerserver.StreamOptions) bool {
	return opts == nil || opts.Priority <= 0
}
