package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// GetCommentSourceAccess は、コメントのソースごとに、使えるかどうかを決める全体の設定とユーザーの設定を返す (#1405、設計書 7.4)。
func (q *Queries) GetCommentSourceAccess(ctx context.Context, userID uuid.UUID) (domain.CommentSourceAccess, error) {
	row, err := sqlc.New(q.pool).GetCommentSourceAccess(ctx, userID)
	if err != nil {
		return domain.CommentSourceAccess{}, fmt.Errorf("failed to read the comment source settings: %w", err)
	}

	return domain.CommentSourceAccess{System: row.SystemSources, User: row.UserSources}, nil
}

// GetJikkyoAccess は、ユーザーが NX-Jikkyo のソースを使えるかどうかを決める、全体の設定とユーザーの設定を返す (設計書 7.4)。
func (q *Queries) GetJikkyoAccess(ctx context.Context, userID uuid.UUID) (domain.JikkyoAccess, error) {
	access, err := q.GetCommentSourceAccess(ctx, userID)
	if err != nil {
		return domain.JikkyoAccess{}, err
	}

	return access.For(domain.CommentSourceNXJikkyo), nil
}
