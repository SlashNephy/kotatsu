package recording

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
)

// LeaseStore は、Worker が割り振りのリースを取り、延長し、手放すための DB 操作。
// 録画の割り振りと、タイムシフトの割り振りで、Worker を共有するために切り出している。
// 割り振りの行は、世代番号 (Generation) と持ち主 (holder) が合うときだけ書き換える。合わなければ、行数 0 を返す。
type LeaseStore interface {
	// Channel は、割り振りの変更を知らせる通知のチャンネル。Worker はこれを LISTEN する。
	Channel() string
	// Acquire は、取れる割り振りを 1 つ取る。なければ false を返す。now は取る時刻で、引き継ぎかどうかの判断に使う。
	Acquire(ctx context.Context, holder uuid.UUID, now, expiresAt time.Time) (*Assignment, bool, error)
	// Renew は、リースを expiresAt まで延ばす。延ばせた行の数を返す。
	Renew(ctx context.Context, a *Assignment, holder uuid.UUID, now, expiresAt time.Time) (int64, error)
	// MarkHandover は、割り振りに引き継ぎ待ちの印を付け、ほかの mikan に知らせる。印を付けたら true を返す。
	// 印と通知を 1 つのトランザクションで行うので、トランザクションはこのメソッドの中に閉じる。
	MarkHandover(ctx context.Context, clk clock.Clock, a *Assignment, holder uuid.UUID) (bool, error)
	// Release は、止まる mikan が、割り振りを手放す。再起動した mikan がすぐに取れる。
	Release(ctx context.Context, now time.Time, a *Assignment, holder uuid.UUID) error
	// ReleaseUnrecordable は、録画できない理由がこの mikan にあるので、割り振りを手放す。
	ReleaseUnrecordable(ctx context.Context, now time.Time, a *Assignment, holder uuid.UUID) error
	// DeleteUnrecorded は、録画を始めずに終えた割り振りを消す。消した行の数を返す。
	// 録画は、始めていれば Finish で終え、始めていなければ消す。タイムシフトでは、終えることは消すことなので、常に消す。
	DeleteUnrecorded(ctx context.Context, a *Assignment, holder uuid.UUID) (int64, error)
	// Finish は、割り振りを終わったことにする。終えた行の数を返す。
	Finish(ctx context.Context, now time.Time, a *Assignment, holder uuid.UUID) (int64, error)
	// Exists は、割り振りが DB に残っているかどうかを返す。
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	// StreamGeneration は、ストリームを開いた世代番号を返す。引き継ぐ mikan がストリームを開いたかを調べるのに使う。
	StreamGeneration(ctx context.Context, id uuid.UUID) (int64, error)
}

// recordingLeases は、録画の割り振り (recording_assignments) の LeaseStore。
type recordingLeases struct {
	pool *pgxpool.Pool
}

var _ LeaseStore = new(recordingLeases)

// NewRecordingLeases は、録画の割り振りの LeaseStore を返す。
func NewRecordingLeases(pool *pgxpool.Pool) LeaseStore {
	return &recordingLeases{pool: pool}
}

func (l *recordingLeases) Channel() string {
	return NotifyChannel
}

func (l *recordingLeases) Acquire(ctx context.Context, holder uuid.UUID, now, expiresAt time.Time) (*Assignment, bool, error) {
	row, err := sqlc.New(l.pool).AcquireRecordingAssignment(ctx, sqlc.AcquireRecordingAssignmentParams{
		Now: now, Holder: &holder, ExpiresAt: &expiresAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to acquire a recording assignment: %w", err)
	}

	// 引き継ぎ待ちでも、古い mikan のリースが切れていれば、引き継ぎの最中に落ちたので、落ちた mikan の代わりとして扱う。
	handedOver := row.PreviousState == string(AssignmentHandover) && row.PreviousLeaseExpiresAt != nil && row.PreviousLeaseExpiresAt.After(now)

	return &Assignment{
		ID: row.ID, ReservationID: row.ReservationID, ServiceID: 0, Priority: int(row.Priority), Generation: row.Generation,
		HandedOver: handedOver, PreviousLeaseExpiresAt: row.PreviousLeaseExpiresAt,
	}, true, nil
}

func (l *recordingLeases) Renew(ctx context.Context, a *Assignment, holder uuid.UUID, now, expiresAt time.Time) (int64, error) {
	return sqlc.New(l.pool).RenewRecordingAssignment(ctx, sqlc.RenewRecordingAssignmentParams{
		ExpiresAt: &expiresAt, Now: now, ID: a.ID, Generation: a.Generation, Holder: &holder,
	})
}

func (l *recordingLeases) MarkHandover(ctx context.Context, clk clock.Clock, a *Assignment, holder uuid.UUID) (bool, error) {
	marked := false
	err := pgx.BeginFunc(ctx, l.pool, func(tx pgx.Tx) error {
		n, err := sqlc.New(tx).MarkRecordingHandover(ctx, sqlc.MarkRecordingHandoverParams{
			Now: clk.Now(), ID: a.ID, Generation: a.Generation, Holder: &holder,
		})
		if err != nil {
			return fmt.Errorf("failed to mark the recording for handover: %w", err)
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

func (l *recordingLeases) Release(ctx context.Context, now time.Time, a *Assignment, holder uuid.UUID) error {
	return sqlc.New(l.pool).ReleaseRecordingAssignment(ctx, sqlc.ReleaseRecordingAssignmentParams{
		Now: now, ID: a.ID, Generation: a.Generation, Holder: &holder,
	})
}

func (l *recordingLeases) ReleaseUnrecordable(ctx context.Context, now time.Time, a *Assignment, holder uuid.UUID) error {
	return sqlc.New(l.pool).ReleaseUnrecordableRecordingAssignment(ctx, sqlc.ReleaseUnrecordableRecordingAssignmentParams{
		Now: now, ID: a.ID, Generation: a.Generation, Holder: &holder,
	})
}

func (l *recordingLeases) DeleteUnrecorded(ctx context.Context, a *Assignment, holder uuid.UUID) (int64, error) {
	return sqlc.New(l.pool).DeleteUnrecordedRecordingAssignment(ctx, sqlc.DeleteUnrecordedRecordingAssignmentParams{
		ID: a.ID, Generation: a.Generation, Holder: &holder,
	})
}

func (l *recordingLeases) Finish(ctx context.Context, now time.Time, a *Assignment, holder uuid.UUID) (int64, error) {
	return sqlc.New(l.pool).FinishRecordingAssignment(ctx, sqlc.FinishRecordingAssignmentParams{
		Now: now, ID: a.ID, Generation: a.Generation, Holder: &holder,
	})
}

func (l *recordingLeases) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	return sqlc.New(l.pool).RecordingAssignmentExists(ctx, id)
}

func (l *recordingLeases) StreamGeneration(ctx context.Context, id uuid.UUID) (int64, error) {
	stream, err := sqlc.New(l.pool).GetRecordingAssignmentStream(ctx, id)
	if err != nil {
		return 0, err
	}

	return stream.StreamGeneration, nil
}
