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

	"github.com/SlashNephy/kotatsu/internal/assign"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/notify"
	"github.com/SlashNephy/kotatsu/internal/reserve"
)

const (
	// NotifyChannel は、割り振りが変わったことを mikan に知らせる変更通知のチャンネル。
	NotifyChannel = "recording_assignments"
	// DefaultLead は、録画の予定の開始の何分前に割り振りを書くか (付録 A)。
	DefaultLead = 2 * time.Minute
	// DefaultInterval は、変化がなくても割り振りを見直す間隔。時間が経って割り振りを書く時刻になった予約に気づくのに使う。
	DefaultInterval = 10 * time.Second
	// DefaultDebounce は、予約の変化を受けてから見直すまで待つ時間。
	DefaultDebounce = time.Second
	// Priority と ConflictPriority は、チューナーサーバーに渡す録画の優先度 (設計書 1.6)。
	// 競合すると判定された録画は、チューナーが空いていれば録るよう、低くする。
	Priority         = 2
	ConflictPriority = 1
)

// AssignmentState は、割り振りの状態 (recording_assignments.state)。
type AssignmentState string

const (
	// AssignmentWaiting は、まだ mikan が取っていない割り振り。
	AssignmentWaiting AssignmentState = "waiting"
	// AssignmentRecording は、mikan が取って録画している割り振り。
	AssignmentRecording AssignmentState = "recording"
	// AssignmentHandover は、止まる mikan が、引き継ぎを待っている割り振り (設計書 5.4)。
	AssignmentHandover AssignmentState = "handover"
	// AssignmentFinished は、録画を終えた割り振り。
	AssignmentFinished AssignmentState = "finished"
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
	o := SchedulerOptions{Lead: DefaultLead, Interval: DefaultInterval, Debounce: DefaultDebounce, PollOnly: false, PollInterval: notify.DefaultPollInterval}
	if opts != nil {
		o.PollOnly = opts.PollOnly
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
}

// RunOnce は、割り振りを 1 回見直す。割り振りが変わったら、mikan に知らせる。
func (s *Scheduler) RunOnce(ctx context.Context) (*ScheduleResult, error) {
	result := &ScheduleResult{Created: 0, Deleted: 0}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.guard(ctx, tx); err != nil {
			return err
		}
		q := sqlc.New(tx)
		now := s.clock.Now()
		dueBefore := now.Add(s.opts.Lead)

		deleted, err := q.DeleteStaleRecordingAssignments(ctx, sqlc.DeleteStaleRecordingAssignmentsParams{DueBefore: dueBefore, Now: now})
		if err != nil {
			return fmt.Errorf("failed to delete stale recording assignments: %w", err)
		}
		result.Deleted = int(deleted)
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
		for _, row := range due {
			priority := int32(Priority)
			if row.AssignmentState == string(assign.StateConflict) {
				priority = ConflictPriority
			}
			err := q.InsertRecordingAssignment(ctx, sqlc.InsertRecordingAssignmentParams{ID: id.New(), ReservationID: row.ID, Priority: priority, Now: now})
			if err != nil {
				return fmt.Errorf("failed to save a recording assignment: %w", err)
			}
		}
		result.Created = len(due)

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
