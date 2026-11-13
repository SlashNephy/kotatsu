// Package licenses は、本体に組み込んだ Go のモジュールのライセンスの一覧を扱う (設計書 10.3、付録 B、#1604)。
//
// 一覧は、ビルドの前に printlicenses で dist/go.json に書き出し、本体に埋め込む。
// 依存を上げるたびに変わるので、リポジトリには置かない。書き出さずにビルドしても go build が通るよう、
// dist には空の .gitkeep だけを置いてリポジトリに残す。そのビルドでは、一覧がないことを返す。
package licenses

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"runtime/debug"
	"slices"
)

// FileName は、埋め込む一覧のファイルの名前。
const FileName = "go.json"

// StandardLibraryPath は、Go の標準ライブラリを表すモジュールのパス。
const StandardLibraryPath = "std"

//go:embed all:dist
var dist embed.FS

// File は、ライセンスの文書のファイル 1 つ。
type File struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// Module は、本体に組み込んだ Go のモジュール 1 つと、そのライセンスの文書。
type Module struct {
	Path    string  `json:"path"`
	Version string  `json:"version"`
	Files   []*File `json:"files"`
}

// List は、本体に組み込んだ Go のモジュールの一覧。
type List struct {
	Modules []*Module `json:"modules"`
}

// ErrNotGenerated は、一覧を書き出さずにビルドしたことを表す。
var ErrNotGenerated = errors.New("the license list was not generated for this build")

// Load は、埋め込んだ一覧を読み、動いているバイナリに組み込まれたモジュールだけを返す。
// 一覧を書き出さずにビルドしたときは、ErrNotGenerated を返す。
// 開くのは「このアプリについて」の画面だけなので、覚えずに毎回読む。
func Load() (*List, error) {
	return load(dist, deps())
}

// load は、fsys の dist/go.json を読み、linked に含まれるモジュールと標準ライブラリだけを返す。
// linked が nil なら、絞らずに返す。
// 一覧は、対象のすべての OS で組み込むモジュールを合わせて書き出すので、ここで、このバイナリのものに絞る。
func load(fsys fs.FS, linked map[string]bool) (*List, error) {
	raw, err := fs.ReadFile(fsys, "dist/"+FileName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotGenerated
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the license list: %w", err)
	}
	var list List
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("failed to parse the license list: %w", err)
	}
	if linked != nil {
		list.Modules = slices.DeleteFunc(list.Modules, func(m *Module) bool {
			return m.Path != StandardLibraryPath && !linked[m.Path]
		})
	}

	return &list, nil
}

// deps は、動いているバイナリに組み込まれたモジュールのパスを返す。分からなければ nil を返す。
func deps() map[string]bool {
	info, ok := debug.ReadBuildInfo()
	if !ok || len(info.Deps) == 0 {
		return nil
	}
	linked := make(map[string]bool, len(info.Deps))
	for _, dep := range info.Deps {
		linked[dep.Path] = true
	}

	return linked
}
