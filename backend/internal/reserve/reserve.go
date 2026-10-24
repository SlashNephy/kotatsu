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
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/notify"
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
	// NotifyChannel は、予約が変わったことを知らせる変更通知のチャンネル。チューナーの割り当てを計算し直すきっかけになる。
	NotifyChannel = "reservations"
	// auditTarget は、監査ログの対象の種類。
	auditTarget = "reservation"
	// maxTimeReservation は、時刻予約の長さの上限。
	maxTimeReservation = 24 * time.Hour
	maxNameLength      = 200
	// maxListLimit は、一覧で一度に返す件数の上限。
	maxListLimit = 1000
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
	// RelayedFrom は、イベントリレーで引き継いだ予約の、引き継ぎ元の予約。引き継いだ予約でなければ nil。
	RelayedFrom *uuid.UUID
	// Assignment は、neko が計算したチューナーの割り当ての結果 (設計書 4.5)。
	Assignment *Assignment
	CreatedAt  time.Time
	CreatedBy  *uuid.UUID
	UpdatedAt  time.Time
	UpdatedBy  *uuid.UUID
}

// AssignmentState は、予約のチューナーの割り当ての状態。値は assign.State と同じで、まだ計算していない状態が加わる。
type AssignmentState string

const (
	// AssignmentPending は、まだ割り当てを計算していないこと。
	AssignmentPending AssignmentState = "pending"
	// AssignmentOK は、全編を録画できる見込みであること。
	AssignmentOK AssignmentState = "ok"
	// AssignmentPartial は、本編の一部が欠ける見込みであること。
	AssignmentPartial AssignmentState = "partial"
	// AssignmentConflict は、チューナーが足りないこと。
	AssignmentConflict AssignmentState = "conflict"
)

// Assignment は、予約のチューナーの割り当ての結果。
type Assignment struct {
	State AssignmentState
	// Tuner は割り当てたチューナーの番号。割り当てていなければ nil。
	Tuner *int64
	// PlannedStartAt と PlannedEndAt は、マージンを削った後の録画する予定の時刻。割り当てていなければ nil。
	PlannedStartAt *time.Time
	PlannedEndAt   *time.Time
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
// すでにあるのがルール予約なら、手動の設定をルールの設定より優先するため、手動の番組予約に切り替える。
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
			EndAt:            endOf(program.StartAt, program.EndAt, program.EndUndecided),
			Name:             program.Name,
			Description:      program.Description,
			Extended:         program.Extended,
			Genres:           program.Genres,
			FollowEventRelay: in.FollowEventRelay,
			Settings:         settings,
			Now:              r.clock.Now(),
			CreatedBy:        userOf(actor),
		})
		promoted := int64(0)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// 同じ番組の予約がすでにある。ルール予約なら、手動の番組予約に切り替える。
			promoted, err = q.PromoteRuleReservation(ctx, sqlc.PromoteRuleReservationParams{
				FollowEventRelay: in.FollowEventRelay, Settings: settings, Now: r.clock.Now(), UpdatedBy: userOf(actor),
				ServiceID: in.ServiceID, EventID: &in.EventID,
			})
			if err != nil {
				return fmt.Errorf("failed to turn the rule reservation into a program reservation: %w", err)
			}
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
		switch {
		case created:
			return r.record(ctx, tx, actor, "reservation.create", reservation, summaryOf(reservation))
		case promoted > 0:
			return r.record(ctx, tx, actor, "reservation.update", reservation, summaryOf(reservation))
		default:
			return nil
		}
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
// ルール予約と、イベントリレーで引き継いだ予約は、消さずに無効にする (設計書 4.3、4.6)。消すと、次の照合や追従でまた作られてしまうからである。
func (r *Reservations) Delete(ctx context.Context, db Beginner, actor *audit.Actor, reservationID uuid.UUID) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		reservation, err := get(ctx, q, reservationID)
		if err != nil {
			return err
		}
		// ルール予約と、イベントリレーで引き継いだ予約は、消すと次の照合や追従でまた作られるので、消さずに無効にする (#522)。
		if reservation.Kind != KindRule && reservation.RelayedFrom == nil {
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

// ListQuery は、予約の一覧の条件。
type ListQuery struct {
	// From と To は時間帯 [From, To)。これに重なる予約を返す。
	From time.Time
	To   time.Time
	// States は、割り当ての状態の集合。空ならすべて。
	States []AssignmentState
	// Enabled は、有効な予約だけ (true) か、無効な予約だけ (false) を返す。nil ならどちらも。
	Enabled *bool
	// RuleID は、このルールに一致した予約だけを返す。nil なら絞らない。
	RuleID *uuid.UUID
	// AfterStartAt と AfterID は、この位置より後ろから返す (開始時刻、ID の順)。最初から返すなら nil。
	AfterStartAt *time.Time
	AfterID      *uuid.UUID
	Limit        int
}

// List は、時間帯に重なる予約を、開始時刻の順に返す。
func (r *Reservations) List(ctx context.Context, db sqlc.DBTX, query *ListQuery) ([]*Reservation, error) {
	if query == nil {
		return nil, fmt.Errorf("%w: query must not be nil", ErrInvalidReservation)
	}
	if (query.AfterStartAt == nil) != (query.AfterID == nil) {
		return nil, fmt.Errorf("%w: afterStartAt and afterID must be given together", ErrInvalidReservation)
	}
	if !query.From.Before(query.To) {
		return nil, fmt.Errorf("%w: from must be before to", ErrInvalidReservation)
	}
	if query.Limit < 1 || query.Limit > maxListLimit {
		return nil, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidReservation, maxListLimit)
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

	rows, err := sqlc.New(db).ListReservations(ctx, sqlc.ListReservationsParams{
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

	return fromRows(rows)
}

// ProgramKey は番組の識別子 (サービスと eventId の組)。
type ProgramKey struct {
	ServiceID int64
	EventID   int64
}

// ListByPrograms は、番組の予約を返す。予約のない番組は結果に含めない。
func (r *Reservations) ListByPrograms(ctx context.Context, db sqlc.DBTX, keys []ProgramKey) (map[ProgramKey]*Reservation, error) {
	if len(keys) == 0 {
		return map[ProgramKey]*Reservation{}, nil
	}
	serviceIDs := make([]int64, 0, len(keys))
	eventIDs := make([]int64, 0, len(keys))
	for _, key := range keys {
		serviceIDs = append(serviceIDs, key.ServiceID)
		eventIDs = append(eventIDs, key.EventID)
	}
	rows, err := sqlc.New(db).ListReservationsByPrograms(ctx, sqlc.ListReservationsByProgramsParams{ServiceIds: serviceIDs, EventIds: eventIDs})
	if err != nil {
		return nil, fmt.Errorf("failed to list reservations of programs: %w", err)
	}
	reservations, err := fromRows(rows)
	if err != nil {
		return nil, err
	}

	byProgram := make(map[ProgramKey]*Reservation, len(reservations))
	for _, reservation := range reservations {
		byProgram[ProgramKey{ServiceID: reservation.ServiceID, EventID: *reservation.EventID}] = reservation
	}

	return byProgram, nil
}

// CountUpcomingByRules は、ルールごとに、一致した予約のうち終わっていないものの件数を返す。無効な予約も数える。
// 予約のないルールは、結果に含めない。
func (r *Reservations) CountUpcomingByRules(ctx context.Context, db sqlc.DBTX, ruleIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	if len(ruleIDs) == 0 {
		return map[uuid.UUID]int{}, nil
	}
	rows, err := sqlc.New(db).CountUpcomingReservationsByRules(ctx, sqlc.CountUpcomingReservationsByRulesParams{
		RuleIds: ruleIDs, Now: r.clock.Now(),
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

// RuleIDs は、予約に一致したルールの ID を、ルールを作った順に返す。ルールに一致していない予約は結果に含めない。
func (r *Reservations) RuleIDs(ctx context.Context, db sqlc.DBTX, reservationIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	if len(reservationIDs) == 0 {
		return map[uuid.UUID][]uuid.UUID{}, nil
	}
	rows, err := sqlc.New(db).ListReservationRules(ctx, reservationIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to list the rules of reservations: %w", err)
	}

	ruleIDs := map[uuid.UUID][]uuid.UUID{}
	for _, row := range rows {
		ruleIDs[row.ReservationID] = append(ruleIDs[row.ReservationID], row.RuleID)
	}

	return ruleIDs, nil
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

func fromRows(rows []sqlc.Reservation) ([]*Reservation, error) {
	reservations := make([]*Reservation, 0, len(rows))
	for _, row := range rows {
		reservation, err := fromRow(row)
		if err != nil {
			return nil, err
		}
		reservations = append(reservations, reservation)
	}

	return reservations, nil
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
		RelayedFrom:      row.RelayedFrom,
		Assignment: &Assignment{
			State:          AssignmentState(row.AssignmentState),
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

// record は、予約の変更を監査ログに残し、予約が変わったことを通知する。
// 予約の書き込みは、必ずこれを同じトランザクションで呼ぶ。
func (r *Reservations) record(ctx context.Context, tx pgx.Tx, actor *audit.Actor, action string, reservation *Reservation, summary map[string]any) error {
	if _, err := notify.Publish(ctx, tx, r.clock, NotifyChannel); err != nil {
		return err
	}

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

// optionalBool は、指定しない条件 (nil) を、DB の NULL にする。
func optionalBool(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{Bool: false, Valid: false}
	}

	return pgtype.Bool{Bool: *v, Valid: true}
}

// UndecidedLength は、終了時刻が未定の番組から予約を作るときの、予約の長さ。終了時刻が決まったら、追従で直す (#521)。
// 未定の長さを、Mahiron は 0、Mirakurun は 1 ミリ秒で返すので、そのまま写すと予約にならない。
const UndecidedLength = time.Hour

// endOf は、番組から作る予約の終わりを返す。終了時刻が未定なら、始まりから UndecidedLength 後にする。
func endOf(startAt, endAt time.Time, undecided bool) time.Time {
	if undecided || !endAt.After(startAt) {
		return startAt.Add(UndecidedLength)
	}

	return endAt
}
