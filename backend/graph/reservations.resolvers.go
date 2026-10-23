package graph

import (
	"context"
	"time"

	"github.com/SlashNephy/kotatsu/graph/generated"
	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/reserve"
)

// CreateProgramReservation is the resolver for the createProgramReservation field.
func (r *mutationResolver) CreateProgramReservation(ctx context.Context, input model.ProgramReservationInput) (*model.Reservation, error) {
	actor, err := scheduler(ctx)
	if err != nil {
		return nil, err
	}
	typeName, raw, err := ParseGlobalID(input.Program)
	if err != nil || typeName != programType {
		return nil, NewError(CodeBadUserInput, "invalid program ID")
	}
	serviceID, eventID, err := parseProgramID(raw)
	if err != nil {
		return nil, err
	}
	created, _, err := r.reservations.CreateProgram(ctx, r.db, actor, &reserve.ProgramInput{
		ServiceID: serviceID, EventID: eventID, FollowEventRelay: input.FollowEventRelay, Overrides: toOverrides(input.Settings),
	})
	if err != nil {
		return nil, err
	}

	return toReservationModel(ctx, created)
}

// CreateTimeReservation is the resolver for the createTimeReservation field.
func (r *mutationResolver) CreateTimeReservation(ctx context.Context, input model.TimeReservationInput) (*model.Reservation, error) {
	actor, err := scheduler(ctx)
	if err != nil {
		return nil, err
	}
	serviceID, err := parseServiceGlobalID(input.Service)
	if err != nil {
		return nil, err
	}
	created, err := r.reservations.CreateTime(ctx, r.db, actor, &reserve.TimeInput{
		ServiceID: serviceID, StartAt: input.StartAt, EndAt: input.EndAt, Name: input.Name, Overrides: toOverrides(input.Settings),
	})
	if err != nil {
		return nil, err
	}

	return toReservationModel(ctx, created)
}

// UpdateReservation is the resolver for the updateReservation field.
func (r *mutationResolver) UpdateReservation(ctx context.Context, id string, input model.ReservationUpdateInput) (*model.Reservation, error) {
	actor, err := scheduler(ctx)
	if err != nil {
		return nil, err
	}
	reservationID, err := parseReservationID(id)
	if err != nil {
		return nil, err
	}
	update := &reserve.Update{
		Enabled: input.Enabled, FollowEventRelay: input.FollowEventRelay, Overrides: nil,
		StartAt: input.StartAt, EndAt: input.EndAt, Name: input.Name,
	}
	if input.Settings != nil {
		update.Overrides = toOverrides(input.Settings)
	}
	updated, err := r.reservations.Update(ctx, r.db, actor, reservationID, update)
	if err != nil {
		return nil, err
	}

	return toReservationModel(ctx, updated)
}

// DeleteReservation is the resolver for the deleteReservation field.
func (r *mutationResolver) DeleteReservation(ctx context.Context, id string) (*model.DeleteReservationPayload, error) {
	return r.deleteReservation(ctx, id)
}

// SetReservationsEnabled is the resolver for the setReservationsEnabled field.
func (r *mutationResolver) SetReservationsEnabled(ctx context.Context, ids []string, enabled bool) (*model.BulkOperationPayload, error) {
	return r.setReservationsEnabled(ctx, ids, enabled)
}

// DeleteReservations is the resolver for the deleteReservations field.
func (r *mutationResolver) DeleteReservations(ctx context.Context, ids []string) (*model.BulkOperationPayload, error) {
	return r.deleteReservations(ctx, ids)
}

// Reservation is the resolver for the reservation field.
func (r *programResolver) Reservation(ctx context.Context, obj *model.Program) (*model.Reservation, error) {
	found, err := loadersFrom(ctx).ProgramReservations.Load(ctx, reserve.ProgramKey{ServiceID: obj.ServiceKey, EventID: int64(obj.EventID)})
	// アーカイブの番組には、eventId を再利用した別の番組の予約を出さない。
	if err != nil || found == nil || obj.Archived {
		return nil, err
	}

	return toReservationModel(ctx, found)
}

// Reservations is the resolver for the reservations field.
func (r *queryResolver) Reservations(ctx context.Context, from time.Time, to time.Time, states []model.AssignmentState, enabled *bool, rule *string, first int, after *string) (*model.ReservationConnection, error) {
	return r.listReservations(ctx, from, to, states, enabled, rule, first, after)
}

// Service is the resolver for the service field.
func (r *reservationResolver) Service(ctx context.Context, obj *model.Reservation) (*model.Service, error) {
	row, err := loadersFrom(ctx).Services.Load(ctx, obj.ServiceKey)
	if err != nil {
		return nil, err
	}

	return toService(row), nil
}

// Rules is the resolver for the rules field.
func (r *reservationResolver) Rules(ctx context.Context, obj *model.Reservation) ([]*model.Rule, error) {
	ruleIDs, err := loadersFrom(ctx).RuleIDs.Load(ctx, obj.Key)
	if err != nil {
		return nil, err
	}
	found, err := loadersFrom(ctx).Rules.LoadAll(ctx, ruleIDs)
	if err != nil {
		return nil, err
	}

	return toRules(ctx, found...)
}

// Reservation returns generated.ReservationResolver implementation.
func (r *Resolver) Reservation() generated.ReservationResolver { return &reservationResolver{r} }

type reservationResolver struct{ *Resolver }
