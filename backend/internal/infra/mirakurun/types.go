// Package mirakurun は、Mirakurun 互換のチューナーサーバーのクライアントを提供する (設計書 3.1)。
//
// Mirakurun 互換の REST API とサービスストリームを基本にし、サーバーの種類 (Mirakurun、mirakc、Mahiron) と
// 追加機能は、接続時と定期的に判定し直す。イベントは、Mirakurun と Mahiron では /api/events/stream、
// mirakc では /events (SSE) から読む。
//
// JSON の形は Mirakurun の api.d.ts に合わせている。Kotatsu が使わない項目は省いている。
package mirakurun

import (
	"encoding/json"
	"time"
)

// Channel は /api/channels の要素。
type Channel struct {
	// Type はチャンネル種別。GR、BS、CS、SKY のほか、Mahiron は BS4K や EXT1 のような任意の文字列を返すので、
	// 列挙型にせず文字列のまま持つ (設計書 3.1)。
	Type     string     `json:"type"`
	Channel  string     `json:"channel"`
	Name     string     `json:"name"`
	Services []*Service `json:"services,omitempty"`
}

// ChannelRef は、Service が属するチャンネル。
type ChannelRef struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
}

// Service は /api/services の要素。
type Service struct {
	// ID は Mirakurun のサービス ID (networkId × 100000 + serviceId)。Kotatsu はこの値でサービスを識別する (設計書 3.2)。
	ID        int64  `json:"id"`
	ServiceID int    `json:"serviceId"`
	NetworkID int    `json:"networkId"`
	Name      string `json:"name"`
	Type      int    `json:"type"`
	// LogoID と RemoteControlKeyID は、サーバーが返さなければ nil。
	LogoID             *int        `json:"logoId,omitempty"`
	RemoteControlKeyID *int        `json:"remoteControlKeyId,omitempty"`
	Channel            *ChannelRef `json:"channel,omitempty"`
}

// Program は /api/programs の要素。
type Program struct {
	// ID は Mirakurun の番組 ID。EID は再利用されうるので、番組の同一性の判定には使わない (設計書 3.2)。
	ID        int64 `json:"id"`
	EventID   int   `json:"eventId"`
	ServiceID int   `json:"serviceId"`
	NetworkID int   `json:"networkId"`
	// StartAt は開始時刻 (UNIX 時間のミリ秒)。
	StartAt int64 `json:"startAt"`
	// Duration は長さ (ミリ秒)。終了時刻が未定のときは、Mirakurun では 1、Mahiron では 0 になる。
	Duration    int64             `json:"duration"`
	IsFree      bool              `json:"isFree"`
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Genres      []*Genre          `json:"genres,omitempty"`
	Extended    map[string]string `json:"extended,omitempty"`
	// RelatedItems は、イベント共有 (shared) とイベントリレー (relay) で関連づけられた番組 (設計書 4.4、4.6)。
	RelatedItems []*RelatedItem `json:"relatedItems,omitempty"`
	// Audios は、番組の音声の構成。音声コンポーネント記述子ごとに 1 つある。
	Audios []*Audio `json:"audios,omitempty"`
	// Video は、番組の映像。EIT のコンポーネント記述子から作られる。上流が返さなければ nil。
	Video *Video `json:"video,omitempty"`
}

// Video は、番組の映像。
type Video struct {
	// Resolution は、映像の解像度 (480i、1080i、2160p、4320p など)。
	// Mahiron は BS4K の番組の type を mpeg2 と返すので、映像の種類は Resolution で見分ける (#806)。
	Resolution string `json:"resolution,omitempty"`
}

// Start は開始時刻を返す。
func (p *Program) Start() time.Time {
	return time.UnixMilli(p.StartAt).UTC()
}

// Genre は番組のジャンル。ARIB STD-B10 の content_nibble をそのまま持つ。
type Genre struct {
	Lv1 int `json:"lv1"`
	Lv2 int `json:"lv2"`
	Un1 int `json:"un1"`
	Un2 int `json:"un2"`
}

// Audio は、番組の音声。ARIB STD-B10 の音声コンポーネント記述子をそのまま持つ。
type Audio struct {
	// ComponentType は component_type。1 はシングルモノ、2 はデュアルモノ、3 はステレオ、9 は 5.1ch など。
	ComponentType int `json:"componentType"`
	// ComponentTag は component_tag。mirakc は返さない。
	ComponentTag *int `json:"componentTag,omitempty"`
	IsMain       bool `json:"isMain"`
	SamplingRate int  `json:"samplingRate"`
	// Langs は、言語のコード (ISO 639-2)。デュアルモノでは、主音声と副音声の 2 つになる。
	Langs []string `json:"langs"`
}

// RelatedItem は、番組に関連づけられた別のサービスの番組。
type RelatedItem struct {
	// Type は shared (同じイベントを複数のサービスで放送する)、relay (別のサービスに引き継ぐ)、movement のどれか。
	Type      string `json:"type"`
	NetworkID int    `json:"networkId,omitempty"`
	ServiceID int    `json:"serviceId"`
	EventID   int    `json:"eventId"`
}

// Tuner は /api/tuners の要素。
type Tuner struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	// Types は、このチューナーが受信できるチャンネル種別。1 本で複数の種別を受信できる機器もある (設計書 3.6)。
	Types       []string     `json:"types"`
	Users       []*TunerUser `json:"users"`
	IsAvailable bool         `json:"isAvailable"`
	IsRemote    bool         `json:"isRemote"`
	IsFree      bool         `json:"isFree"`
	IsUsing     bool         `json:"isUsing"`
	IsFault     bool         `json:"isFault"`
}

// TunerUser は、チューナーを使っている利用者。
type TunerUser struct {
	ID       string `json:"id"`
	Priority int    `json:"priority"`
	Agent    string `json:"agent,omitempty"`
}

// Version は /api/version の応答。
type Version struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	// Server は、Mahiron が "mahiron" を返す。Mirakurun と mirakc は返さない。
	Server string `json:"server,omitempty"`
}

// Event は、チューナーサーバーから届いたイベント。
//
// Mirakurun と Mahiron のイベントは、Resource が program、service、tuner のどれかで、
// Type が create、update、remove のどれかになる。
// mirakc のイベントは、SSE のイベント名 (例: epg.programs-updated) を最初の "." で分け、
// Resource を epg、Type を programs-updated のようにする。mirakc のイベントには時刻がないので、Time はゼロ値になる。
//
// Mirakurun のイベントは取りこぼしや遅れがあるので、データの正しさは全量の取得で保証する (設計書 3.3)。
type Event struct {
	Resource string
	Type     string
	// Data はイベントの中身。Resource と Type によって形が変わるので、受け取った JSON のまま持つ。
	Data json.RawMessage
	Time time.Time
}
