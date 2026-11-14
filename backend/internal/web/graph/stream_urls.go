package graph

import (
	"context"
	"errors"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// issuedStreamURLType は、発行した外部プレイヤー用の URL の記録のグローバル ID の型名。
const issuedStreamURLType = "IssuedStreamUrl"

// issuedStreamURLs は、期限の過ぎていない、発行した外部プレイヤー用の URL の記録を返す (#1757)。管理者だけが使える。
func (r *Resolver) issuedStreamURLs(ctx context.Context) ([]*model.IssuedStreamURL, error) {
	if _, _, err := r.administratorActor(ctx, "only administrators can list the issued stream URLs"); err != nil {
		return nil, err
	}
	urls, err := r.streamURLs.List(ctx)
	if err != nil {
		return nil, err
	}
	issued := make([]*model.IssuedStreamURL, 0, len(urls))
	for _, url := range urls {
		issued = append(issued, &model.IssuedStreamURL{
			ID: GlobalID(issuedStreamURLType, url.ID.String()), UserDisplayName: url.UserDisplayName, TargetName: url.TargetName,
			Live: url.Live, CreatedAt: url.CreatedAt, ExpiresAt: url.ExpiresAt, LastUsedAt: url.LastUsedAt,
		})
	}

	return issued, nil
}

// revokeStreamURL は、発行した外部プレイヤー用の URL を取り消す (#1757)。管理者だけが使える。
// 権限を広げない操作なので、再認証は求めない (設計書 9.5)。
func (r *Resolver) revokeStreamURL(ctx context.Context, globalID string) (bool, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can revoke the issued stream URLs")
	if err != nil {
		return false, err
	}
	typeName, raw, err := ParseGlobalID(globalID)
	if err != nil || typeName != issuedStreamURLType {
		return false, NewError(CodeBadUserInput, "invalid issued stream URL ID")
	}
	urlID, err := uuid.Parse(raw)
	if err != nil {
		return false, NewError(CodeBadUserInput, "invalid issued stream URL ID")
	}
	err = r.streamURLs.Delete(ctx, actor, urlID)
	if errors.Is(err, auth.ErrStreamURLNotFound) {
		return false, NewError(CodeNotFound, "issued stream URL not found")
	}
	if err != nil {
		return false, err
	}

	return true, nil
}
