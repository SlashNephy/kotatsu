package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/password"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

// Database は、読み出しと、トランザクションでの書き込みに使う DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Beginner
}

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
//
// 設定したら、ユーザーのほかのセッションを失効させる (設計書 9.5)。パスワードが漏れたと思って変えたときに、漏れた先のセッションを切るためである。
// keepSession が nil でなければ、そのセッション (本人が変えたときの今のセッション) は残す。
func (p *Passwords) Set(ctx context.Context, db Database, actor *audit.Actor, userID uuid.UUID, secret string, keepSession *uuid.UUID) error {
	// 強度の判定、Have I Been Pwned への照会、ハッシュ化は時間がかかるので、トランザクションの外で行う。
	user, err := sqlc.New(db).GetUser(ctx, userID)
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

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		n, err := sqlc.New(tx).SetUserPassword(ctx, sqlc.SetUserPasswordParams{ID: userID, PasswordHash: &encoded, Now: p.clock.Now()})
		if err != nil {
			return fmt.Errorf("failed to set the password: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("%w: %s", ErrUserNotFound, userID)
		}
		if err := revokeUserSessions(ctx, tx, userID, keepSession, p.clock.Now()); err != nil {
			return err
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

// revokeUserSessions は、ユーザーのセッションをまとめて失効させ、第二要素の確認を待つログインも消す。
// keep が nil でなければ、そのセッションは残す。
// 待っているログインを残すと、前のパスワードで確かめたログインを、第二要素を通して完了させられてしまう。
func revokeUserSessions(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, keep *uuid.UUID, now time.Time) error {
	q := sqlc.New(db)
	if _, err := q.RevokeUserSessions(ctx, sqlc.RevokeUserSessionsParams{Now: now, UserID: userID, KeepID: keep}); err != nil {
		return fmt.Errorf("failed to revoke the sessions of the user: %w", err)
	}
	if err := q.DeleteUserLoginChallenges(ctx, userID); err != nil {
		return fmt.Errorf("failed to delete the login challenges of the user: %w", err)
	}

	return nil
}
