package reserve

import (
	"strings"
	"text/template"
	"time"
)

// fileNameFuncs は、ファイル名のテンプレートで使える関数 (設計書 4.7)。Go のテンプレートを書かない利用者向けに、
// 画面では FileNameTemplateItems の表で書き方を示す。
func fileNameFuncs() template.FuncMap {
	return template.FuncMap{
		"weekdayJa": WeekdayJa,
		"half":      HalfWidth,
	}
}

// WeekdayJa は、t の曜日を漢字 1 文字 (日〜土) で返す。
func WeekdayJa(t time.Time) string {
	return []string{"日", "月", "火", "水", "木", "金", "土"}[t.Weekday()]
}

// HalfWidth は、全角の英数字と、英数字に添えて使う記号と、全角の空白を半角にする。
// 画面の表示で使う半角化 (web/src/futon/halfWidth.ts) と同じ範囲にする。「！」「？」「（）」「～」は、和文の約物として使うので残す。
func HalfWidth(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '　':
			return ' '
		case r >= 'Ａ' && r <= 'Ｚ', r >= 'ａ' && r <= 'ｚ', r >= '０' && r <= '９', strings.ContainsRune("＃＄％＆＊＋－．／：；＝＠＿", r):
			return r - 0xfee0
		default:
			return r
		}
	}, s)
}

// SampleFileNameData は、テンプレートを確かめ、画面で書き方の例を示すのに使う、見本の番組。
func SampleFileNameData() *FileNameData {
	start := time.Date(2026, 10, 12, 0, 30, 0, 0, time.FixedZone("JST", 9*60*60))

	return &FileNameData{
		StartAt:     start,
		EndAt:       start.Add(30 * time.Minute),
		ServiceName: "ＫＯＴＡＴＳＵ　ＴＶ１",
		ChannelType: "GR",
		Channel:     "16",
		Name:        "こたつでみかんＳＰ　＃２",
		EventID:     1234,
	}
}

// FileNameTemplateItem は、ファイル名のテンプレートの書き方の 1 つ。
type FileNameTemplateItem struct {
	// Snippet は、テンプレートに書くもの。
	Snippet string
	// Description は、何に置き換わるか。
	Description string
}

// FileNameTemplateItems は、画面で表にして示す、ファイル名のテンプレートの書き方。
// テンプレートの項目や関数を足したら、ここにも足す。
func FileNameTemplateItems() []FileNameTemplateItem {
	return []FileNameTemplateItem{
		{Snippet: `{{.Name}}`, Description: "番組名"},
		{Snippet: `{{half .Name}}`, Description: "番組名 (英数字と空白を半角にする)"},
		{Snippet: `{{.ServiceName}}`, Description: "サービス名"},
		{Snippet: `{{half .ServiceName}}`, Description: "サービス名 (英数字と空白を半角にする)"},
		{Snippet: `{{.ChannelType}}`, Description: "放送波 (GR、BS、CS など)"},
		{Snippet: `{{.Channel}}`, Description: "チャンネル"},
		{Snippet: `{{.EventID}}`, Description: "番組の eventId (時刻予約では 0)"},
		{Snippet: `{{.StartAt.Format "2006"}}`, Description: "開始の年 (4 桁)"},
		{Snippet: `{{.StartAt.Format "06"}}`, Description: "開始の年 (2 桁)"},
		{Snippet: `{{.StartAt.Format "01"}}`, Description: "開始の月 (2 桁)"},
		{Snippet: `{{.StartAt.Format "02"}}`, Description: "開始の日 (2 桁)"},
		{Snippet: `{{weekdayJa .StartAt}}`, Description: "開始の曜日"},
		{Snippet: `{{.StartAt.Format "15"}}`, Description: "開始の時 (24 時間制、2 桁)"},
		{Snippet: `{{.StartAt.Format "04"}}`, Description: "開始の分 (2 桁)"},
		{Snippet: `{{.EndAt.Format "1504"}}`, Description: "終了の時分 (EndAt も StartAt と同じ書き方)"},
	}
}

// FileNameTemplatePreset は、よく使うファイル名のテンプレート。
type FileNameTemplatePreset struct {
	Name     string
	Template string
}

// FileNameTemplatePresets は、画面で選べる、よく使うファイル名のテンプレート。
func FileNameTemplatePresets() []FileNameTemplatePreset {
	return []FileNameTemplatePreset{
		{Name: "既定値", Template: DefaultFileNameTemplate},
		{
			Name:     "EPGStation と同じ",
			Template: `[{{.StartAt.Format "06-01-02"}}({{weekdayJa .StartAt}})_{{.StartAt.Format "15-04"}}][{{.ChannelType}}_{{half .ServiceName}}]{{half .Name}}`,
		},
	}
}
