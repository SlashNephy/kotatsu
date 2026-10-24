package thumbnail

import (
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/kago"
)

// NewGenerateWorkerWith は、FFmpeg の代わりに newExtractor の Extractor を使う GenerateWorker を返す。
func NewGenerateWorkerWith(db sqlc.DBTX, store *kago.Store, clk clock.Clock, preview bool, newExtractor func() (Extractor, error)) *GenerateWorker {
	return newGenerateWorker(db, store, clk, preview, newExtractor)
}
