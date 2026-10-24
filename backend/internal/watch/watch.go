// Package watch は、視聴予約を扱う (設計書 4.8)。
//
// 視聴予約は、録画はせず、放送の少し前に、予約したユーザーだけに通知するためのもの。
// 録画の予約と違ってチューナーを使わない個人のものなので、予約エンジン (reserve) とは分けて持つ。
package watch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/notification"
)

const (
	// NotifyInterval は、通知を送る時刻を過ぎた視聴予約を探す間隔。
	NotifyInterval = 30 * time.Second
	// DefaultReminderMinutes は、視聴予約の通知を、放送の何分前に送るかの既定値。
	DefaultReminderMinutes = 5
	// MaxReminderMinutes は、視聴予約の通知を、放送の何分前に送るかの上限。
	MaxReminderMinutes = 60
)

var (
	// ErrProgramUnavailable は、番組が EPG にないか、放送が始まっていて、視聴予約を付けられないことを表す。
	ErrProgramUnavailable = errors.New("the program is not found or has already started")
	// ErrInvalidReminderMinutes は、通知を送る時刻の設定が範囲の外であることを表す。
	ErrInvalidReminderMinutes = errors.New("the reminder minutes are out of range")
)

// ProgramKey は、視聴予約した番組を表す (サービス, eventId) の組。
type ProgramKey struct {
	ServiceID int64
	EventID   int64
}

// Reserve は、ユーザー userID の、番組 key の視聴予約を付ける。付けてあれば何もしない。
// 番組が EPG にないか、放送が始まっていれば ErrProgramUnavailable を返す。
func Reserve(ctx context.Context, db sqlc.DBTX, clk clock.Clock, userID uuid.UUID, key ProgramKey) error {
	q := sqlc.New(db)
	inserted, err := q.InsertWatchReservation(ctx, sqlc.InsertWatchReservationParams{
		UserID: userID, ServiceID: key.ServiceID, EventID: key.EventID, Now: clk.Now(),
	})
	if err != nil {
		return fmt.Errorf("failed to insert the watch reservation: %w", err)
	}
	if inserted > 0 {
		return nil
	}
	// 付けられなかったのは、付けてあったからか、番組がないか始まっていたからかを見分ける。
	reserved, err := q.ListWatchReservedPrograms(ctx, sqlc.ListWatchReservedProgramsParams{
		UserID: userID, ServiceIds: []int64{key.ServiceID}, EventIds: []int64{key.EventID},
	})
	if err != nil {
		return fmt.Errorf("failed to get the watch reservation: %w", err)
	}
	if len(reserved) == 0 {
		return ErrProgramUnavailable
	}

	return nil
}

// Cancel は、ユーザー userID の、番組 key の視聴予約を外す。付けていなければ何もしない。
func Cancel(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, key ProgramKey) error {
	if _, err := sqlc.New(db).DeleteWatchReservation(ctx, sqlc.DeleteWatchReservationParams{
		UserID: userID, ServiceID: key.ServiceID, EventID: key.EventID,
	}); err != nil {
		return fmt.Errorf("failed to delete the watch reservation: %w", err)
	}

	return nil
}

// Reserved は、keys のうち、ユーザー userID が視聴予約した番組を返す。
func Reserved(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, keys []ProgramKey) (map[ProgramKey]bool, error) {
	serviceIDs := make([]int64, 0, len(keys))
	eventIDs := make([]int64, 0, len(keys))
	for _, key := range keys {
		serviceIDs = append(serviceIDs, key.ServiceID)
		eventIDs = append(eventIDs, key.EventID)
	}
	rows, err := sqlc.New(db).ListWatchReservedPrograms(ctx, sqlc.ListWatchReservedProgramsParams{
		UserID: userID, ServiceIds: serviceIDs, EventIds: eventIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the watch reservations: %w", err)
	}
	reserved := make(map[ProgramKey]bool, len(rows))
	for _, row := range rows {
		reserved[ProgramKey{ServiceID: row.ServiceID, EventID: row.EventID}] = true
	}

	return reserved, nil
}

// ReminderMinutes は、ユーザー userID の、視聴予約の通知を放送の何分前に送るかの設定を返す。
func ReminderMinutes(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) (int, error) {
	minutes, err := sqlc.New(db).GetWatchReminderMinutes(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get the watch reminder minutes: %w", err)
	}

	return int(minutes), nil
}

// SetReminderMinutes は、ユーザー userID の、視聴予約の通知を放送の何分前に送るかを変える。
// 0 から MaxReminderMinutes の外なら ErrInvalidReminderMinutes を返す。
func SetReminderMinutes(ctx context.Context, db sqlc.DBTX, clk clock.Clock, userID uuid.UUID, minutes int) (int, error) {
	if minutes < 0 || minutes > MaxReminderMinutes {
		return 0, fmt.Errorf("%w: %d", ErrInvalidReminderMinutes, minutes)
	}
	updated, err := sqlc.New(db).UpsertWatchReminderMinutes(ctx, sqlc.UpsertWatchReminderMinutesParams{
		UserID: userID, WatchReminderMinutes: int32(minutes), Now: clk.Now(),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to update the watch reminder minutes: %w", err)
	}

	return int(updated), nil
}

// Notifier は、通知を送る時刻を過ぎた視聴予約を、予約したユーザーに通知する。
type Notifier struct {
	pool  *pgxpool.Pool
	clock clock.Clock
	// liveURL は、サービスのライブ視聴の画面のパスを返す。通知を押したときに開く。
	liveURL func(serviceID int64) string
}

// NewNotifier は Notifier を作る。liveURL は、サービスのライブ視聴の画面のパスを返す。
func NewNotifier(pool *pgxpool.Pool, clk clock.Clock, liveURL func(serviceID int64) string) *Notifier {
	return &Notifier{pool: pool, clock: clk, liveURL: liveURL}
}

// Notify は、通知を送る時刻を過ぎた視聴予約を消し、それぞれ予約したユーザーだけに届く通知を記録する。
// あわせて、放送が終わった番組と、EPG から消えた番組の視聴予約を消す。通知した数と、片付けた数を返す。
// 消すのと通知を記録するのを 1 つのトランザクションで行うので、nekonote が何台あっても二度は通知しない。
func (n *Notifier) Notify(ctx context.Context) (int, int64, error) {
	notified := 0
	var pruned int64
	err := pgx.BeginFunc(ctx, n.pool, func(tx pgx.Tx) error {
		now := n.clock.Now()
		q := sqlc.New(tx)
		due, err := q.TakeDueWatchReservations(ctx, now)
		if err != nil {
			return fmt.Errorf("failed to take the due watch reservations: %w", err)
		}
		for _, row := range due {
			payload := map[string]any{
				"name": row.Name, "serviceName": row.ServiceName, "startAt": row.StartAt.UTC().Format(time.RFC3339Nano),
				"serviceId": row.ServiceID, "eventId": row.EventID, "url": n.liveURL(row.ServiceID),
			}
			if err := notification.EmitTo(ctx, tx, n.clock, notification.KindWatchReminder, payload,
				&notification.Audience{UserIDs: []uuid.UUID{row.UserID}, ByRule: false}); err != nil {
				return err
			}
		}
		notified = len(due)
		pruned, err = q.PruneWatchReservations(ctx, now)
		if err != nil {
			return fmt.Errorf("failed to prune the watch reservations: %w", err)
		}

		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	return notified, pruned, nil
}

// NotifyArgs は、視聴予約を通知するジョブの引数。
type NotifyArgs struct{}

var _ river.JobArgs = new(NotifyArgs)

func (NotifyArgs) Kind() string { return "watch_notify" }

// NotifyWorker は、視聴予約を通知するジョブを実行する。
type NotifyWorker struct {
	river.WorkerDefaults[NotifyArgs]

	notifier *Notifier
}

var _ river.Worker[NotifyArgs] = new(NotifyWorker)

// NewNotifyWorker は NotifyWorker を作る。
func NewNotifyWorker(notifier *Notifier) *NotifyWorker {
	return &NotifyWorker{WorkerDefaults: river.WorkerDefaults[NotifyArgs]{}, notifier: notifier}
}

func (w *NotifyWorker) Work(ctx context.Context, _ *river.Job[NotifyArgs]) error {
	notified, pruned, err := w.notifier.Notify(ctx)
	if err != nil {
		return err
	}
	if err := river.RecordOutput(ctx, map[string]int64{"notified": int64(notified), "pruned": pruned}); err != nil {
		return fmt.Errorf("failed to record the job output: %w", err)
	}

	return nil
}

// NotifyPeriodicJob は、視聴予約を NotifyInterval ごとに通知する定期ジョブを返す。
func NotifyPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(NotifyInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return NotifyArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "watch_notify", RunOnStart: true},
	)
}
