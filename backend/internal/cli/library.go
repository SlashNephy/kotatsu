package cli

import (
	"log/slog"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
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
