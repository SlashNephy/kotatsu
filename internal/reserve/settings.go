package reserve

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"text/template"
	"time"

	"github.com/SlashNephy/kotatsu/internal/config"
)

const (
	// DefaultMarginSeconds は、録画の開始と終了のマージンの既定値 (設計書の付録 A の初期値)。
	DefaultMarginSeconds = 15
	// DefaultFileNameTemplate は、ファイル名のテンプレートの既定値。設計書に定めはないので、日時、サービス名、番組名を並べる。
	DefaultFileNameTemplate = `{{.StartAt.Format "2006-01-02 1504"}} {{.ServiceName}} {{.Name}}`
	// DefaultPriority は、予約の優先度の既定値。設計書に定めはないので 0 にする。
	// チューナーの割り当て (設計書 4.5) では、優先度の高い予約から順に割り当てる。
	DefaultPriority = 0

	// 録画設定の値の範囲。
	maxMarginSeconds     = 60 * 60
	minPriority          = -10
	maxPriority          = 10
	maxFileNameTemplateB = 1024
)

// ErrInvalidSettings は、録画設定が正しくないことを表す。
var ErrInvalidSettings = errors.New("invalid recording settings")

// Settings は、録画設定の全項目 (設計書 4.7)。全体の既定値に、ルールや予約の上書きを重ねて求める。
type Settings struct {
	// PoolID は保存先プール。空なら、推定サイズが収まるプールを kago が選ぶ (設計書 5.6)。
	PoolID string
	// FileNameTemplate は、ファイル名の text/template。データは FileNameData。
	FileNameTemplate string
	// StartMarginSeconds と EndMarginSeconds は、番組の前後に足して録画する秒数。
	StartMarginSeconds int
	EndMarginSeconds   int
	// Priority は予約の優先度。チューナーが足りないときは、高い予約から割り当てる。
	Priority int
}

// DefaultSettings は、全体の既定値を返す。
// 設計書 11.2 では全体の既定値は画面から変える設定なので、DB に置くのはシステム設定の API (#48) で行う。
func DefaultSettings() *Settings {
	return &Settings{
		PoolID:             "",
		FileNameTemplate:   DefaultFileNameTemplate,
		StartMarginSeconds: DefaultMarginSeconds,
		EndMarginSeconds:   DefaultMarginSeconds,
		Priority:           DefaultPriority,
	}
}

// Overrides は、ルールや予約ごとの録画設定の上書き。nil の項目は上書きしない。
type Overrides struct {
	PoolID             *string `json:"poolId,omitempty"`
	FileNameTemplate   *string `json:"fileNameTemplate,omitempty"`
	StartMarginSeconds *int    `json:"startMarginSeconds,omitempty"`
	EndMarginSeconds   *int    `json:"endMarginSeconds,omitempty"`
	Priority           *int    `json:"priority,omitempty"`
}

// Resolve は、base に o の上書きを重ねた設定を返す。o は nil でもよい。
func (s *Settings) Resolve(o *Overrides) *Settings {
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

	return &resolved
}

// FileNameData は、ファイル名のテンプレートに渡すデータ。
type FileNameData struct {
	// StartAt と EndAt は、表示のタイムゾーンの放送時刻。
	StartAt     time.Time
	EndAt       time.Time
	ServiceName string
	ChannelType string
	Channel     string
	Name        string
	EventID     int64
}

// ParseFileNameTemplate は、ファイル名のテンプレートを解釈する。知らない項目を参照するテンプレートはエラーにする。
func ParseFileNameTemplate(text string) (*template.Template, error) {
	tmpl, err := template.New("file_name").Option("missingkey=error").Parse(text)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid file name template: %w", ErrInvalidSettings, err)
	}
	// 項目の名前の誤りは、実行してみないと分からないので、見本のデータで試す。
	if err := tmpl.Execute(io.Discard, sampleFileNameData()); err != nil {
		return nil, fmt.Errorf("%w: invalid file name template: %w", ErrInvalidSettings, err)
	}

	return tmpl, nil
}

func sampleFileNameData() *FileNameData {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	return &FileNameData{
		StartAt:     start,
		EndAt:       start.Add(time.Hour),
		ServiceName: "サービス",
		ChannelType: "GR",
		Channel:     "27",
		Name:        "番組",
		EventID:     1,
	}
}

// Validate は、上書きの値が正しいかを確かめる。プールが設定にあるかどうかは、設定を持つ API の層で確かめる。
func (o *Overrides) Validate() error {
	if o == nil {
		return nil
	}

	errs := []error{}
	if o.PoolID != nil && *o.PoolID != "" && !config.IsValidPoolID(*o.PoolID) {
		errs = append(errs, fmt.Errorf("%w: poolId %q must consist of lowercase letters, digits, - and _", ErrInvalidSettings, *o.PoolID))
	}
	if o.FileNameTemplate != nil {
		if len(*o.FileNameTemplate) > maxFileNameTemplateB {
			errs = append(errs, fmt.Errorf("%w: fileNameTemplate must be at most %d bytes", ErrInvalidSettings, maxFileNameTemplateB))
		} else if _, err := ParseFileNameTemplate(*o.FileNameTemplate); err != nil {
			errs = append(errs, err)
		}
	}
	for _, margin := range []struct {
		name  string
		value *int
	}{{"startMarginSeconds", o.StartMarginSeconds}, {"endMarginSeconds", o.EndMarginSeconds}} {
		if margin.value != nil && (*margin.value < 0 || *margin.value > maxMarginSeconds) {
			errs = append(errs, fmt.Errorf("%w: %s must be between 0 and %d", ErrInvalidSettings, margin.name, maxMarginSeconds))
		}
	}
	if o.Priority != nil && (*o.Priority < minPriority || *o.Priority > maxPriority) {
		errs = append(errs, fmt.Errorf("%w: priority must be between %d and %d", ErrInvalidSettings, minPriority, maxPriority))
	}

	return errors.Join(errs...)
}

// encodeOverrides は、検証した上書きを JSONB に保存する形にする。nil なら空のオブジェクトにする。
func encodeOverrides(o *Overrides) ([]byte, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if o == nil {
		o = new(Overrides)
	}
	b, err := json.Marshal(o)
	if err != nil {
		return nil, fmt.Errorf("failed to encode recording settings: %w", err)
	}

	return b, nil
}

// decodeOverrides は、JSONB に保存した上書きを読む。
// ローリング更新の間は、新しいバイナリが足した項目を古いバイナリが読むことがあるので、知らない項目は無視する。
func decodeOverrides(b []byte) (*Overrides, error) {
	o := new(Overrides)
	if err := json.Unmarshal(b, o); err != nil {
		return nil, fmt.Errorf("failed to decode recording settings: %w", err)
	}

	return o, nil
}
