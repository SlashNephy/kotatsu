package model

import (
	"time"

	"github.com/google/uuid"
)

// Recording は録画 (設計書 5.5、8.2)。サービスは、リゾルバで dataloader から引く。
type Recording struct {
	ID                 string                  `json:"id"`
	EventID            *int                    `json:"eventId,omitempty"`
	Name               string                  `json:"name"`
	Description        string                  `json:"description"`
	StartAt            time.Time               `json:"startAt"`
	EndAt              time.Time               `json:"endAt"`
	State              RecordingState          `json:"state"`
	FailureReason      *RecordingFailureReason `json:"failureReason,omitempty"`
	Protected          bool                    `json:"protected"`
	Drops              int64                   `json:"drops"`
	Errors             int64                   `json:"errors"`
	Scrambled          int64                   `json:"scrambled"`
	Gaps               []*RecordingGap         `json:"gaps"`
	Audios             []*ProgramAudio         `json:"audios"`
	Genres             []*Genre                `json:"genres"`
	RuleName           *string                 `json:"ruleName,omitempty"`
	ProgramLost        bool                    `json:"programLost"`
	BroadcastStartedAt *time.Time              `json:"broadcastStartedAt,omitempty"`
	BroadcastEndedAt   *time.Time              `json:"broadcastEndedAt,omitempty"`
	NotBroadcast       bool                    `json:"notBroadcast"`
	RecordedFrom       time.Time               `json:"recordedFrom"`
	RecordedUntil      *time.Time              `json:"recordedUntil,omitempty"`
	Tags               []*Tag                  `json:"tags"`
	Files              []*RecordingFile        `json:"files"`
	WatchPositionMs    *int64                  `json:"watchPositionMs,omitempty"`
	ThumbnailURL       *string                 `json:"thumbnailUrl,omitempty"`
	PreviewURL         *string                 `json:"previewUrl,omitempty"`
	StreamURL          string                  `json:"streamUrl"`
	JikkyoCommentsURL  string                  `json:"jikkyoCommentsUrl"`
	// ServiceKey は、Mirakurun のサービス ID。スキーマには出さない。
	ServiceKey int64 `json:"-"`
	// RuleKey は、録画を始めたときに一致していたルールの ID。スキーマには出さず、rule のリゾルバーで引く。
	RuleKey *uuid.UUID `json:"-"`
}

var _ Node = new(Recording)

func (Recording) IsNode() {}
