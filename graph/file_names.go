package graph

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/reserve"
)

// previewExt は、プレビューで付ける録画ファイルの拡張子 (recording パッケージの segmentExt と同じ)。
const previewExt = ".m2ts"

// fileNameTemplateHelp は、ファイル名のテンプレートの書き方と、よく使うテンプレートに、見本の番組を当てはめた例を付けて返す。
func (r *Resolver) fileNameTemplateHelp(ctx context.Context) (*model.FileNameTemplateHelp, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	sample := reserve.SampleFileNameData()
	items := reserve.FileNameTemplateItems()
	help := &model.FileNameTemplateHelp{
		Items:   make([]*model.FileNameTemplateItem, 0, len(items)),
		Presets: []*model.FileNameTemplatePreset{},
	}
	for _, item := range items {
		tmpl, err := reserve.ParseFileNameTemplate(item.Snippet)
		if err != nil {
			return nil, fmt.Errorf("failed to parse the file name template item %q: %w", item.Snippet, err)
		}
		var example strings.Builder
		if err := tmpl.Execute(&example, sample); err != nil {
			return nil, fmt.Errorf("failed to render the file name template item %q: %w", item.Snippet, err)
		}
		help.Items = append(help.Items, &model.FileNameTemplateItem{
			Snippet: item.Snippet, Description: item.Description, Example: example.String(),
		})
	}
	for _, preset := range reserve.FileNameTemplatePresets() {
		name, err := renderFileName(preset.Template, sample)
		if err != nil {
			return nil, fmt.Errorf("failed to render the file name template preset %q: %w", preset.Name, err)
		}
		help.Presets = append(help.Presets, &model.FileNameTemplatePreset{Name: preset.Name, Template: preset.Template, Example: name})
	}

	return help, nil
}

// previewFileName は、テンプレートに番組 (省けば見本の番組) を当てはめたファイル名を、録画と同じ整え方で返す。
// テンプレートの誤りは、GraphQL のエラーではなく、結果の error で返す。
func (r *Resolver) previewFileName(ctx context.Context, text string, programID *string) (*model.FileNamePreview, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	data := reserve.SampleFileNameData()
	if programID != nil {
		typeName, raw, err := ParseGlobalID(*programID)
		if err != nil || typeName != programType {
			return nil, NewError(CodeBadUserInput, "invalid program ID")
		}
		serviceID, eventID, err := parseProgramID(raw)
		if err != nil {
			return nil, err
		}
		row, err := sqlc.New(r.db).GetProgramForFileName(ctx, sqlc.GetProgramForFileNameParams{ServiceID: serviceID, EventID: eventID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, NewError(CodeNotFound, "program not found")
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get the program: %w", err)
		}
		data = &reserve.FileNameData{
			StartAt: row.StartAt.In(r.location), EndAt: row.EndAt.In(r.location), ServiceName: row.ServiceName,
			ChannelType: row.ChannelType, Channel: row.Channel, Name: row.Name, EventID: row.EventID,
		}
	}
	name, err := renderFileName(text, data)
	if err != nil {
		return &model.FileNamePreview{FileName: nil, Error: new(templateError(err))}, nil
	}

	return &model.FileNamePreview{FileName: &name, Error: nil}, nil
}

func renderFileName(text string, data *reserve.FileNameData) (string, error) {
	tmpl, err := reserve.ParseFileNameTemplate(text)
	if err != nil {
		return "", err
	}

	return kago.RenderName(tmpl, data, previewExt)
}

// templateError は、テンプレートの誤りを、利用者に見せる文にする。よくある誤り (知らない項目と関数) は日本語にし、
// ほかは text/template のメッセージをそのまま返す。
func templateError(err error) string {
	message := err.Error()
	if m := unknownFieldPattern.FindStringSubmatch(message); m != nil {
		return "知らない項目 ." + m[1] + " があります"
	}
	if m := unknownFuncPattern.FindStringSubmatch(message); m != nil {
		return "知らない関数 " + m[1] + " があります"
	}
	if _, rest, ok := strings.Cut(message, "invalid file name template: "); ok {
		message = rest
	}

	return strings.TrimPrefix(message, "template: file_name:")
}

var (
	unknownFieldPattern = regexp.MustCompile(`can't evaluate field (\w+)`)
	unknownFuncPattern  = regexp.MustCompile(`function "(\w+)" not defined`)
)
