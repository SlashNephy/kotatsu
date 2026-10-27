package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
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
// 管理者でなければ、message を付けて FORBIDDEN を返す。第三者に影響しうる操作 (ユーザーの管理、外部のサービスとの連携) に使う (設計書 9.5)。
func (r *Resolver) recentAdministrator(ctx context.Context, message string) (*auth.Principal, *audit.Actor, error) {
	principal, actor, err := r.administratorActor(ctx, message)
	if err != nil {
		return nil, nil, err
	}
	if err := auth.RequireRecentAuthentication(ctx, r.clock); err != nil {
		return nil, nil, err
	}

	return principal, actor, nil
}

// administratorActor は、ログイン中のユーザーが管理者であることを確かめ、操作者を返す。再認証は求めない。
// 管理者でなければ、message を付けて FORBIDDEN を返す。第三者に影響しない管理の操作 (録画やサービスの設定、ファイルの片付けなど) に使う (設計書 9.5)。
func (r *Resolver) administratorActor(ctx context.Context, message string) (*auth.Principal, *audit.Actor, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !principal.Role.Can(auth.PermissionAdminister) {
		return nil, nil, NewError(CodeForbidden, message)
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

// revokeSession は、ログイン中のユーザーのセッション sessionID を失効させ、監査ログに残す (設計書 8.5)。
func (r *Resolver) revokeSession(ctx context.Context, principal *auth.Principal, sessionID uuid.UUID) error {
	return r.sessions.RevokeAndRecord(ctx, r.db, r.recorder, actorOf(ctx, principal), principal.UserID, sessionID)
}

// revokeOtherSessions は、ログイン中のユーザーの、今のセッションのほかをまとめて失効させ、監査ログに残す。
func (r *Resolver) revokeOtherSessions(ctx context.Context, principal *auth.Principal) (int64, error) {
	return r.sessions.RevokeOthersAndRecord(ctx, r.db, r.recorder, actorOf(ctx, principal), principal.UserID, principal.SessionID)
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
	row, err := r.queries.GetUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, NewError(CodeNotFound, "user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the user: %w", err)
	}

	return toUser(&row), nil
}

// forgetAuthorizations は、ユーザーについて、DB に届かない間に使うために覚えた認可の結果を忘れる (#1084)。keep のセッションは残す。
// ユーザーを無効にした、ロールやパスワードを変えた直後に DB が止まっても、覚えた結果で前の権限のまま認可しないようにする。
func (r *Resolver) forgetAuthorizations(userID uuid.UUID, keep *uuid.UUID) {
	if r.sessions != nil {
		r.sessions.ForgetUser(userID, keep)
	}
	if r.urlUsers != nil {
		r.urlUsers.Forget(userID)
	}
}
