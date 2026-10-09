// Package reserve は、予約と録画設定を扱う (設計書 4.1、4.7)。
package reserve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
)

// Kind は予約の種類。
type Kind string

const (
	// KindProgram は番組予約。番組の放送時間の変更とイベントリレーに追従する。
	KindProgram Kind = "program"
	// KindTime は時刻予約。サービスと開始と終了の時刻を指定し、番組の変更には追従しない。
	KindTime Kind = "time"
	// KindRule はルール予約。ルールの照合が作る (#19)。
	KindRule Kind = "rule"
)

const (
	// auditTarget は、監査ログの対象の種類。
	auditTarget = "reservation"
	// maxTimeReservation は、時刻予約の長さの上限。
	maxTimeReservation = 24 * time.Hour
	maxNameLength      = 200
)

var (
	// ErrProgramNotFound は、予約しようとした番組が EPG にないことを表す。
	ErrProgramNotFound = errors.New("program not found")
	// ErrNotFound は、予約がないことを表す。
	ErrNotFound = errors.New("reservation not found")
	// ErrInvalidReservation は、予約の内容が正しくないことを表す。
	ErrInvalidReservation = errors.New("invalid reservation")
)

// Reservation は予約。番組予約とルール予約は、作ったときの番組情報のコピーを持つ。
type Reservation struct {
	ID        uuid.UUID
	Kind      Kind
	ServiceID int64
	// EventID は番組の eventId。時刻予約では nil。
	EventID     *int64
	StartAt     time.Time
	EndAt       time.Time
	Name        string
	Description string
	// Extended と Genres は、番組の詳細とジャンルを、EPG の JSON のまま持つ。
	Extended         json.RawMessage
	Genres           json.RawMessage
	Enabled          bool
	FollowEventRelay bool
	// Overrides は、この予約の録画設定の上書き。
	Overrides *Overrides
	CreatedAt time.Time
	CreatedBy *uuid.UUID
	UpdatedAt time.Time
	UpdatedBy *uuid.UUID
}

// Settings は、全体の既定値 defaults に、この予約の上書きを重ねた録画設定を返す。
func (r *Reservation) Settings(defaults *Settings) *Settings {
	return defaults.Resolve(r.Overrides)
}

// Beginner はトランザクションを始められるもの。*pgxpool.Pool と pgx.Tx が満たす。
// 予約の書き込みと監査ログを 1 つのトランザクションにまとめるために使う。
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Reservations は予約を扱う。
type Reservations struct {
	clock    clock.Clock
	recorder audit.Recorder
}

// NewReservations は Reservations を作る。
func NewReservations(clk clock.Clock, recorder audit.Recorder) *Reservations {
	return &Reservations{clock: clk, recorder: recorder}
}

// ProgramInput は、番組予約の内容。
type ProgramInput struct {
	ServiceID        int64
	EventID          int64
	FollowEventRelay bool
	Overrides        *Overrides
}

// CreateProgram は番組予約を作る。番組情報は EPG からコピーする。
// 同じ番組の予約がすでにあれば、作らずにその予約を返し、created を false にする (設計書 4.1)。
func (r *Reservations) CreateProgram(ctx context.Context, db Beginner, actor *audit.Actor, in *ProgramInput) (*Reservation, bool, error) {
	settings, err := EncodeOverrides(in.Overrides)
	if err != nil {
		return nil, false, err
	}

	var reservation *Reservation
	created := false
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		program, err := q.GetProgramForReservation(ctx, sqlc.GetProgramForReservationParams{ServiceID: in.ServiceID, EventID: in.EventID})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: service %d, event %d", ErrProgramNotFound, in.ServiceID, in.EventID)
		}
		if err != nil {
			return fmt.Errorf("failed to look up the program: %w", err)
		}

		_, err = q.InsertProgramReservation(ctx, sqlc.InsertProgramReservationParams{
			ID:               id.New(),
			ServiceID:        program.ServiceID,
			EventID:          &program.EventID,
			StartAt:          program.StartAt,
			EndAt:            program.EndAt,
			Name:             program.Name,
			Description:      program.Description,
			Extended:         program.Extended,
			Genres:           program.Genres,
			FollowEventRelay: in.FollowEventRelay,
			Settings:         settings,
			Now:              r.clock.Now(),
			CreatedBy:        userOf(actor),
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// 同じ番組の予約がすでにある。
		case err != nil:
			return fmt.Errorf("failed to create a reservation: %w", err)
		default:
			created = true
		}

		row, err := q.GetReservationByProgram(ctx, sqlc.GetReservationByProgramParams{ServiceID: in.ServiceID, EventID: &in.EventID})
		if err != nil {
			return fmt.Errorf("failed to read the reservation: %w", err)
		}
		if reservation, err = fromRow(row); err != nil {
			return err
		}
		if !created {
			return nil
		}

		return r.record(ctx, tx, actor, "reservation.create", reservation, summaryOf(reservation))
	})
	if err != nil {
		return nil, false, err
	}

	return reservation, created, nil
}

// CreateRule は、ルールが一致した番組のルール予約を、番組情報をコピーして作る (設計書 4.3)。
// 同じ番組の予約がすでにあれば、作らずにその予約を返し、created を false にする。
// ルール予約の録画設定は、一致したルールから求めるので、予約自体には上書きを持たせない。
func (r *Reservations) CreateRule(ctx context.Context, db Beginner, actor *audit.Actor, serviceID, eventID int64, followEventRelay bool) (*Reservation, bool, error) {
	var reservation *Reservation
	created := false
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		_, err := q.InsertRuleReservation(ctx, sqlc.InsertRuleReservationParams{
			ID:               id.New(),
			FollowEventRelay: followEventRelay,
			Now:              r.clock.Now(),
			ServiceID:        serviceID,
			EventID:          eventID,
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// 同じ番組の予約がすでにあるか、番組が EPG にない。
		case err != nil:
			return fmt.Errorf("failed to create a rule reservation: %w", err)
		default:
			created = true
		}

		row, err := q.GetReservationByProgram(ctx, sqlc.GetReservationByProgramParams{ServiceID: serviceID, EventID: &eventID})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: service %d, event %d", ErrProgramNotFound, serviceID, eventID)
		}
		if err != nil {
			return fmt.Errorf("failed to read the reservation: %w", err)
		}
		if reservation, err = fromRow(row); err != nil {
			return err
		}
		if !created {
			return nil
		}

		return r.record(ctx, tx, actor, "reservation.create", reservation, summaryOf(reservation))
	})
	if err != nil {
		return nil, false, err
	}

	return reservation, created, nil
}

// Remove は、予約を消す。ルールの照合が、どのルールにも一致しなくなったルール予約を消すのに使う。
// ユーザーの操作には Delete を使う。
func (r *Reservations) Remove(ctx context.Context, db Beginner, actor *audit.Actor, reservationID uuid.UUID) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		return r.remove(ctx, tx, actor, reservationID)
	})
}

// TimeInput は、時刻予約の内容。
type TimeInput struct {
	ServiceID int64
	StartAt   time.Time
	EndAt     time.Time
	// Name は、画面に出す名前。空でもよい。
	Name      string
	Overrides *Overrides
}

// CreateTime は時刻予約を作る。
func (r *Reservations) CreateTime(ctx context.Context, db Beginner, actor *audit.Actor, in *TimeInput) (*Reservation, error) {
	if err := validateSchedule(in.StartAt, in.EndAt, in.Name); err != nil {
		return nil, err
	}
	settings, err := EncodeOverrides(in.Overrides)
	if err != nil {
		return nil, err
	}

	var reservation *Reservation
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		reservationID := id.New()
		err := q.InsertTimeReservation(ctx, sqlc.InsertTimeReservationParams{
			ID:        reservationID,
			ServiceID: in.ServiceID,
			StartAt:   in.StartAt,
			EndAt:     in.EndAt,
			Name:      in.Name,
			Settings:  settings,
			Now:       r.clock.Now(),
			CreatedBy: userOf(actor),
		})
		if err != nil {
			return fmt.Errorf("failed to create a reservation: %w", err)
		}
		if reservation, err = get(ctx, q, reservationID); err != nil {
			return err
		}

		return r.record(ctx, tx, actor, "reservation.create", reservation, summaryOf(reservation))
	})
	if err != nil {
		return nil, err
	}

	return reservation, nil
}

// Update は、予約の変更。nil の項目は変えない。
type Update struct {
	Enabled          *bool
	FollowEventRelay *bool
	// Overrides は、録画設定の上書きを丸ごと置き換える。
	Overrides *Overrides
	// StartAt、EndAt、Name は、時刻予約でだけ変えられる。番組予約の時刻と名前は EPG に従う。
	StartAt *time.Time
	EndAt   *time.Time
	Name    *string
}

// Update は予約を変える。
func (r *Reservations) Update(ctx context.Context, db Beginner, actor *audit.Actor, reservationID uuid.UUID, update *Update) (*Reservation, error) {
	if update == nil {
		return nil, fmt.Errorf("%w: update must not be nil", ErrInvalidReservation)
	}

	var reservation *Reservation
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		current, err := get(ctx, q, reservationID)
		if err != nil {
			return err
		}
		next, changed, err := apply(current, update)
		if err != nil {
			return err
		}
		settings, err := EncodeOverrides(next.Overrides)
		if err != nil {
			return err
		}

		n, err := q.UpdateReservation(ctx, sqlc.UpdateReservationParams{
			ID:               reservationID,
			StartAt:          next.StartAt,
			EndAt:            next.EndAt,
			Name:             next.Name,
			Enabled:          next.Enabled,
			FollowEventRelay: next.FollowEventRelay,
			Settings:         settings,
			Now:              r.clock.Now(),
			UpdatedBy:        userOf(actor),
		})
		if err != nil {
			return fmt.Errorf("failed to update the reservation: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("%w: %s", ErrNotFound, reservationID)
		}
		if reservation, err = get(ctx, q, reservationID); err != nil {
			return err
		}

		summary := summaryOf(reservation)
		summary["changed"] = changed

		return r.record(ctx, tx, actor, "reservation.update", reservation, summary)
	})
	if err != nil {
		return nil, err
	}

	return reservation, nil
}

// apply は、current に update を当てた予約と、変わった項目の名前を返す。
func apply(current *Reservation, update *Update) (*Reservation, []string, error) {
	next := *current
	changed := []string{}
	if update.Enabled != nil && *update.Enabled != current.Enabled {
		next.Enabled = *update.Enabled
		changed = append(changed, "enabled")
	}
	if update.FollowEventRelay != nil && *update.FollowEventRelay != current.FollowEventRelay {
		next.FollowEventRelay = *update.FollowEventRelay
		changed = append(changed, "followEventRelay")
	}
	if update.Overrides != nil {
		next.Overrides = update.Overrides
		changed = append(changed, "settings")
	}
	if update.StartAt == nil && update.EndAt == nil && update.Name == nil {
		return &next, changed, nil
	}

	if current.Kind != KindTime {
		return nil, nil, fmt.Errorf("%w: only time reservations can change their schedule and name", ErrInvalidReservation)
	}
	next.StartAt = valueOr(update.StartAt, next.StartAt)
	next.EndAt = valueOr(update.EndAt, next.EndAt)
	next.Name = valueOr(update.Name, next.Name)
	if err := validateSchedule(next.StartAt, next.EndAt, next.Name); err != nil {
		return nil, nil, err
	}

	return &next, append(changed, "schedule"), nil
}

func valueOr[T any](v *T, fallback T) T {
	if v == nil {
		return fallback
	}

	return *v
}

// Delete は、ユーザーの操作で予約を消す。
// ルール予約は消さずに無効にする (設計書 4.3)。消すと、次の照合でまた作られてしまうからである。
func (r *Reservations) Delete(ctx context.Context, db Beginner, actor *audit.Actor, reservationID uuid.UUID) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		reservation, err := get(ctx, q, reservationID)
		if err != nil {
			return err
		}
		if reservation.Kind != KindRule {
			return r.remove(ctx, tx, actor, reservationID)
		}

		n, err := q.SetReservationEnabled(ctx, sqlc.SetReservationEnabledParams{
			ID: reservationID, Enabled: false, Now: r.clock.Now(), UpdatedBy: userOf(actor),
		})
		if err != nil {
			return fmt.Errorf("failed to disable the reservation: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("%w: %s", ErrNotFound, reservationID)
		}
		reservation.Enabled = false

		return r.record(ctx, tx, actor, "reservation.disable", reservation, summaryOf(reservation))
	})
}

// Get は予約を返す。
func (r *Reservations) Get(ctx context.Context, db sqlc.DBTX, reservationID uuid.UUID) (*Reservation, error) {
	return get(ctx, sqlc.New(db), reservationID)
}

func (r *Reservations) remove(ctx context.Context, tx pgx.Tx, actor *audit.Actor, reservationID uuid.UUID) error {
	q := sqlc.New(tx)
	reservation, err := get(ctx, q, reservationID)
	if err != nil {
		return err
	}
	n, err := q.DeleteReservation(ctx, reservationID)
	if err != nil {
		return fmt.Errorf("failed to delete the reservation: %w", err)
	}
	// 読んだ後に、別の操作が先に消していたら、消したことにしない。
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, reservationID)
	}

	return r.record(ctx, tx, actor, "reservation.delete", reservation, summaryOf(reservation))
}

func get(ctx context.Context, q *sqlc.Queries, reservationID uuid.UUID) (*Reservation, error) {
	row, err := q.GetReservation(ctx, reservationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, reservationID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the reservation: %w", err)
	}

	return fromRow(row)
}

func fromRow(row sqlc.Reservation) (*Reservation, error) {
	overrides, err := DecodeOverrides(row.Settings)
	if err != nil {
		return nil, err
	}

	return &Reservation{
		ID:               row.ID,
		Kind:             Kind(row.Kind),
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
		CreatedAt:        row.CreatedAt.UTC(),
		CreatedBy:        row.CreatedBy,
		UpdatedAt:        row.UpdatedAt.UTC(),
		UpdatedBy:        row.UpdatedBy,
	}, nil
}

func validateSchedule(start, end time.Time, name string) error {
	if !start.Before(end) {
		return fmt.Errorf("%w: startAt must be before endAt", ErrInvalidReservation)
	}
	if end.Sub(start) > maxTimeReservation {
		return fmt.Errorf("%w: a time reservation must be at most %s", ErrInvalidReservation, maxTimeReservation)
	}
	if utf8.RuneCountInString(name) > maxNameLength {
		return fmt.Errorf("%w: name must be at most %d characters", ErrInvalidReservation, maxNameLength)
	}

	return nil
}

// userOf は、操作者がユーザーならその ID を返す。システムの操作なら nil を返す。
func userOf(actor *audit.Actor) *uuid.UUID {
	if actor == nil || actor.Type != audit.ActorTypeUser {
		return nil
	}

	return &actor.UserID
}

func (r *Reservations) record(ctx context.Context, tx pgx.Tx, actor *audit.Actor, action string, reservation *Reservation, summary map[string]any) error {
	return r.recorder.Record(ctx, tx, &audit.Event{
		Actor:      actor,
		Action:     action,
		TargetType: auditTarget,
		TargetID:   reservation.ID.String(),
		Summary:    summary,
		SourceIP:   netip.Addr{},
		UserAgent:  "",
	})
}

// summaryOf は、監査ログに残す予約の要約。
func summaryOf(r *Reservation) map[string]any {
	summary := map[string]any{
		"kind":      r.Kind,
		"serviceId": strconv.FormatInt(r.ServiceID, 10),
		"startAt":   r.StartAt,
		"endAt":     r.EndAt,
		"name":      r.Name,
	}
	if r.EventID != nil {
		summary["eventId"] = *r.EventID
	}

	return summary
}
