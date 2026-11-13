package domain

import (
	"slices"
	"time"
	"uuid"
)

// Recording は、録画。
type Recording struct {
	ID            uuid.UUID
	APIID         int64
	ReservationID *uuid.UUID
	ServiceID     int64
	EventID       *int64
	Name          string
	Description   string
	// NameNormalized と DescriptionNormalized は、NFKC で正規化した番組名と概要 (設計書 3.5)。録画の検索に使う。
	// 列を知らない古いバイナリが書いた録画では nil。
	NameNormalized        *string
	DescriptionNormalized *string
	StartAt               time.Time
	EndAt                 time.Time
	State                 RecordingState
	FailureReason         *RecordingFailureReason
	// FailureDetail は、失敗した理由に至った、最後のエラーの本文 (#1467)。管理者にだけ見せる。
	FailureDetail *string
	Protected     bool
	Drops         int64
	Errors        int64
	Scrambled     int64
	// ScrambledRatio は、ES の PID だけで出した、スクランブルされていたパケットの割合 (#1517)。判定しなかった録画では nil。
	ScrambledRatio *float64
	// ScrambleWarning は、スクランブルされたまま録れたという警告の印 (設計書 5.5)。状態とは別に持つ。
	ScrambleWarning    bool
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

// RecordingLiveStats は、録画中の録画の、書いているセグメントの様子 (#829)。mikan が 10 秒ごとに書く。
type RecordingLiveStats struct {
	RecordingID uuid.UUID
	Bytes       int64
	Drops       int64
	Errors      int64
	Scrambled   int64
	// Bitrate は、直前の間隔で書いたビットレート (bps)。
	Bitrate   int64
	UpdatedAt time.Time
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
	// Video は、セグメントの映像から読んだ映像の形式の JSON (#1310)。読めなければ nil。
	Video []byte
	// TunerIndex と TunerName は、セグメントを書いたストリームを受信していたチューナー (#1467)。見つけられなければ nil。
	TunerIndex *int64
	TunerName  *string
	// TranscodeFrameRate は、Kotatsu が録画を変換した MP4 の、1 秒あたりのコマ数 (#1509)。ほかのファイルと、記録する前に変換した MP4 では nil。
	TranscodeFrameRate *int32
	CreatedAt          time.Time
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
	State          RecordingState
	Protected      bool
	Extended       []byte
	Size           int64
	ImageSegmentID uuid.UUID
}

// PlayerSettings は、ユーザーのプレイヤーの設定。コメントと字幕の設定は JSON のまま持つ。
type PlayerSettings struct {
	JikkyoEnabled   bool
	CommentSources  []string
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

// JikkyoAccess は、実況を使えるかどうかを決める、全体の設定とユーザーの設定 (設計書 7.4)。
type JikkyoAccess struct {
	// SystemEnabled は、管理者が全体の設定で実況を有効にしたかどうか。
	SystemEnabled bool
	// UserEnabled は、ユーザーが実況を有効にしたかどうか。
	UserEnabled bool
}

// Allowed は、外部と通信してよいかどうかを返す。全体の設定とユーザーの設定の両方で有効なときだけ true にする。
func (a JikkyoAccess) Allowed() bool {
	return a.SystemEnabled && a.UserEnabled
}

// CommentSourceNXJikkyo は、NX-Jikkyo (ライブのコメントと過去ログ) のコメントのソース (#1405、設計書 7.4)。
const CommentSourceNXJikkyo = "nx_jikkyo"

// CommentSourceNicolog は、NICOLOG (ニコニコ生放送の上映会のコメント) のソース (#1363、設計書 7.4)。
const CommentSourceNicolog = "nicolog"

// CommentSourceAccess は、コメントのソースごとに、使えるかどうかを決める全体の設定とユーザーの設定 (#1405)。
type CommentSourceAccess struct {
	// System は、管理者が全体の設定でオプトインしたソース。
	System []string
	// User は、ユーザーがオプトインしたソース。
	User []string
}

// For は、ソース source を使えるかどうかを返す。
func (a CommentSourceAccess) For(source string) JikkyoAccess {
	return JikkyoAccess{SystemEnabled: slices.Contains(a.System, source), UserEnabled: slices.Contains(a.User, source)}
}

// RecordingJikkyoSource は、録画の実況のコメントを引くための、録画したサービスと時刻。
type RecordingJikkyoSource struct {
	RecordedFrom  time.Time
	RecordedUntil *time.Time
	NetworkID     int64
	Sid           int64
}

// OtherAnimeBroadcast は、録画と同じ話の、別の放送 (#1361)。StartAt と EndAt を OffsetSeconds だけずらしたものが、実際の放送の時間帯。
type OtherAnimeBroadcast struct {
	PID             int32
	StartAt         time.Time
	EndAt           time.Time
	OffsetSeconds   int32
	JikkyoChannelID int32
}
