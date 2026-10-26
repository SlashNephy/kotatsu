package domain

import (
	"time"

	"github.com/google/uuid"
)

// Recording は、録画。
type Recording struct {
	ID                 uuid.UUID
	APIID              int64
	ReservationID      *uuid.UUID
	ServiceID          int64
	EventID            *int64
	Name               string
	Description        string
	StartAt            time.Time
	EndAt              time.Time
	State              string
	FailureReason      *string
	Protected          bool
	Drops              int64
	Errors             int64
	Scrambled          int64
	Gaps               []byte
	Audios             []byte
	VideoResolution    *string
	Genres             []byte
	RuleID             *uuid.UUID
	RuleName           *string
	ProgramLost        bool
	BroadcastStartedAt *time.Time
	BroadcastEndedAt   *time.Time
	// WaitedForBroadcast は、番組が遅れて始まったので、前の番組の TS を消し、放送中になるまで待ってから書き始めたかどうか (#1046)。
	WaitedForBroadcast bool
	NotBroadcast       bool
	AiredEventID       *int64
	RecordedFrom       time.Time
	RecordedUntil      *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// RecordingFile は、録画のファイル (録画ファイル、生成物)。
type RecordingFile struct {
	ID          uuid.UUID
	RecordingID uuid.UUID
	Kind        string
	PoolID      string
	Path        string
	Size        int64
	StartedAt   time.Time
	EndedAt     *time.Time
	PIDStats    []byte
	CreatedAt   time.Time
}

// RecordingTag は、録画に付いたタグ。Recordings は、タグの付いた録画の数。
type RecordingTag struct {
	RecordingID uuid.UUID
	ID          uuid.UUID
	Name        string
	Recordings  int64
}

// Tag は、タグ。Recordings は、タグの付いた録画の数。
type Tag struct {
	ID         uuid.UUID
	Name       string
	Recordings int64
}

// APIRecording は、EPGStation 互換 API が返す録画。
type APIRecording struct {
	APIID          int64
	ID             uuid.UUID
	ServiceID      int64
	Name           string
	Description    string
	StartAt        time.Time
	EndAt          time.Time
	Genres         []byte
	State          string
	Protected      bool
	Extended       []byte
	Size           int64
	ImageSegmentID uuid.UUID
}

// PlayerSettings は、ユーザーのプレイヤーの設定。コメントと字幕の設定は JSON のまま持つ。
type PlayerSettings struct {
	JikkyoEnabled   bool
	CaptionsEnabled bool
	CommentSettings []byte
	CaptionSettings []byte
}

// ContinueWatching は、続きから見る録画。
type ContinueWatching struct {
	Recording Recording
}

// WatchProgress は、ユーザーが録画を見た位置。
type WatchProgress struct {
	UserID      uuid.UUID
	RecordingID uuid.UUID
	PositionMs  int64
	UpdatedAt   time.Time
}

// RecordingJikkyoSource は、録画の実況のコメントを引くための、録画したサービスと時刻。
type RecordingJikkyoSource struct {
	RecordedFrom  time.Time
	RecordedUntil *time.Time
	NetworkID     int64
	Sid           int64
}
