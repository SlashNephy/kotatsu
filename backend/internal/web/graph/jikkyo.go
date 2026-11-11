package graph

import (
	"context"
	"log/slog"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// serviceJikkyo は、サービスの実況のチャンネルを返す (設計書 7.4)。対応表で見つかっても、NX-Jikkyo が扱っていなければ nil を返す。
// NX-Jikkyo に届かないときも、番組表や視聴の画面を止めないよう、エラーにせず nil を返す。
// 全体の設定かユーザーの設定で実況を切っていれば、外部へ問い合わせずに nil を返す (設計書 0.3)。
func (r *Resolver) serviceJikkyo(ctx context.Context, service *model.Service) *model.JikkyoChannel {
	if r.jikkyoTable == nil || r.jikkyoClient == nil || !r.jikkyoAccess(ctx).Allowed() {
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

// serviceJikkyoChannelID は、同梱の対応表で引いた実況のチャンネルの ID を返す。外部へは問い合わせない。
// 全体の設定で実況を切っていれば nil を返す。ユーザーが切っていても返し、プレイヤーで有効にできるようにする。
func (r *Resolver) serviceJikkyoChannelID(ctx context.Context, service *model.Service) *string {
	if r.jikkyoTable == nil || !r.jikkyoAccess(ctx).SystemEnabled {
		return nil
	}
	id, ok := r.jikkyoTable.Lookup(int64(service.NetworkID), int64(service.ServiceID))
	if !ok {
		return nil
	}

	return &id
}

// jikkyoAccess は、ログイン中のユーザーが実況を使えるかどうかを返す。読めなければ、外部と通信しないよう、使えないものとして扱う。
func (r *Resolver) jikkyoAccess(ctx context.Context) domain.JikkyoAccess {
	principal, err := viewer(ctx)
	if err != nil {
		return domain.JikkyoAccess{SystemEnabled: false, UserEnabled: false}
	}
	access, err := loadersFrom(ctx).JikkyoAccess.Load(ctx, principal.UserID)()
	if err != nil {
		r.logger.WarnContext(ctx, "failed to read whether jikkyo is enabled", slog.Any("error", err))

		return domain.JikkyoAccess{SystemEnabled: false, UserEnabled: false}
	}

	return access
}
