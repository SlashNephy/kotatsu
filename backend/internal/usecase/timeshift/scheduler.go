package timeshift

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/assign"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
)

const (
	// NotifyChannel は、タイムシフトの割り振りが変わったことを mikan に知らせる変更通知のチャンネル。
	NotifyChannel = "timeshift_assignments"
	// DefaultInterval は、変化がなくても割り振りを見直す間隔。チャンネルの設定の変更や、予約の割り振りの時間に入ったことに気づくのに使う (付録 A)。
	DefaultInterval = 10 * time.Second
	// Priority は、チューナーサーバーに渡すタイムシフトの優先度 (設計書 1.6)。予約の録画にもライブ視聴にも譲る。
	Priority = -1
)

// Guard は、書き込みのトランザクションの中で、まだ neko のリーダーであることを確かめる。
type Guard func(ctx context.Context, tx pgx.Tx) error

// SchedulerOptions は Scheduler の設定。0 の項目は既定値にする。
type SchedulerOptions struct {
	Interval time.Duration
	// Lead は、予約の録画がチューナーを使い始める何分前から、そのチューナーのタイムシフトを外すか。
	// 0 なら、録画の割り振りを書く時刻の前 (recording.DefaultLead) に、この見直しの間隔の分だけ余裕を持たせる (LeadFor)。
	Lead time.Duration
	// PollOnly は、変更の通知を LISTEN で受けずに、ポーリングだけで知るかどうか。
	PollOnly     bool
	PollInterval time.Duration
}

// Scheduler は、タイムシフトで録るチャンネルのうち、チューナーを使えるものの割り振りを書き、使えなくなったものの割り振りを消す (設計書 5.7)。
type Scheduler struct {
	pool   *pgxpool.Pool
	clock  clock.Clock
	guard  Guard
	logger *slog.Logger
	opts   SchedulerOptions
}

// NewScheduler は Scheduler を返す。opts は nil でもよい。
func NewScheduler(pool *pgxpool.Pool, clk clock.Clock, guard Guard, logger *slog.Logger, opts *SchedulerOptions) *Scheduler {
	o := SchedulerOptions{Interval: DefaultInterval, Lead: 0, PollOnly: false, PollInterval: notify.DefaultPollInterval}
	if opts != nil {
		o.PollOnly = opts.PollOnly
		if opts.Interval > 0 {
			o.Interval = opts.Interval
		}
		if opts.Lead > 0 {
			o.Lead = opts.Lead
		}
		if opts.PollInterval > 0 {
			o.PollInterval = opts.PollInterval
		}
	}

	if o.Lead == 0 {
		o.Lead = LeadFor(recording.DefaultLead, o.Interval)
	}

	return &Scheduler{pool: pool, clock: clk, guard: guard, logger: logger, opts: o}
}

// LeadFor は、録画の割り振りを書く時刻の前 (recordingLead) から、タイムシフトの割り振りを外し始める時間を返す。
// 予約へのチューナーの割り振りより前にタイムシフトを外す必要がある (設計書 5.7)。
// 二つの見直しは独立に動き、タイムシフトの見直しは interval ごとなので、recordingLead と同じにすると、
// 外すのが最大で interval 遅れ、録画の割り振りがタイムシフトのストリームと重なりうる。interval の分だけ早めに外す。
func LeadFor(recordingLead, interval time.Duration) time.Duration {
	return recordingLead + interval
}

// Run は、ctx が終わるまで、始めたとき、予約か割り当てが変わったとき、Interval ごとに割り振りを見直す。
func (s *Scheduler) Run(ctx context.Context) error {
	return notify.Loop(ctx, s.pool, s.logger, &notify.LoopOptions{
		Channels: []string{reserve.NotifyChannel, assign.NotifyChannel},
		Debounce: time.Second,
		Interval: s.opts.Interval,
		Watcher:  notify.WatcherOptions{UseListen: !s.opts.PollOnly, PollInterval: s.opts.PollInterval},
	}, func(ctx context.Context) {
		if _, err := s.RunOnce(ctx); err != nil && ctx.Err() == nil {
			s.logger.WarnContext(ctx, "failed to schedule timeshift", slog.Any("error", err))
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

		want, channels, err := s.plan(ctx, q, now)
		if err != nil {
			return err
		}
		if result.Created, result.Deleted, err = apply(ctx, q, now, channels, want); err != nil {
			return err
		}

		if result.Created == 0 && result.Deleted == 0 {
			return nil
		}
		if _, err := notify.Publish(ctx, tx, s.clock, NotifyChannel); err != nil {
			return fmt.Errorf("failed to publish the timeshift assignment change: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// plan は、今の設定とチューナーと予約から、タイムシフトで録るサービスの ID の集合と、設定されたチャンネルを返す。
func (s *Scheduler) plan(ctx context.Context, q *sqlc.Queries, now time.Time) (map[int64]bool, []*Channel, error) {
	channelRows, err := q.ListTimeshiftChannels(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list timeshift channels: %w", err)
	}
	channels := make([]*Channel, 0, len(channelRows))
	for _, row := range channelRows {
		channels = append(channels, &Channel{ServiceID: row.ServiceID, ChannelType: row.ChannelType, Channel: row.Channel})
	}
	multiplexRows, err := q.ListServiceChannels(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list service channels: %w", err)
	}
	multiplexes := make([]*Multiplex, 0, len(multiplexRows))
	for _, row := range multiplexRows {
		multiplexes = append(multiplexes, &Multiplex{ChannelType: row.ChannelType, Channel: row.Channel})
	}
	tunerRows, err := q.ListUsableTuners(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list tuners: %w", err)
	}
	tuners := make([]*assign.Tuner, 0, len(tunerRows))
	for _, row := range tunerRows {
		tuners = append(tuners, &assign.Tuner{Index: int(row.TunerIndex), Types: row.Types})
	}
	busyRows, err := q.ListReservationsUsingTunersBetween(ctx, sqlc.ListReservationsUsingTunersBetweenParams{FromAt: now, ToAt: now.Add(s.opts.Lead)})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list reservations using tuners: %w", err)
	}
	busy := make([]*Busy, 0, len(busyRows))
	for _, row := range busyRows {
		// assigned_tuner が NULL の行は、クエリで除いている。
		busy = append(busy, &Busy{Tuner: int(*row.AssignedTuner), ChannelType: row.ChannelType, Channel: row.Channel})
	}

	want := map[int64]bool{}
	for _, serviceID := range Plan(channels, multiplexes, tuners, busy) {
		want[serviceID] = true
	}

	return want, channels, nil
}

// apply は、割り振りを want に合わせる。書いた数と消した数を返す。
// 新しい割り振りは、channels の順 (チャンネル種別の並び順、サービス ID の順) に書く。
// 終わったことになった割り振りは、どの mikan も取らず、残すとそのチャンネルを録れなくなるので、ないものとして消し、要れば書き直す。
func apply(ctx context.Context, q *sqlc.Queries, now time.Time, channels []*Channel, want map[int64]bool) (int, int, error) {
	assignments, err := q.ListTimeshiftAssignments(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list timeshift assignments: %w", err)
	}
	have := map[int64]bool{}
	deleted := 0
	for _, a := range assignments {
		finished := a.State == string(recording.AssignmentFinished)
		if !finished {
			have[a.ServiceID] = true
		}
		if want[a.ServiceID] && !finished {
			continue
		}
		if _, err := q.DeleteTimeshiftAssignment(ctx, a.ServiceID); err != nil {
			return 0, 0, fmt.Errorf("failed to delete the timeshift assignment of %d: %w", a.ServiceID, err)
		}
		deleted++
	}
	created := 0
	for _, channel := range channels {
		if !want[channel.ServiceID] || have[channel.ServiceID] {
			continue
		}
		if err := q.InsertTimeshiftAssignment(ctx, sqlc.InsertTimeshiftAssignmentParams{ID: id.New(), ServiceID: channel.ServiceID, Priority: Priority, Now: now}); err != nil {
			return 0, 0, fmt.Errorf("failed to save the timeshift assignment of %d: %w", channel.ServiceID, err)
		}
		created++
	}

	return created, deleted, nil
}
