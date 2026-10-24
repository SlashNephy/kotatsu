package usersettings

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
)

// pgUniqueViolation は、一意制約の違反を表す PostgreSQL のエラーコード。
const pgUniqueViolation = "23505"

var (
	// ErrGuideViewNotFound は、ユーザーの番組表ビューがないことを表す。
	ErrGuideViewNotFound = errors.New("guide view not found")
	// ErrDuplicateGuideViewName は、同じ名前の番組表ビューがすでにあることを表す。
	ErrDuplicateGuideViewName = errors.New("a guide view with the same name already exists")
)

// Beginner はトランザクションを始められるもの。*pgxpool.Pool と pgx.Tx が満たす。
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// GuideView は、番組表ビューの内容。Settings は、条件を JSON にしたもの。
type GuideView struct {
	Name              string
	Settings          []byte
	HideEmptyServices bool
}

// SaveGuideView は、ユーザー userID の番組表ビューを作るか、viewID のものを置き換え、同じユーザーのほかの端末に知らせる。
// viewID が nil なら作る。作るか置き換えた番組表ビューの ID を返す。
// viewID の番組表ビューがなければ ErrGuideViewNotFound を、同じ名前のものがあれば ErrDuplicateGuideViewName を返す。
func SaveGuideView(ctx context.Context, db Beginner, clk clock.Clock, userID uuid.UUID, viewID *uuid.UUID, view *GuideView) (uuid.UUID, error) {
	var saved uuid.UUID
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		now := clk.Now()
		if viewID == nil {
			saved = id.New()
			err := q.InsertGuideView(ctx, sqlc.InsertGuideViewParams{
				ID: saved, UserID: userID, Name: view.Name, Settings: view.Settings, HideEmptyServices: view.HideEmptyServices, Now: now,
			})
			if err != nil {
				return err
			}
		} else {
			saved = *viewID
			n, err := q.UpdateGuideView(ctx, sqlc.UpdateGuideViewParams{
				ID: saved, UserID: userID, Name: view.Name, Settings: view.Settings, HideEmptyServices: view.HideEmptyServices, Now: now,
			})
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrGuideViewNotFound
			}
		}

		return Touch(ctx, tx, clk, userID)
	})
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgUniqueViolation {
		return uuid.Nil, ErrDuplicateGuideViewName
	}
	if errors.Is(err, ErrGuideViewNotFound) {
		return uuid.Nil, err
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to save the guide view: %w", err)
	}

	return saved, nil
}

// DeleteGuideView は、ユーザー userID の番組表ビュー viewID を消し、同じユーザーのほかの端末に知らせる。
// なければ ErrGuideViewNotFound を返す。
func DeleteGuideView(ctx context.Context, db Beginner, clk clock.Clock, userID, viewID uuid.UUID) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		n, err := sqlc.New(tx).DeleteGuideView(ctx, sqlc.DeleteGuideViewParams{ID: viewID, UserID: userID})
		if err != nil {
			return fmt.Errorf("failed to delete the guide view: %w", err)
		}
		if n == 0 {
			return ErrGuideViewNotFound
		}

		return Touch(ctx, tx, clk, userID)
	})
}

// SetServicePinned は、ユーザー userID の、サービス serviceID のピン留めを付けるか外し、同じユーザーのほかの端末に知らせる。
func SetServicePinned(ctx context.Context, db Beginner, clk clock.Clock, userID uuid.UUID, serviceID int64, pinned bool) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		var err error
		if pinned {
			err = q.PinService(ctx, sqlc.PinServiceParams{UserID: userID, ServiceID: serviceID, Now: clk.Now()})
		} else {
			err = q.UnpinService(ctx, sqlc.UnpinServiceParams{UserID: userID, ServiceID: serviceID})
		}
		if err != nil {
			return fmt.Errorf("failed to change the pinned service: %w", err)
		}

		return Touch(ctx, tx, clk, userID)
	})
}
