package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/password"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

var (
	// ErrLastAdmin は、有効な管理者が 1 人もいなくなる操作であることを表す。
	// 管理者がいなくなると、ユーザーやシステムの設定を誰も変えられなくなる。
	ErrLastAdmin = errors.New("at least one enabled administrator is required")
	// ErrCannotDisableSelf は、自分を無効にしようとしたことを表す。
	ErrCannotDisableSelf = errors.New("you cannot disable yourself")
	// ErrCannotDeleteSelf は、管理者の操作で自分を消そうとしたことを表す。自分のアカウントは、本人の操作 (DeleteSelf) で消す。
	ErrCannotDeleteSelf = errors.New("you cannot delete yourself from the user management")
)

// UserDataEraser は、users の行の ON DELETE CASCADE だけでは消えない、ユーザーのデータを消す (#1382)。
type UserDataEraser interface {
	// PrepareUserErasure は、ユーザーの行を消すトランザクションの中で、行を消す前に呼ばれ、行と一緒に消える情報を読み取る。
	// 返す関数は、コミットの後に呼ばれ、DB の外のもの (ファイル、外部のサービスのトークン) を片付ける。
	// 片付けに失敗してもユーザーの削除は戻さないので、返す関数はエラーを返さず、失敗を自分でログに残す。
	PrepareUserErasure(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (func(context.Context), error)
}

// UserAdmin は、管理者によるユーザーの管理を扱う (設計書 9.7)。どの操作も監査ログに残す。
type UserAdmin struct {
	clock     clock.Clock
	recorder  audit.Recorder
	policy    *password.Policy
	passwords *Passwords
}

// NewUserAdmin は UserAdmin を返す。policy で、管理者が設定するパスワードの規定を確かめる。
func NewUserAdmin(clk clock.Clock, recorder audit.Recorder, policy *password.Policy) *UserAdmin {
	return &UserAdmin{clock: clk, recorder: recorder, policy: policy, passwords: NewPasswords(clk, recorder, policy)}
}

// NewAccount は、管理者が作るユーザーの項目。
type NewAccount struct {
	Username    string
	DisplayName string
	Role        Role
	// Password は、ローカルアカウントのパスワード。空なら、パスワードを持たないユーザー (OIDC やパスキーだけで使う) にする。
	Password string
}

// UserChanges は、ユーザーの変える項目。nil の項目は変えない。
type UserChanges struct {
	DisplayName *string
	Role        *Role
}

// List は、すべてのユーザーを、作った順に返す。
func (a *UserAdmin) List(ctx context.Context, db sqlc.DBTX) ([]*User, error) {
	rows, err := sqlc.New(db).ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	users := make([]*User, 0, len(rows))
	for i := range rows {
		user, err := userOf(&rows[i])
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, nil
}

// Create はユーザーを作る。パスワードを渡したときは、規定を満たすかを確かめてから、ハッシュにして保存する。
func (a *UserAdmin) Create(ctx context.Context, db Database, actor *audit.Actor, account *NewAccount) (*User, error) {
	if account == nil {
		return nil, fmt.Errorf("%w: account is nil", ErrInvalidUser)
	}
	var hash *string
	if account.Password != "" {
		// パスワードの規定の確認とハッシュ化は時間がかかるので、トランザクションの外で行う。
		if err := a.policy.Check(ctx, account.Password, []string{account.Username, account.DisplayName}); err != nil {
			return nil, err
		}
		encoded, err := password.Hash(account.Password)
		if err != nil {
			return nil, err
		}
		hash = &encoded
	}

	var created *User
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		var err error
		created, err = NewUsers(a.clock).Create(ctx, tx, &NewUser{
			Username: account.Username, DisplayName: account.DisplayName, Role: account.Role, PasswordHash: hash,
		})
		if err != nil {
			return err
		}

		return a.record(ctx, tx, actor, "user.created", created.ID, map[string]any{
			summaryUsername: created.Username, summaryRole: string(created.Role), "hasPassword": hash != nil,
		})
	})
	if err != nil {
		return nil, err
	}

	return created, nil
}

// Update は、ユーザーの表示名とロールを変える。最後の有効な管理者を、管理者から外せない。
// 何も変わらなければ、保存も監査ログへの記録もしない。
func (a *UserAdmin) Update(ctx context.Context, db Database, actor *audit.Actor, userID uuid.UUID, changes *UserChanges) (*User, error) {
	if changes == nil {
		return nil, fmt.Errorf("%w: changes is nil", ErrInvalidUser)
	}
	var updated *User
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, admins, err := a.lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		current, err := userOf(row)
		if err != nil {
			return err
		}
		next := *current
		if changes.DisplayName != nil {
			next.DisplayName = *changes.DisplayName
		}
		if changes.Role != nil {
			next.Role = *changes.Role
		}
		if err := validateNewUser(&NewUser{Username: next.Username, DisplayName: next.DisplayName, Role: next.Role, PasswordHash: nil}); err != nil {
			return err
		}
		if next.DisplayName == current.DisplayName && next.Role == current.Role {
			updated = current

			return nil
		}
		if current.Role == RoleAdmin && current.DisabledAt == nil && next.Role != RoleAdmin && admins <= 1 {
			return ErrLastAdmin
		}
		saved, err := sqlc.New(tx).UpdateUserProfile(ctx, sqlc.UpdateUserProfileParams{
			ID: userID, DisplayName: next.DisplayName, Role: string(next.Role), Now: a.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to update the user: %w", err)
		}
		if updated, err = userOf(&saved); err != nil {
			return err
		}

		return a.record(ctx, tx, actor, "user.updated", userID, map[string]any{
			summaryUsername: current.Username,
			"displayName":   map[string]string{"from": current.DisplayName, "to": next.DisplayName},
			summaryRole:     map[string]string{"from": string(current.Role), "to": string(next.Role)},
		})
	})
	if err != nil {
		return nil, err
	}

	return updated, nil
}

// Disable は、ユーザーを無効にし、そのセッションを失効させる。有効に戻しても、前のセッションは使えない。
// 自分自身と、最後の有効な管理者は無効にできない。by は、操作するユーザー。
func (a *UserAdmin) Disable(ctx context.Context, db Database, actor *audit.Actor, by, userID uuid.UUID) error {
	if by == userID {
		return ErrCannotDisableSelf
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, admins, err := a.lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if row.DisabledAt != nil {
			// 既に無効なら、何もしない。
			return nil
		}
		if row.Role == string(RoleAdmin) && admins <= 1 {
			return ErrLastAdmin
		}
		if err := NewUsers(a.clock).Disable(ctx, tx, userID); err != nil {
			return err
		}
		if err := revokeUserSessions(ctx, tx, userID, nil, a.clock.Now()); err != nil {
			return err
		}

		return a.record(ctx, tx, actor, "user.disabled", userID, map[string]any{summaryUsername: row.Username})
	})
}

// Enable は、無効にしたユーザーを有効に戻す。既に有効なら、何もしない。
func (a *UserAdmin) Enable(ctx context.Context, db Database, actor *audit.Actor, userID uuid.UUID) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		row, err := q.GetUserForUpdate(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrUserNotFound, userID)
		}
		if err != nil {
			return fmt.Errorf("failed to get the user: %w", err)
		}
		n, err := q.EnableUser(ctx, sqlc.EnableUserParams{ID: userID, Now: a.clock.Now()})
		if err != nil {
			return fmt.Errorf("failed to enable the user: %w", err)
		}
		if n == 0 {
			return nil
		}

		return a.record(ctx, tx, actor, "user.enabled", userID, map[string]any{summaryUsername: row.Username})
	})
}

// ResetPassword は、管理者がユーザーのパスワードを設定し直す。規定を満たさなければ、*password.RejectedError を返す。
func (a *UserAdmin) ResetPassword(ctx context.Context, db Database, actor *audit.Actor, userID uuid.UUID, secret string) error {
	return a.passwords.Set(ctx, db, actor, userID, secret, nil)
}

// Delete は、管理者の操作でユーザーを消し、個人のデータも消す (#1382)。自分自身と、最後の有効な管理者は消せない。by は、操作するユーザー。
func (a *UserAdmin) Delete(ctx context.Context, db Database, actor *audit.Actor, by, userID uuid.UUID, erasers ...UserDataEraser) error {
	if by == userID {
		return ErrCannotDeleteSelf
	}

	return a.erase(ctx, db, actor, userID, erasers)
}

// DeleteSelf は、本人の操作で自分のアカウントを消し、個人のデータも消す (#1382)。最後の有効な管理者は消せない。
func (a *UserAdmin) DeleteSelf(ctx context.Context, db Database, actor *audit.Actor, userID uuid.UUID, erasers ...UserDataEraser) error {
	return a.erase(ctx, db, actor, userID, erasers)
}

// erase は、ユーザーの行を消す。個人のデータは外部キーの ON DELETE CASCADE で一緒に消え、監査ログは残す (設計書 8.5)。
func (a *UserAdmin) erase(ctx context.Context, db Database, actor *audit.Actor, userID uuid.UUID, erasers []UserDataEraser) error {
	finishers := make([]func(context.Context), 0, len(erasers))
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		// ユーザーの行をロックしてから読むので、キャプチャの追加のように行を参照する書き込みは、消し終わるまで待つ。
		row, admins, err := a.lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if row.Role == string(RoleAdmin) && row.DisabledAt == nil && admins <= 1 {
			return ErrLastAdmin
		}
		for _, eraser := range erasers {
			finish, err := eraser.PrepareUserErasure(ctx, tx, userID)
			if err != nil {
				return err
			}
			finishers = append(finishers, finish)
		}
		q := sqlc.New(tx)
		if err := q.RemoveUserFromNotificationAudiences(ctx, userID); err != nil {
			return fmt.Errorf("failed to remove the user from the notification audiences: %w", err)
		}
		if _, err := q.DeleteUser(ctx, userID); err != nil {
			return fmt.Errorf("failed to delete the user: %w", err)
		}

		return a.record(ctx, tx, actor, "user.deleted", userID, map[string]any{summaryUsername: row.Username})
	})
	if err != nil {
		return err
	}
	// 本人の操作では、セッションごと消えてリクエストが切れることがあるので、切られない context で片付ける。
	cleanup := context.WithoutCancel(ctx)
	for _, finish := range finishers {
		finish(cleanup)
	}

	return nil
}

// lockUser は、有効な管理者の行と、対象のユーザーの行をロックし、対象のユーザーと、有効な管理者の数を返す。
// 管理者の行を先にロックするので、管理者を同時に外したり無効にしたりして、1 人もいなくなることはない。
func (a *UserAdmin) lockUser(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (*sqlc.User, int, error) {
	q := sqlc.New(tx)
	admins, err := q.LockActiveAdmins(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to lock administrators: %w", err)
	}
	row, err := q.GetUserForUpdate(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, fmt.Errorf("%w: %s", ErrUserNotFound, userID)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get the user: %w", err)
	}

	return &row, len(admins), nil
}

func (a *UserAdmin) record(ctx context.Context, tx pgx.Tx, actor *audit.Actor, action string, userID uuid.UUID, summary map[string]any) error {
	return a.recorder.Record(ctx, tx, &audit.Event{
		Actor:      actor,
		Action:     action,
		TargetType: loginAuditTarget,
		TargetID:   userID.String(),
		Summary:    summary,
		// 送信元は、操作者 (actor) に持たせる。
		SourceIP:  netip.Addr{},
		UserAgent: "",
	})
}

// userOf は、DB の行を User にする。
func userOf(row *sqlc.User) (*User, error) {
	role, err := ParseRole(row.Role)
	if err != nil {
		return nil, err
	}

	return &User{
		ID: row.ID, Username: row.Username, DisplayName: row.DisplayName, Role: role, DisabledAt: row.DisabledAt, CreatedAt: row.CreatedAt,
	}, nil
}
