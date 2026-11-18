package dramarecord

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// session は、1 人のユーザーと 1 つの記録先の連携で、記録先に問い合わせる。アクセストークンが切れていれば更新する。
type session struct {
	service     *Service
	db          Database
	userID      uuid.UUID
	name        string
	destination Destination
	app         *App
	// access は、いま使うアクセストークン。
	access string
}

// open は、ユーザー userID の記録先 name の連携で問い合わせる session を返す。アクセストークンの期限が近ければ、更新してから返す。
// 連携が無効か、アプリがないか、連携していなければ、ErrPermanent を包んで返す。連携し直す必要があれば ErrReauthorizationRequired を返す。
func (s *Service) open(ctx context.Context, db Database, userID uuid.UUID, name string) (*session, error) {
	destination, err := s.destination(name)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	app, err := s.app(ctx, db, name)
	if errors.Is(err, ErrUnavailable) {
		return nil, fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	if err != nil {
		return nil, err
	}
	row, err := sqlc.New(db).GetDramaRecordConnection(ctx, sqlc.GetDramaRecordConnectionParams{UserID: userID, Destination: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %w", ErrPermanent, ErrNotConnected)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the drama record connection: %w", err)
	}
	if row.ReauthorizationRequired {
		return nil, ErrReauthorizationRequired
	}
	access, err := s.tokenBox.Open(row.AccessTokenEncrypted)
	if err != nil {
		return nil, err
	}
	c := &session{service: s, db: db, userID: userID, name: name, destination: destination, app: app, access: string(access)}
	if row.AccessTokenExpiresAt.Sub(s.clock.Now()) <= refreshMargin {
		if err := c.refresh(ctx, false); err != nil {
			return nil, err
		}
	}

	return c, nil
}

// refresh は、アクセストークンを更新する。Simkl は、更新すると前のアクセストークンをすぐに無効にするので、連携の行をロックして
// 読み直し、ほかのプロセスがもう更新していれば、そのトークンを使う。force でなければ、期限が近いときだけ更新する。
// 更新のトークンが断られたら、連携し直すまで問い合わせないよう、連携に印を付けて ErrReauthorizationRequired を返す。
func (c *session) refresh(ctx context.Context, force bool) error {
	s := c.service
	used := c.access
	err := pgx.BeginFunc(ctx, c.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		row, err := q.LockDramaRecordConnection(ctx, sqlc.LockDramaRecordConnectionParams{UserID: c.userID, Destination: c.name})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %w", ErrPermanent, ErrNotConnected)
		}
		if err != nil {
			return fmt.Errorf("failed to lock the drama record connection: %w", err)
		}
		if row.ReauthorizationRequired {
			return ErrReauthorizationRequired
		}
		current, err := s.tokenBox.Open(row.AccessTokenEncrypted)
		if err != nil {
			return err
		}
		// ほかのプロセスが、ロックを取る前に更新していた。
		if string(current) != used {
			c.access = string(current)

			return nil
		}
		if !force && row.AccessTokenExpiresAt.Sub(s.clock.Now()) > refreshMargin {
			return nil
		}
		refreshToken, err := s.tokenBox.Open(row.RefreshTokenEncrypted)
		if err != nil {
			return err
		}
		done, err := s.wait(ctx)
		if err != nil {
			return err
		}
		token, err := c.destination.Refresh(ctx, c.app, string(refreshToken))
		done()
		if err != nil {
			return err
		}
		access, err := s.tokenBox.Seal([]byte(token.AccessToken))
		if err != nil {
			return err
		}
		refresh, err := s.tokenBox.Seal([]byte(token.RefreshToken))
		if err != nil {
			return err
		}
		if err := q.UpdateDramaRecordTokens(ctx, sqlc.UpdateDramaRecordTokensParams{
			AccessTokenEncrypted: access, RefreshTokenEncrypted: refresh, AccessTokenExpiresAt: s.clock.Now().Add(token.ExpiresIn),
			UserID: c.userID, Destination: c.name,
		}); err != nil {
			return fmt.Errorf("failed to save the drama record tokens: %w", err)
		}
		c.access = token.AccessToken

		return nil
	})
	if errors.Is(err, ErrReauthorizationRequired) {
		// 印は、ロールバックしたトランザクションの外で付ける。
		if markErr := sqlc.New(c.db).RequireDramaRecordReauthorization(context.WithoutCancel(ctx), sqlc.RequireDramaRecordReauthorizationParams{
			UserID: c.userID, Destination: c.name,
		}); markErr != nil {
			return fmt.Errorf("failed to mark the drama record connection: %w", markErr)
		}
	}

	return err
}

// do は、アクセストークンで fn を呼ぶ。問い合わせは、このプロセスの中で 1 つずつ、間を空けて送る。
// 記録先がトークンを断ったら (401)、更新して 1 回だけ送り直す。ほかのプロセスが先に更新していれば、更新せずにそのトークンを使う。
// 送り直しても断られたら、ErrPermanent を包んで返す。
func (c *session) do(ctx context.Context, fn func(ctx context.Context, access string) error) error {
	err := c.once(ctx, fn)
	if !errors.Is(err, ErrUnauthorized) {
		return err
	}
	if err := c.refresh(ctx, true); err != nil {
		return err
	}
	if err := c.once(ctx, fn); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return fmt.Errorf("%w: %w", ErrPermanent, err)
		}

		return err
	}

	return nil
}

func (c *session) once(ctx context.Context, fn func(ctx context.Context, access string) error) error {
	done, err := c.service.wait(ctx)
	if err != nil {
		return err
	}
	defer done()

	return fn(ctx, c.access)
}
