package filelib

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
)

// cmName は、ライブラリのファイルの CM の区間の記録の名前。nekonote のジョブが書くので、tenban が書く source.json と分ける。
const cmName = "cm.json"

// CMResult は、ライブラリのファイルの CM の区間を見分けた結果 (cm.json)。
type CMResult struct {
	// FileID は、見分けたファイルの ID。ファイルを書き換えたら ID が変わるので、前のファイルの結果は使わない。
	FileID string `json:"fileId"`
	// Detector と Version は、見分け方の名前とバージョン。
	Detector string `json:"detector"`
	Version  int32  `json:"version"`
	// Sections は、CM とみなした区間を、始まりの順に並べたもの。見つからなければ空。
	Sections []*CMSection `json:"sections"`
	// Note は、いつもの見分け方で決められなかった理由。決められたら空。
	Note string `json:"note"`
}

// CMSection は、CM とみなした区間。Start と End は、再生の 0 秒 (インデックスの最初の I フレーム) からの時間。
type CMSection struct {
	Start time.Duration `json:"start"`
	End   time.Duration `json:"end"`
}

// CM は、ref の保存先に、ファイルの ID が fileID のファイルを見分けた結果があれば返す。
func (x *Indexer) CM(ref Ref, fileID string) (*CMResult, bool) {
	data, err := sharedfile.ReadFile(filepath.Join(x.dir(ref), cmName))
	if err != nil {
		return nil, false
	}
	var result CMResult
	if err := json.Unmarshal(data, &result); err != nil || result.FileID != fileID {
		return nil, false
	}

	return &result, true
}

// WriteCM は、ref の保存先に、見分けた結果を書く。
func (x *Indexer) WriteCM(ref Ref, result *CMResult) error {
	dir, err := x.ImageDir(ref)
	if err != nil {
		return err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to encode the CM sections: %w", err)
	}

	return WriteFile(dir, cmName, data)
}
