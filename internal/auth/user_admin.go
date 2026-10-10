package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/password"
)

var (
	// ErrLastAdmin は、有効な管理者が 1 人もいなくなる操作であることを表す。
	// 管理者がいなくなると、ユーザーやシステムの設定を誰も変えられなくなる。
	ErrLastAdmin = errors.New("at least one enabled administrator is required")
	// ErrCannotDisableSelf は、自分を無効にしようとしたことを表す。
	ErrCannotDisableSelf = errors.New("you cannot disable yourself")
)

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
func (a *UserAdmin) Update(ctx context.Context, db Database, actor *audit.Actor, userID uuid.UUID, changes *UserChanges) (*User, error) {
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
			"displayName": map[string]string{"from": current.DisplayName, "to": next.DisplayName},
			summaryRole:   map[string]string{"from": string(current.Role), "to": string(next.Role)},
		})
	})
	if err != nil {
		return nil, err
	}

	return updated, nil
}

// Disable は、ユーザーを無効にする。無効にしたユーザーのセッションは、その時点から使えなくなる。
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
	return a.passwords.Set(ctx, db, actor, userID, secret)
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
