package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// CreateUser is the resolver for the createUser field.
func (r *mutationResolver) CreateUser(ctx context.Context, input model.CreateUserInput) (*model.User, error) {
	_, actor, err := r.administrator(ctx)
	if err != nil {
		return nil, err
	}
	account := &auth.NewAccount{Username: input.Username, DisplayName: input.DisplayName, Role: roleOf(input.Role), Password: ""}
	if input.Password != nil {
		account.Password = *input.Password
	}
	created, err := r.userAdmin.Create(ctx, r.db, actor, account)
	if err != nil {
		return nil, err
	}

	return userModel(created), nil
}

// UpdateUser is the resolver for the updateUser field.
func (r *mutationResolver) UpdateUser(ctx context.Context, id string, input model.UpdateUserInput) (*model.User, error) {
	_, actor, err := r.administrator(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := parseUserID(id)
	if err != nil {
		return nil, err
	}
	changes := &auth.UserChanges{DisplayName: input.DisplayName, Role: nil}
	if input.Role != nil {
		changes.Role = new(roleOf(*input.Role))
	}
	updated, err := r.userAdmin.Update(ctx, r.db, actor, userID, changes)
	if err != nil {
		return nil, err
	}
	// 直後に DB が止まっても、前のロールのまま認可しないよう、覚えた結果を忘れる (#1084)。
	r.forgetAuthorizations(userID, nil)

	return userModel(updated), nil
}

// DisableUser is the resolver for the disableUser field.
func (r *mutationResolver) DisableUser(ctx context.Context, id string) (*model.User, error) {
	principal, actor, err := r.administrator(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := parseUserID(id)
	if err != nil {
		return nil, err
	}
	if err := r.userAdmin.Disable(ctx, r.db, actor, principal.UserID, userID); err != nil {
		return nil, err
	}
	r.forgetAuthorizations(userID, nil)

	return r.userByID(ctx, userID)
}

// EnableUser is the resolver for the enableUser field.
func (r *mutationResolver) EnableUser(ctx context.Context, id string) (*model.User, error) {
	_, actor, err := r.administrator(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := parseUserID(id)
	if err != nil {
		return nil, err
	}
	if err := r.userAdmin.Enable(ctx, r.db, actor, userID); err != nil {
		return nil, err
	}

	return r.userByID(ctx, userID)
}

// ResetUserPassword is the resolver for the resetUserPassword field.
func (r *mutationResolver) ResetUserPassword(ctx context.Context, id string, password string) (*model.User, error) {
	_, actor, err := r.administrator(ctx)
	if err != nil {
		return nil, err
	}
	userID, err := parseUserID(id)
	if err != nil {
		return nil, err
	}
	if err := r.userAdmin.ResetPassword(ctx, r.db, actor, userID, password); err != nil {
		return nil, err
	}
	r.forgetAuthorizations(userID, nil)

	return r.userByID(ctx, userID)
}

// ChangePassword is the resolver for the changePassword field.
func (r *mutationResolver) ChangePassword(ctx context.Context, password string) (bool, error) {
	if r.passwords == nil {
		return false, errUsersUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return false, err
	}
	// パスワードを変えるのは重要な操作なので、直前の再認証を求める (設計書 9.5)。
	if err := auth.RequireRecentAuthentication(ctx, r.clock); err != nil {
		return false, err
	}
	// 今のセッションは残し、ほかの端末のセッションを失効させる。
	if err := r.passwords.Set(ctx, r.db, actorOf(ctx, principal), principal.UserID, password, &principal.SessionID); err != nil {
		return false, err
	}
	r.forgetAuthorizations(principal.UserID, &principal.SessionID)

	return true, nil
}

// RevokeSession is the resolver for the revokeSession field.
func (r *mutationResolver) RevokeSession(ctx context.Context, id string) (string, error) {
	if r.sessions == nil {
		return "", errUsersUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return "", err
	}
	sessionID, err := parseSessionID(id)
	if err != nil {
		return "", err
	}
	if err := r.revokeSession(ctx, principal, sessionID); err != nil {
		return "", err
	}

	return id, nil
}

// RevokeOtherSessions is the resolver for the revokeOtherSessions field.
func (r *mutationResolver) RevokeOtherSessions(ctx context.Context) (int, error) {
	if r.sessions == nil {
		return 0, errUsersUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return 0, err
	}
	revoked, err := r.revokeOtherSessions(ctx, principal)
	if err != nil {
		return 0, err
	}

	return int(revoked), nil
}

// Users is the resolver for the users field.
func (r *queryResolver) Users(ctx context.Context) ([]*model.User, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if !principal.Role.Can(auth.PermissionAdminister) {
		return nil, NewError(CodeForbidden, "only administrators can list users")
	}
	if r.userAdmin == nil {
		return nil, errUsersUnavailable
	}
	all, err := r.userAdmin.List(ctx, r.db)
	if err != nil {
		return nil, err
	}
	result := make([]*model.User, 0, len(all))
	for _, user := range all {
		result = append(result, userModel(user))
	}

	return result, nil
}

// Sessions is the resolver for the sessions field.
func (r *queryResolver) Sessions(ctx context.Context) ([]*model.Session, error) {
	if r.sessions == nil {
		return nil, errUsersUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	sessions, err := r.sessions.List(ctx, r.db, principal.UserID)
	if err != nil {
		return nil, err
	}
	result := make([]*model.Session, 0, len(sessions))
	for _, session := range sessions {
		item := &model.Session{
			ID:         GlobalID(sessionType, session.ID.String()),
			Current:    session.ID == principal.SessionID,
			CreatedAt:  session.CreatedAt,
			LastSeenAt: session.LastSeenAt,
			UserAgent:  session.UserAgent,
			SourceIP:   nil,
		}
		if session.SourceIP.IsValid() {
			item.SourceIP = new(session.SourceIP.String())
		}
		result = append(result, item)
	}

	return result, nil
}
