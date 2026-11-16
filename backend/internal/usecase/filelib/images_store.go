package filelib

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
)

const (
	// ThumbnailFileName、PreviewFileName、ManifestFileName は、ライブラリのファイルの代表画像、シークバー用のプレビュー画像と、その説明の名前。
	// 録画の画像 (thumbnail パッケージ) と同じ名前にし、索引の置き場に置く。
	ThumbnailFileName = "thumbnail.jpg"
	PreviewFileName   = "preview.jpg"
	ManifestFileName  = "preview.json"
	// imagesName は、置き場の画像が、どのファイル (大きさと更新日時で決まる FileID) から作ったかの記録の名前。
	// 画像は nekonote のジョブが作るので、tenban が書く source.json と分ける。
	imagesName = "images.json"
)

// images は、置き場の画像の記録 (images.json)。
type images struct {
	FileID string `json:"fileId"`
}

// ImageDir は、ref の画像を置くディレクトリ (索引の置き場) のパスを返す。なければ作る。
func (x *Indexer) ImageDir(ref Ref) (string, error) {
	dir := x.dir(ref)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create the image directory: %w", err)
	}

	return dir, nil
}

// HasImages は、ref の置き場に、ファイルの ID が fileID のファイルから作った画像があるかを返す。
// ファイルを書き換えたら ID が変わるので、前のファイルの画像は使わない。
func (x *Indexer) HasImages(ref Ref, fileID string) bool {
	data, err := sharedfile.ReadFile(filepath.Join(x.dir(ref), imagesName))
	if err != nil {
		return false
	}
	var record images
	if err := json.Unmarshal(data, &record); err != nil {
		return false
	}

	return record.FileID == fileID
}

// MarkImages は、ref の置き場の画像を、ファイルの ID が fileID のファイルから作ったと記録する。画像を書き終えてから呼ぶ。
func (x *Indexer) MarkImages(ref Ref, fileID string) error {
	data, err := json.Marshal(&images{FileID: fileID})
	if err != nil {
		return fmt.Errorf("failed to encode the image record: %w", err)
	}

	return WriteFile(x.dir(ref), imagesName, data)
}

// ImagePath は、ref の置き場の、name の画像のパスを返す。name は ThumbnailFileName、PreviewFileName、ManifestFileName のどれか。
func (x *Indexer) ImagePath(ref Ref, name string) string {
	return filepath.Join(x.dir(ref), name)
}
