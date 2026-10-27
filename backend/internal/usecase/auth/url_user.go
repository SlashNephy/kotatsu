package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

var (
	// ErrUserUnusable は、署名付き URL を発行したユーザーがいないか、無効になったことを表す。
	ErrUserUnusable = errors.New("the user is missing or disabled")
	// ErrDatabaseUnavailable は、DB に届かず、覚えた結果もないので、確かめられないことを表す (#1084)。
	ErrDatabaseUnavailable = errors.New("the database is unavailable")
)

// URLUsers は、署名付き URL を発行したユーザーを引く (設計書 9.6)。
// DB に届かなければ、database.FallbackTTL の中で引けたユーザーに限って、覚えた結果を使う (#1084)。
type URLUsers struct {
	db       sqlc.DBTX
	fallback *database.Fallback[uuid.UUID, Role]
}

// NewURLUsers は URLUsers を返す。logger には、DB に届かずに覚えた結果を使ったことを出す。
func NewURLUsers(db sqlc.DBTX, clk clock.Clock, logger *slog.Logger) *URLUsers {
	return &URLUsers{db: db, fallback: database.NewFallback[uuid.UUID, Role]("signed_url_users", database.FallbackTTL, clk, logger)}
}

// Role は、URL を発行したユーザーのロールを返す。ユーザーがいないか、無効にしたなら ErrUserUnusable を返す。
// DB に届かず、覚えた結果もなければ、ErrDatabaseUnavailable を包んで返す。
func (u *URLUsers) Role(ctx context.Context, userID uuid.UUID) (Role, error) {
	role, _, err := u.fallback.Load(ctx, userID, func(ctx context.Context) (Role, error) {
		user, err := sqlc.New(u.db).GetUser(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrUserUnusable
		}
		if err != nil {
			return "", fmt.Errorf("failed to get the user: %w", err)
		}
		if user.DisabledAt != nil {
			return "", ErrUserUnusable
		}

		return ParseRole(user.Role)
	})
	if database.IsUnavailable(err) {
		return "", fmt.Errorf("%w: %w", ErrDatabaseUnavailable, err)
	}

	return role, err
}

// FallbackUsed は、DB に届かずに、覚えた結果を使った回数を返す。
func (u *URLUsers) FallbackUsed() int64 {
	return u.fallback.Used()
}
