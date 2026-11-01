package graph

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// errChannelInspectionUnavailable は、チャンネルの TS を調べられない (チューナーサーバーを設定していない) ときのエラー。
var errChannelInspectionUnavailable = errors.New("channel inspection is unavailable")

// inspectChannel は、物理チャンネルの TS を数秒受け取り、サービスごとの構成とビットレートを返す (#830)。管理者だけが使える。
func (r *Resolver) inspectChannel(ctx context.Context, channelType, channel string) (*model.ChannelInspection, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}
	if r.channelInspector == nil {
		return nil, errChannelInspectionUnavailable
	}
	result, err := r.channelInspector.Inspect(ctx, channelType, channel, stream.DefaultInspectDuration)
	switch {
	case errors.Is(err, stream.ErrNoTuner):
		return nil, errChannelInspectionUnavailable
	case errors.Is(err, stream.ErrInspectionInterrupted):
		return nil, NewError(CodeConflict, "the channel stream ended before the inspection finished; the tuner may have been taken")
	case err != nil:
		r.logger.WarnContext(ctx, "failed to inspect the channel", slog.String("channel_type", channelType), slog.String("channel", channel), slog.Any("error", err))

		return nil, NewError(CodeConflict, "failed to inspect the channel; no tuner may be available")
	}
	// program_number (サービス ID) から、Kotatsu が知っている、そのチャンネルのサービスの名前を引く。
	services, err := r.queries.ListGuideServices(ctx, query.ListGuideServicesParams{IncludeHidden: true, MediaServiceTypes: []int64{}, IDs: []int64{}})
	if err != nil {
		return nil, err
	}
	names := map[int64]string{}
	for _, s := range services {
		if s.ChannelType == channelType && s.Channel == channel {
			names[s.Sid] = s.Name
		}
	}
	toStreams := func(streams []stream.InspectedStream) []*model.InspectedStream {
		out := make([]*model.InspectedStream, 0, len(streams))
		for _, s := range streams {
			kind := model.StreamKind(strings.ToUpper(s.Kind))
			if !kind.IsValid() {
				kind = model.StreamKindUnknown
			}
			var streamType *int
			if s.StreamType != nil {
				streamType = new(int(*s.StreamType))
			}
			out = append(out, &model.InspectedStream{
				Pid: int(s.PID), Kind: kind, StreamType: streamType, Bps: int(s.BPS), Drops: s.Drops, Errors: s.Errors, Scrambled: s.Scrambled,
			})
		}

		return out
	}
	inspection := &model.ChannelInspection{
		TransportStreamID: nil, Seconds: result.Duration.Seconds(), TotalBps: int(result.TotalBPS),
		Programs: make([]*model.InspectedProgram, 0, len(result.Programs)), Others: toStreams(result.Others),
	}
	if result.TransportStreamID != nil {
		inspection.TransportStreamID = new(int(*result.TransportStreamID))
	}
	for _, p := range result.Programs {
		var name *string
		if n, ok := names[int64(p.Number)]; ok {
			name = new(n)
		}
		inspection.Programs = append(inspection.Programs, &model.InspectedProgram{
			ServiceID: int(p.Number), Name: name, Bps: int(p.BPS), Streams: toStreams(p.Streams),
		})
	}

	return inspection, nil
}
