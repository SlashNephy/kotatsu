package reserve

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"text/template"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
)

const (

	// 録画設定の値の範囲。
	maxMarginSeconds     = 60 * 60
	minPriority          = -10
	maxPriority          = 10
	maxFileNameTemplateB = 1024
)

// ErrInvalidSettings は、録画設定が正しくないことを表す。
var ErrInvalidSettings = errors.New("invalid recording settings")

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
// 使える書き方は、checkFileNameNodes の許可リストに限る。
func ParseFileNameTemplate(text string) (*template.Template, error) {
	if len(text) > maxFileNameTemplateB {
		return nil, fmt.Errorf("%w: fileNameTemplate must be at most %d bytes", ErrInvalidSettings, maxFileNameTemplateB)
	}
	tmpl, err := template.New("file_name").Option("missingkey=error").Funcs(fileNameFuncs()).Parse(text)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid file name template: %w", ErrInvalidSettings, err)
	}
	if len(tmpl.Templates()) > 1 {
		return nil, fmt.Errorf("%w: invalid file name template: define and block are not allowed", ErrInvalidSettings)
	}
	if err := checkFileNameNodes(tmpl.Root); err != nil {
		return nil, fmt.Errorf("%w: invalid file name template: %w", ErrInvalidSettings, err)
	}
	// 項目の名前の誤りは、実行してみないと分からないので、見本のデータで試す。
	if err := ExecuteFileNameTemplate(tmpl, io.Discard, SampleFileNameData()); err != nil {
		return nil, fmt.Errorf("%w: invalid file name template: %w", ErrInvalidSettings, err)
	}

	return tmpl, nil
}

// ValidateOverrides は、上書きの値が正しいかを確かめる。プールが設定にあるかどうかは、設定を持つ API の層で確かめる。
func ValidateOverrides(o *domain.RecordingOverrides) error {
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

// EncodeOverrides は、上書きを検証し、JSONB に保存する形にする。ルールも同じ形で保存する。nil なら空のオブジェクトにする。
func EncodeOverrides(o *domain.RecordingOverrides) ([]byte, error) {
	if err := ValidateOverrides(o); err != nil {
		return nil, err
	}
	if o == nil {
		o = new(domain.RecordingOverrides)
	}
	b, err := json.Marshal(o)
	if err != nil {
		return nil, fmt.Errorf("failed to encode recording settings: %w", err)
	}

	return b, nil
}
