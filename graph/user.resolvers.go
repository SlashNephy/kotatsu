package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/graph/generated"
	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// AvatarURL is the resolver for the avatarUrl field.
func (r *userResolver) AvatarURL(ctx context.Context, obj *model.User) (*string, error) {
	userID, err := parseUserID(obj.ID)
	if err != nil {
		return nil, err
	}
	var avatar *string
	_, err = sqlc.New(r.db).GetUserPicture(ctx, userID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// IdP が picture を返さなければ、画面は表示名の頭文字を出す。
	case err != nil:
		return nil, fmt.Errorf("failed to get the avatar: %w", err)
	default:
		// 画像は、ブラウザに IdP から読ませず、tenban が中継する。
		avatar = new("/media/users/" + userID.String() + "/avatar")
	}

	return avatar, nil
}

// User returns generated.UserResolver implementation.
func (r *Resolver) User() generated.UserResolver { return &userResolver{r} }

type userResolver struct{ *Resolver }
