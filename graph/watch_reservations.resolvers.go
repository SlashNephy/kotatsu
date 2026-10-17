package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/graph/model"
)

// SetWatchReservation is the resolver for the setWatchReservation field.
func (r *mutationResolver) SetWatchReservation(ctx context.Context, program string, reserved bool) (*model.Program, error) {
	return r.setWatchReservation(ctx, program, reserved)
}

// UpdateWatchReminderSettings is the resolver for the updateWatchReminderSettings field.
func (r *mutationResolver) UpdateWatchReminderSettings(ctx context.Context, minutesBefore int) (*model.WatchReminderSettings, error) {
	return r.updateWatchReminderSettings(ctx, minutesBefore)
}

// WatchReserved is the resolver for the watchReserved field.
func (r *programResolver) WatchReserved(ctx context.Context, obj *model.Program) (bool, error) {
	return r.watchReserved(ctx, obj)
}

// WatchReservations is the resolver for the watchReservations field.
func (r *queryResolver) WatchReservations(ctx context.Context) ([]*model.WatchReservation, error) {
	return r.listWatchReservations(ctx)
}

// WatchReminderSettings is the resolver for the watchReminderSettings field.
func (r *queryResolver) WatchReminderSettings(ctx context.Context) (*model.WatchReminderSettings, error) {
	return r.watchReminderSettings(ctx)
}
