package reserve_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/reserve"
)

func TestWeekdayJa(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "日曜日", at: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC), want: "日"},
		{name: "月曜日", at: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), want: "月"},
		{name: "土曜日", at: time.Date(2026, 10, 17, 23, 59, 0, 0, time.UTC), want: "土"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, reserve.WeekdayJa(tt.at))
		})
	}
}

// TestFileNameTemplatePresets は、よく使うテンプレートが、どれも解釈でき、EPGStation と同じものは EPGStation と同じ名前になることを確かめる。
func TestFileNameTemplatePresets(t *testing.T) {
	t.Parallel()

	jst := time.FixedZone("JST", 9*60*60)
	start := time.Date(2026, 10, 12, 0, 30, 0, 0, jst)
	data := &reserve.FileNameData{
		StartAt: start, EndAt: start.Add(30 * time.Minute), ServiceName: "ＴＯＫＹＯ　ＭＸ１", ChannelType: "GR", Channel: "16",
		Name: "番組名ＭＡＸ　＃２", EventID: 1,
	}
	want := map[string]string{
		"既定値":            "2026-10-12 0030 ＴＯＫＹＯ　ＭＸ１ 番組名ＭＡＸ　＃２.m2ts",
		"EPGStation と同じ": "[26-10-12(月)_00-30][GR_TOKYO MX1]番組名MAX #2.m2ts",
	}
	presets := reserve.FileNameTemplatePresets()
	require.Len(t, presets, len(want))
	for _, preset := range presets {
		t.Run(preset.Name, func(t *testing.T) {
			t.Parallel()

			tmpl, err := reserve.ParseFileNameTemplate(preset.Template)
			require.NoError(t, err)
			got, err := kago.RenderName(tmpl, data, ".m2ts")
			require.NoError(t, err)
			assert.Equal(t, want[preset.Name], got)
		})
	}
}

// TestFileNameTemplateItems は、書き方の表のどれもが、テンプレートとして解釈できることを確かめる。
func TestFileNameTemplateItems(t *testing.T) {
	t.Parallel()

	for _, item := range reserve.FileNameTemplateItems() {
		t.Run(item.Snippet, func(t *testing.T) {
			t.Parallel()

			_, err := reserve.ParseFileNameTemplate(item.Snippet)
			assert.NoError(t, err)
		})
	}
}

// TestParseFileNameTemplateRejects は、当てはめる時間や結果の長さを際限なく延ばせる書き方を受け付けないことを確かめる。
func TestParseFileNameTemplateRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
	}{
		{name: "繰り返し", text: `{{range 1000000}}x{{end}}`},
		{name: "条件の中の繰り返し", text: `{{if .Name}}{{range 10}}x{{end}}{{end}}`},
		{name: "テンプレートの定義と呼び出し", text: `{{define "a"}}x{{end}}{{template "a"}}`},
		{name: "長すぎる", text: strings.Repeat("x", 1025)},
		{name: "組み込みの関数", text: `{{printf "%01000000000d" 1}}`},
		{name: "変数", text: `{{$x := .Name}}{{$x}}`},
		{name: "知らない関数の呼び出し", text: `{{len .Name}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := reserve.ParseFileNameTemplate(tt.text)
			require.ErrorIs(t, err, reserve.ErrInvalidSettings)
		})
	}
	for _, text := range []string{
		`{{if .Name}}{{half .Name}}{{else}}untitled{{end}}`,
		`{{with .ServiceName}}{{half .}}{{end}}`,
		`{{.StartAt.Format "2006-01-02"}} {{weekdayJa .StartAt}}`,
	} {
		_, err := reserve.ParseFileNameTemplate(text)
		assert.NoError(t, err, text)
	}
}

// TestRenderFileName は、テンプレートに書いた / だけをディレクトリの区切りにし、値に入っている / は、
// half で半角になる全角のものも含めて取り除くことを確かめる。
func TestRenderFileName(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 12, 0, 30, 0, 0, time.UTC)
	data := &reserve.FileNameData{
		StartAt: start, EndAt: start.Add(time.Hour), ServiceName: "ＴＶ／１", ChannelType: "GR", Channel: "16", Name: "前編/後編＼完結", EventID: 1,
	}
	tmpl, err := reserve.ParseFileNameTemplate(`{{.ChannelType}}/{{half .ServiceName}}/{{half .Name}}`)
	require.NoError(t, err)
	got, err := reserve.RenderFileName(tmpl, data, ".m2ts")
	require.NoError(t, err)
	assert.Equal(t, "GR/TV1/前編後編完結.m2ts", got)
}

// TestRenderFileNameRemovesWindowsUnsafeCharacters は、half で半角になった : や * も、
// Windows で使えないので、ファイル名から取り除くことを確かめる (#881)。
func TestRenderFileNameRemovesWindowsUnsafeCharacters(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 12, 0, 30, 0, 0, time.UTC)
	data := &reserve.FileNameData{
		StartAt: start, EndAt: start.Add(time.Hour), ServiceName: "ＴＶ１", ChannelType: "GR", Channel: "16", Name: "第１話：前編＊", EventID: 1,
	}
	tmpl, err := reserve.ParseFileNameTemplate(`{{half .Name}}`)
	require.NoError(t, err)
	got, err := reserve.RenderFileName(tmpl, data, ".m2ts")
	require.NoError(t, err)
	assert.Equal(t, "第1話前編.m2ts", got)
}
