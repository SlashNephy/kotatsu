package filelib

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
)

// captionsName は、ライブラリのファイルの音声から自動生成した字幕の記録の名前。nekonote のジョブが書く。
const captionsName = "captions.json"

// CaptionsResult は、ライブラリのファイルの音声から自動生成した字幕 (captions.json)。
type CaptionsResult struct {
	// FileID は、字幕を作ったファイルの ID。ファイルを書き換えたら ID が変わるので、前のファイルの字幕は使わない。
	FileID string `json:"fileId"`
	// Model は、使ったモデルの名前。
	Model string `json:"model"`
	// GeneratedAt は、生成し終えた日時。
	GeneratedAt time.Time `json:"generatedAt"`
	// Cues は、字幕を始まりの順に並べたもの。台詞がなければ空。
	Cues []*CaptionCue `json:"cues"`
}

// CaptionCue は、自動生成した字幕の 1 つ。Start と End は、再生の 0 秒 (最初の I フレーム) からの時間。
type CaptionCue struct {
	Start time.Duration `json:"start"`
	End   time.Duration `json:"end"`
	Text  string        `json:"text"`
}

// Captions は、ref の保存先に、ファイルの ID が fileID のファイルから作った字幕があれば返す。
func (x *Indexer) Captions(ref Ref, fileID string) (*CaptionsResult, bool) {
	data, err := sharedfile.ReadFile(filepath.Join(x.dir(ref), captionsName))
	if err != nil {
		return nil, false
	}
	var result CaptionsResult
	if err := json.Unmarshal(data, &result); err != nil || result.FileID != fileID {
		return nil, false
	}

	return &result, true
}

// WriteCaptions は、ref の保存先に、自動生成した字幕を書く。前の字幕は置き換える。
func (x *Indexer) WriteCaptions(ref Ref, result *CaptionsResult) error {
	dir, err := x.ImageDir(ref)
	if err != nil {
		return err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to encode the captions: %w", err)
	}

	return WriteFile(dir, captionsName, data)
}
