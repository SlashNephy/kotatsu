package domain

import (
	"time"

	"github.com/google/uuid"
)

// Service は、番組表に並べるサービス。
type Service struct {
	ID                 int64
	NetworkID          int64
	Sid                int64
	Name               string
	ServiceType        int64
	ChannelType        string
	Channel            string
	RemoteControlKeyID *int64
	LogoID             *int64
	Hidden             bool
	SortOrder          *int32
}

// ChannelType は、チャンネルの種類と、画面での名前と並び順。
type ChannelType struct {
	Type        string
	DisplayName string
	SortOrder   int32
}

// Program は、番組。現役の番組とアーカイブの番組で同じ形にする。詳細、ジャンル、音声、関連する番組は、上流の JSON のまま持つ。
type Program struct {
	ServiceID    int64
	EventID      int64
	StartAt      time.Time
	EndAt        time.Time
	EndUndecided bool
	IsFree       bool
	Name         string
	Description  string
	Extended     []byte
	Genres       []byte
	Audios       []byte
	RelatedItems []byte
}

// ProgramCandidate は、番組の検索の候補。照合に使う、正規化した列も持つ。
type ProgramCandidate struct {
	Archived              bool
	ServiceID             int64
	EventID               int64
	StartAt               time.Time
	EndAt                 time.Time
	EndUndecided          bool
	IsFree                bool
	Name                  string
	Description           string
	Extended              []byte
	Genres                []byte
	Audios                []byte
	RelatedItems          []byte
	ChannelType           string
	NameNormalized        string
	DescriptionNormalized string
	ExtendedNormalized    *string
}

// FileNameProgram は、録画ファイルの名前の見本に当てはめる番組。
type FileNameProgram struct {
	StartAt     time.Time
	EndAt       time.Time
	Name        string
	EventID     int64
	ServiceName string
	ChannelType string
	Channel     string
}

// GuideView は、ユーザーの番組表ビュー。条件は JSON のまま持つ。
type GuideView struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	Name              string
	Settings          []byte
	HideEmptyServices bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// WatchReservation は、視聴予約と、その番組。
type WatchReservation struct {
	CreatedAt    time.Time
	ServiceID    int64
	EventID      int64
	StartAt      time.Time
	EndAt        time.Time
	EndUndecided bool
	IsFree       bool
	Name         string
	Description  string
	Extended     []byte
	Genres       []byte
	Audios       []byte
	RelatedItems []byte
}
