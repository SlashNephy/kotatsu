// Package drcstable は、字幕の外字 (DRCS) を文字に置き換える既定のテーブルを扱う (設計書 7.4、#825、#2216)。
//
// テーブルは、frontend/scripts/fetch-libaribcaption-drcs.mjs が、frontend/vendor/libaribcaption/SOURCE で固定した commit の
// libaribcaption の定義から、フロントエンドのテーブルと一緒に dist/drcs.json に作り、本体に埋め込む。リポジトリには置かない。
// 作らずにビルドしても go build が通るよう、dist には空の .gitkeep だけを置いてリポジトリに残す。そのビルドでは、
// テーブルがないことを返す。
package drcstable

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
)

// FileName は、埋め込むテーブルのファイルの名前。
const FileName = "drcs.json"

//go:embed all:dist
var dist embed.FS

// ErrNotGenerated は、テーブルを作らずにビルドしたことを表す。
var ErrNotGenerated = errors.New("the default DRCS table was not generated for this build")

// table は、dist/drcs.json の形。frontend/scripts/fetch-libaribcaption-drcs.mjs が書く。
type table struct {
	// Commit は、テーブルを作った libaribcaption の commit。
	Commit string `json:"commit"`
	// Replacements は、外字の画像の MD5 (小文字の 16 進) から、置き換える文字へのマップ。
	Replacements map[string]string `json:"replacements"`
}

// Default は、埋め込んだ既定のテーブルを読んで返す。返すマップは呼び出し側で変えてよい。
// テーブルを作らずにビルドしたときは、ErrNotGenerated を返す。
// テーブルは十数 KB で、使うのは録画ごとの処理なので、キャッシュせずに毎回読む。
func Default() (map[string]string, error) {
	return load(dist)
}

// load は、fsys の dist/drcs.json を読む。
func load(fsys fs.FS) (map[string]string, error) {
	raw, err := fs.ReadFile(fsys, "dist/"+FileName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotGenerated
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the default DRCS table: %w", err)
	}
	var decoded table
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("failed to decode the default DRCS table: %w", err)
	}
	// 作るスクリプトは、上流の書き方が変わって空に近くなったテーブルを書かない。それでも空なら、壊れたものとして扱う。
	if decoded.Commit == "" || len(decoded.Replacements) == 0 {
		return nil, errors.New("the default DRCS table is empty")
	}

	return decoded.Replacements, nil
}

// Merge は、既定のテーブル (defaults) に、管理者の登録 (registered) を重ねたテーブルを返す。登録を優先する。
// 字幕の履歴 (frontend/src/yunomi/drcs.ts の setDRCSReplacements) と同じ重ね方にする。
// 登録のキーは、保存するときに小文字にそろえてある (usecase/settings)。
func Merge(defaults, registered map[string]string) map[string]string {
	merged := make(map[string]string, len(defaults)+len(registered))
	maps.Copy(merged, defaults)
	maps.Copy(merged, registered)

	return merged
}
