package graph

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
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

// revokeSession は、ログイン中のユーザーのセッション sessionID を失効させ、監査ログに残す (設計書 8.5)。
// 失効と記録を 1 つのトランザクションにまとめ、記録できなければ失効させない。
func (r *Resolver) revokeSession(ctx context.Context, principal *auth.Principal, sessionID uuid.UUID) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if err := r.sessions.Revoke(ctx, tx, principal.UserID, sessionID); err != nil {
			return err
		}

		return r.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actorOf(ctx, principal),
			Action:     "user.session_revoked",
			TargetType: "user",
			TargetID:   principal.UserID.String(),
			Summary:    map[string]any{"sessionId": sessionID.String()},
			// 送信元は、操作者 (actor) に持たせる。
			SourceIP:  netip.Addr{},
			UserAgent: "",
		})
	})
}

// revokeOtherSessions は、ログイン中のユーザーの、今のセッションのほかをまとめて失効させ、監査ログに残す。
func (r *Resolver) revokeOtherSessions(ctx context.Context, principal *auth.Principal) (int64, error) {
	var revoked int64
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		n, err := r.sessions.RevokeOthers(ctx, tx, principal.UserID, principal.SessionID)
		if err != nil {
			return err
		}
		revoked = n

		return r.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actorOf(ctx, principal),
			Action:     "user.other_sessions_revoked",
			TargetType: "user",
			TargetID:   principal.UserID.String(),
			Summary:    map[string]any{"count": n},
			// 送信元は、操作者 (actor) に持たせる。
			SourceIP:  netip.Addr{},
			UserAgent: "",
		})
	})
	if err != nil {
		return 0, err
	}

	return revoked, nil
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
