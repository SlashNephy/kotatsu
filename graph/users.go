package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// sessionType は、Session の ID に付ける型の名前。
const sessionType = "Session"

// errUsersUnavailable は、ユーザーの管理に使う部品が設定されていないことを表す。
var errUsersUnavailable = errors.New("user management is not configured")

// administrator は、ログイン中のユーザーが管理者で、直近 10 分以内に認証したことを確かめ、操作者を返す (設計書 9.5、9.7)。
func (r *Resolver) administrator(ctx context.Context) (*auth.Principal, *audit.Actor, error) {
	if r.userAdmin == nil {
		return nil, nil, errUsersUnavailable
	}

	return r.recentAdministrator(ctx, "only administrators can manage users")
}

// recentAdministrator は、ログイン中のユーザーが管理者で、直近 10 分以内に認証したことを確かめ、操作者を返す。
// 管理者でなければ、message を付けて FORBIDDEN を返す。
func (r *Resolver) recentAdministrator(ctx context.Context, message string) (*auth.Principal, *audit.Actor, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !principal.Role.Can(auth.PermissionAdminister) {
		return nil, nil, NewError(CodeForbidden, message)
	}
	if err := auth.RequireRecentAuthentication(ctx, r.clock); err != nil {
		return nil, nil, err
	}

	return principal, actorOf(ctx, principal), nil
}

// userModel は、auth のユーザーを GraphQL のユーザーにする。
func userModel(user *auth.User) *model.User {
	return &model.User{
		ID:          GlobalID(userType, user.ID.String()),
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        model.Role(strings.ToUpper(string(user.Role))),
		Disabled:    user.DisabledAt != nil,
		CreatedAt:   user.CreatedAt,
		AvatarURL:   nil,
	}
}

// roleOf は、GraphQL のロールを auth のロールにする。
func roleOf(role model.Role) auth.Role {
	return auth.Role(strings.ToLower(string(role)))
}

func parseUserID(globalID string) (uuid.UUID, error) {
	return parseTypedID(globalID, userType, "invalid user ID")
}

func parseSessionID(globalID string) (uuid.UUID, error) {
	return parseTypedID(globalID, sessionType, "invalid session ID")
}

// parseTypedID は、typeName の Node の ID から、UUID を取り出す。
func parseTypedID(globalID, typeName, message string) (uuid.UUID, error) {
	got, raw, err := ParseGlobalID(globalID)
	if err != nil || got != typeName {
		return uuid.Nil, NewError(CodeBadUserInput, message)
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, NewError(CodeBadUserInput, message)
	}

	return parsed, nil
}

// userByID は、ユーザーの管理の操作の後に、変わったユーザーを DB から引き直して返す。
func (r *Resolver) userByID(ctx context.Context, userID uuid.UUID) (*model.User, error) {
	row, err := sqlc.New(r.db).GetUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, NewError(CodeNotFound, "user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the user: %w", err)
	}

	return toUser(&row), nil
}
