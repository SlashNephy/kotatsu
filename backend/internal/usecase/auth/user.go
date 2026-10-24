package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
)

const (
	maxUsernameLength    = 64
	maxDisplayNameLength = 64
	// pgUniqueViolation は、一意制約の違反を表す PostgreSQL のエラーコード。
	pgUniqueViolation = "23505"
)

var (
	// ErrUsernameTaken は、同じユーザー名 (大文字と小文字を区別しない) のユーザーがいることを表す。
	ErrUsernameTaken = errors.New("username is already taken")
	// ErrInvalidUser は、ユーザーの項目が正しくないことを表す。
	ErrInvalidUser = errors.New("invalid user")
	// ErrUserNotFound は、対象の有効なユーザーがいないことを表す。
	ErrUserNotFound = errors.New("user not found")
)

// User はユーザー。
type User struct {
	ID          uuid.UUID
	Username    string
	DisplayName string
	Role        Role
	// DisabledAt は無効にした日時。有効なら nil。
	DisabledAt *time.Time
	CreatedAt  time.Time
}

// NewUser は、作るユーザーの項目。
type NewUser struct {
	Username    string
	DisplayName string
	Role        Role
	// PasswordHash はパスワードのハッシュ。OIDC だけでログインするユーザーでは nil。
	PasswordHash *string
}

// Users はユーザーを扱う。
type Users struct {
	clock clock.Clock
}

// NewUsers は Users を作る。
func NewUsers(clk clock.Clock) *Users {
	return &Users{clock: clk}
}

// Create はユーザーを作る。
func (u *Users) Create(ctx context.Context, db sqlc.DBTX, user *NewUser) (*User, error) {
	if err := validateNewUser(user); err != nil {
		return nil, err
	}

	created := &User{
		ID:          id.New(),
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		DisabledAt:  nil,
		CreatedAt:   u.clock.Now(),
	}
	err := sqlc.New(db).InsertUser(ctx, sqlc.InsertUserParams{
		ID:           created.ID,
		Username:     created.Username,
		DisplayName:  created.DisplayName,
		Role:         string(created.Role),
		PasswordHash: user.PasswordHash,
		CreatedAt:    created.CreatedAt,
	})
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return nil, fmt.Errorf("%w: %s", ErrUsernameTaken, user.Username)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create a user: %w", err)
	}

	return created, nil
}

// Disable はユーザーを無効にする。無効にしたユーザーのセッションは、その時点から使えなくなる。
// ユーザーがいないか、すでに無効なら ErrUserNotFound を返す。
func (u *Users) Disable(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) error {
	n, err := sqlc.New(db).DisableUser(ctx, sqlc.DisableUserParams{ID: userID, DisabledAt: new(u.clock.Now())})
	if err != nil {
		return fmt.Errorf("failed to disable a user: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrUserNotFound, userID)
	}

	return nil
}

func validateNewUser(user *NewUser) error {
	if user == nil {
		return fmt.Errorf("%w: user is nil", ErrInvalidUser)
	}

	errs := []error{}
	if !validName(user.Username, maxUsernameLength) {
		errs = append(errs, fmt.Errorf("%w: username must be 1 to %d characters without surrounding spaces or control characters", ErrInvalidUser, maxUsernameLength))
	}
	if !validName(user.DisplayName, maxDisplayNameLength) {
		errs = append(errs, fmt.Errorf("%w: display name must be 1 to %d characters without surrounding spaces or control characters", ErrInvalidUser, maxDisplayNameLength))
	}
	if _, err := ParseRole(string(user.Role)); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// validName は、名前が 1 文字以上 maxLength 文字以下で、前後に空白がなく、制御文字を含まないかどうかを返す。
func validName(name string, maxLength int) bool {
	if name == "" || utf8.RuneCountInString(name) > maxLength || strings.TrimSpace(name) != name {
		return false
	}

	return !strings.ContainsFunc(name, unicode.IsControl)
}
