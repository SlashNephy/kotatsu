package model

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	_ Node = new(Service)
	_ Node = new(Program)
	_ Node = new(GuideView)
	_ Node = new(Rule)
	_ Node = new(Reservation)
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
	SortOrder          *int   `json:"sortOrder,omitempty"`
	// HasLogo は、放送がロゴを送っているかどうか。スキーマには出さない。
	HasLogo bool `json:"-"`
	// ChannelTypeKey は、チャンネル種別の文字列。スキーマには出さない。
	ChannelTypeKey string `json:"-"`
}

func (Service) IsNode() {}

// StreamURL は、ライブ視聴の TS を流す URL のパス。tenban の GET /streams/services/{id} で、Mirakurun のサービス ID を使う。
// IsNode と揃えて、値のレシーバーにする。
func (s Service) StreamURL() string {
	return fmt.Sprintf("/streams/services/%d", s.MirakurunID())
}

// LogoURL は、ロゴの画像の URL のパス。tenban の GET /media/services/{id}/logo で、Mirakurun のサービス ID を使う。
// 放送がロゴを送っていなければ nil。
func (s Service) LogoURL() *string {
	if !s.HasLogo {
		return nil
	}

	return new(fmt.Sprintf("/media/services/%d/logo", s.MirakurunID()))
}

// MirakurunID は、Mirakurun のサービス ID。DB の services の ID と同じ。
func (s Service) MirakurunID() int64 {
	return int64(s.NetworkID)*mirakurunServiceIDBase + int64(s.ServiceID)
}

// mirakurunServiceIDBase は、Mirakurun のサービス ID の、ネットワーク ID の桁の重み (network_id * 100000 + service_id)。
const mirakurunServiceIDBase = 100000

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
	Audios       []*ProgramAudio `json:"audios"`
	Archived     bool            `json:"archived"`
	// ServiceKey は、Mirakurun のサービス ID。スキーマには出さない。
	ServiceKey int64 `json:"-"`
	// SharedFromKey は、イベント共有の元のサービスの Mirakurun のサービス ID。元がなければ 0。スキーマには出さない。
	SharedFromKey int64 `json:"-"`
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
	FreeOnly           bool              `json:"freeOnly"`
	// ServiceKeys は、チャンネルの並び (Mirakurun のサービス ID)。スキーマには出さない。
	ServiceKeys []int64 `json:"-"`
}

func (GuideView) IsNode() {}

// ProgramConditions は番組の条件。サービスは、リゾルバが dataloader で引く。
type ProgramConditions struct {
	Keywords           []*Keyword        `json:"keywords"`
	KeywordMatch       KeywordMatch      `json:"keywordMatch"`
	ExcludeKeywords    []*Keyword        `json:"excludeKeywords"`
	ChannelTypes       []string          `json:"channelTypes"`
	Genres             []*GenreCondition `json:"genres"`
	ExcludeGenres      []*GenreCondition `json:"excludeGenres"`
	TimeRanges         []*TimeRange      `json:"timeRanges"`
	MinDurationSeconds *int              `json:"minDurationSeconds,omitempty"`
	MaxDurationSeconds *int              `json:"maxDurationSeconds,omitempty"`
	FreeOnly           bool              `json:"freeOnly"`
	Marks              []string          `json:"marks"`
	ExcludeMarks       []string          `json:"excludeMarks"`
	// ServiceKeys は、サービス (Mirakurun のサービス ID)。スキーマには出さない。
	ServiceKeys []int64 `json:"-"`
	// ExcludeServiceKeys は、除外するサービス (Mirakurun のサービス ID)。スキーマには出さない。
	ExcludeServiceKeys []int64 `json:"-"`
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
	// Key は、ルールの ID。スキーマには出さない。
	Key uuid.UUID `json:"-"`
}

func (Rule) IsNode() {}

// Reservation は予約。サービスと一致したルールは、リゾルバが dataloader で引く。
type Reservation struct {
	ID               string                      `json:"id"`
	Kind             ReservationKind             `json:"kind"`
	EventID          *int                        `json:"eventId,omitempty"`
	StartAt          time.Time                   `json:"startAt"`
	EndAt            time.Time                   `json:"endAt"`
	Name             string                      `json:"name"`
	Description      string                      `json:"description"`
	Genres           []*Genre                    `json:"genres"`
	Enabled          bool                        `json:"enabled"`
	FollowEventRelay bool                        `json:"followEventRelay"`
	Settings         *RecordingSettingsOverrides `json:"settings"`
	Assignment       *ReservationAssignment      `json:"assignment"`
	CreatedAt        time.Time                   `json:"createdAt"`
	CreatedBy        *User                       `json:"createdBy,omitempty"`
	UpdatedAt        time.Time                   `json:"updatedAt"`
	UpdatedBy        *User                       `json:"updatedBy,omitempty"`
	// Key と ServiceKey は、予約の ID と Mirakurun のサービス ID。スキーマには出さない。
	Key        uuid.UUID `json:"-"`
	ServiceKey int64     `json:"-"`
}

func (Reservation) IsNode() {}
