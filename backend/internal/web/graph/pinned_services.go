package graph

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usersettings"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// pinnedServices は、ログイン中のユーザーがピン留めしたサービスを、ピン留めした順に返す (#485)。
func (r *Resolver) pinnedServices(ctx context.Context) ([]*model.Service, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}

	return r.pinnedServicesOf(ctx, sqlc.New(r.db), principal.UserID)
}

// setServicePinned は、サービスをピン留めするか外し、ほかの端末に知らせる。
func (r *Resolver) setServicePinned(ctx context.Context, globalID string, pinned bool) ([]*model.Service, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	serviceID, err := parseServiceGlobalID(globalID)
	if err != nil {
		return nil, err
	}
	if _, err := loadersFrom(ctx).Services.Load(ctx, serviceID); err != nil {
		return nil, NewError(CodeNotFound, "service not found")
	}
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if pinned {
			err = q.PinService(ctx, sqlc.PinServiceParams{UserID: principal.UserID, ServiceID: serviceID, Now: r.clock.Now()})
		} else {
			err = q.UnpinService(ctx, sqlc.UnpinServiceParams{UserID: principal.UserID, ServiceID: serviceID})
		}
		if err != nil {
			return fmt.Errorf("failed to change the pinned service: %w", err)
		}

		return usersettings.Touch(ctx, tx, r.clock, principal.UserID)
	})
	if err != nil {
		return nil, err
	}

	return r.pinnedServicesOf(ctx, sqlc.New(r.db), principal.UserID)
}

// pinnedServicesOf は、ユーザー userID がピン留めしたサービスを、ピン留めした順に返す。
func (r *Resolver) pinnedServicesOf(ctx context.Context, q *sqlc.Queries, userID uuid.UUID) ([]*model.Service, error) {
	ids, err := q.ListPinnedServiceIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the pinned services: %w", err)
	}

	return r.loadServices(ctx, ids)
}
