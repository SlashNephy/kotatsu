package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/watch"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// setWatchReservation は、ログイン中のユーザーの、番組の視聴予約を付け外しする (設計書 4.8)。
// 閲覧だけのユーザーも使える。同じユーザーのほかの端末に知らせる。
func (r *Resolver) setWatchReservation(ctx context.Context, programID string, reserved bool) (*model.Program, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	typeName, raw, err := ParseGlobalID(programID)
	if err != nil || typeName != programType {
		return nil, NewError(CodeBadUserInput, "invalid program ID")
	}
	serviceID, eventID, err := parseProgramID(raw)
	if err != nil {
		return nil, err
	}
	key := domain.ProgramKey{ServiceID: serviceID, EventID: eventID}
	err = watch.SetReserved(ctx, r.db, r.clock, principal.UserID, key, reserved)
	if errors.Is(err, watch.ErrProgramUnavailable) {
		return nil, NewError(CodeBadUserInput, "the program is not found or has already started")
	}
	if err != nil {
		return nil, err
	}
	row, err := r.queries.GetGuideProgram(ctx, query.GetGuideProgramParams{ServiceID: serviceID, EventID: eventID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, NewError(CodeNotFound, "program not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the program: %w", err)
	}

	return toProgram(&row)
}

// watchReserved は、ログイン中のユーザーが、番組 obj を視聴予約しているかどうかを返す。過去の番組では false を返す。
func (r *Resolver) watchReserved(ctx context.Context, obj *model.Program) (bool, error) {
	if obj.Archived {
		return false, nil
	}
	principal, err := viewer(ctx)
	if err != nil {
		return false, err
	}

	return loadersFrom(ctx).WatchReserved.Load(ctx, watchReservedKey{
		UserID: principal.UserID, Program: domain.ProgramKey{ServiceID: obj.ServiceKey, EventID: int64(obj.EventID)},
	})
}

// listWatchReservations は、ログイン中のユーザーの視聴予約を、放送の早い順に返す。
func (r *Resolver) listWatchReservations(ctx context.Context) ([]*model.WatchReservation, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	minutes, err := r.queries.WatchReminderMinutes(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	rows, err := r.queries.ListWatchReservations(ctx, principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the watch reservations: %w", err)
	}
	reservations := make([]*model.WatchReservation, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		program, err := toProgram(&domain.Program{
			ServiceID: row.ServiceID, EventID: row.EventID, StartAt: row.StartAt, EndAt: row.EndAt, EndUndecided: row.EndUndecided,
			IsFree: row.IsFree, Name: row.Name, Description: row.Description, Extended: row.Extended, Genres: row.Genres,
			Audios: row.Audios, RelatedItems: row.RelatedItems,
		})
		if err != nil {
			return nil, err
		}
		reservations = append(reservations, &model.WatchReservation{
			Program:   program,
			NotifyAt:  row.StartAt.Add(-time.Duration(minutes) * time.Minute).UTC(),
			CreatedAt: row.CreatedAt.UTC(),
		})
	}

	return reservations, nil
}

// watchReminderSettings は、ログイン中のユーザーの、視聴予約の通知の設定を返す。
func (r *Resolver) watchReminderSettings(ctx context.Context) (*model.WatchReminderSettings, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	minutes, err := r.queries.WatchReminderMinutes(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}

	return &model.WatchReminderSettings{MinutesBefore: minutes}, nil
}

// updateWatchReminderSettings は、視聴予約の通知を放送の何分前に送るかを変え、同じユーザーのほかの端末に知らせる。
func (r *Resolver) updateWatchReminderSettings(ctx context.Context, minutesBefore int) (*model.WatchReminderSettings, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	updated, err := watch.UpdateReminderMinutes(ctx, r.db, r.clock, principal.UserID, minutesBefore)
	if errors.Is(err, watch.ErrInvalidReminderMinutes) {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("minutesBefore must be between 0 and %d", watch.MaxReminderMinutes))
	}
	if err != nil {
		return nil, err
	}

	return &model.WatchReminderSettings{MinutesBefore: updated}, nil
}
