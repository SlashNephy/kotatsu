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
	"github.com/SlashNephy/kotatsu/internal/usecase/nabe"
	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
	"github.com/SlashNephy/kotatsu/internal/web/mirakurunapi"
)

// runNabe は、nabe のロールの本体。TS を配信するのは、インスタンスのサーバーに載せた API なので、ここでは終わりを待つだけにする。
func runNabe(ctx context.Context, logger *slog.Logger, deps *Deps) error {
	logger.InfoContext(ctx, "role started")
	defer logger.InfoContext(ctx, "role stopped")
	if deps.Config.Tuner.Kind != config.TunerKindEDCB {
		logger.InfoContext(ctx, "nabe shares EDCB NetworkTV streams only; nothing to do unless tuner.kind is edcb")
	}
	<-ctx.Done()

	return nil
}

// serveNabe は、このプロセスで、nabe の API を公開するかどうかを返す。EDCB を使うときだけ公開する。
func serveNabe(deps *Deps, roles []*Role) bool {
	return deps.Config.Tuner.Kind == config.TunerKindEDCB &&
		slices.ContainsFunc(roles, func(role *Role) bool { return role.Name == nabe.Role })
}

// newNabeHandler は、このプロセスの EDCB のチューナーサーバーを、nabe の API として公開するハンドラーを返す。
// ストリームは、tenban の配信と同じく、止めるときに打ち切る。受け手は、ほかの nabe か自分で開いて、つなぎ直す。
func newNabeHandler(deps *Deps, logger *slog.Logger) (http.Handler, error) {
	token, err := nabe.NewToken(deps.Config.Auth.SecretKey.Value())
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

	return httpserver.StopOnShutdown(mirakurunapi.NabeHandler(server, token, buildinfo.Get().Version, logger.With(slog.String("component", "nabe")))), nil
}

// nabeTuner は、EDCB のチューナーサーバーのうち、ストリームだけを nabe から受け取る (#1535)。
// サービスと番組、チューナー、イベント、ロゴは、EDCB から直接読む。
type nabeTuner struct {
	*edcb.TunerServer

	streams *nabe.Streams
}

var _ tunerserver.Client = new(nabeTuner)

func (t *nabeTuner) OpenServiceStream(ctx context.Context, serviceID int64, opts *tunerserver.StreamOptions) (io.ReadCloser, error) {
	return t.streams.OpenServiceStream(ctx, serviceID, opts)
}

func (t *nabeTuner) OpenChannelStream(ctx context.Context, channelType, channel string, opts *tunerserver.StreamOptions) (io.ReadCloser, error) {
	return t.streams.OpenChannelStream(ctx, channelType, channel, opts)
}
