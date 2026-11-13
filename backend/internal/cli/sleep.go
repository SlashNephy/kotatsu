package cli

import (
	"context"
	"errors"
	"log/slog"

	"github.com/SlashNephy/kotatsu/internal/infra/power"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
	"github.com/SlashNephy/kotatsu/internal/usecase/sleep"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
	"github.com/SlashNephy/kotatsu/internal/web/streamapi"
)

// SleepSystem は、OS にスリープの抑止と起こす予約を伝える口。止めるときは Close を呼ぶ。
type SleepSystem interface {
	sleep.System
	Close() error
}

var _ SleepSystem = new(power.System)

// newSleepSystem は、この OS の SleepSystem を返す。
// nil の *power.System を interface に入れると nil と比べられなくなるので、失敗したら interface のまま nil を返す。
func newSleepSystem(opts *power.Options) (SleepSystem, error) {
	system, err := power.New(opts)
	if err != nil {
		return nil, err
	}

	return system, nil
}

// sleepEncodeKinds は、スリープさせない理由にする、録画の後処理のジョブの種類 (設計書 6.1)。FFmpeg で録画を読み続けるものだけを入れる。
func sleepEncodeKinds() []string {
	return []string{
		new(transcode.Args).Kind(),
		new(transcode.ChaptersArgs).Kind(),
		new(cmdetect.DetectArgs).Kind(),
	}
}

// runSleepControl は、ctx が終わるまで、OS のスリープを扱う (#1522、設計書 11.7)。止まるときは、抑止を外し、起こす予約を消す。
// all の構成のときだけ扱う。録画と視聴とジョブを、1 つのプロセスで見渡せるからである。
// 設定で切ったときと、スリープを扱わない OS (Linux とコンテナ) では、何もしない。扱えなくても、ほかのロールは止めない。
func runSleepControl(ctx context.Context, logger *slog.Logger, opts *Options, deps *Deps, all bool, api *apiHandler, live *kyusu.Live) {
	// DB がなければ、判定に使う値を読めない。テストで差し替えた OpenDatabase は nil を返すことがある。
	if !all || !deps.Config.Power.ManageSleep || opts.SleepSystem == nil || deps.DB == nil {
		return
	}
	logger = logger.With(slog.String("component", "sleep"))
	system, err := opts.SleepSystem(&power.Options{DataDir: deps.Config.Storage.DataDir, TaskTimeLimit: 0})
	if errors.Is(err, power.ErrUnsupported) {
		return
	}
	if err != nil {
		logger.WarnContext(ctx, "failed to load the power management API; the system may sleep during recordings", slog.Any("error", err))

		return
	}
	defer func() {
		if err := system.Close(); err != nil {
			logger.WarnContext(ctx, "failed to close the power management API", slog.Any("error", err))
		}
	}()
	// ライブの配信は、tenban が変換せずに流すものと、kyusu がトランスコードするものを足す。録画の再生は入れない。
	viewers := func() int64 {
		var n int64
		if api != nil {
			n += int64(api.streams.Streams(streamapi.StreamKindLive) + api.streams.Streams(streamapi.StreamKindTimeshift))
		}
		if live != nil {
			n += int64(live.Sessions())
		}

		return n
	}
	controller := sleep.NewController(sleep.NewDBSource(deps.DB, sleepEncodeKinds(), viewers), system, deps.Clock, logger, nil)
	if err := controller.Run(ctx); err != nil {
		logger.WarnContext(ctx, "sleep control stopped with an error", slog.Any("error", err))
	}
}
