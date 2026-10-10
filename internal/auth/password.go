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

// Passwords は、ローカルアカウントのパスワードを設定する (設計書 9.3)。
type Passwords struct {
	clock    clock.Clock
	recorder audit.Recorder
	policy   *password.Policy
}

// NewPasswords は Passwords を返す。policy でパスワードの規定を確かめる。
func NewPasswords(clk clock.Clock, recorder audit.Recorder, policy *password.Policy) *Passwords {
	return &Passwords{clock: clk, recorder: recorder, policy: policy}
}

// Set は、パスワードが規定を満たすかを確かめ、Argon2id でハッシュ化して保存する。
// 規定を満たさなければ、*password.RejectedError を返す。
// ユーザー名と表示名は、パスワードに含まれると推測しやすい語として、強度の判定に渡す。
func (p *Passwords) Set(ctx context.Context, db Beginner, actor *audit.Actor, userID uuid.UUID, secret string) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		user, err := q.GetUser(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrUserNotFound, userID)
		}
		if err != nil {
			return fmt.Errorf("failed to get the user: %w", err)
		}
		if err := p.policy.Check(ctx, secret, []string{user.Username, user.DisplayName}); err != nil {
			return err
		}
		encoded, err := password.Hash(secret)
		if err != nil {
			return err
		}
		if _, err := q.SetUserPassword(ctx, sqlc.SetUserPasswordParams{ID: userID, PasswordHash: &encoded, Now: p.clock.Now()}); err != nil {
			return fmt.Errorf("failed to set the password: %w", err)
		}

		return p.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "user.password_set",
			TargetType: loginAuditTarget,
			TargetID:   userID.String(),
			Summary:    map[string]any{},
			// 送信元は、操作者 (actor) に持たせる。
			SourceIP:  netip.Addr{},
			UserAgent: "",
		})
	})
}

// UserIDByUsername は、ユーザー名 (大文字と小文字を区別しない) からユーザーの ID を引く。
func UserIDByUsername(ctx context.Context, db sqlc.DBTX, username string) (uuid.UUID, error) {
	user, err := sqlc.New(db).GetUserByUsername(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("%w: %s", ErrUserNotFound, username)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to get the user: %w", err)
	}

	return user.ID, nil
}
