package reserve_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/reserve"
)

// TestHalfWidth は、画面の半角化 (web/src/futon/halfWidth.test.ts) と同じ例で、同じ結果になることを確かめる。
func TestHalfWidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "全角の英数字を半角にする", input: "ＮＨＫ総合１", want: "NHK総合1"},
		{name: "英数字に添える記号と、全角の空白も半角にする", input: "相棒１４　＃４", want: "相棒14 #4"},
		{name: "時刻や割合の記号", input: "１２：３０　５０％　Ａ＆Ｂ", want: "12:30 50% A&B"},
		{name: "和文の約物は全角のまま", input: "参戦！（２８）～？", want: "参戦！（28）～？"},
		{name: "半角の文字はそのまま", input: "NHK 1", want: "NHK 1"},
		{name: "仮名や漢字はそのまま", input: "ニュース・天気", want: "ニュース・天気"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, reserve.HalfWidth(tt.input))
		})
	}
}

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
