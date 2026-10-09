// Package mirakurun は、Mirakurun 互換のチューナーサーバーのクライアントを提供する (設計書 3.1)。
//
// Mirakurun、mirakc、Mahiron が共通して持つ、Mirakurun 互換の REST API とサービスストリームを扱う。
//
// JSON の形は Mirakurun の api.d.ts に合わせている。Kotatsu が使わない項目は省いている。
package mirakurun

import "time"

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
	ID                 int64       `json:"id"`
	ServiceID          int         `json:"serviceId"`
	NetworkID          int         `json:"networkId"`
	Name               string      `json:"name"`
	Type               int         `json:"type"`
	LogoID             int         `json:"logoId,omitempty"`
	RemoteControlKeyID int         `json:"remoteControlKeyId,omitempty"`
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
	// Duration は長さ (ミリ秒)。終了時刻が未定のときは 1 になる。
	Duration    int64             `json:"duration"`
	IsFree      bool              `json:"isFree"`
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Genres      []*Genre          `json:"genres,omitempty"`
	Extended    map[string]string `json:"extended,omitempty"`
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
