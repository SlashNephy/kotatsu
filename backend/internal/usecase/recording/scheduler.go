// Package recording は、録画の割り振りと実行を扱う (設計書 1.5、5 章)。
//
// neko の Scheduler が、録画開始の少し前に、予約ごとの割り振り (recording_assignments) を書く。
// mikan は、割り振りの行をロックして取り、リースを延長しながら録画する。
package recording

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/assign"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
)

const (
	// NotifyChannel は、割り振りが変わったことを mikan に知らせる変更通知のチャンネル。
	NotifyChannel = "recording_assignments"
	// DefaultLead は、録画の予定の開始の何分前に割り振りを書くか (付録 A)。
	DefaultLead = 2 * time.Minute
	// StuckGrace は、番組の終わりが過ぎてから、録画している割り振りのない「録画中」の録画を失敗にするまでの猶予 (#516)。
	StuckGrace = 10 * time.Minute
	// DefaultInterval は、変化がなくても割り振りを見直す間隔。時間が経って割り振りを書く時刻になった予約に気づくのに使う。
	DefaultInterval = 10 * time.Second
	// DefaultDebounce は、予約の変化を受けてから見直すまで待つ時間。
	DefaultDebounce = time.Second
	// Priority と ConflictPriority は、チューナーサーバーに渡す録画の優先度 (設計書 1.6)。
	// 競合すると判定された録画は、チューナーが空いていれば録るよう、低くする。
	Priority         = 2
	ConflictPriority = 1
)

// Guard は、書き込みのトランザクションの中で、まだ neko のリーダーであることを確かめる。
type Guard func(ctx context.Context, tx pgx.Tx) error

// SchedulerOptions は Scheduler の設定。0 の項目は既定値にする。
type SchedulerOptions struct {
	Lead     time.Duration
	Interval time.Duration
	Debounce time.Duration
	// PollOnly は、変更の通知を LISTEN で受けずに、ポーリングだけで知るかどうか。
	PollOnly     bool
	PollInterval time.Duration
	// SkipConflicts は、競合した予約を録りに行かないかどうか。チューナーを奪えない EDCB で使う (#1536、設計書 1.6)。
	// Mirakurun では、競合した予約を優先度 1 で試み、後から優先度 2 の録画が来ればチューナーを譲らせる。EDCB には優先度がなく、
	// 先に録り始めた競合した予約が、後から始まる割り当て済みの予約のチューナーを取ってしまうので、試みない。
	// 試みない予約は、予定の開始が過ぎたら、チューナーを確保できなかった (tuner_unavailable) 録画の失敗として記録し、通知する。
	SkipConflicts bool
}

// Scheduler は、録画の予定の開始が Lead 以内に来た予約の割り振りを書き、録画しなくなった予約の割り振りを消す (設計書 1.5)。
type Scheduler struct {
	pool   *pgxpool.Pool
	clock  clock.Clock
	guard  Guard
	logger *slog.Logger
	opts   SchedulerOptions
}

// NewScheduler は Scheduler を返す。opts は nil でもよい。
func NewScheduler(pool *pgxpool.Pool, clk clock.Clock, guard Guard, logger *slog.Logger, opts *SchedulerOptions) *Scheduler {
	o := SchedulerOptions{
		Lead: DefaultLead, Interval: DefaultInterval, Debounce: DefaultDebounce, PollOnly: false, PollInterval: notify.DefaultPollInterval,
		SkipConflicts: false,
	}
	if opts != nil {
		o.PollOnly = opts.PollOnly
		o.SkipConflicts = opts.SkipConflicts
		if opts.Lead > 0 {
			o.Lead = opts.Lead
		}
		if opts.Interval > 0 {
			o.Interval = opts.Interval
		}
		if opts.Debounce > 0 {
			o.Debounce = opts.Debounce
		}
		if opts.PollInterval > 0 {
			o.PollInterval = opts.PollInterval
		}
	}

	return &Scheduler{pool: pool, clock: clk, guard: guard, logger: logger, opts: o}
}

// Run は、ctx が終わるまで、始めたとき、予約か割り当てが変わったとき、Interval ごとに割り振りを見直す。
func (s *Scheduler) Run(ctx context.Context) error {
	return notify.Loop(ctx, s.pool, s.logger, &notify.LoopOptions{
		Channels: []string{reserve.NotifyChannel, assign.NotifyChannel},
		Debounce: s.opts.Debounce,
		Interval: s.opts.Interval,
		Watcher:  notify.WatcherOptions{UseListen: !s.opts.PollOnly, PollInterval: s.opts.PollInterval},
	}, func(ctx context.Context) {
		if _, err := s.RunOnce(ctx); err != nil && ctx.Err() == nil {
			s.logger.WarnContext(ctx, "failed to schedule recordings", slog.Any("error", err))
		}
	})
}

// ScheduleResult は、1 回の見直しの結果。
type ScheduleResult struct {
	// Created と Deleted は、書いた割り振りと消した割り振りの数。
	Created int
	Deleted int
	// Failed は、録画している割り振りがないまま「録画中」で残っていたので、失敗にした録画の数。
	Failed int
	// Missed は、mikan が取らないまま予定の終わりが過ぎたので、失敗として記録した録画の数。
	Missed int
	// Skipped は、SkipConflicts のときに録りに行かなかった競合した予約を、失敗として記録した録画の数 (#1536)。
	Skipped int
}

// RunOnce は、割り振りを 1 回見直す。割り振りが変わったら、mikan に知らせる。
func (s *Scheduler) RunOnce(ctx context.Context) (*ScheduleResult, error) {
	result := &ScheduleResult{Created: 0, Deleted: 0, Failed: 0, Missed: 0, Skipped: 0}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.guard(ctx, tx); err != nil {
			return err
		}
		q := sqlc.New(tx)
		now := s.clock.Now()
		dueBefore := now.Add(s.opts.Lead)
		var err error

		// 取られなかった割り振りは、消すと録り逃したことが分からなくなるので、先に失敗として記録する (#517)。
		if result.Missed, err = s.recordMissed(ctx, tx, now); err != nil {
			return err
		}
		if result.Skipped, err = s.recordSkippedConflicts(ctx, tx, now); err != nil {
			return err
		}
		deleted, err := q.DeleteStaleRecordingAssignments(ctx, sqlc.DeleteStaleRecordingAssignmentsParams{DueBefore: dueBefore, Now: now})
		if err != nil {
			return fmt.Errorf("failed to delete stale recording assignments: %w", err)
		}
		result.Deleted = int(deleted)
		if result.Failed, err = s.failStuck(ctx, tx, now); err != nil {
			return err
		}
		reprioritized, err := q.UpdateWaitingRecordingPriorities(ctx, sqlc.UpdateWaitingRecordingPrioritiesParams{
			ConflictPriority: ConflictPriority, Priority: Priority, Now: now,
		})
		if err != nil {
			return fmt.Errorf("failed to update recording priorities: %w", err)
		}

		due, err := q.ListDueReservations(ctx, sqlc.ListDueReservationsParams{DueBefore: dueBefore, Now: now})
		if err != nil {
			return fmt.Errorf("failed to list due reservations: %w", err)
		}
		if result.Created, err = s.writeAssignments(ctx, q, due, now); err != nil {
			return err
		}

		if result.Created == 0 && result.Deleted == 0 && reprioritized == 0 {
			return nil
		}
		if _, err := notify.Publish(ctx, tx, s.clock, NotifyChannel); err != nil {
			return fmt.Errorf("failed to publish the recording assignment change: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// failStuck は、番組の終わりから StuckGrace が過ぎても「録画中」のままで、録画している割り振りもない録画を失敗にし、
// 録画の失敗を通知する (#516)。録画していた mikan がいなくなったか、予約を消して割り振りが消えたもので、ほかに録画を終える人がいない。
// 失敗にした録画の数を返す。
func (s *Scheduler) failStuck(ctx context.Context, tx pgx.Tx, now time.Time) (int, error) {
	rows, err := sqlc.New(tx).FailStuckRecordings(ctx, sqlc.FailStuckRecordingsParams{Now: now, Before: now.Add(-StuckGrace)})
	if err != nil {
		return 0, fmt.Errorf("failed to fail stuck recordings: %w", err)
	}
	if len(rows) > 0 {
		if _, err := notify.Publish(ctx, tx, s.clock, notify.RecordingsChannel); err != nil {
			return 0, err
		}
	}
	for _, row := range rows {
		s.logger.WarnContext(ctx, "failed a recording left recording without a mikan", slog.String("recording_id", row.ID.String()))
		payload := recordingPayload(row.ID, row.Name, row.ServiceName, row.StartAt, row.EndAt)
		payload["state"] = string(domain.RecordingStateFailed)
		payload["reason"] = string(domain.RecordingFailureReasonStreamInterrupted)
		var audience *notification.Audience
		if row.ReservationID != nil {
			target, err := notification.LoadReservationContext(ctx, tx, *row.ReservationID)
			if err != nil {
				return 0, err
			}
			target.AddTo(payload)
			audience = target.Audience
		}
		if err := notification.EmitTo(ctx, tx, s.clock, domain.NotificationKindRecordingFailed, payload, audience); err != nil {
			return 0, err
		}
	}

	return len(rows), nil
}

// writeAssignments は、録画の予定の開始が近づいた予約の割り振りを書き、書いた数を返す。
// 競合した予約は優先度 1 で書く。SkipConflicts なら、競合した予約は書かない (#1536)。
func (s *Scheduler) writeAssignments(ctx context.Context, q *sqlc.Queries, due []sqlc.ListDueReservationsRow, now time.Time) (int, error) {
	created := 0
	for _, row := range due {
		priority := int32(Priority)
		if row.AssignmentState == domain.AssignmentStateConflict {
			if s.opts.SkipConflicts {
				continue
			}
			priority = ConflictPriority
		}
		err := q.InsertRecordingAssignment(ctx, sqlc.InsertRecordingAssignmentParams{ID: id.New(), ReservationID: row.ID, Priority: priority, Now: now})
		if err != nil {
			return 0, fmt.Errorf("failed to save a recording assignment: %w", err)
		}
		created++
	}

	return created, nil
}

// recordMissed は、mikan が 1 台も取らないまま予定の終わりが過ぎた割り振りの予約について、
// 録画を失敗 (recorder_unavailable) として記録し、録画の失敗を通知する (#517)。記録した録画の数を返す。
func (s *Scheduler) recordMissed(ctx context.Context, tx pgx.Tx, now time.Time) (int, error) {
	rows, err := sqlc.New(tx).ListMissedRecordingAssignments(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("failed to list missed recording assignments: %w", err)
	}
	for _, row := range rows {
		s.logger.WarnContext(ctx, "no mikan took the recording assignment before it ended", slog.String("reservation_id", row.ID.String()))
	}

	return s.recordUnstarted(ctx, tx, rows, domain.RecordingFailureReasonRecorderUnavailable, now)
}

// recordSkippedConflicts は、SkipConflicts のとき、録りに行かない競合した予約の予定の開始が過ぎたら、
// 録画を失敗 (tuner_unavailable) として記録し、録画の失敗を通知する (#1536)。記録した録画の数を返す。
// Mirakurun で、チューナーを確保できずに失敗したときと同じ理由と通知にする。録画の行は予約ごとに 1 つなので、
// 同じ予約を何度も記録したり通知したりしない。
func (s *Scheduler) recordSkippedConflicts(ctx context.Context, tx pgx.Tx, now time.Time) (int, error) {
	if !s.opts.SkipConflicts {
		return 0, nil
	}
	skipped, err := sqlc.New(tx).ListSkippedConflictReservations(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("failed to list skipped conflicting reservations: %w", err)
	}
	rows := make([]sqlc.ListMissedRecordingAssignmentsRow, 0, len(skipped))
	for _, row := range skipped {
		s.logger.WarnContext(ctx, "the conflicting reservation was not recorded; the tuner server cannot preempt tuners",
			slog.String("reservation_id", row.ID.String()))
		rows = append(rows, sqlc.ListMissedRecordingAssignmentsRow(row))
	}

	return s.recordUnstarted(ctx, tx, rows, domain.RecordingFailureReasonTunerUnavailable, now)
}

// recordUnstarted は、録り始めなかった予約の録画を、reason の失敗として記録し、録画の失敗を通知する。記録した録画の数を返す。
func (s *Scheduler) recordUnstarted(
	ctx context.Context, tx pgx.Tx, rows []sqlc.ListMissedRecordingAssignmentsRow, reason domain.RecordingFailureReason, now time.Time,
) (int, error) {
	q := sqlc.New(tx)
	recorded := 0
	for _, row := range rows {
		recordingID := id.New()
		recordedFrom := row.StartAt
		if row.PlannedStartAt != nil {
			recordedFrom = *row.PlannedStartAt
		}
		inserted, err := q.InsertMissedRecording(ctx, sqlc.InsertMissedRecordingParams{
			ID: recordingID, ReservationID: &row.ID, ServiceID: row.ServiceID, EventID: row.EventID, Name: row.Name, Description: row.Description,
			Extended: row.Extended, NameNormalized: new(domain.Normalize(row.Name)), DescriptionNormalized: new(domain.Normalize(row.Description)),
			StartAt: row.StartAt, EndAt: row.EndAt, FailureReason: &reason, RecordedFrom: recordedFrom, Now: now,
		})
		if err != nil {
			return 0, fmt.Errorf("failed to record an unstarted recording: %w", err)
		}
		// 一覧を読んだ後に、ほかで録画の行ができていれば、記録も通知もしない。
		if inserted == 0 {
			continue
		}
		recorded++
		payload := recordingPayload(recordingID, row.Name, row.ServiceName, row.StartAt, row.EndAt)
		payload["state"] = string(domain.RecordingStateFailed)
		payload["reason"] = string(reason)
		target, err := notification.LoadReservationContext(ctx, tx, row.ID)
		if err != nil {
			return 0, err
		}
		target.AddTo(payload)
		if err := notification.EmitTo(ctx, tx, s.clock, domain.NotificationKindRecordingFailed, payload, target.Audience); err != nil {
			return 0, err
		}
	}
	if recorded > 0 {
		if _, err := notify.Publish(ctx, tx, s.clock, notify.RecordingsChannel); err != nil {
			return 0, err
		}
	}

	return recorded, nil
}
