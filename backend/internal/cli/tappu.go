package cli

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/innertoken"
	"github.com/SlashNephy/kotatsu/internal/usecase/tappu"
	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

// runTappu は、tappu のロールの本体。TS を配るのは、インスタンスのサーバーに載せた API なので、ここでは終わりを待つだけにする。
func runTappu(ctx context.Context, logger *slog.Logger, deps *Deps) error {
	logger.InfoContext(ctx, "role started")
	defer logger.InfoContext(ctx, "role stopped")
	if deps.Config.Tuner.Kind != config.TunerKindEDCB {
		logger.InfoContext(ctx, "tappu shares EDCB NetworkTV streams only; nothing to do unless tuner.kind is edcb")
	}
	<-ctx.Done()

	return nil
}

// serveTappu は、このプロセスで、tappu の API を公開するかどうかを返す。EDCB を使うときだけ公開する。
func serveTappu(deps *Deps, roles []*Role) bool {
	return deps.Config.Tuner.Kind == config.TunerKindEDCB &&
		slices.ContainsFunc(roles, func(role *Role) bool { return role.Name == tappu.Role })
}

// newTappuHandler は、このプロセスの EDCB のチューナーサーバーを、tappu の API として公開するハンドラーを返す。
// ストリームは、tenban の配信と同じく、止めるときに打ち切る。受け手は、ほかの tappu か自分で開いて、つなぎ直す。
func newTappuHandler(deps *Deps, logger *slog.Logger) (http.Handler, error) {
	token, err := tappu.NewToken(deps.Config.Auth.SecretKey.Value())
	if errors.Is(err, innertoken.ErrNoKey) {
		return nil, errSecretKeyRequired
	}
	if err != nil {
		return nil, err
	}
	server, err := sharedEDCBServer(deps)
	if err != nil {
		return nil, err
	}

	return httpserver.StopOnShutdown(tappu.Handler(server, token, buildinfo.Get().Version, logger.With(slog.String("component", "tappu")))), nil
}

// tappuTuner は、EDCB のチューナーサーバーのうち、ストリームだけを tappu から受け取る (#1535)。
// サービスと番組、チューナー、イベント、ロゴは、EDCB から直接読む。
type tappuTuner struct {
	*edcb.TunerServer

	streams *tappu.Streams
}

var _ tunerserver.Client = new(tappuTuner)

func (t *tappuTuner) OpenServiceStream(ctx context.Context, serviceID int64, opts *tunerserver.StreamOptions) (io.ReadCloser, error) {
	return t.streams.OpenServiceStream(ctx, serviceID, opts)
}

func (t *tappuTuner) OpenChannelStream(ctx context.Context, channelType, channel string, opts *tunerserver.StreamOptions) (io.ReadCloser, error) {
	return t.streams.OpenChannelStream(ctx, channelType, channel, opts)
}
