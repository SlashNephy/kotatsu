package edcb

import (
	"context"
	"time"
)

// コマンド (EDCB の Common/CtrlCmdDef.h)。EDCB からの移行で、キーワード予約、予約、録画済みの一覧を読む (#1503)。
const (
	cmdEnumReserve2       = 2011
	cmdEnumRecInfoBasic2  = 2020
	cmdEnumAutoAdd2       = 2131
	unsetSystemTimeBefore = 1900
)

// RecSetting は、録画設定 (REC_SETTING_DATA) のうち、移行で使う項目。
type RecSetting struct {
	// RecMode は録画モード。0〜3 は録る、4 は視聴、5〜9 は無効にした 0〜4 (EDCB の REC_SETTING_DATA::DIV_RECMODE)。
	RecMode uint8
	// Priority は優先度。
	Priority uint8
}

// Records は、録画するかどうか。無効でなく、視聴でもなければ録る。
func (r RecSetting) Records() bool {
	return r.RecMode < 4
}

// ContentData は、ジャンル (EPGDB_CONTENT_DATA)。
type ContentData struct {
	Level1, Level2, User1, User2 uint8
}

// SearchDate は、時間帯 (EPGDB_SEARCH_DATE_INFO)。曜日は 0 が日曜。
type SearchDate struct {
	StartDayOfWeek uint8
	StartHour      uint16
	StartMinute    uint16
	EndDayOfWeek   uint8
	EndHour        uint16
	EndMinute      uint16
}

// SearchKey は、検索条件 (EPGDB_SEARCH_KEY_INFO)。
type SearchKey struct {
	AndKey    string
	NotKey    string
	Regexp    bool
	TitleOnly bool
	Contents  []ContentData
	Dates     []SearchDate
	// Services は、ONID、TSID、SID を 16 ビットずつ詰めた値 (上から ONID、TSID、SID)。
	Services    []uint64
	Fuzzy       bool
	NotContents bool
	NotDates    bool
	// FreeCA は、無料と有料の条件。0 はどちらでも、1 は無料だけ、2 は有料だけ。
	FreeCA uint8
	// CheckRecorded は、録画済みかのチェックをするかどうか (版 3 から)。
	CheckRecorded bool
}

// AutoAdd は、キーワード予約 (EPG_AUTO_ADD_DATA)。
type AutoAdd struct {
	ID         uint32
	Search     SearchKey
	RecSetting RecSetting
}

// Reserve は、予約 (RESERVE_DATA) のうち、移行で使う項目。
type Reserve struct {
	ServiceKey

	Title       string
	Start       time.Time
	Duration    time.Duration
	StationName string
	// EventID が 0xFFFF なら、番組に結び付かない時刻だけの予約。
	EventID    uint16
	Comment    string
	ID         uint32
	RecSetting RecSetting
}

// RecordedFile は、録画済み (REC_FILE_INFO) のうち、移行で使う項目。番組情報とエラーの情報は読まない。
type RecordedFile struct {
	ServiceKey

	ID          uint32
	Title       string
	Start       time.Time
	Duration    time.Duration
	ServiceName string
	EventID     uint16
	// Status は録画の結果 (EDCB の REC_END_STATUS)。
	Status uint32
	// StartEPG は、予約したときの開始。未設定なら零値。
	StartEPG time.Time
}

// AutoAdds は、キーワード予約の一覧を返す (ENUM_AUTO_ADD2)。
func (c *Client) AutoAdds(ctx context.Context) ([]*AutoAdd, error) {
	return enumerate(ctx, c, cmdEnumAutoAdd2, decodeAutoAdd)
}

// Reserves は、予約の一覧を返す (ENUM_RESERVE2)。
func (c *Client) Reserves(ctx context.Context) ([]*Reserve, error) {
	return enumerate(ctx, c, cmdEnumReserve2, decodeReserve)
}

// RecordedFiles は、録画済みの一覧を返す (ENUM_RECINFO_BASIC2)。番組情報とエラーの情報は、EpgTimerSrv が省く。
func (c *Client) RecordedFiles(ctx context.Context) ([]*RecordedFile, error) {
	return enumerate(ctx, c, cmdEnumRecInfoBasic2, decodeRecordedFile)
}

// enumerate は、版だけを送る「2」の付く一覧のコマンドを送り、応答の配列を読む。
func enumerate[T any](ctx context.Context, c *Client, cmd uint32, item func(*decoder) T) ([]T, error) {
	var e encoder
	e.u16(cmdVersion)
	payload, err := c.send(ctx, cmd, e.buf, c.requestTimeout)
	if err != nil {
		return nil, err
	}

	return decode(cmd, payload, versioned(func(d *decoder) []T {
		list := []T{}
		d.vector(func(d *decoder) { list = append(list, item(d)) })

		return list
	}))
}

func decodeRecSetting(d *decoder) RecSetting {
	var r RecSetting
	d.structure(func(d *decoder) {
		r.RecMode, r.Priority = d.u8(), d.u8()
		// 後ろの項目 (追従、保存先など) は、移行では使わない。構造体の終わりまで飛ばす。
	})

	return r
}

func decodeSearchKey(d *decoder) SearchKey {
	k := SearchKey{
		AndKey: "", NotKey: "", Regexp: false, TitleOnly: false, Contents: []ContentData{}, Dates: []SearchDate{}, Services: []uint64{},
		Fuzzy: false, NotContents: false, NotDates: false, FreeCA: 0, CheckRecorded: false,
	}
	d.structure(func(d *decoder) {
		k.AndKey, k.NotKey = d.str(), d.str()
		k.Regexp, k.TitleOnly = d.u32() != 0, d.u32() != 0
		d.vector(func(d *decoder) {
			d.structure(func(d *decoder) {
				k.Contents = append(k.Contents, ContentData{Level1: d.u8(), Level2: d.u8(), User1: d.u8(), User2: d.u8()})
			})
		})
		d.vector(func(d *decoder) {
			d.structure(func(d *decoder) {
				k.Dates = append(k.Dates, SearchDate{
					StartDayOfWeek: d.u8(), StartHour: d.u16(), StartMinute: d.u16(),
					EndDayOfWeek: d.u8(), EndHour: d.u16(), EndMinute: d.u16(),
				})
			})
		})
		d.vector(func(d *decoder) { k.Services = append(k.Services, d.u64()) })
		d.vector(func(d *decoder) { _ = d.u16() }) // 映像
		d.vector(func(d *decoder) { _ = d.u16() }) // 音声
		k.Fuzzy, k.NotContents, k.NotDates, k.FreeCA = d.u8() != 0, d.u8() != 0, d.u8() != 0, d.u8()
		// 版 3 から、録画済みかのチェックと、その期間が続く。期間は使わない。
		if d.remaining() > 0 {
			k.CheckRecorded = d.u8() != 0
		}
	})

	return k
}

func decodeAutoAdd(d *decoder) *AutoAdd {
	a := new(AutoAdd)
	d.structure(func(d *decoder) {
		a.ID = d.u32()
		a.Search = decodeSearchKey(d)
		a.RecSetting = decodeRecSetting(d)
	})

	return a
}

func decodeReserve(d *decoder) *Reserve {
	r := new(Reserve)
	d.structure(func(d *decoder) {
		r.Title = d.str()
		r.Start = d.systemTime()
		r.Duration = time.Duration(d.u32()) * time.Second
		r.StationName = d.str()
		r.ONID, r.TSID, r.SID, r.EventID = d.u16(), d.u16(), d.u16(), d.u16()
		r.Comment = d.str()
		r.ID = d.u32()
		_ = d.u8()         // 待機の印 (廃止)
		_ = d.u8()         // 重なりの状態
		_ = d.str()        // 録画先のパス (廃止)
		_ = d.systemTime() // 予約したときの開始
		r.RecSetting = decodeRecSetting(d)
	})

	return r
}

func decodeRecordedFile(d *decoder) *RecordedFile {
	f := new(RecordedFile)
	d.structure(func(d *decoder) {
		f.ID = d.u32()
		_ = d.str() // 録画のファイルのパス
		f.Title = d.str()
		f.Start = d.systemTime()
		f.Duration = time.Duration(d.u32()) * time.Second
		f.ServiceName = d.str()
		f.ONID, f.TSID, f.SID, f.EventID = d.u16(), d.u16(), d.u16(), d.u16()
		_, _ = d.u64(), d.u64() // ドロップとスクランブルの数
		f.Status = d.u32()
		if start := d.systemTime(); start.Year() >= unsetSystemTimeBefore {
			f.StartEPG = start
		}
	})

	return f
}
