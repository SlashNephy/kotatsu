//go:build wireinject

package cli

import (
	"log/slog"

	"github.com/goforj/wire"

	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

// このファイルは、wire が読む組み立ての宣言 (#1607)。ビルドには入らず、wire が wire_gen.go に中身を生成する。
// 宣言を変えたら、mise run //backend:generate で生成し直す。

// newSealedStores は、アプリの秘密キーで暗号化した秘密情報を扱う部品を作る。
func newSealedStores(deps *Deps, logger *slog.Logger) (*sealedStores, error) {
	panic(wire.Build(
		clockOf,
		appSecretOf,
		audit.NewRecorder,
		wire.Bind(new(audit.Recorder), new(*audit.DBRecorder)),
		newWebhookStore,
		newWebPush,
		jobInserterOf,
		newLinkedAnimeRecords,
		wire.Struct(new(sealedStores), "*"),
	))
}
