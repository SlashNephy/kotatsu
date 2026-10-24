package reserve

import (
	"errors"
	"io"
	"strings"
	"text/template"
	"text/template/parse"
	"time"

	"github.com/SlashNephy/kotatsu/internal/halfwidth"
	"github.com/SlashNephy/kotatsu/internal/kago"
)

// fileNameFuncs は、ファイル名のテンプレートで使える関数 (設計書 4.7)。Go のテンプレートを書かない利用者向けに、
// 画面では FileNameTemplateItems の表で書き方を示す。
func fileNameFuncs() template.FuncMap {
	return template.FuncMap{
		"weekdayJa": WeekdayJa,
		"half":      halfwidth.Convert,
	}
}

// WeekdayJa は、t の曜日を漢字 1 文字 (日〜土) で返す。
func WeekdayJa(t time.Time) string {
	return []string{"日", "月", "火", "水", "木", "金", "土"}[t.Weekday()]
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

// errDisallowedSyntax は、ファイル名のテンプレートで使えない書き方を使ったことを表す。
var errDisallowedSyntax = errors.New("only fields, if, with and the functions half and weekdayJa are allowed")

// maxFileNameOutputB は、テンプレートを当てはめた結果の長さの上限。ファイル名は 255 バイトまでで、ディレクトリも数段しか掘らない。
const maxFileNameOutputB = 4096

// checkFileNameNodes は、テンプレートが許した書き方だけを使っているかを確かめる (許可リスト)。
// 使えるのは、文字列、項目 (.Name や .StartAt.Format "..." のようなメソッドの呼び出し)、if と with、
// 関数 half と weekdayJa だけにする。printf などの組み込みの関数や、繰り返し、変数、テンプレートの呼び出しは、
// 当てはめる時間や確保するメモリを際限なく延ばせるので受け付けない。
func checkFileNameNodes(node parse.Node) error {
	children, ok := allowedChildren(node)
	if !ok {
		return errDisallowedSyntax
	}
	for _, child := range children {
		if err := checkFileNameNodes(child); err != nil {
			return err
		}
	}

	return nil
}

// allowedChildren は、node が許した書き方なら、その中の確かめるべきノードを返す。許していなければ false を返す。
func allowedChildren(node parse.Node) ([]parse.Node, bool) {
	switch n := node.(type) {
	case *parse.TextNode, *parse.FieldNode, *parse.DotNode, *parse.StringNode, *parse.NumberNode, *parse.BoolNode, *parse.NilNode:
		return nil, true
	case *parse.ListNode:
		return listNodes(n), true
	case *parse.ActionNode:
		return []parse.Node{n.Pipe}, true
	case *parse.IfNode:
		return branchNodes(&n.BranchNode), true
	case *parse.WithNode:
		return branchNodes(&n.BranchNode), true
	case *parse.PipeNode:
		return pipeNodes(n)
	case *parse.CommandNode:
		return n.Args, true
	case *parse.ChainNode:
		return []parse.Node{n.Node}, true
	case *parse.IdentifierNode:
		_, ok := fileNameFuncs()[n.Ident]

		return nil, ok
	default:
		return nil, false
	}
}

func listNodes(n *parse.ListNode) []parse.Node {
	if n == nil {
		return nil
	}

	return n.Nodes
}

func branchNodes(n *parse.BranchNode) []parse.Node {
	return append([]parse.Node{n.Pipe}, append(listNodes(n.List), listNodes(n.ElseList)...)...)
}

// pipeNodes は、パイプラインのコマンドを返す。変数の宣言と代入 ($x := ...) は許さない。
func pipeNodes(n *parse.PipeNode) ([]parse.Node, bool) {
	if n == nil {
		return nil, true
	}
	if len(n.Decl) > 0 {
		return nil, false
	}
	nodes := make([]parse.Node, 0, len(n.Cmds))
	for _, cmd := range n.Cmds {
		nodes = append(nodes, cmd)
	}

	return nodes, true
}

// limitedWriter は、maxFileNameOutputB を超えて書こうとしたら、書くのをやめてエラーにする。
type limitedWriter struct {
	w       io.Writer
	written int
}

var _ io.Writer = new(limitedWriter)

// errFileNameTooLong は、テンプレートを当てはめた結果が長すぎることを表す。
var errFileNameTooLong = errors.New("the rendered file name is too long")

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.written+len(p) > maxFileNameOutputB {
		return 0, errFileNameTooLong
	}
	l.written += len(p)

	return l.w.Write(p)
}

// ExecuteFileNameTemplate は、テンプレートに data を当てはめ、結果を w に書く。結果は maxFileNameOutputB までにする。
func ExecuteFileNameTemplate(tmpl *template.Template, w io.Writer, data any) error {
	return tmpl.Execute(&limitedWriter{w: w, written: 0}, data)
}

// RenderFileName は、テンプレートに data を当てはめた、録画ファイルのプール内の相対パスを返す (kago.RenderName)。
// ディレクトリの区切りにするのは、テンプレートに書いた / だけにする。番組名などの値に入っている / と \ は、half で半角になる
// 全角のものも含めて取り除き、
// 「前編/後編」のような番組名でディレクトリを掘らないようにする。
func RenderFileName(tmpl *template.Template, data *FileNameData, ext string) (string, error) {
	clean := *data
	for _, field := range []*string{&clean.ServiceName, &clean.ChannelType, &clean.Channel, &clean.Name} {
		*field = strings.NewReplacer("/", "", "\\", "", "／", "", "＼", "").Replace(*field)
	}

	return kago.RenderName(tmpl, &clean, ext)
}
