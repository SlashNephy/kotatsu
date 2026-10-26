package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/generated"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// SetTimeshiftChannel is the resolver for the setTimeshiftChannel field.
func (r *mutationResolver) SetTimeshiftChannel(ctx context.Context, serviceID string, retentionSeconds *int) (*model.TimeshiftChannel, error) {
	return r.setTimeshiftChannel(ctx, serviceID, retentionSeconds)
}

// RemoveTimeshiftChannel is the resolver for the removeTimeshiftChannel field.
func (r *mutationResolver) RemoveTimeshiftChannel(ctx context.Context, serviceID string) (string, error) {
	return r.removeTimeshiftChannel(ctx, serviceID)
}

// TimeshiftAvailable is the resolver for the timeshiftAvailable field.
func (r *queryResolver) TimeshiftAvailable(ctx context.Context) (bool, error) {
	if err := administer(ctx); err != nil {
		return false, err
	}

	return r.store.HasTimeshiftDir(), nil
}

// TimeshiftChannels is the resolver for the timeshiftChannels field.
func (r *queryResolver) TimeshiftChannels(ctx context.Context) ([]*model.TimeshiftChannel, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}

	return r.timeshiftChannels(ctx)
}

// Service is the resolver for the service field.
func (r *timeshiftChannelResolver) Service(ctx context.Context, obj *model.TimeshiftChannel) (*model.Service, error) {
	row, err := loadersFrom(ctx).Services.Load(ctx, obj.ServiceKey)
	if err != nil {
		return nil, err
	}

	return toService(row), nil
}

// TimeshiftChannel returns generated.TimeshiftChannelResolver implementation.
func (r *Resolver) TimeshiftChannel() generated.TimeshiftChannelResolver {
	return &timeshiftChannelResolver{r}
}

type timeshiftChannelResolver struct{ *Resolver }
