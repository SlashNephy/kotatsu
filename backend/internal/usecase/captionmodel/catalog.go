// Package captionmodel は、字幕の自動生成 (usecase/autocaption) に使うモデルを、管理者が画面からダウンロードして置く (#1682、設計書 6.1)。
//
// 選べるモデルは、入手先の URL、大きさ、SHA-256 をコードに固定した一覧 (Catalog) に限る。画面からは一覧の ID だけを受け取り、URL は受け取らない。
// ダウンロードは、whisper.cpp を動かす nekonote の River のジョブで行い、データディレクトリの既定の場所 (autocaption.DefaultModelPath と
// autocaption.DefaultVADModelPath) に、.part に書いてから置き換える。置いたモデルは DB (caption_models) に記録する。
package captionmodel

import (
	"path/filepath"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
)

//go:generate go tool go-enum --values --nocomments

// Kind は、モデルの種類。種類ごとに、置き場所が 1 つある。
//
// ENUM(
// Whisper = WHISPER // 書き起こしのモデル (whisper-cli が読む)。
// VAD = VAD // 声のある区間を求めるモデル (whisper-vad-speech-segments が読む)。任意。
// )
type Kind string

// Model は、画面からダウンロードできるモデル。
type Model struct {
	// ID は、画面とジョブの引数と DB で使う、一覧の中の ID。
	ID   string
	Kind Kind
	// Name は、画面に出す名前。
	Name string
	// URL は、入手先。Hugging Face の resolve の URL に固定する。
	URL string
	// Size は、ファイルの大きさ (バイト)。
	Size int64
	// SHA256 は、ファイルの SHA-256 (16 進の小文字)。ダウンロードした後に照合する。
	SHA256 string
	// License は、モデルのライセンス (SPDX の識別子)。
	License string
	// Recommended は、その種類で勧めるモデルかどうか。
	Recommended bool
}

// licenseMIT は、whisper.cpp の配布するモデルと Silero VAD のライセンス。
const licenseMIT = "MIT"

// Catalog は、画面からダウンロードできるモデルの一覧を返す。URL、大きさ、SHA-256 は、2026 年 11 月にダウンロードして確認した。
// 勧めるものを、種類ごとに先に並べる。
func Catalog() []Model {
	return []Model{
		{
			// 日本語の放送で、誤りが少なく速い (#1682 の計測)。
			ID: "kotoba-whisper-v2.0-q5_0", Kind: KindWhisper, Name: "kotoba-whisper v2.0 (q5_0)",
			URL:  "https://huggingface.co/kotoba-tech/kotoba-whisper-v2.0-ggml/resolve/main/ggml-kotoba-whisper-v2.0-q5_0.bin",
			Size: 537819875, SHA256: "4a3b92192b5d3578ff854a5876213e2e27af0c2d357492c2d14271e82c303658",
			License: "Apache-2.0", Recommended: true,
		},
		{
			ID: "whisper-small-q5_1", Kind: KindWhisper, Name: "Whisper small (q5_1)",
			URL:  "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small-q5_1.bin",
			Size: 190085487, SHA256: "ae85e4a935d7a567bd102fe55afc16bb595bdb618e11b2fc7591bc08120411bb",
			License: licenseMIT, Recommended: false,
		},
		{
			ID: "whisper-large-v3-turbo-q5_0", Kind: KindWhisper, Name: "Whisper large-v3-turbo (q5_0)",
			URL:  "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo-q5_0.bin",
			Size: 574041195, SHA256: "394221709cd5ad1f40c46e6031ca61bce88931e6e088c188294c6d5a55ffa7e2",
			License: licenseMIT, Recommended: false,
		},
		{
			ID: "silero-v5.1.2", Kind: KindVAD, Name: "Silero VAD v5.1.2",
			URL:  "https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v5.1.2.bin",
			Size: 885098, SHA256: "29940d98d42b91fbd05ce489f3ecf7c72f0a42f027e4875919a28fb4c04ea2cf",
			License: licenseMIT, Recommended: true,
		},
	}
}

// Lookup は、一覧から ID が id のモデルを返す。なければ false を返す。
func Lookup(id string) (*Model, bool) {
	return find(Catalog(), id)
}

// find は、models から ID が id のモデルを返す。
func find(models []Model, id string) (*Model, bool) {
	for i := range models {
		if models[i].ID == id {
			return &models[i], true
		}
	}

	return nil, false
}

// relativePath は、種類 kind のモデルを置く、データディレクトリの中のパス (スラッシュ区切り)。
// 設定で場所を省いたときに autocaption が読む場所と同じにする。
func relativePath(kind Kind) string {
	if kind == KindVAD {
		return autocaption.DefaultVADModelPath
	}

	return autocaption.DefaultModelPath
}

// Path は、データディレクトリ dataDir の中の、種類 kind のモデルを置くパスを返す。
func Path(dataDir string, kind Kind) string {
	return filepath.Join(dataDir, filepath.FromSlash(relativePath(kind)))
}

// Overridden は、設定 cfg で、種類 kind のモデルの場所を書いているかを返す。書いていれば、autocaption はそちらを読み、
// 画面から置いたモデルは使わない。
func Overridden(cfg *config.Captions, kind Kind) bool {
	if kind == KindVAD {
		return cfg.VADModel != ""
	}

	return cfg.WhisperModel != ""
}
