package cli

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
	"github.com/SlashNephy/kotatsu/internal/usecase/analysis"
	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
	"github.com/SlashNephy/kotatsu/internal/usecase/captionmodel"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/segjoin"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/usecase/thumbnail"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
	"github.com/SlashNephy/kotatsu/internal/usecase/tvtplay"
)

// mediaJobParts は、録画の後処理のジョブの Worker (#1607)。代表画像、解析、セグメントのつなぎ、CM の区間の見分け、字幕の自動生成とそのモデルのダウンロード、
// 変換、チャプター。
type mediaJobParts struct {
	thumbnails     *thumbnail.GenerateWorker
	liveThumbnails *thumbnail.LiveWorker
	analyze        *analysis.AnalyzeWorker
	join           *segjoin.JoinWorker
	finalize       *segjoin.FinalizeWorker
	cmDetect       *cmdetect.DetectWorker
	captions       *autocaption.Worker
	captionModels  *captionmodel.Worker
	transcode      *transcode.Worker
	tvtplay        *tvtplay.WriteWorker
	chapters       *transcode.ChaptersWorker
}

// mediaStoreOf は、録画のファイルを置く保存先プールを返す。設定だけを持つので、後処理のジョブで 1 つを共有する。
func mediaStoreOf(deps *Deps) *kago.Store {
	return storeOf(deps.Config)
}

// newLowerer は、録画の後処理の FFmpeg を、OS の最低の優先度で動かす部品を作る (#1511)。ライブ視聴の変換 (kyusu) に CPU を譲る。
// CM の区間の見分け、変換、チャプターの書き込みで共有する。
func newLowerer(logger *slog.Logger) *procprio.Lowerer {
	return procprio.New(logger.With(slog.String("component", "procprio")))
}

// newThumbnailWorker は、録画を終えたら、代表画像とプレビュー画像を作る Worker を作る (設計書 6.1)。
// FFmpeg がなければ、ジョブが理由を残して取りやめる。プレビュー画像を作るかは、管理者が画面で切り替える (#1869) ので、ジョブごとに読む。
func newThumbnailWorker(pool *pgxpool.Pool, store *kago.Store, clk clock.Clock) *thumbnail.GenerateWorker {
	return thumbnail.NewGenerateWorker(pool, store, clk, previewsEnabled(pool))
}

// previewsEnabled は、プレビュー画像を作るかどうかを、呼ぶたびに設定から読む関数を返す。
func previewsEnabled(pool *pgxpool.Pool) func(ctx context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		system, err := settings.Load(ctx, pool)
		if err != nil {
			return false, err
		}

		return system.ThumbnailPreviewsEnabled, nil
	}
}

// newLiveThumbnailWorker は、録画中の録画の代表画像を、一定の間隔で作り直す Worker を作る (#1236)。
func newLiveThumbnailWorker(pool *pgxpool.Pool, inserter *jobs.Client, clk clock.Clock) *thumbnail.LiveWorker {
	return thumbnail.NewLiveWorker(pool, inserter, clk)
}

// newAnalyzeWorker は、取り込んだ録画のファイルを読み直して索引と集計を作り、画像を作るジョブに渡す Worker を作る (設計書 6.3)。
// 索引が変わらず画像もあれば、画像を作り直さない (#2012)。プレビュー画像があるかは、作る設定のときだけ確かめる。
func newAnalyzeWorker(pool *pgxpool.Pool, store *kago.Store, clk clock.Clock, inserter *jobs.Client) *analysis.AnalyzeWorker {
	return analysis.NewAnalyzeWorker(pool, store, clk, inserter, previewsEnabled(pool))
}

// newJoinWorker は、引き継ぎや再起動で分かれた録画のファイルを、録画が終わったら 1 つにつなぐ Worker を作る (設計書 5.4)。
// つないだあとは、索引と集計を作り直すために、解析のジョブに渡す。管理者が有効にしていれば、TvtPlay のチャプターのファイルを、
// つないだセグメントで書き直させる (#1737)。
func newJoinWorker(pool *pgxpool.Pool, store *kago.Store, inserter *jobs.Client, logger *slog.Logger) *segjoin.JoinWorker {
	return segjoin.NewJoinWorker(pool, store, inserter, logger.With(slog.String("component", "segjoin")), 0).WithJoinedHook(tvtplay.NewEnqueuer(inserter))
}

// newFinalizeWorker は、つないだファイルを、元のセグメントと置き換える Worker を作る (#1719)。
func newFinalizeWorker(pool *pgxpool.Pool, store *kago.Store, inserter *jobs.Client) *segjoin.FinalizeWorker {
	return segjoin.NewFinalizeWorker(pool, store, inserter)
}

// newCMDetectWorker は、録画を終えたら、CM の区間を見分ける Worker を作る (設計書 6.1)。FFmpeg がなければ、ジョブが理由を残して取りやめる。
// 見分け終えたら、変換した MP4 にチャプターを書くジョブ (#1510) と、管理者が有効にしていれば、TS のセグメントの隣に
// TvtPlay のチャプターのファイルを書くジョブ (#1737) と、代表画像を CM の区間の外から選び直すジョブ (#1810) を入れる。
func newCMDetectWorker(
	pool *pgxpool.Pool, store *kago.Store, clk clock.Clock, lowerer *procprio.Lowerer, inserter *jobs.Client, logger *slog.Logger,
) *cmdetect.DetectWorker {
	return cmdetect.NewDetectWorker(pool, store, clk, lowerer,
		cmdetect.DetectedHooks{transcode.NewChaptersEnqueuer(inserter), tvtplay.NewEnqueuer(inserter), thumbnail.NewCMEnqueuer(inserter)},
		logger.With(slog.String("component", "cmdetect")))
}

// newCaptionsWorker は、録画を終えたら、音声から字幕を自動生成する Worker を作る (設計書 6.1、#1682)。
// whisper-cli とモデルは、ジョブごとに探すので、管理者が置き直したら、nekonote を再起動しなくても次のジョブから使える。
// スレッドの数も、管理者が画面で変える (#1869) ので、ジョブごとに読む。
// どちらかがなければ、ジョブが理由を残して取りやめる。取り出した音声は、kyusu と同じ一時領域に置く。
func newCaptionsWorker(
	deps *Deps, pool *pgxpool.Pool, store *kago.Store, clk clock.Clock, lowerer *procprio.Lowerer, logger *slog.Logger,
) *autocaption.Worker {
	return autocaption.NewWorker(pool, store, clk, logger.With(slog.String("component", "autocaption")),
		func(ctx context.Context) (autocaption.Transcriber, error) {
			system, err := settings.Load(ctx, pool)
			if err != nil {
				return nil, err
			}

			return autocaption.NewWhisper(&deps.Config.Captions, system.CaptionThreads, deps.Config.Storage.DataDir, lowerer)
		},
		func() (autocaption.Extractor, error) { return autocaption.FFmpegExtractor(lowerer) },
		deps.Config.Kyusu.WorkDir)
}

// newCaptionModelWorker は、管理者が画面から頼んだ、字幕の自動生成のモデルをダウンロードして置く Worker を作る (#1682)。
// モデルは、whisper-cli を動かす nekonote のデータディレクトリの既定の場所に置く。
func newCaptionModelWorker(deps *Deps, pool *pgxpool.Pool, clk clock.Clock, logger *slog.Logger) *captionmodel.Worker {
	return captionmodel.NewWorker(pool, clk, logger.With(slog.String("component", "captionmodel")), deps.Network, deps.Config.Storage.DataDir)
}

// newTranscodeWorker は、録画を終えた後に変換する Worker を作る (#1253)。映像のエンコーダーは、kyusu と同じ設定を使う。
// CM の区間があれば、変換し終えたらチャプターを書くジョブを入れる。進み具合は、ジョブの画面とログに出す (#1511)。
func newTranscodeWorker(
	deps *Deps, pool *pgxpool.Pool, store *kago.Store, clk clock.Clock, inserter *jobs.Client, lowerer *procprio.Lowerer, logger *slog.Logger,
) *transcode.Worker {
	return transcode.NewWorker(pool, store, clk, inserter, logger.With(slog.String("component", "transcode")), func() (transcode.Encoder, error) {
		return transcode.NewFFmpegEncoder(&deps.Config.Kyusu, lowerer)
	})
}

// newTvtplayWorker は、TS のセグメントの隣に、TvtPlay のチャプターのファイルを書く Worker を作る (#1737)。
func newTvtplayWorker(pool *pgxpool.Pool, store *kago.Store, clk clock.Clock) *tvtplay.WriteWorker {
	return tvtplay.NewWriteWorker(pool, store, clk)
}

// newChaptersWorker は、変換した MP4 に、CM の区間の境目をチャプターとして書く Worker を作る (#1510)。
func newChaptersWorker(pool *pgxpool.Pool, store *kago.Store, clk clock.Clock, lowerer *procprio.Lowerer) *transcode.ChaptersWorker {
	return transcode.NewChaptersWorker(pool, store, clk, func() (transcode.Chapterer, error) {
		return transcode.NewFFmpegChapterer(lowerer)
	})
}
