package thumbnail

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// NewGenerateWorkerWith は、FFmpeg の代わりに newExtractor の Extractor を使い、プレビュー画像を作るかを preview に固定した GenerateWorker を返す。
func NewGenerateWorkerWith(db sqlc.DBTX, store *kago.Store, clk clock.Clock, preview bool, newExtractor func() (Extractor, error)) *GenerateWorker {
	return newGenerateWorker(db, store, clk, func(context.Context) (bool, error) { return preview, nil }, newExtractor)
}
