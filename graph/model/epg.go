package model

import "time"

// Service はサービス。チャンネル種別は、リゾルバが dataloader で引く。
type Service struct {
	ID                 string `json:"id"`
	NetworkID          int    `json:"networkId"`
	ServiceID          int    `json:"serviceId"`
	Name               string `json:"name"`
	Channel            string `json:"channel"`
	RemoteControlKeyID *int   `json:"remoteControlKeyId,omitempty"`
	Hidden             bool   `json:"hidden"`
	// ChannelTypeKey は、チャンネル種別の文字列。スキーマには出さない。
	ChannelTypeKey string `json:"-"`
}

func (Service) IsNode() {}

// Program は番組。サービスは、リゾルバが dataloader で引く。
type Program struct {
	ID           string          `json:"id"`
	EventID      int             `json:"eventId"`
	StartAt      time.Time       `json:"startAt"`
	EndAt        time.Time       `json:"endAt"`
	EndUndecided bool            `json:"endUndecided"`
	IsFree       bool            `json:"isFree"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Extended     []*ExtendedItem `json:"extended"`
	Genres       []*Genre        `json:"genres"`
	// ServiceKey は、Mirakurun のサービス ID。スキーマには出さない。
	ServiceKey int64 `json:"-"`
}

func (Program) IsNode() {}

// GuideRow は、番組表の 1 列。
type GuideRow struct {
	Service  *Service   `json:"service"`
	Programs []*Program `json:"programs"`
}
