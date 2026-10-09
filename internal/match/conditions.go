// Package match は、ルールの条件と番組を照合する (設計書 3.5、4.2)。
//
// ルールの照合 (neko) と番組の検索 (tenban) は、この照合を共有するので、両者の結果は一致する。
// キーワードの照合は、NFKC で正規化した文字列どうしで行い、正規表現は Go の regexp (RE2) で評価する。
// RE2 は入力の長さに対して線形の時間で動くので、ユーザーが書いた正規表現で処理が止まることはない。
package match

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"unicode/utf8"
)

// SchemaVersion は、JSONB に保存する条件のスキーマのバージョン (設計書 8.3)。
// 型を変えるときはバージョンを上げ、Decode で古いバージョンを今の型に変換する。
const SchemaVersion = 1

// 条件の大きさの上限。照合は番組ごとに全条件を評価するので、際限なく大きな条件を受け付けない。
const (
	maxKeywords      = 32
	maxKeywordLength = 1000
	maxListLength    = 1000
	minutesPerDay    = 24 * 60
	minutesPerWeek   = 7 * minutesPerDay
	maxGenreLevel    = 0xF
)

// ErrInvalidConditions は、条件が正しくないことを表す。
var ErrInvalidConditions = errors.New("invalid conditions")

// Field は、キーワードを照合する番組の項目。
type Field string

const (
	// FieldName は番組名。
	FieldName Field = "name"
	// FieldDescription は概要。
	FieldDescription Field = "description"
	// FieldExtended は詳細。
	FieldExtended Field = "extended"
)

// Keyword はキーワードと、その照合の仕方。
type Keyword struct {
	Text string `json:"text"`
	// Regexp は、Text を正規表現として扱うかどうか。
	// 正規表現は書かれたとおりに評価する。全角の括弧などが記号の意味を変えないよう、パターン自体は正規化しない。
	Regexp bool `json:"regexp"`
	// CaseSensitive は、大文字と小文字を区別するかどうか。
	CaseSensitive bool `json:"caseSensitive"`
	// Fields は、照合する項目。どれか 1 つに一致すれば、このキーワードに一致したとみなす。
	Fields []Field `json:"fields"`
}

// Genre はジャンル。ARIB STD-B10 の content_nibble の大分類と中分類で表す。
type Genre struct {
	Lv1 int `json:"lv1"`
	// Lv2 は中分類。nil なら大分類のすべてを表す。
	Lv2 *int `json:"lv2,omitempty"`
}

// TimeRange は、曜日と時間帯の組。開始時刻が範囲に入る番組に一致する。
type TimeRange struct {
	// Weekdays は曜日の集合 (0 が日曜日、6 が土曜日)。
	Weekdays []int `json:"weekdays"`
	// StartMinute は、0 時からの分で表した範囲の始まり (0〜1439)。
	StartMinute int `json:"startMinute"`
	// DurationMinutes は範囲の長さ (分)。日付をまたいでよい。最長で 1 週間。
	DurationMinutes int `json:"durationMinutes"`
}

// Conditions はルールの条件。空の項目では絞り込まない。
type Conditions struct {
	// Keywords は、すべてに一致する番組だけを対象にする。
	Keywords []*Keyword `json:"keywords"`
	// ExcludeKeywords は、どれか 1 つに一致したら除外する。
	ExcludeKeywords []*Keyword `json:"excludeKeywords"`
	// ChannelTypes と ServiceIDs は、番組のチャンネル種別かサービスのどちらかが含まれていれば対象にする。
	// 例えば、種別に BS を、サービスに地上波の 1 局を指定すると、BS の全局とその 1 局が対象になる。
	ChannelTypes []string `json:"channelTypes"`
	ServiceIDs   []int64  `json:"serviceIds"`
	// Genres は、どれか 1 つに一致する番組を対象にする。ExcludeGenres は、どれか 1 つに一致したら除外する。
	Genres        []*Genre `json:"genres"`
	ExcludeGenres []*Genre `json:"excludeGenres"`
	// TimeRanges は、どれか 1 つに開始時刻が入る番組を対象にする。
	TimeRanges []*TimeRange `json:"timeRanges"`
	// MinDurationSeconds と MaxDurationSeconds は、番組の長さ (秒) の範囲。nil なら制限しない。
	MinDurationSeconds *int `json:"minDurationSeconds,omitempty"`
	MaxDurationSeconds *int `json:"maxDurationSeconds,omitempty"`
	// FreeOnly は、無料放送に限るかどうか。
	FreeOnly bool `json:"freeOnly"`
}

// Validate は、条件が正しいかを確かめる。正しくなければ、理由をすべてまとめた ErrInvalidConditions を返す。
func (c *Conditions) Validate() error {
	if c == nil {
		return invalid("conditions must not be null")
	}

	errs := []error{}
	errs = append(errs, validateKeywords("keywords", c.Keywords)...)
	errs = append(errs, validateKeywords("excludeKeywords", c.ExcludeKeywords)...)
	for _, list := range []struct {
		name   string
		length int
	}{
		{"channelTypes", len(c.ChannelTypes)},
		{"serviceIds", len(c.ServiceIDs)},
		{"genres", len(c.Genres)},
		{"excludeGenres", len(c.ExcludeGenres)},
		{"timeRanges", len(c.TimeRanges)},
	} {
		if list.length > maxListLength {
			errs = append(errs, invalid("%s must have at most %d items", list.name, maxListLength))
		}
	}
	if slices.Contains(c.ChannelTypes, "") {
		errs = append(errs, invalid("channelTypes must not contain an empty string"))
	}
	errs = append(errs, validateGenres("genres", c.Genres)...)
	errs = append(errs, validateGenres("excludeGenres", c.ExcludeGenres)...)
	for i, r := range c.TimeRanges {
		if err := r.validate(); err != nil {
			errs = append(errs, invalid("timeRanges[%d]: %v", i, err))
		}
	}
	if err := validateDurations(c.MinDurationSeconds, c.MaxDurationSeconds); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// invalid は、理由を添えた ErrInvalidConditions を作る。
func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidConditions}, args...)...)
}

func validateKeywords(name string, keywords []*Keyword) []error {
	errs := []error{}
	if len(keywords) > maxKeywords {
		errs = append(errs, invalid("%s must have at most %d items", name, maxKeywords))
	}
	for i, keyword := range keywords {
		if err := keyword.validate(); err != nil {
			errs = append(errs, invalid("%s[%d]: %v", name, i, err))
		}
	}

	return errs
}

func validateGenres(name string, genres []*Genre) []error {
	errs := []error{}
	inRange := func(level int) bool { return level >= 0 && level <= maxGenreLevel }
	for i, genre := range genres {
		if genre == nil || !inRange(genre.Lv1) || (genre.Lv2 != nil && !inRange(*genre.Lv2)) {
			errs = append(errs, invalid("%s[%d] must have lv1 and lv2 between 0 and %d", name, i, maxGenreLevel))
		}
	}

	return errs
}

func validateDurations(minimum, maximum *int) error {
	if (minimum != nil && *minimum < 0) || (maximum != nil && *maximum < 0) {
		return invalid("durations must not be negative")
	}
	if minimum != nil && maximum != nil && *minimum > *maximum {
		return invalid("minDurationSeconds must not exceed maxDurationSeconds")
	}

	return nil
}

func (k *Keyword) validate() error {
	if k == nil {
		return errors.New("must not be null")
	}
	if k.Text == "" || utf8.RuneCountInString(k.Text) > maxKeywordLength {
		return fmt.Errorf("text must be 1 to %d characters", maxKeywordLength)
	}
	if len(k.Fields) == 0 {
		return errors.New("fields must not be empty")
	}
	for _, field := range k.Fields {
		if field != FieldName && field != FieldDescription && field != FieldExtended {
			return fmt.Errorf("unknown field %q", field)
		}
	}
	if k.Regexp {
		if _, err := regexp.Compile(k.Text); err != nil {
			return fmt.Errorf("invalid regular expression: %w", err)
		}
	}

	return nil
}

func (r *TimeRange) validate() error {
	if r == nil {
		return errors.New("must not be null")
	}
	if len(r.Weekdays) == 0 {
		return errors.New("weekdays must not be empty")
	}
	for _, weekday := range r.Weekdays {
		if weekday < 0 || weekday > 6 {
			return errors.New("weekdays must be between 0 (Sunday) and 6 (Saturday)")
		}
	}
	if r.StartMinute < 0 || r.StartMinute >= minutesPerDay {
		return fmt.Errorf("startMinute must be between 0 and %d", minutesPerDay-1)
	}
	if r.DurationMinutes < 1 || r.DurationMinutes > minutesPerWeek {
		return fmt.Errorf("durationMinutes must be between 1 and %d", minutesPerWeek)
	}

	return nil
}

// envelope は、JSONB に保存する形。条件にスキーマのバージョンを添える。
type envelope struct {
	Version    int             `json:"version"`
	Conditions json.RawMessage `json:"conditions"`
}

// Encode は、条件を検証してから、スキーマのバージョンを付けた JSON にする。
func Encode(c *Conditions) ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	conditions, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("failed to encode conditions: %w", err)
	}
	b, err := json.Marshal(&envelope{Version: SchemaVersion, Conditions: conditions})
	if err != nil {
		return nil, fmt.Errorf("failed to encode conditions: %w", err)
	}

	return b, nil
}

// Decode は、Encode で保存した JSON を読み、検証した条件を返す。知らない項目やバージョンは拒否する。
func Decode(b []byte) (*Conditions, error) {
	var e envelope
	if err := strictUnmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConditions, err)
	}
	if e.Version != SchemaVersion {
		return nil, fmt.Errorf("%w: unsupported schema version %d", ErrInvalidConditions, e.Version)
	}

	c := new(Conditions)
	if err := strictUnmarshal(e.Conditions, c); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConditions, err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}

	return c, nil
}

func strictUnmarshal(b []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()

	return decoder.Decode(v)
}
