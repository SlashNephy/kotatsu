package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// GetJikkyoAccess は、ユーザーが実況を使えるかどうかを決める、全体の設定とユーザーの設定を返す (設計書 7.4)。
func (q *Queries) GetJikkyoAccess(ctx context.Context, userID uuid.UUID) (domain.JikkyoAccess, error) {
	row, err := sqlc.New(q.pool).GetJikkyoAccess(ctx, userID)
	if err != nil {
		return domain.JikkyoAccess{}, fmt.Errorf("failed to read the jikkyo settings: %w", err)
	}

	return domain.JikkyoAccess{SystemEnabled: row.SystemEnabled, UserEnabled: row.UserEnabled}, nil
}
