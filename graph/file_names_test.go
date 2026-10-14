package graph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fileNameHelpQuery    = `{ fileNameTemplateHelp { items { snippet description example } presets { name template example } } }`
	previewFileNameQuery = `query($template: String!, $program: ID) { previewFileName(template: $template, program: $program) { fileName error } }`
	varTemplate          = "template"
)

// TestFileNameTemplateHelp は、書き方の表に、見本の番組を当てはめた例を付けて返すことを確かめる。
func TestFileNameTemplateHelp(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	type item struct {
		Snippet string `json:"snippet"`
		Example string `json:"example"`
	}
	var got struct {
		FileNameTemplateHelp struct {
			Items   []item `json:"items"`
			Presets []struct {
				Name    string `json:"name"`
				Example string `json:"example"`
			} `json:"presets"`
		} `json:"fileNameTemplateHelp"`
	}
	require.Empty(t, f.query(t, fileNameHelpQuery, nil, &got))
	assert.Contains(t, got.FileNameTemplateHelp.Items, item{Snippet: `{{weekdayJa .StartAt}}`, Example: "月"})
	assert.Contains(t, got.FileNameTemplateHelp.Items, item{Snippet: `{{half .ServiceName}}`, Example: "KOTATSU TV1"})
	require.Len(t, got.FileNameTemplateHelp.Presets, 2)
	assert.Equal(t, "[26-10-12(月)_00-30][GR_KOTATSU TV1]こたつでみかんSP #2.m2ts", got.FileNameTemplateHelp.Presets[1].Example)
}

// TestPreviewFileName は、テンプレートに番組か見本を当てはめ、録画と同じ整え方をしたファイル名を返し、
// テンプレートの誤りは結果の error で返すことを確かめる。
func TestPreviewFileName(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	tests := []struct {
		name      string
		variables map[string]any
		wantName  *string
		wantError *string
		wantCodes []string
	}{
		{
			name: "見本の番組を当てはめ、パス区切りを取り除く", variables: map[string]any{varTemplate: `{{half .Name}}/{{.Channel}}`},
			wantName: new("こたつでみかんSP #216.m2ts"),
		},
		{
			name: "番組を当てはめる", variables: map[string]any{varTemplate: `{{.ChannelType}}_{{.Channel}}_{{.Name}}`, varProgram: programID(6)},
			wantName: new("GR_27_こたつテスト1 テスト番組 6.m2ts"),
		},
		{name: "知らない項目は誤り", variables: map[string]any{varTemplate: `{{.Title}}`}, wantError: new("知らない項目 .Title があります")},
		{name: "知らない関数は誤り", variables: map[string]any{varTemplate: `{{upper .Name}}`}, wantError: new("知らない関数 upper があります")},
		{name: "閉じていない括弧は誤り", variables: map[string]any{varTemplate: `{{.Name`}, wantError: new("1: unclosed action")},
		{name: "ない番組は NOT_FOUND", variables: map[string]any{varTemplate: `{{.Name}}`, varProgram: programID(9999)}, wantCodes: []string{codeNotFound}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got struct {
				PreviewFileName struct {
					FileName *string `json:"fileName"`
					Error    *string `json:"error"`
				} `json:"previewFileName"`
			}
			codes := f.query(t, previewFileNameQuery, tt.variables, &got)
			if tt.wantCodes != nil {
				assert.Equal(t, tt.wantCodes, codes)

				return
			}
			require.Empty(t, codes)
			assert.Equal(t, tt.wantName, got.PreviewFileName.FileName)
			assert.Equal(t, tt.wantError, got.PreviewFileName.Error)
		})
	}
}
