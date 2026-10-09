package model

import "time"

var (
	_ Node = new(Service)
	_ Node = new(Program)
	_ Node = new(GuideView)
	_ Node = new(Rule)
)

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
	Archived     bool            `json:"archived"`
	// ServiceKey は、Mirakurun のサービス ID。スキーマには出さない。
	ServiceKey int64 `json:"-"`
}

func (Program) IsNode() {}

// GuideRow は、番組表の 1 列。
type GuideRow struct {
	Service  *Service   `json:"service"`
	Programs []*Program `json:"programs"`
}

// GuideView は番組表ビュー。サービスは、リゾルバが dataloader で引く。
type GuideView struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Genres             []*GenreCondition `json:"genres"`
	ExcludeGenres      []*GenreCondition `json:"excludeGenres"`
	MinDurationSeconds *int              `json:"minDurationSeconds,omitempty"`
	HideEmptyServices  bool              `json:"hideEmptyServices"`
	// ServiceKeys は、チャンネルの並び (Mirakurun のサービス ID)。スキーマには出さない。
	ServiceKeys []int64 `json:"-"`
}

func (GuideView) IsNode() {}

// ProgramConditions は番組の条件。サービスは、リゾルバが dataloader で引く。
type ProgramConditions struct {
	Keywords           []*Keyword        `json:"keywords"`
	ExcludeKeywords    []*Keyword        `json:"excludeKeywords"`
	ChannelTypes       []string          `json:"channelTypes"`
	Genres             []*GenreCondition `json:"genres"`
	ExcludeGenres      []*GenreCondition `json:"excludeGenres"`
	TimeRanges         []*TimeRange      `json:"timeRanges"`
	MinDurationSeconds *int              `json:"minDurationSeconds,omitempty"`
	MaxDurationSeconds *int              `json:"maxDurationSeconds,omitempty"`
	FreeOnly           bool              `json:"freeOnly"`
	// ServiceKeys は、サービス (Mirakurun のサービス ID)。スキーマには出さない。
	ServiceKeys []int64 `json:"-"`
}

// Rule はルール。
type Rule struct {
	ID                  string                      `json:"id"`
	Name                string                      `json:"name"`
	Memo                string                      `json:"memo"`
	Enabled             bool                        `json:"enabled"`
	Conditions          *ProgramConditions          `json:"conditions"`
	Settings            *RecordingSettingsOverrides `json:"settings"`
	ValidFrom           *time.Time                  `json:"validFrom,omitempty"`
	ValidUntil          *time.Time                  `json:"validUntil,omitempty"`
	AvoidDuplicates     bool                        `json:"avoidDuplicates"`
	DuplicateWindowDays *int                        `json:"duplicateWindowDays,omitempty"`
	FollowEventRelay    bool                        `json:"followEventRelay"`
	CreatedAt           time.Time                   `json:"createdAt"`
	CreatedBy           *User                       `json:"createdBy,omitempty"`
	UpdatedAt           time.Time                   `json:"updatedAt"`
	UpdatedBy           *User                       `json:"updatedBy,omitempty"`
}

func (Rule) IsNode() {}
