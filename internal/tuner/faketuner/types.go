// Package faketuner は、テスト用の偽のチューナーサーバーを提供する (設計書 11.6)。
//
// Mirakurun 互換の API を持ち、合成した EPG とイベントを返し、合成した TS を流す。
// テストからは、番組の追加、時間の変更、延長、削除のイベントを発生させられる。
// チューナーの本数と、priority によるチューナーの奪い合いも再現する (設計書 1.6)。
//
// JSON の形は Mirakurun の api.d.ts に合わせている。使っていない項目は省いている。
package faketuner

// Channel は /api/channels の要素。
type Channel struct {
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
	// ID は Mirakurun のサービス ID (networkId × 100000 + serviceId)。
	ID        int64  `json:"id"`
	ServiceID int    `json:"serviceId"`
	NetworkID int    `json:"networkId"`
	Name      string `json:"name"`
	Type      int    `json:"type"`
	// RemoteControlKeyID はリモコンのキーの番号。0 なら返さない。
	RemoteControlKeyID int `json:"remoteControlKeyId,omitempty"`
	// LogoID はロゴの番号。nil なら返さない。
	LogoID  *int        `json:"logoId,omitempty"`
	Channel *ChannelRef `json:"channel,omitempty"`
	// Logo は、/api/services/{id}/logo で返す PNG。nil なら、Mirakurun がロゴをまだ受信していないときと同じく 503 を返す。
	Logo []byte `json:"-"`
}

// Program は /api/programs の要素。
type Program struct {
	// ID は Mirakurun の番組 ID。ProgramID で作る。
	ID        int64 `json:"id"`
	EventID   int   `json:"eventId"`
	ServiceID int   `json:"serviceId"`
	NetworkID int   `json:"networkId"`
	// StartAt は開始時刻 (UNIX 時間のミリ秒)。
	StartAt int64 `json:"startAt"`
	// Duration は長さ (ミリ秒)。延長中で終了時刻が未定のときは 1 を返すのが Mirakurun の慣習である。
	Duration    int64  `json:"duration"`
	IsFree      bool   `json:"isFree"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	// RelatedItems は、イベント共有 (shared) とイベントリレー (relay) で関連づけた番組。
	RelatedItems []*RelatedItem `json:"relatedItems,omitempty"`
	// Audios は、番組の音声の構成。
	Audios []*Audio `json:"audios,omitempty"`
}

// Audio は、番組の音声。Mirakurun の audios の要素と同じ形にする。
type Audio struct {
	ComponentType int `json:"componentType"`
	// ComponentTag は component_tag。nil なら、mirakc と同じく返さない。
	ComponentTag *int     `json:"componentTag,omitempty"`
	IsMain       bool     `json:"isMain"`
	SamplingRate int      `json:"samplingRate"`
	Langs        []string `json:"langs"`
}

// RelatedItem は、番組に関連づけた別のサービスの番組。
type RelatedItem struct {
	Type      string `json:"type"`
	NetworkID int    `json:"networkId,omitempty"`
	ServiceID int    `json:"serviceId"`
	EventID   int    `json:"eventId"`
}

// Tuner は /api/tuners の要素。
type Tuner struct {
	Index       int          `json:"index"`
	Name        string       `json:"name"`
	Types       []string     `json:"types"`
	Command     string       `json:"command"`
	PID         int          `json:"pid"`
	Users       []*TunerUser `json:"users"`
	IsAvailable bool         `json:"isAvailable"`
	IsRemote    bool         `json:"isRemote"`
	IsFree      bool         `json:"isFree"`
	IsUsing     bool         `json:"isUsing"`
	IsFault     bool         `json:"isFault"`
}

// TunerUser は、チューナーを使っている利用者。Mirakurun は利用者を「IP:ポート」で区別する。
type TunerUser struct {
	ID       string `json:"id"`
	Priority int    `json:"priority"`
	Agent    string `json:"agent,omitempty"`
}

// Version は /api/version の応答。
type Version struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
}

// Event は /api/events/stream で送るイベント。
type Event struct {
	// Resource は program、service、tuner のどれか。
	Resource string `json:"resource"`
	// Type は create、update、remove のどれか。
	Type string `json:"type"`
	Data any    `json:"data"`
	// Time はイベントの時刻 (UNIX 時間のミリ秒)。
	Time int64 `json:"time"`
}

// ProgramID は、Mirakurun と同じ方法で番組 ID を作る。
func ProgramID(networkID, serviceID, eventID int) int64 {
	return int64(networkID)*10_000_000_000 + int64(serviceID)*100_000 + int64(eventID)
}

// ServiceItemID は、Mirakurun と同じ方法でサービス ID を作る。
func ServiceItemID(networkID, serviceID int) int64 {
	return int64(networkID)*100_000 + int64(serviceID)
}
