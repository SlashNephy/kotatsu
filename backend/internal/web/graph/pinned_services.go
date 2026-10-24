package graph

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/usecase/usersettings"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// pinnedServices は、ログイン中のユーザーがピン留めしたサービスを、ピン留めした順に返す (#485)。
func (r *Resolver) pinnedServices(ctx context.Context) ([]*model.Service, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}

	return r.pinnedServicesOf(ctx, principal.UserID)
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
	if err := usersettings.SetServicePinned(ctx, r.db, r.clock, principal.UserID, serviceID, pinned); err != nil {
		return nil, err
	}

	return r.pinnedServicesOf(ctx, principal.UserID)
}

// pinnedServicesOf は、ユーザー userID がピン留めしたサービスを、ピン留めした順に返す。
func (r *Resolver) pinnedServicesOf(ctx context.Context, userID uuid.UUID) ([]*model.Service, error) {
	ids, err := r.queries.ListPinnedServiceIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the pinned services: %w", err)
	}

	return r.loadServices(ctx, ids)
}
