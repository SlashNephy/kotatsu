package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

var (
	// ErrUserUnusable は、署名付き URL を発行したユーザーがいないか、無効になったか、URL を取り消したことを表す。
	ErrUserUnusable = errors.New("the user is missing or disabled, or the URL is revoked")
	// ErrDatabaseUnavailable は、DB に届かず、覚えた結果もないので、確かめられないことを表す (#1084)。
	ErrDatabaseUnavailable = errors.New("the database is unavailable")
)

// URLUsers は、署名付き URL の記録 (#1757) から、URL を発行したユーザーを引く (設計書 9.6)。
// DB に届かなければ、database.FallbackTTL の中で引けた URL に限って、覚えた結果を使う (#1084)。
type URLUsers struct {
	db       sqlc.DBTX
	clock    clock.Clock
	fallback *database.Fallback[uuid.UUID, urlUser]
}

// urlUser は、URL の記録から引いた、発行したユーザー。
type urlUser struct {
	userID     uuid.UUID
	role       domain.Role
	lastUsedAt *time.Time
}

// urlTouchInterval は、URL が開かれた日時を書き直す間隔。開くたびに書かないようにする。
// 流している間も、確かめ直すたびに (StreamURLActiveWindow より十分に短く) 書き直すので、使われ続けている URL が分かる。
const urlTouchInterval = time.Minute

// NewURLUsers は URLUsers を返す。logger には、DB に届かずに覚えた結果を使ったことを出す。
func NewURLUsers(db sqlc.DBTX, clk clock.Clock, logger *slog.Logger) *URLUsers {
	return &URLUsers{db: db, clock: clk, fallback: database.NewFallback[uuid.UUID, urlUser]("signed_url_users", database.FallbackTTL, clk, logger)}
}

// Role は、URL の記録 urlID から、URL を発行したユーザーのロールを返す。記録がないか (取り消したか)、記録のユーザーが userID でないか、
// ユーザーを無効にしたなら ErrUserUnusable を返す。DB に届かず、覚えた結果もなければ、ErrDatabaseUnavailable を包んで返す。
// あわせて、記録に使われた日時を残す。開くときと、流している間に確かめ直すときに呼ぶので、使われ続けている URL が分かる (#1757)。
// 書き込みを減らすため urlTouchInterval ごとにしか書かず、DB から引けたときだけ、短い上限の時間で書く。書けなくても配信は止めない。
func (u *URLUsers) Role(ctx context.Context, urlID, userID uuid.UUID) (domain.Role, error) {
	found, fromCache, err := u.lookup(ctx, urlID)
	if err != nil {
		return "", err
	}
	if found.userID != userID {
		return "", ErrUserUnusable
	}
	if !fromCache {
		u.touch(ctx, urlID, found.lastUsedAt)
	}

	return found.role, nil
}

// Forget は、ユーザー userID の URL について覚えた結果を忘れる。ユーザーを無効にしたときや、ロールを変えたときに呼ぶ。
func (u *URLUsers) Forget(userID uuid.UUID) {
	u.fallback.ForgetIf(func(found urlUser) bool { return found.userID == userID })
}

// ForgetURL は、URL urlID について覚えた結果を忘れる。URL を取り消したときに呼ぶ。
func (u *URLUsers) ForgetURL(urlID uuid.UUID) {
	u.fallback.Forget(urlID)
}

// FallbackUsed は、DB に届かずに、覚えた結果を使った回数を返す。
func (u *URLUsers) FallbackUsed() int64 {
	return u.fallback.Used()
}

func (u *URLUsers) lookup(ctx context.Context, urlID uuid.UUID) (urlUser, bool, error) {
	found, fromCache, err := u.fallback.Load(ctx, urlID, func(ctx context.Context) (urlUser, error) {
		row, err := sqlc.New(u.db).GetStreamURLUser(ctx, urlID)
		if errors.Is(err, pgx.ErrNoRows) {
			return urlUser{}, ErrUserUnusable
		}
		if err != nil {
			return urlUser{}, fmt.Errorf("failed to get the stream URL: %w", err)
		}

		return urlUser{userID: row.UserID, role: row.Role, lastUsedAt: row.LastUsedAt}, nil
	})
	if database.IsUnavailable(err) {
		return urlUser{}, false, fmt.Errorf("%w: %w", ErrDatabaseUnavailable, err)
	}

	return found, fromCache, err
}

// touch は、URL の記録 urlID に、使われた日時を残す。前に残した日時 lastUsedAt から urlTouchInterval が経っていなければ書かない。
func (u *URLUsers) touch(ctx context.Context, urlID uuid.UUID, lastUsedAt *time.Time) {
	now := u.clock.Now()
	if lastUsedAt != nil && now.Sub(*lastUsedAt) < urlTouchInterval {
		return
	}
	touchCtx, cancel := context.WithTimeout(ctx, database.FallbackQueryTimeout)
	defer cancel()
	// 一覧に出すだけの日時なので、書けなくても配信は止めない。
	_ = sqlc.New(u.db).TouchStreamURL(touchCtx, sqlc.TouchStreamURLParams{ID: urlID, Now: &now})
}
