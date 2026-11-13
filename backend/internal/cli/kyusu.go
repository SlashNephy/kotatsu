package cli

import (
	"context"
	"errors"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
	"github.com/SlashNephy/kotatsu/internal/usecase/metrics"
	"github.com/SlashNephy/kotatsu/internal/usecase/scramble"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

// kyusuParts は、kyusu のライブ視聴と録画の仮想 HLS のセッションと、tenban からのリクエストを確かめるトークン。
type kyusuParts struct {
	token *kyusu.Token
	live  *kyusu.Live
	vod   *kyusu.VOD
}

// kyusuLogger は、kyusu の部品に渡すロガー。wire は型で依存を見分けるので、ほかのロガーと分ける。
type kyusuLogger *slog.Logger

// newKyusu は、kyusu の部品を作り、セッションの数をメトリクスに登録する。
// アプリの秘密キーがなければ、ほかの部品を作る前に断る。ライブ視聴のストリームを開く先は、チューナーサーバーを使う構成でだけ作る。FFmpeg か tsreadex がなければ、起動はするが、
// トランスコードには理由を添えて 503 を返す。どちらも設定や環境で作るかが変わるので、wire の外で決めて渡す (#1607)。
// 録画は、このインスタンスにマウントした保存先プールから読む。
func newKyusu(ctx context.Context, logger *slog.Logger, deps *Deps) (*kyusu.Live, *kyusu.VOD, *kyusu.Token, error) {
	token, err := newKyusuToken(appSecretOf(deps))
	if err != nil {
		return nil, nil, nil, err
	}
	var opener kyusu.ServiceOpener
	if deps.Config.Tuner.UsesTunerServer() {
		client, err := newTunerClient(deps)
		if err != nil {
			return nil, nil, nil, err
		}
		opener = client
	}
	transcoder, err := kyusu.NewTranscoder(&deps.Config.Kyusu)
	switch {
	case errors.Is(err, kyusu.ErrToolNotFound):
		logger.WarnContext(ctx, "FFmpeg or tsreadex is not found; server-side transcoding is disabled", slog.Any("error", err))
		transcoder = nil
	case err != nil:
		return nil, nil, nil, err
	}
	parts, err := newKyusuParts(deps, logger, token, opener, transcoder)
	if err != nil {
		return nil, nil, nil, err
	}
	for kind, sessions := range map[string]func() int{"live": parts.live.Sessions, "vod": parts.vod.Sessions} {
		deps.Metrics.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: metrics.Namespace, Name: "kyusu_sessions", Help: "Number of kyusu transcoding sessions on this instance.",
			ConstLabels: prometheus.Labels{kindLabel: kind},
		}, func() float64 { return float64(sessions()) }))
	}

	return parts.live, parts.vod, parts.token, nil
}

// newKyusuToken は、tenban だけが付けて kyusu に中継する、アプリの秘密キーから導いたトークンを作る。
func newKyusuToken(secret appSecret) (*kyusu.Token, error) {
	token, err := kyusu.NewToken(string(secret))
	if errors.Is(err, kyusu.ErrNoKey) {
		return nil, errSecretKeyRequired
	}

	return token, err
}

func kyusuLoggerOf(logger *slog.Logger) kyusuLogger {
	return logger.With(slog.String("component", "kyusu"))
}

// newKyusuLive は、ライブ視聴の仮想 HLS のセッションを作る。トランスコーダーがなければ、エンコーダーを渡さない。
// 型付きの nil を interface に入れると nil と比べられなくなるので、nil なら interface の nil を渡す。
func newKyusuLive(deps *Deps, logger kyusuLogger, opener kyusu.ServiceOpener, transcoder *kyusu.Transcoder) (*kyusu.Live, error) {
	var encoder kyusu.Encoder
	if transcoder != nil {
		encoder = transcoder
	}

	return kyusu.NewLive(&kyusu.LiveOptions{
		Opener: opener, Encoder: encoder, WorkDir: deps.Config.Kyusu.WorkDir, Clock: deps.Clock, Logger: (*slog.Logger)(logger), IdleTimeout: 0,
		Scramble: scramble.NewReporter(deps.DB, deps.Clock, (*slog.Logger)(logger)),
	})
}

// newKyusuVOD は、録画とタイムシフトの仮想 HLS のセッションを作る。トランスコーダーがなければ、エンコーダーを渡さない。
func newKyusuVOD(deps *Deps, logger kyusuLogger, transcoder *kyusu.Transcoder) (*kyusu.VOD, error) {
	var encoder kyusu.VODEncoder
	if transcoder != nil {
		encoder = transcoder
	}

	return kyusu.NewVOD(&kyusu.VODOptions{
		Opener:    stream.NewRecordingSource(deps.DB, storeOf(deps.Config), 0),
		Timeshift: stream.NewTimeshiftSource(deps.DB, storeOf(deps.Config), 0),
		Encoder:   encoder, WorkDir: deps.Config.Kyusu.WorkDir, Clock: deps.Clock, Logger: (*slog.Logger)(logger), IdleTimeout: 0, MaxSessions: 0,
	})
}
