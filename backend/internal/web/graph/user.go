package graph

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// userType は、User の Node の ID に使う型の名前。
const userType = "User"

func toUser(user *sqlc.User) *model.User {
	return &model.User{
		ID:          GlobalID(userType, user.ID.String()),
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        model.Role(strings.ToUpper(user.Role)),
		Disabled:    user.DisabledAt != nil,
		CreatedAt:   user.CreatedAt,
		AvatarURL:   nil,
	}
}

// users は、ユーザーをまとめて引き、ID から引ける表にする。nil の ID は飛ばす。
// ユーザーを消すと、作成者などの参照は NULL になるので、見つからないユーザーはない。
func users(ctx context.Context, ids ...*uuid.UUID) (map[uuid.UUID]*model.User, error) {
	keys := make([]uuid.UUID, 0, len(ids))
	for _, userID := range ids {
		if userID != nil && !slices.Contains(keys, *userID) {
			keys = append(keys, *userID)
		}
	}
	found, err := loadersFrom(ctx).Users.LoadAll(ctx, keys)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]*model.User, len(found))
	for _, user := range found {
		byID[user.ID] = toUser(user)
	}

	return byID, nil
}

// userOf は、表から userID のユーザーを返す。userID が nil なら nil を返す。
func userOf(byID map[uuid.UUID]*model.User, userID *uuid.UUID) *model.User {
	if userID == nil {
		return nil
	}

	return byID[*userID]
}
