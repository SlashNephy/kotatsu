// Package mirakurun は、Mirakurun 互換のチューナーサーバーのクライアントを提供する (設計書 3.1)。
//
// Mirakurun 互換の REST API とサービスストリームを基本にし、サーバーの種類 (Mirakurun、mirakc、Mahiron) と
// 追加機能は、接続時と定期的に判定し直す。イベントは、Mirakurun と Mahiron では /api/events/stream、
// mirakc では /events (SSE) から読む。
//
// 応答は、チューナーサーバーの種類に依らない型 (domain/tunerserver) に変換して返す。
// JSON の形は Mirakurun の api.d.ts に合わせている。Kotatsu が使わない項目は省いている。
package mirakurun

import (
	"encoding/json"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
)

// channel は /api/channels の要素。
type channel struct {
	Type     string     `json:"type"`
	Channel  string     `json:"channel"`
	Name     string     `json:"name"`
	Services []*service `json:"services,omitempty"`
}

// channelRef は、service が属するチャンネル。
type channelRef struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
}

// service は /api/services の要素。
type service struct {
	// ID は Mirakurun のサービス ID (networkId × 100000 + serviceId)。
	ID                 int64       `json:"id"`
	ServiceID          int         `json:"serviceId"`
	NetworkID          int         `json:"networkId"`
	Name               string      `json:"name"`
	Type               int         `json:"type"`
	LogoID             *int        `json:"logoId,omitempty"`
	RemoteControlKeyID *int        `json:"remoteControlKeyId,omitempty"`
	Channel            *channelRef `json:"channel,omitempty"`
}

// program は /api/programs の要素。
type program struct {
	ID        int64 `json:"id"`
	EventID   int   `json:"eventId"`
	ServiceID int   `json:"serviceId"`
	NetworkID int   `json:"networkId"`
	// StartAt は開始時刻 (UNIX 時間のミリ秒)。
	StartAt int64 `json:"startAt"`
	// Duration は長さ (ミリ秒)。終了時刻が未定のときは、Mirakurun では 1、Mahiron では 0 になる。
	Duration     int64                 `json:"duration"`
	IsFree       bool                  `json:"isFree"`
	Name         string                `json:"name,omitempty"`
	Description  string                `json:"description,omitempty"`
	Genres       []*domain.Genre       `json:"genres,omitempty"`
	Extended     map[string]string     `json:"extended,omitempty"`
	RelatedItems []*domain.RelatedItem `json:"relatedItems,omitempty"`
	Audios       []*domain.Audio       `json:"audios,omitempty"`
	Video        *video                `json:"video,omitempty"`
	Series       *series               `json:"series,omitempty"`
}

// series は、番組のシリーズの記述子。Mirakurun と mirakc は同じ形で返す。
type series struct {
	ID          int    `json:"id"`
	Repeat      int    `json:"repeat"`
	Episode     int    `json:"episode"`
	LastEpisode int    `json:"lastEpisode"`
	Name        string `json:"name,omitempty"`
}

// video は、番組の映像。
type video struct {
	Resolution string `json:"resolution,omitempty"`
}

// onAirProgram は、mirakc の /api/onair の要素。
type onAirProgram struct {
	ServiceID int64    `json:"serviceId"`
	Current   *program `json:"current"`
	Next      *program `json:"next"`
}

// tuner は /api/tuners の要素。
type tuner struct {
	Index       int          `json:"index"`
	Name        string       `json:"name"`
	Types       []string     `json:"types"`
	Users       []*tunerUser `json:"users"`
	IsAvailable bool         `json:"isAvailable"`
	IsRemote    bool         `json:"isRemote"`
	IsFree      bool         `json:"isFree"`
	IsUsing     bool         `json:"isUsing"`
	IsFault     bool         `json:"isFault"`
}

// tunerUser は、チューナーを使っている利用者。
type tunerUser struct {
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

// RawEvent は、チューナーサーバーから届いたイベントの、解釈する前の形。
//
// Mirakurun と Mahiron のイベントは、Resource が program、service、tuner のどれかで、
// Type が create、update、remove のどれかになる。
// mirakc のイベントは、SSE のイベント名 (例: epg.programs-updated) を最初の "." で分け、
// Resource を epg、Type を programs-updated のようにする。mirakc のイベントには時刻がないので、Time はゼロ値になる。
type RawEvent struct {
	Resource string
	Type     string
	// Data はイベントの中身。Resource と Type によって形が変わるので、受け取った JSON のまま持つ。
	Data json.RawMessage
	Time time.Time
}

func (c *channel) toTunerServer() *tunerserver.Channel {
	return &tunerserver.Channel{Type: c.Type, Channel: c.Channel, Name: c.Name, Services: convert(c.Services, (*service).toTunerServer)}
}

func (s *service) toTunerServer() *tunerserver.Service {
	var ref *tunerserver.ChannelRef
	if s.Channel != nil {
		ref = &tunerserver.ChannelRef{Type: s.Channel.Type, Channel: s.Channel.Channel}
	}

	return &tunerserver.Service{
		ID:                 s.ID,
		ServiceID:          s.ServiceID,
		NetworkID:          s.NetworkID,
		Name:               s.Name,
		Type:               s.Type,
		LogoID:             s.LogoID,
		RemoteControlKeyID: s.RemoteControlKeyID,
		Channel:            ref,
	}
}

// toTunerServer は、番組を変換する。p が nil なら nil を返す。
func (p *program) toTunerServer() *tunerserver.Program {
	if p == nil {
		return nil
	}
	var v *tunerserver.Video
	if p.Video != nil {
		v = &tunerserver.Video{Resolution: p.Video.Resolution}
	}
	var s *tunerserver.Series
	if p.Series != nil {
		s = &tunerserver.Series{
			ID:          p.Series.ID,
			Repeat:      p.Series.Repeat,
			Episode:     p.Series.Episode,
			LastEpisode: p.Series.LastEpisode,
			Name:        p.Series.Name,
		}
	}

	return &tunerserver.Program{
		ID:           p.ID,
		EventID:      p.EventID,
		ServiceID:    p.ServiceID,
		NetworkID:    p.NetworkID,
		StartAt:      p.StartAt,
		Duration:     p.Duration,
		IsFree:       p.IsFree,
		Name:         p.Name,
		Description:  p.Description,
		Genres:       p.Genres,
		Extended:     p.Extended,
		RelatedItems: p.RelatedItems,
		Audios:       p.Audios,
		Video:        v,
		Series:       s,
	}
}

func (o *onAirProgram) toTunerServer() *tunerserver.OnAirProgram {
	return &tunerserver.OnAirProgram{ServiceID: o.ServiceID, Current: o.Current.toTunerServer(), Next: o.Next.toTunerServer()}
}

func (t *tuner) toTunerServer() *tunerserver.Tuner {
	return &tunerserver.Tuner{
		Index:       t.Index,
		Name:        t.Name,
		Types:       t.Types,
		Users:       convert(t.Users, (*tunerUser).toTunerServer),
		IsAvailable: t.IsAvailable,
		IsRemote:    t.IsRemote,
		IsFree:      t.IsFree,
		IsUsing:     t.IsUsing,
		IsFault:     t.IsFault,
	}
}

func (u *tunerUser) toTunerServer() *tunerserver.TunerUser {
	return &tunerserver.TunerUser{ID: u.ID, Priority: u.Priority, Agent: u.Agent}
}

// convert は、JSON の配列の要素を変換する。JSON の null の要素は、変換せずに nil のまま残す。
func convert[T any, U any](items []*T, f func(*T) *U) []*U {
	converted := make([]*U, 0, len(items))
	for _, item := range items {
		if item == nil {
			converted = append(converted, nil)

			continue
		}
		converted = append(converted, f(item))
	}

	return converted
}
