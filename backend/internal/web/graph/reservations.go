package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	reservationType = "Reservation"
	// maxReservations は、予約の一覧で一度に返す件数の上限。
	maxReservations = 100
)

// reservationCursor は、予約の一覧のカーソルの位置。並べ替えのキーと同じ。
type reservationCursor struct {
	StartAt time.Time `json:"startAt"`
	ID      uuid.UUID `json:"id"`
}

// parseReservationID は、Reservation の ID から、予約の ID を取り出す。
func parseReservationID(globalID string) (uuid.UUID, error) {
	typeName, raw, err := ParseGlobalID(globalID)
	if err != nil || typeName != reservationType {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid reservation ID")
	}
	reservationID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid reservation ID")
	}

	return reservationID, nil
}

// toReservations は、予約を GraphQL の型にする。作成者と更新者は、まとめて引く。
func toReservations(ctx context.Context, reservations ...*reserve.Reservation) ([]*model.Reservation, error) {
	ids := make([]*uuid.UUID, 0, 2*len(reservations))
	for _, r := range reservations {
		ids = append(ids, r.CreatedBy, r.UpdatedBy)
	}
	byID, err := users(ctx, ids...)
	if err != nil {
		return nil, err
	}

	models := make([]*model.Reservation, 0, len(reservations))
	for _, r := range reservations {
		m, err := toReservation(r)
		if err != nil {
			return nil, err
		}
		m.CreatedBy, m.UpdatedBy = userOf(byID, r.CreatedBy), userOf(byID, r.UpdatedBy)
		models = append(models, m)
	}

	return models, nil
}

// toReservationModel は、予約 1 件を GraphQL の型にする。
func toReservationModel(ctx context.Context, r *reserve.Reservation) (*model.Reservation, error) {
	models, err := toReservations(ctx, r)
	if err != nil {
		return nil, err
	}

	return models[0], nil
}

func toReservation(r *reserve.Reservation) (*model.Reservation, error) {
	upstreamGenres := []*domain.Genre{}
	if err := json.Unmarshal(r.Genres, &upstreamGenres); err != nil {
		return nil, fmt.Errorf("failed to decode the genres of reservation %s: %w", r.ID, err)
	}
	genres := make([]*model.Genre, 0, len(upstreamGenres))
	for _, g := range upstreamGenres {
		genres = append(genres, &model.Genre{Lv1: g.Lv1, Lv2: g.Lv2})
	}
	var eventID *int
	if r.EventID != nil {
		eventID = new(int(*r.EventID))
	}
	var tuner *int
	if r.Assignment.Tuner != nil {
		tuner = new(int(*r.Assignment.Tuner))
	}

	return &model.Reservation{
		ID:               GlobalID(reservationType, r.ID.String()),
		Kind:             model.ReservationKind(strings.ToUpper(string(r.Kind))),
		EventID:          eventID,
		StartAt:          r.StartAt,
		EndAt:            r.EndAt,
		Name:             r.Name,
		Description:      r.Description,
		Genres:           genres,
		Enabled:          r.Enabled,
		FollowEventRelay: r.FollowEventRelay,
		Settings:         toOverridesModel(r.Overrides),
		Assignment: &model.ReservationAssignment{
			State:          model.AssignmentState(strings.ToUpper(string(r.Assignment.State))),
			Tuner:          tuner,
			PlannedStartAt: r.Assignment.PlannedStartAt,
			PlannedEndAt:   r.Assignment.PlannedEndAt,
		},
		CreatedAt:  r.CreatedAt,
		CreatedBy:  nil,
		UpdatedAt:  r.UpdatedAt,
		UpdatedBy:  nil,
		Key:        r.ID,
		ServiceKey: r.ServiceID,
	}, nil
}

// reservation は、予約を引く。
func (r *Resolver) reservation(ctx context.Context, globalID string) (*model.Reservation, error) {
	reservationID, err := parseReservationID(globalID)
	if err != nil {
		return nil, err
	}
	found, err := r.reservations.Get(ctx, r.db, reservationID)
	if err != nil {
		return nil, err
	}

	return toReservationModel(ctx, found)
}

// listReservations は、時間帯に重なる予約を、開始時刻の順に first 件まで返す。
func (r *Resolver) listReservations(
	ctx context.Context,
	from, to time.Time,
	states []model.AssignmentState,
	enabled *bool,
	ruleGlobalID *string,
	first int,
	after *string,
) (*model.ReservationConnection, error) {
	if first < 1 || first > maxReservations {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("first must be between 1 and %d", maxReservations))
	}
	query := &reserve.ListQuery{
		From: from, To: to, States: make([]reserve.AssignmentState, 0, len(states)), Enabled: enabled, RuleID: nil, AfterStartAt: nil, AfterID: nil,
		// 次のページがあるかを知るため、1 件多く読む。
		Limit: first + 1,
	}
	for _, state := range states {
		query.States = append(query.States, reserve.AssignmentState(strings.ToLower(string(state))))
	}
	if ruleGlobalID != nil {
		ruleID, err := parseRuleID(*ruleGlobalID)
		if err != nil {
			return nil, err
		}
		query.RuleID = &ruleID
	}
	if after != nil {
		var cursor reservationCursor
		if err := DecodeCursor(*after, &cursor); err != nil {
			return nil, NewError(CodeBadUserInput, "invalid cursor")
		}
		query.AfterStartAt, query.AfterID = &cursor.StartAt, &cursor.ID
	}

	found, err := r.reservations.List(ctx, r.db, query)
	if err != nil {
		return nil, err
	}
	hasNext := len(found) > first
	found = found[:min(len(found), first)]
	models, err := toReservations(ctx, found...)
	if err != nil {
		return nil, err
	}
	edges := make([]*model.ReservationEdge, 0, len(models))
	for i, m := range models {
		cursor, err := EncodeCursor(reservationCursor{StartAt: found[i].StartAt, ID: found[i].ID})
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.ReservationEdge{Cursor: cursor, Node: m})
	}

	info := &model.PageInfo{HasNextPage: hasNext, HasPreviousPage: after != nil, StartCursor: nil, EndCursor: nil}
	if len(edges) > 0 {
		info.StartCursor, info.EndCursor = &edges[0].Cursor, &edges[len(edges)-1].Cursor
	}

	return &model.ReservationConnection{Edges: edges, PageInfo: info}, nil
}

// bulkReservations は、予約 ids のそれぞれに apply を行う (#471)。skipRecording なら、録画中の予約は飛ばす。
// 見つからない予約も、飛ばした数に数える。
func (r *Resolver) bulkReservations(
	ctx context.Context, ids []string, skipRecording bool, apply func(actor *audit.Actor, reservationID uuid.UUID) error,
) (*model.BulkOperationPayload, error) {
	actor, err := scheduler(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) > maxBulkOperations {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("ids must be at most %d", maxBulkOperations))
	}
	reservationIDs := make([]uuid.UUID, 0, len(ids))
	for _, globalID := range ids {
		reservationID, err := parseReservationID(globalID)
		if err != nil {
			return nil, err
		}
		reservationIDs = append(reservationIDs, reservationID)
	}
	payload := &model.BulkOperationPayload{Processed: 0, Skipped: 0}
	for _, reservationID := range reservationIDs {
		if skipRecording {
			recording, err := r.queries.IsReservationRecording(ctx, &reservationID)
			if err != nil {
				return nil, fmt.Errorf("failed to check the recording of the reservation: %w", err)
			}
			if recording {
				payload.Skipped++

				continue
			}
		}
		err := apply(actor, reservationID)
		switch {
		case errors.Is(err, reserve.ErrNotFound):
			payload.Skipped++
		case err != nil:
			return nil, err
		default:
			payload.Processed++
		}
	}

	return payload, nil
}

// setReservationsEnabled は、予約 ids をまとめて有効か無効にする。無効にするときは、録画中の予約を飛ばす。
func (r *Resolver) setReservationsEnabled(ctx context.Context, ids []string, enabled bool) (*model.BulkOperationPayload, error) {
	return r.bulkReservations(ctx, ids, !enabled, func(actor *audit.Actor, reservationID uuid.UUID) error {
		_, err := r.reservations.Update(ctx, r.db, actor, reservationID, &reserve.Update{
			Enabled: &enabled, FollowEventRelay: nil, Overrides: nil, StartAt: nil, EndAt: nil, Name: nil,
		})

		return err
	})
}

// deleteReservations は、予約 ids をまとめて消す。ルール予約は消さずに無効にし、録画中の予約は飛ばす。
func (r *Resolver) deleteReservations(ctx context.Context, ids []string) (*model.BulkOperationPayload, error) {
	return r.bulkReservations(ctx, ids, true, func(actor *audit.Actor, reservationID uuid.UUID) error {
		return r.reservations.Delete(ctx, r.db, actor, reservationID)
	})
}

// deleteReservation は、予約を消す。ルール予約は消さずに無効にし、その予約を返す。
func (r *Resolver) deleteReservation(ctx context.Context, globalID string) (*model.DeleteReservationPayload, error) {
	actor, err := scheduler(ctx)
	if err != nil {
		return nil, err
	}
	reservationID, err := parseReservationID(globalID)
	if err != nil {
		return nil, err
	}
	if err := r.reservations.Delete(ctx, r.db, actor, reservationID); err != nil {
		return nil, err
	}

	payload := &model.DeleteReservationPayload{ID: globalID, Reservation: nil}
	disabled, err := r.reservations.Get(ctx, r.db, reservationID)
	if errors.Is(err, reserve.ErrNotFound) {
		return payload, nil
	}
	if err != nil {
		return nil, err
	}
	if payload.Reservation, err = toReservationModel(ctx, disabled); err != nil {
		return nil, err
	}

	return payload, nil
}
