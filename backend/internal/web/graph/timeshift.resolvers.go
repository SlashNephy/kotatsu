package graph

import (
	"context"
	"time"

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

// PromoteTimeshiftProgram is the resolver for the promoteTimeshiftProgram field.
func (r *mutationResolver) PromoteTimeshiftProgram(ctx context.Context, serviceID string, startAt time.Time) (*model.TimeshiftPromotion, error) {
	return r.promoteTimeshiftProgram(ctx, serviceID, startAt)
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

// TimeshiftProgram is the resolver for the timeshiftProgram field.
func (r *queryResolver) TimeshiftProgram(ctx context.Context, serviceID string, startAt time.Time) (*model.TimeshiftProgram, error) {
	return r.timeshiftProgram(ctx, serviceID, startAt)
}

// TimeshiftPrograms is the resolver for the timeshiftPrograms field.
func (r *serviceResolver) TimeshiftPrograms(ctx context.Context, obj *model.Service) ([]*model.TimeshiftProgram, error) {
	return r.timeshiftPrograms(ctx, obj.MirakurunID())
}

// TimeshiftBuffer is the resolver for the timeshiftBuffer field.
func (r *serviceResolver) TimeshiftBuffer(ctx context.Context, obj *model.Service) (*model.TimeshiftBuffer, error) {
	buffer, ok, err := r.timeshiftBuffer(ctx, obj.MirakurunID())
	if err != nil || !ok {
		return nil, err
	}

	return buffer, nil
}

// Service is the resolver for the service field.
func (r *timeshiftChannelResolver) Service(ctx context.Context, obj *model.TimeshiftChannel) (*model.Service, error) {
	row, err := loadersFrom(ctx).Services.Load(ctx, obj.ServiceKey)
	if err != nil {
		return nil, err
	}

	return toService(row), nil
}

// Service is the resolver for the service field.
func (r *timeshiftProgramResolver) Service(ctx context.Context, obj *model.TimeshiftProgram) (*model.Service, error) {
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

// TimeshiftProgram returns generated.TimeshiftProgramResolver implementation.
func (r *Resolver) TimeshiftProgram() generated.TimeshiftProgramResolver {
	return &timeshiftProgramResolver{r}
}

type (
	timeshiftChannelResolver struct{ *Resolver }
	timeshiftProgramResolver struct{ *Resolver }
)
