package domain

import (
	"time"

	"github.com/google/uuid"
)

// Capture は、視聴画面で撮ったキャプチャ (設計書 7.5)。撮ったユーザーだけのもの。
type Capture struct {
	ID     uuid.UUID
	UserID uuid.UUID
	// RecordingID は、録画で撮ったときの録画。ライブとタイムシフトで撮ったもの、撮った後に録画を消したものは nil。
	RecordingID *uuid.UUID
	// Position は、録画で撮ったときの、録画の始まりからの再生位置。録画でなければ nil。
	Position    *time.Duration
	ServiceName string
	// ProgramName は、番組名。番組が分からなければ nil。
	ProgramName *string
	// Caption は、撮ったときに出ていた字幕の文 (#458)。出ていなければ nil。
	Caption *string
	// BroadcastAt は、撮った場面の放送時刻。TakenAt は、撮った時刻 (端末の時計)。
	BroadcastAt time.Time
	TakenAt     time.Time
	// Size は、画像の大きさ (バイト)。
	Size      int64
	CreatedAt time.Time
}

// CapturePosition は、キャプチャの一覧での位置。記録した日時と ID で並べる。
type CapturePosition struct {
	CreatedAt time.Time
	ID        uuid.UUID
}
