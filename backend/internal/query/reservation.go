package query

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

const (
	// maxReservationListLimit は、予約の一覧で一度に返す件数の上限。
	maxReservationListLimit = 1000
)

// GetReservation は予約を返す。なければ domain.ErrReservationNotFound を返す。
func (q *Queries) GetReservation(ctx context.Context, reservationID uuid.UUID) (*domain.Reservation, error) {
	row, err := sqlc.New(q.pool).GetReservation(ctx, reservationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", domain.ErrReservationNotFound, reservationID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the reservation: %w", err)
	}

	return ReservationFromRow(row)
}

// ReservationListQuery は、予約の一覧の条件。
type ReservationListQuery struct {
	// From と To は時間帯 [From, To)。これに重なる予約を返す。
	From time.Time
	To   time.Time
	// States は、割り当ての状態の集合。空ならすべて。
	States []domain.AssignmentState
	// Enabled は、有効な予約だけ (true) か、無効な予約だけ (false) を返す。nil ならどちらも。
	Enabled *bool
	// RuleID は、このルールに一致した予約だけを返す。nil なら絞らない。
	RuleID *uuid.UUID
	// AfterStartAt と AfterID は、この位置より後ろから返す (開始時刻、ID の順)。最初から返すなら nil。
	AfterStartAt *time.Time
	AfterID      *uuid.UUID
	Limit        int
}

// ListReservations は、時間帯に重なる予約を、開始時刻の順に返す。
func (q *Queries) ListReservations(ctx context.Context, query *ReservationListQuery) ([]*domain.Reservation, error) {
	if query == nil {
		return nil, fmt.Errorf("%w: query must not be nil", domain.ErrInvalidReservation)
	}
	if (query.AfterStartAt == nil) != (query.AfterID == nil) {
		return nil, fmt.Errorf("%w: afterStartAt and afterID must be given together", domain.ErrInvalidReservation)
	}
	if !query.From.Before(query.To) {
		return nil, fmt.Errorf("%w: from must be before to", domain.ErrInvalidReservation)
	}
	if query.Limit < 1 || query.Limit > maxReservationListLimit {
		return nil, fmt.Errorf("%w: limit must be between 1 and %d", domain.ErrInvalidReservation, maxReservationListLimit)
	}
	states := make([]string, 0, len(query.States))
	for _, state := range query.States {
		states = append(states, string(state))
	}
	// 位置を指定しなければ、どの予約よりも前の位置から読む。
	afterStartAt, afterID := time.Time{}, uuid.Nil
	if query.AfterStartAt != nil {
		afterStartAt, afterID = *query.AfterStartAt, *query.AfterID
	}

	rows, err := sqlc.New(q.pool).ListReservations(ctx, sqlc.ListReservationsParams{
		FromAt:       query.From,
		ToAt:         query.To,
		States:       states,
		Enabled:      optionalBool(query.Enabled),
		RuleID:       query.RuleID,
		AfterStartAt: afterStartAt,
		AfterID:      afterID,
		RowLimit:     int32(query.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list reservations: %w", err)
	}

	return reservationsFromRows(rows)
}

// ListReservationsByPrograms は、番組の予約を返す。予約のない番組は結果に含めない。
func (q *Queries) ListReservationsByPrograms(ctx context.Context, keys []domain.ProgramKey) (map[domain.ProgramKey]*domain.Reservation, error) {
	if len(keys) == 0 {
		return map[domain.ProgramKey]*domain.Reservation{}, nil
	}
	serviceIDs := make([]int64, 0, len(keys))
	eventIDs := make([]int64, 0, len(keys))
	for _, key := range keys {
		serviceIDs = append(serviceIDs, key.ServiceID)
		eventIDs = append(eventIDs, key.EventID)
	}
	rows, err := sqlc.New(q.pool).ListReservationsByPrograms(ctx, sqlc.ListReservationsByProgramsParams{ServiceIds: serviceIDs, EventIds: eventIDs})
	if err != nil {
		return nil, fmt.Errorf("failed to list reservations of programs: %w", err)
	}
	reservations, err := reservationsFromRows(rows)
	if err != nil {
		return nil, err
	}

	byProgram := make(map[domain.ProgramKey]*domain.Reservation, len(reservations))
	for _, reservation := range reservations {
		byProgram[domain.ProgramKey{ServiceID: reservation.ServiceID, EventID: *reservation.EventID}] = reservation
	}

	return byProgram, nil
}

// CountUpcomingReservationsByRules は、ルールごとに、一致した予約のうち、now に終わっていないものの件数を返す。無効な予約も数える。
// 予約のないルールは、結果に含めない。
func (q *Queries) CountUpcomingReservationsByRules(ctx context.Context, ruleIDs []uuid.UUID, now time.Time) (map[uuid.UUID]int, error) {
	if len(ruleIDs) == 0 {
		return map[uuid.UUID]int{}, nil
	}
	rows, err := sqlc.New(q.pool).CountUpcomingReservationsByRules(ctx, sqlc.CountUpcomingReservationsByRulesParams{
		RuleIds: ruleIDs, Now: now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to count the reservations of the rules: %w", err)
	}
	counts := make(map[uuid.UUID]int, len(rows))
	for _, row := range rows {
		counts[row.RuleID] = int(row.Reservations)
	}

	return counts, nil
}

// ReservationRuleIDs は、予約に一致したルールの ID を、ルールを作った順に返す。ルールに一致していない予約は結果に含めない。
func (q *Queries) ReservationRuleIDs(ctx context.Context, reservationIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	if len(reservationIDs) == 0 {
		return map[uuid.UUID][]uuid.UUID{}, nil
	}
	rows, err := sqlc.New(q.pool).ListReservationRules(ctx, reservationIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to list the rules of reservations: %w", err)
	}

	ruleIDs := map[uuid.UUID][]uuid.UUID{}
	for _, row := range rows {
		ruleIDs[row.ReservationID] = append(ruleIDs[row.ReservationID], row.RuleID)
	}

	return ruleIDs, nil
}

func reservationsFromRows(rows []sqlc.Reservation) ([]*domain.Reservation, error) {
	reservations := make([]*domain.Reservation, 0, len(rows))
	for _, row := range rows {
		reservation, err := ReservationFromRow(row)
		if err != nil {
			return nil, err
		}
		reservations = append(reservations, reservation)
	}

	return reservations, nil
}

// ReservationFromRow は、予約の行を domain の予約にする。usecase がトランザクションの中で読んだ行にも使う。
func ReservationFromRow(row sqlc.Reservation) (*domain.Reservation, error) {
	overrides, err := domain.DecodeOverrides(row.Settings)
	if err != nil {
		return nil, err
	}

	return &domain.Reservation{
		ID:               row.ID,
		Kind:             domain.ReservationKind(row.Kind),
		ServiceID:        row.ServiceID,
		EventID:          row.EventID,
		StartAt:          row.StartAt.UTC(),
		EndAt:            row.EndAt.UTC(),
		Name:             row.Name,
		Description:      row.Description,
		Extended:         row.Extended,
		Genres:           row.Genres,
		Enabled:          row.Enabled,
		FollowEventRelay: row.FollowEventRelay,
		Overrides:        overrides,
		RelayedFrom:      row.RelayedFrom,
		Assignment: &domain.Assignment{
			State:          domain.AssignmentState(row.AssignmentState),
			Tuner:          row.AssignedTuner,
			PlannedStartAt: utc(row.PlannedStartAt),
			PlannedEndAt:   utc(row.PlannedEndAt),
		},
		CreatedAt: row.CreatedAt.UTC(),
		CreatedBy: row.CreatedBy,
		UpdatedAt: row.UpdatedAt.UTC(),
		UpdatedBy: row.UpdatedBy,
	}, nil
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}

	return new(t.UTC())
}

// optionalBool は、指定しない条件 (nil) を、DB の NULL にする。
func optionalBool(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{Bool: false, Valid: false}
	}

	return pgtype.Bool{Bool: *v, Valid: true}
}
