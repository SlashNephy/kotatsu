package cli

import (
	"log/slog"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/webhook"
	"github.com/SlashNephy/kotatsu/internal/usecase/webpush"
)

// sealedStores は、アプリの秘密キーで暗号化した秘密情報 (設計書 8.4) を扱う部品。
type sealedStores struct {
	webhooks     *webhook.Store
	webPush      *webpush.Service
	animeRecords *animerecord.Service
}

// appSecret は、アプリの秘密キー (auth.secret_key)。wire は型で依存を見分けるので、ほかの文字列と分ける。
type appSecret string

func clockOf(deps *Deps) clock.Clock {
	return deps.Clock
}

func appSecretOf(deps *Deps) appSecret {
	return appSecret(deps.Config.Auth.SecretKey.Value())
}

func newWebhookStore(clk clock.Clock, recorder audit.Recorder, secret appSecret) (*webhook.Store, error) {
	return webhook.NewStore(clk, recorder, string(secret))
}

// newWebPush は、Web Push の部品を作る。VAPID の鍵は、ブラウザが購読するために公開鍵を求めたときか、初めて通知を送るときに作る。
func newWebPush(clk clock.Clock, secret appSecret) (*webpush.Service, error) {
	return webpush.NewService(clk, string(secret))
}

// jobInserterOf は、ジョブを入れる先を返す。ジョブを入れられないプロセスでは nil を返す。
// 型付きの nil を interface に入れると nil と比べられなくなるので、Jobs が nil なら interface の nil を返す。
func jobInserterOf(deps *Deps) anime.Inserter {
	if deps.Jobs == nil {
		return nil
	}

	return deps.Jobs
}

// newLinkedAnimeRecords は、Annict と AniList との連携を扱う部品を作る。
// 連携した後の見たいリストの取り込み (#928) は、ジョブを入れられるときだけ始める。
func newLinkedAnimeRecords(deps *Deps, logger *slog.Logger, inserter anime.Inserter) (*animerecord.Service, error) {
	return newAnimeRecords(deps, logger, inserter, netactivity.PurposeAnimeLink)
}
