package cli

import (
	"context"
	"log/slog"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
	"github.com/SlashNephy/kotatsu/internal/usecase/google"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

// libraryParts は、ライブラリ (録画として登録していない TS のフォルダ) を読む部品 (設計書 5.8、#1734)。
type libraryParts struct {
	libraries *medialib.Set
	indexer   *filelib.Indexer
	source    *filelib.Source
	// google は、Google の OAuth クライアントと Google Drive の接続を扱う (#1902)。アプリの秘密キーがなければ nil。
	google *google.Service
}

// newLibraryParts は、設定の保存先プールとライブラリから、ライブラリを読む部品を返す。build が true なら (tenban)、
// 開いたファイルのインデックスを、データディレクトリに作る。kyusu は読むだけにする。
func newLibraryParts(deps *Deps, logger *slog.Logger, build bool) *libraryParts {
	storage := &deps.Config.Storage
	libraries := medialib.NewSet(storage.Pools, storage.PoolOrder, storage.Libraries)
	googleService, err := newGoogleService(deps, logger, libraries.Names())
	if err != nil {
		logger.Warn("Google Drive is unavailable without the secret key", slog.Any("error", err))
		googleService = nil
	}
	if googleService != nil {
		// Google Drive は、接続していなくても一覧に入れておき、接続しているかどうかは、読むたびに DB で確認する。
		tokens := googleService.DriveTokens(deps.DB)
		libraries = libraries.WithProvider(medialib.NewDrive(googleService.DriveClient(tokens), tokens, deps.Clock.Now))
	}
	indexer := filelib.NewIndexer(storage.DataDir, libraries, deps.Clock, logger.With(slog.String("component", "library_index")), build)

	return &libraryParts{libraries: libraries, indexer: indexer, source: filelib.NewSource(libraries, indexer), google: googleService}
}

// newGoogleService は、Google の OAuth クライアントと Google Drive の接続を扱う部品を作る (#1902)。
func newGoogleService(deps *Deps, logger *slog.Logger, reserved []string) (*google.Service, error) {
	return google.NewService(&google.Options{
		Clock: deps.Clock, Recorder: audit.NewRecorder(deps.Clock), AppSecret: string(appSecretOf(deps)),
		// 何 GB もあるファイルを読み通すので、全体のタイムアウトを付けない。
		HTTPClient: netactivity.NewClient(deps.Network, netactivity.PurposeGoogleDrive, netactivity.ClientOptions{
			Transport: nil, Timeout: 0, CheckRedirect: nil,
		}),
		Logger: logger.With(slog.String("component", "google")), URL: deps.Config.Google.URL, ReservedLibraryNames: reserved,
	})
}

// libraryJobsOf は、ライブラリのファイルの画像、CM の区間、字幕の自動生成のジョブを入れる River のクライアントを返す。なければ nil を返す。
// 型付きの nil を interface に入れると nil と比較できなくなるので、nil なら interface の nil を返す。
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
		func(ctx context.Context) (autocaption.Transcriber, error) {
			// 録画と同じく、管理者が画面で選んだスレッドの数をジョブごとに読む。
			system, err := settings.Load(ctx, poolOf(deps))
			if err != nil {
				return nil, err
			}

			return autocaption.NewWhisper(&deps.Config.Captions, system.CaptionThreads, deps.Config.Storage.DataDir, lowerer)
		},
		func() (autocaption.ReaderExtractor, error) { return autocaption.FFmpegReaderExtractor(lowerer) },
		deps.Config.Kyusu.WorkDir)
}
