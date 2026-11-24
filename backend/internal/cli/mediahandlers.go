package cli

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/capture"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/usecase/thumbnail"
	"github.com/SlashNephy/kotatsu/internal/usecase/tvtplay"
	"github.com/SlashNephy/kotatsu/internal/web/animeimage"
	"github.com/SlashNephy/kotatsu/internal/web/avatar"
	"github.com/SlashNephy/kotatsu/internal/web/captureapi"
	"github.com/SlashNephy/kotatsu/internal/web/databroadcastapi"
)

// mediaHandlers は、画像とファイル (/internal/media/*) のハンドラーのうち、Deps と互いだけで組めるもの (#1607)。
// 認可で包むのと、ルートへの登録は、newMediaRoutes で手で行う。
type mediaHandlers struct {
	avatars       *avatar.Handler
	animeImages   *animeimage.Handler
	thumbnails    *thumbnail.ImageHandler
	tvtplay       *tvtplay.ChapterHandler
	dataBroadcast *databroadcastapi.Relay
	captureUpload *captureapi.UploadHandler
	captureImage  *captureapi.ImageHandler
}

// captureLogger は、キャプチャの部品のロガー。wire は型で依存を見分けるので、ほかのロガーと分ける。
type captureLogger struct {
	*slog.Logger
}

func captureLoggerOf(logger *slog.Logger) captureLogger {
	return captureLogger{Logger: logger.With(slog.String("component", "capture"))}
}

// newAvatarHandler は、ユーザーのアイコンを IdP から取ってプロキシするハンドラーを作る (#778)。
func newAvatarHandler(deps *Deps, pool *pgxpool.Pool, logger *slog.Logger) *avatar.Handler {
	return avatar.NewHandler(pool, deps.Config.Auth.OIDC.Issuer, logger.With(slog.String("component", "avatar")), deps.Network)
}

// newAnimeImageHandler は、見たいリストの作品の画像と、フォロー中のユーザーのアバターを、記録先が示した URL から取って
// プロキシするハンドラーを作る (#1235、#1372)。
func newAnimeImageHandler(deps *Deps, pool *pgxpool.Pool, logger *slog.Logger) *animeimage.Handler {
	return animeimage.NewHandler(pool, logger.With(slog.String("component", "anime_image")), deps.Network)
}

// newThumbnailImageHandler は、録画の代表画像とプレビュー画像を配信するハンドラーを作る。
// このインスタンスにマウントしたプールのものだけ配信できる。
func newThumbnailImageHandler(pool *pgxpool.Pool, store *kago.Store, logger *slog.Logger) *thumbnail.ImageHandler {
	return thumbnail.NewImageHandler(pool, store, logger.With(slog.String("component", "stream")))
}

// newTvtplayChapterHandler は、録画をダウンロードした TS と組で使う、TvtPlay の CM のチャプターのファイルを返すハンドラーを作る (#1737)。
func newTvtplayChapterHandler(pool *pgxpool.Pool, urlGate *settings.URLGate, logger *slog.Logger) *tvtplay.ChapterHandler {
	return tvtplay.NewChapterHandler(pool, urlGate, logger.With(slog.String("component", "tvtplay")))
}

// newDataBroadcastRelay は、データ放送の文書の双方向の通信をプロキシする部品を作る (#501、設計書 7.4)。
func newDataBroadcastRelay(deps *Deps, pool *pgxpool.Pool, clk clock.Clock, logger *slog.Logger) *databroadcastapi.Relay {
	return databroadcastapi.New(pool, clk, logger.With(slog.String("component", "data_broadcast")), deps.Network)
}

// newCaptures は、キャプチャをデータディレクトリに置く部品を作る (設計書 5.6、7.5)。
func newCaptures(store *kago.Store, clk clock.Clock, logger captureLogger) *capture.Captures {
	return capture.New(store, clk, logger.Logger, 0)
}

// newCaptureUploadHandler は、キャプチャを受け取るハンドラーを作る。
func newCaptureUploadHandler(pool *pgxpool.Pool, captures *capture.Captures, logger captureLogger) *captureapi.UploadHandler {
	return captureapi.NewUploadHandler(pool, captures, logger.Logger)
}

// newCaptureImageHandler は、キャプチャの画像を返すハンドラーを作る。
func newCaptureImageHandler(pool *pgxpool.Pool, store *kago.Store, logger captureLogger) *captureapi.ImageHandler {
	return captureapi.NewImageHandler(pool, store, logger.Logger)
}
