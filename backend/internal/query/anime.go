package query

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// GetAnimeWishlistImageURL は、ユーザー userID の見たいリストにある、記録先 destination の作品 workID の画像の URL を返す (#1235)。
// 見たいリストにないか、画像がなければ pgx.ErrNoRows を返す。
func (q *Queries) GetAnimeWishlistImageURL(ctx context.Context, userID uuid.UUID, destination string, workID int32) (string, error) {
	result, err := sqlc.New(q.pool).GetAnimeWishlistImageURL(ctx, sqlc.GetAnimeWishlistImageURLParams{
		UserID: userID, Destination: destination, WorkID: workID,
	})
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", pgx.ErrNoRows
	}

	return *result, nil
}
