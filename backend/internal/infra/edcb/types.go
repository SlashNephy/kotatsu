package edcb

import (
	"time"
)

// ServiceKey は、EDCB でサービスを見分ける組 (original_network_id、transport_stream_id、service_id)。
type ServiceKey struct {
	ONID uint16
	TSID uint16
	SID  uint16
}

// key は、EDCB がサービスを引くのに使う 64 ビットのキー (ONID << 32 | TSID << 16 | SID)。
func (k ServiceKey) key() int64 {
	return int64(k.ONID)<<32 | int64(k.TSID)<<16 | int64(k.SID)
}

// Service は、EPG のデータベースにあるサービス (EPGDB_SERVICE_INFO)。
type Service struct {
	ServiceKey

	// Type は service_type。テレビは 0x01、デジタル音声は 0x02、データは 0xC0 など。
	Type uint8
	// PartialReception は、部分受信 (ワンセグ) のサービスかどうか。
	PartialReception bool
	ProviderName     string
	Name             string
	NetworkName      string
	TSName           string
	// RemoteControlKeyID は、リモコンキー ID。EDCB は「無い」を表す値を持たず、無ければ 0 になる。
	RemoteControlKeyID uint8
}

// Event は、番組 (EPGDB_EVENT_INFO)。持たない情報は nil か空にする。
type Event struct {
	ServiceKey

	EventID uint16
	// Start は、番組の開始時刻 (日本標準時)。未定なら StartKnown が false で、ゼロ値。
	Start      time.Time
	StartKnown bool
	// Duration は、番組の長さ。未定なら DurationKnown が false で、0。
	Duration      time.Duration
	DurationKnown bool
	// Short は、番組名と概要 (短形式イベント記述子)。
	Short *ShortEvent
	// Extended は、詳細 (拡張形式イベント記述子) を EDCB が 1 本にまとめた文字列。
	// 「- 項目名」の行と本文を並べたもので、本文の行頭の「- 」はエスケープされている。
	Extended *string
	// Genres は、ジャンル (コンテント記述子)。
	Genres []Genre
	// Video は、映像のコンポーネント。EDCB は 1 つだけ持つ。
	Video *Component
	// Audios は、音声のコンポーネント。
	Audios []AudioComponent
	// Group と Relay は、イベントグループ記述子のうち、共有 (1) と、リレー (2、4) の番組。移動 (3、5) は EDCB が捨てる。
	Group *EventGroup
	Relay *EventGroup
	// Scrambled は、free_CA_mode が 1 (有料放送) かどうか。EDCB の freeCAFlag で、Mirakurun の isFree とは真偽が逆になる。
	Scrambled bool
}

// ShortEvent は、番組名と概要。
type ShortEvent struct {
	Name string
	Text string
}

// Genre は、ジャンルの組 (content_nibble_level_1、content_nibble_level_2、user_nibble の 2 つ)。
type Genre struct {
	Level1, Level2 uint8
	User1, User2   uint8
}

// Component は、映像のコンポーネント記述子。
type Component struct {
	StreamContent uint8
	ComponentType uint8
	ComponentTag  uint8
	Text          string
}

// AudioComponent は、音声コンポーネント記述子。SamplingRate は、放送のコード (sampling_rate) のまま持つ。
type AudioComponent struct {
	StreamContent     uint8
	ComponentType     uint8
	ComponentTag      uint8
	StreamType        uint8
	SimulcastGroupTag uint8
	ESMultiLingual    bool
	MainComponent     bool
	QualityIndicator  uint8
	SamplingRate      uint8
	Text              string
}

// EventGroup は、イベントグループ記述子。Type は group_type。
type EventGroup struct {
	Type   uint8
	Events []EventRef
}

// EventRef は、ほかの番組を指す組。
type EventRef struct {
	ServiceKey

	EventID uint16
}

// ServiceEvents は、サービスと、その番組。
type ServiceEvents struct {
	Service *Service
	Events  []*Event
}

// TunerReserve は、チューナーと、そのチューナーに割り当てた予約の ID (TUNER_RESERVE_INFO)。
// 最後の要素は、チューナーが足りない予約を集めたもので、TunerID が UnassignedTunerID になる。
type TunerReserve struct {
	TunerID    uint32
	TunerName  string
	ReserveIDs []uint32
}

// UnassignedTunerID は、チューナーが足りない予約を集めた TunerReserve の TunerID。
const UnassignedTunerID = 0xFFFFFFFF

// TunerProcess は、EpgTimerSrv が起動しているチューナーのプロセスの状態 (TUNER_PROCESS_STATUS_INFO)。
// 起動していないチューナーは含まない。
type TunerProcess struct {
	TunerID     uint32
	ProcessID   int32
	Drop        uint64
	Scramble    uint64
	SignalLevel float32
	Space       int32
	Channel     int32
	ONID        int32
	TSID        int32
	// Recording と CapturingEPG は、録画と EPG の取得に使っているかどうか。
	Recording    bool
	CapturingEPG bool
	ExtraFlags   uint16
}

// Notification は、EpgTimerSrv の変化の通知 (NOTIFY_SRV_INFO)。Count は、通知の巡回カウンタ (param3)。
type Notification struct {
	ID     uint32
	Time   time.Time
	Param1 uint32
	Param2 uint32
	Count  uint32
	Param4 string
	Param5 string
	Param6 string
}

// 通知の種類 (EDCB の Common/CommonDef.h)。
const (
	NotifyUpdateEPGData     = 1
	NotifyUpdateReserveInfo = 2
	NotifyUpdateRecInfo     = 3
	NotifyUpdateSrvStatus   = 100
	NotifyPreRecStart       = 101
	NotifyRecStart          = 102
	NotifyRecEnd            = 103
	NotifyPreEPGCapStart    = 106
	NotifyEPGCapStart       = 107
	NotifyEPGCapEnd         = 108
)

// File は、FILE_COPY2 で受け取ったファイル。EpgTimerSrv が返さなかったファイルは Data が空になる。
type File struct {
	Name string
	Data []byte
}
