package graph

import (
	"context"
	"log/slog"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// serviceJikkyo は、サービスの実況のチャンネルを返す (設計書 7.4)。対応表で見つかっても、NX-Jikkyo が扱っていなければ nil を返す。
// NX-Jikkyo に届かないときも、番組表や視聴の画面を止めないよう、エラーにせず nil を返す。
func (r *Resolver) serviceJikkyo(ctx context.Context, service *model.Service) *model.JikkyoChannel {
	if r.jikkyoTable == nil || r.jikkyoClient == nil {
		return nil
	}
	id, ok := r.jikkyoTable.Lookup(int64(service.NetworkID), int64(service.ServiceID))
	if !ok {
		return nil
	}
	channel, err := r.jikkyoClient.Channel(ctx, id)
	if err != nil {
		r.logger.WarnContext(ctx, "failed to look up the jikkyo channel", slog.String("channel", id), slog.Any("error", err))

		return nil
	}
	if channel == nil {
		return nil
	}

	result := &model.JikkyoChannel{ID: channel.ID, Name: channel.Name, Force: nil, Viewers: nil}
	if channel.Activity != nil {
		result.Force, result.Viewers = new(channel.Activity.Force), new(channel.Activity.Viewers)
	}

	return result
}
