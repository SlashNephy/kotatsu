package epgstation

import "time"

// Snapshot は、EPGStation の DB から読み取った、取り込みの対象のデータ。
type Snapshot struct {
	Rules []*Rule
	// ManualReserves は手動予約。ルール予約は、取り込んだルールから作り直すので読まない。
	ManualReserves []*Reserve
	Recordings     []*Recording
	Histories      []*RecordedHistory
	Tags           []*Tag
}

// KeywordOption は、ルールのキーワードと、その照合の仕方。
type KeywordOption struct {
	Keyword       string
	CaseSensitive bool
	Regexp        bool
	// Name、Description、Extended は、番組名、概要、詳細のどれと照合するか。
	Name        bool
	Description bool
	Extended    bool
}

// Genre は番組のジャンル。SubGenre が nil なら、大分類のすべてを表す。
type Genre struct {
	Genre    int  `json:"genre"`
	SubGenre *int `json:"subGenre,omitempty"`
}

// SearchTime は、ルールの時刻の範囲。
// 番組の照合では Start が 0〜23 時の開始時刻、Range が 1〜23 時間の長さを表す。
// 時刻指定のルールでは、Start が 0 時からの秒、Range が秒の長さを表す。
type SearchTime struct {
	Start *int `json:"start,omitempty"`
	Range *int `json:"range,omitempty"`
	// Week は曜日のビット集合。0x01 が日曜日、0x40 が土曜日。
	Week int `json:"week"`
}

// Period は、ルールで番組を探す期間。
type Period struct {
	StartAt time.Time
	EndAt   time.Time
}

// Rule は EPGStation のルール。エンコードの設定は取り込まないので読まない。
type Rule struct {
	ID                  int64
	Enabled             bool
	IsTimeSpecification bool
	Keyword             KeywordOption
	IgnoreKeyword       KeywordOption
	// ChannelTypes は、GR、BS、CS、SKY の真偽値の列を集合にしたもの。
	ChannelTypes []string
	ChannelIDs   []int64
	Genres       []*Genre
	Times        []*SearchTime
	FreeOnly     bool
	// DurationMin と DurationMax は番組の長さ (秒) の範囲。nil なら制限しない。
	DurationMin   *int
	DurationMax   *int
	SearchPeriods []*Period
	// AvoidDuplicate は、録画済みの番組と重複するものを録画しないかどうか。
	AvoidDuplicate bool
	// PeriodToAvoidDuplicate は重複を避ける期間 (日)。nil なら期間を区切らない。
	PeriodToAvoidDuplicate *int
	AllowEndLack           bool
	// TagIDs は、録画に付けるタグの ID。
	TagIDs              []int64
	ParentDirectoryName string
	Directory           string
	RecordedFormat      string
}

// Reserve は EPGStation の予約。
type Reserve struct {
	ID              int64
	IsSkip          bool
	AllowEndLack    bool
	IsTimeSpecified bool
	IsEventRelay    bool
	// ProgramID は番組予約の番組の ID。時刻予約では nil。
	ProgramID   *int64
	ChannelID   int64
	ChannelType string
	StartAt     time.Time
	EndAt       time.Time
	Name        string
	// TagIDs は、録画に付けるタグの ID。
	TagIDs              []int64
	ParentDirectoryName string
	Directory           string
	RecordedFormat      string
}

// Recording は EPGStation の録画済みの番組。
type Recording struct {
	ID          int64
	RuleID      *int64
	ProgramID   *int64
	ChannelID   int64
	IsProtected bool
	// IsRecording は、録画中かどうか。
	IsRecording bool
	StartAt     time.Time
	EndAt       time.Time
	Name        string
	Description string
	Extended    string
	Genres      []*Genre
	Files       []*VideoFile
	// Drop はドロップの集計。集計していなければ nil。
	Drop   *DropLog
	TagIDs []int64
}

// VideoFileType は録画のファイルの種類。
type VideoFileType string

const (
	// VideoFileTypeTS は、録画した TS。
	VideoFileTypeTS VideoFileType = "ts"
	// VideoFileTypeEncoded は、エンコードしたファイル。
	VideoFileTypeEncoded VideoFileType = "encoded"
)

// VideoFile は録画のファイル。
type VideoFile struct {
	ID int64
	// ParentDirectoryName は、EPGStation の設定の録画の保存先ディレクトリの名前。
	ParentDirectoryName string
	// FilePath は、保存先ディレクトリからの相対パス。
	FilePath string
	Type     VideoFileType
	// Name は、画面に出すファイルの名前 (「TS」や、エンコードの設定の名前)。
	Name string
	Size int64
}

// DropLog はドロップの集計。
type DropLog struct {
	ErrorCount      int64
	DropCount       int64
	ScramblingCount int64
}

// RecordedHistory は、ルールの重複の回避に使う録画履歴。
type RecordedHistory struct {
	ID        int64
	Name      string
	ChannelID int64
	EndAt     time.Time
}

// Tag は録画のタグ。
type Tag struct {
	ID    int64
	Name  string
	Color string
}
