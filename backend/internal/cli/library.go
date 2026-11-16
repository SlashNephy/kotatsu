package cli

import (
	"log/slog"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
)

// libraryParts は、ライブラリ (録画として登録していない TS のフォルダ) を読む部品 (設計書 5.8、#1734)。
type libraryParts struct {
	libraries *medialib.Set
	indexer   *filelib.Indexer
	source    *filelib.Source
}

// newLibraryParts は、設定の保存先プールとライブラリから、ライブラリを読む部品を返す。build が true なら (tenban)、
// 開いたファイルの索引を、データディレクトリに作る。kyusu は読むだけにする。
func newLibraryParts(deps *Deps, logger *slog.Logger, build bool) *libraryParts {
	storage := &deps.Config.Storage
	libraries := medialib.NewSet(storage.Pools, storage.PoolOrder, storage.Libraries)
	indexer := filelib.NewIndexer(storage.DataDir, libraries, deps.Clock, logger.With(slog.String("component", "library_index")), build)

	return &libraryParts{libraries: libraries, indexer: indexer, source: filelib.NewSource(libraries, indexer)}
}

// libraryJobsOf は、ライブラリのファイルの画像、CM の区間、字幕の自動生成のジョブを入れる River のクライアントを返す。なければ nil を返す。
// 型付きの nil を interface に入れると nil と比べられなくなるので、nil なら interface の nil を返す。
func libraryJobsOf(deps *Deps) filelib.JobInserter {
	if deps.Jobs == nil {
		return nil
	}

	return deps.Jobs
}

// newLibraryCaptionsWorker は、ライブラリのファイルの音声から字幕を自動生成する Worker を作る (設計書 5.8)。録画と同じく、whisper-cli とモデルは
// ジョブごとに探し、取り出した音声は kyusu と同じ一時領域に置く。
func newLibraryCaptionsWorker(deps *Deps, parts *libraryParts, lowerer *procprio.Lowerer) *filelib.CaptionsWorker {
	return filelib.NewCaptionsWorker(parts.libraries, parts.indexer, deps.Clock,
		func() (autocaption.Transcriber, error) {
			return autocaption.NewWhisper(&deps.Config.Captions, deps.Config.Storage.DataDir, lowerer)
		},
		func() (autocaption.ReaderExtractor, error) { return autocaption.FFmpegReaderExtractor(lowerer) },
		deps.Config.Kyusu.WorkDir)
}
