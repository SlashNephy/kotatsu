//go:build wireinject

package cli

import (
	"log/slog"

	"github.com/goforj/wire"

	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
	"github.com/SlashNephy/kotatsu/internal/web/streamapi"
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

// newKyusuParts は、kyusu のライブ視聴と録画の仮想 HLS のセッションと、tenban からのリクエストを確かめるトークンを作る。
// トークンと、構成によってないことがあるストリームを開く先とトランスコーダーは、newKyusu が先に決めて渡す。
func newKyusuParts(
	deps *Deps, logger *slog.Logger, token *kyusu.Token, opener kyusu.ServiceOpener, transcoder *kyusu.Transcoder,
) (*kyusuParts, error) {
	panic(wire.Build(
		kyusuLoggerOf,
		newKyusuLive,
		newKyusuVOD,
		wire.Struct(new(kyusuParts), "*"),
	))
}

// newAccountParts は、ユーザーのアカウントとログインを扱う部品を作る。アプリの秘密キーがあるかは、newAPI が先に確かめる。
func newAccountParts(deps *Deps, logger *slog.Logger) (*accountParts, error) {
	panic(wire.Build(
		clockOf,
		appSecretOf,
		poolOf,
		audit.NewRecorder,
		wire.Bind(new(audit.Recorder), new(*audit.DBRecorder)),
		newSessions,
		newSecondFactors,
		newPasskeys,
		newAccountPasswordPolicy,
		auth.NewUserAdmin,
		auth.NewPasswords,
		auth.NewAPITokens,
		wire.Struct(new(accountParts), "*"),
	))
}

// newAccessParts は、セッションの Cookie 以外で認可するのに使う部品を作る。アプリの秘密キーがあるかは、newAPI が先に確かめる。
func newAccessParts(deps *Deps, logger *slog.Logger) (*accessParts, error) {
	panic(wire.Build(
		clockOf,
		appSecretOf,
		poolOf,
		networkOf,
		audit.NewRecorder,
		wire.Bind(new(audit.Recorder), new(*audit.DBRecorder)),
		newURLUsers,
		newStreamSigner,
		newCalendarFeeds,
		newIPTVFeeds,
		newAuthorizationServer,
		wire.Struct(new(accessParts), "*"),
	))
}

// newJikkyoParts は、実況のコメントを扱う部品を作る。
func newJikkyoParts(deps *Deps) (*jikkyoParts, error) {
	panic(wire.Build(
		clockOf,
		networkOf,
		newJikkyoTable,
		newJikkyoClient,
		wire.Struct(new(jikkyoParts), "*"),
	))
}

// newStreamParts は、変換せずに流している配信を数える部品と、止める前につなぎ直すのを待つ部品を作る。
func newStreamParts(deps *Deps) *streamParts {
	panic(wire.Build(
		newStreamCounter,
		streamapi.NewDrainer,
		wire.Struct(new(streamParts), "*"),
	))
}
