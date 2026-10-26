package timeshift

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// leases は、タイムシフトの割り振り (timeshift_assignments) の LeaseStore。録画と同じ Worker で、リースを取って録る。
type leases struct {
	pool *pgxpool.Pool
}

var _ recording.LeaseStore = new(leases)

// NewLeases は、タイムシフトの割り振りの LeaseStore を返す。
func NewLeases(pool *pgxpool.Pool) recording.LeaseStore {
	return &leases{pool: pool}
}

func (l *leases) Channel() string {
	return NotifyChannel
}

func (l *leases) Acquire(ctx context.Context, holder uuid.UUID, now, expiresAt time.Time) (*recording.Assignment, bool, error) {
	row, err := sqlc.New(l.pool).AcquireTimeshiftAssignment(ctx, sqlc.AcquireTimeshiftAssignmentParams{
		Holder: &holder, ExpiresAt: &expiresAt, Now: now,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to acquire a timeshift assignment: %w", err)
	}

	// 引き継ぎ待ちでも、古い mikan のリースが切れていれば、引き継ぎの最中に落ちたので、落ちた mikan の代わりとして扱う。
	handedOver := row.PreviousState == string(recording.AssignmentHandover) && row.PreviousLeaseExpiresAt != nil && row.PreviousLeaseExpiresAt.After(now)

	return &recording.Assignment{
		ID: row.ID, ReservationID: uuid.Nil, ServiceID: row.ServiceID, Priority: int(row.Priority), Generation: row.Generation,
		HandedOver: handedOver, PreviousLeaseExpiresAt: row.PreviousLeaseExpiresAt,
	}, true, nil
}

func (l *leases) Renew(ctx context.Context, a *recording.Assignment, holder uuid.UUID, now, expiresAt time.Time) (int64, error) {
	return sqlc.New(l.pool).RenewTimeshiftAssignment(ctx, sqlc.RenewTimeshiftAssignmentParams{
		ExpiresAt: &expiresAt, Now: now, ID: a.ID, Generation: a.Generation, Holder: &holder,
	})
}

func (l *leases) MarkHandover(ctx context.Context, clk clock.Clock, a *recording.Assignment, holder uuid.UUID) (bool, error) {
	marked := false
	err := pgx.BeginFunc(ctx, l.pool, func(tx pgx.Tx) error {
		n, err := sqlc.New(tx).MarkTimeshiftHandover(ctx, sqlc.MarkTimeshiftHandoverParams{
			Now: clk.Now(), ID: a.ID, Generation: a.Generation, Holder: &holder,
		})
		if err != nil {
			return fmt.Errorf("failed to mark the timeshift for handover: %w", err)
		}
		if n == 0 {
			return nil
		}
		marked = true
		_, err = notify.Publish(ctx, tx, clk, NotifyChannel)

		return err
	})

	return marked, err
}

func (l *leases) Release(ctx context.Context, now time.Time, a *recording.Assignment, holder uuid.UUID) error {
	return sqlc.New(l.pool).ReleaseTimeshiftAssignment(ctx, sqlc.ReleaseTimeshiftAssignmentParams{
		Now: now, ID: a.ID, Generation: a.Generation, Holder: &holder,
	})
}

func (l *leases) ReleaseUnrecordable(ctx context.Context, now time.Time, a *recording.Assignment, holder uuid.UUID) error {
	return sqlc.New(l.pool).ReleaseUnrecordableTimeshiftAssignment(ctx, sqlc.ReleaseUnrecordableTimeshiftAssignmentParams{
		Now: now, ID: a.ID, Generation: a.Generation, Holder: &holder,
	})
}

// DeleteUnrecorded は、割り振りを消す。タイムシフトには録画の行がないので、終えることは消すこと。
// 録り続けるべきチャンネルなら、neko が次の見直しで書き直す。
func (l *leases) DeleteUnrecorded(ctx context.Context, a *recording.Assignment, holder uuid.UUID) (int64, error) {
	return sqlc.New(l.pool).DeleteTimeshiftAssignmentByGeneration(ctx, sqlc.DeleteTimeshiftAssignmentByGenerationParams{
		ID: a.ID, Generation: a.Generation, Holder: &holder,
	})
}

func (l *leases) Finish(ctx context.Context, now time.Time, a *recording.Assignment, holder uuid.UUID) (int64, error) {
	return sqlc.New(l.pool).FinishTimeshiftAssignment(ctx, sqlc.FinishTimeshiftAssignmentParams{
		Now: now, ID: a.ID, Generation: a.Generation, Holder: &holder,
	})
}

func (l *leases) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	return sqlc.New(l.pool).TimeshiftAssignmentExists(ctx, id)
}

func (l *leases) StreamGeneration(ctx context.Context, id uuid.UUID) (int64, error) {
	stream, err := sqlc.New(l.pool).GetTimeshiftAssignmentStream(ctx, id)
	if err != nil {
		return 0, err
	}

	return stream.StreamGeneration, nil
}
