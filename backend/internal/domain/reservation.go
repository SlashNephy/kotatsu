package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ReservationKind は予約の種類。
type ReservationKind string

const (
	// ReservationKindProgram は番組予約。番組の放送時間の変更とイベントリレーに追従する。
	ReservationKindProgram ReservationKind = "program"
	// ReservationKindTime は時刻予約。サービスと開始と終了の時刻を指定し、番組の変更には追従しない。
	ReservationKindTime ReservationKind = "time"
	// ReservationKindRule はルール予約。ルールの照合が作る (#19)。
	ReservationKindRule ReservationKind = "rule"
)

var (
	// ErrReservationNotFound は、予約がないことを表す。
	ErrReservationNotFound = errors.New("reservation not found")
	// ErrInvalidReservation は、予約の内容が正しくないことを表す。
	ErrInvalidReservation = errors.New("invalid reservation")
)

// Reservation は予約。番組予約とルール予約は、作ったときの番組情報のコピーを持つ。
type Reservation struct {
	ID        uuid.UUID
	Kind      ReservationKind
	ServiceID int64
	// EventID は番組の eventId。時刻予約では nil。
	EventID     *int64
	StartAt     time.Time
	EndAt       time.Time
	Name        string
	Description string
	// Extended と Genres は、番組の詳細とジャンルを、EPG の JSON のまま持つ。
	Extended         json.RawMessage
	Genres           json.RawMessage
	Enabled          bool
	FollowEventRelay bool
	// Overrides は、この予約の録画設定の上書き。
	Overrides *RecordingOverrides
	// RelayedFrom は、イベントリレーで引き継いだ予約の、引き継ぎ元の予約。引き継いだ予約でなければ nil。
	RelayedFrom *uuid.UUID
	// Assignment は、neko が計算したチューナーの割り当ての結果 (設計書 4.5)。
	Assignment *Assignment
	CreatedAt  time.Time
	CreatedBy  *uuid.UUID
	UpdatedAt  time.Time
	UpdatedBy  *uuid.UUID
}

// AssignmentState は、予約のチューナーの割り当ての状態。値は assign.State と同じで、まだ計算していない状態が加わる。
type AssignmentState string

const (
	// AssignmentPending は、まだ割り当てを計算していないこと。
	AssignmentPending AssignmentState = "pending"
	// AssignmentOK は、全編を録画できる見込みであること。
	AssignmentOK AssignmentState = "ok"
	// AssignmentPartial は、本編の一部が欠ける見込みであること。
	AssignmentPartial AssignmentState = "partial"
	// AssignmentConflict は、チューナーが足りないこと。
	AssignmentConflict AssignmentState = "conflict"
)

// Assignment は、予約のチューナーの割り当ての結果。
type Assignment struct {
	State AssignmentState
	// Tuner は割り当てたチューナーの番号。割り当てていなければ nil。
	Tuner *int64
	// PlannedStartAt と PlannedEndAt は、マージンを削った後の録画する予定の時刻。割り当てていなければ nil。
	PlannedStartAt *time.Time
	PlannedEndAt   *time.Time
}

// Settings は、全体の既定値 defaults に、この予約の上書きを重ねた録画設定を返す。
func (r *Reservation) Settings(defaults *RecordingSettings) *RecordingSettings {
	return defaults.Resolve(r.Overrides)
}

// ProgramKey は番組の識別子 (サービスと eventId の組)。
type ProgramKey struct {
	ServiceID int64
	EventID   int64
}

const (
	// DefaultMarginSeconds は、録画の開始と終了のマージンの既定値 (設計書の付録 A の初期値)。
	DefaultMarginSeconds = 15
	// DefaultFileNameTemplate は、ファイル名のテンプレートの既定値。設計書に定めはないので、日時、サービス名、番組名を並べる。
	DefaultFileNameTemplate = `{{.StartAt.Format "2006-01-02 1504"}} {{.ServiceName}} {{.Name}}`
	// DefaultPriority は、予約の優先度の既定値。設計書に定めはないので 0 にする。
	// チューナーの割り当て (設計書 4.5) では、優先度の高い予約から順に割り当てる。
	DefaultPriority = 0
)

// RecordingSettings は、録画設定の全項目 (設計書 4.7)。全体の既定値に、ルールや予約の上書きを重ねて求める。
type RecordingSettings struct {
	// PoolID は保存先プール。空なら、推定サイズが収まるプールを kago が選ぶ (設計書 5.6)。
	PoolID string
	// FileNameTemplate は、ファイル名の text/template。データは FileNameData。
	FileNameTemplate string
	// StartMarginSeconds と EndMarginSeconds は、番組の前後に足して録画する秒数。
	StartMarginSeconds int
	EndMarginSeconds   int
	// Priority は予約の優先度。チューナーが足りないときは、高い予約から割り当てる。
	Priority int
	// JoinSegments は、引き継ぎや再起動で分かれた録画のファイルを、録画が終わったら 1 つにつなぐかどうか (設計書 5.4)。
	JoinSegments bool
	// DetectCM は、録画が終わったら、CM とみなす区間を見分けるかどうか (設計書 6.1)。既定では見分けない。
	DetectCM bool
}

// DefaultRecordingSettings は、Go の既定値を返す。
// 画面で変えた全体の既定値 (設計書 11.2) は、settings パッケージが、この値に重ねて返す。
func DefaultRecordingSettings() *RecordingSettings {
	return &RecordingSettings{
		PoolID:             "",
		FileNameTemplate:   DefaultFileNameTemplate,
		StartMarginSeconds: DefaultMarginSeconds,
		EndMarginSeconds:   DefaultMarginSeconds,
		Priority:           DefaultPriority,
		JoinSegments:       true,
		DetectCM:           false,
	}
}

// RecordingOverrides は、ルールや予約ごとの録画設定の上書き。nil の項目は上書きしない。
type RecordingOverrides struct {
	PoolID             *string `json:"poolId,omitempty"`
	FileNameTemplate   *string `json:"fileNameTemplate,omitempty"`
	StartMarginSeconds *int    `json:"startMarginSeconds,omitempty"`
	EndMarginSeconds   *int    `json:"endMarginSeconds,omitempty"`
	Priority           *int    `json:"priority,omitempty"`
	JoinSegments       *bool   `json:"joinSegments,omitempty"`
	DetectCM           *bool   `json:"detectCm,omitempty"`
}

// Resolve は、base に o の上書きを重ねた設定を返す。o は nil でもよい。
func (s *RecordingSettings) Resolve(o *RecordingOverrides) *RecordingSettings {
	resolved := *s
	if o == nil {
		return &resolved
	}
	if o.PoolID != nil {
		resolved.PoolID = *o.PoolID
	}
	if o.FileNameTemplate != nil {
		resolved.FileNameTemplate = *o.FileNameTemplate
	}
	if o.StartMarginSeconds != nil {
		resolved.StartMarginSeconds = *o.StartMarginSeconds
	}
	if o.EndMarginSeconds != nil {
		resolved.EndMarginSeconds = *o.EndMarginSeconds
	}
	if o.Priority != nil {
		resolved.Priority = *o.Priority
	}
	if o.JoinSegments != nil {
		resolved.JoinSegments = *o.JoinSegments
	}
	if o.DetectCM != nil {
		resolved.DetectCM = *o.DetectCM
	}

	return &resolved
}

// DecodeOverrides は、JSONB に保存した上書きを読む。
// ローリング更新の間は、新しいバイナリが足した項目を古いバイナリが読むことがあるので、知らない項目は無視する。
func DecodeOverrides(b []byte) (*RecordingOverrides, error) {
	o := new(RecordingOverrides)
	if err := json.Unmarshal(b, o); err != nil {
		return nil, fmt.Errorf("failed to decode recording settings: %w", err)
	}

	return o, nil
}
