package model

import (
	"time"
)

// Recording は録画 (設計書 5.5、8.2)。サービスは、リゾルバで dataloader から引く。
type Recording struct {
	ID              string                  `json:"id"`
	EventID         *int                    `json:"eventId,omitempty"`
	Name            string                  `json:"name"`
	Description     string                  `json:"description"`
	StartAt         time.Time               `json:"startAt"`
	EndAt           time.Time               `json:"endAt"`
	State           RecordingState          `json:"state"`
	FailureReason   *RecordingFailureReason `json:"failureReason,omitempty"`
	Protected       bool                    `json:"protected"`
	Drops           int64                   `json:"drops"`
	Errors          int64                   `json:"errors"`
	Scrambled       int64                   `json:"scrambled"`
	Gaps            []*RecordingGap         `json:"gaps"`
	ProgramLost     bool                    `json:"programLost"`
	RecordedFrom    time.Time               `json:"recordedFrom"`
	RecordedUntil   *time.Time              `json:"recordedUntil,omitempty"`
	Tags            []*Tag                  `json:"tags"`
	Files           []*RecordingFile        `json:"files"`
	WatchPositionMs *int64                  `json:"watchPositionMs,omitempty"`
	ThumbnailURL    *string                 `json:"thumbnailUrl,omitempty"`
	PreviewURL      *string                 `json:"previewUrl,omitempty"`
	StreamURL       string                  `json:"streamUrl"`
	// ServiceKey は、Mirakurun のサービス ID。スキーマには出さない。
	ServiceKey int64 `json:"-"`
}

var _ Node = new(Recording)

func (Recording) IsNode() {}
