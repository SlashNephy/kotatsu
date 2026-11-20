package timeshift

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
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
	// ShareByService は、相乗りの単位をチャンネルでなくサービスにするかどうか。EDCB で使う (#1856、PlanOptions)。
	ShareByService bool
	// EPGCapture は、チューナーサーバー自身の EPG の取得 (EDCB) のために空けておくチューナーを決める設定を返す。EDCB で使う (#1856)。
	// nil なら、Mirakurun 互換のサーバーと同じく、チューナーサーバーの番組表の取得に、録られないチャンネルの種別ごとに 1 本を残す。
	EPGCapture tunerserver.EPGCaptureReporter
}

// epgCaptureTimeout は、EPG の取得の設定を読むのを打ち切る時間。読めなければ、前に読んだものを使う。
const epgCaptureTimeout = 5 * time.Second

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
	o := SchedulerOptions{
		Interval: DefaultInterval, Lead: 0, PollOnly: false, PollInterval: notify.DefaultPollInterval, ShareByService: false, EPGCapture: nil,
	}
	if opts != nil {
		o.PollOnly = opts.PollOnly
		o.ShareByService = opts.ShareByService
		o.EPGCapture = opts.EPGCapture
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
		Channels: []string{reserve.NotifyChannel, assign.NotifyChannel, WatchNotifyChannel},
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
	now := s.clock.Now()
	// チューナーサーバーに問い合わせるのは、トランザクションを始める前にする。
	capture := s.epgCapture(ctx, now)
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.guard(ctx, tx); err != nil {
			return err
		}
		q := sqlc.New(tx)

		want, channels, err := s.plan(ctx, q, now, capture)
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

// epgCapture は、now に EPG の取得のために空けておくチューナーを決める設定を返す。EPGCapture がなければ nil。
// 読めなければ、取得の時刻も数も分からないもの (Known が偽) とする。EDCB は、一度読めた設定を、読み直せないときも使い続けるので
// (edcb.TunerServer の timerSettings)、読めないのは、起動してから一度も EDCB に届いていないときである。
func (s *Scheduler) epgCapture(ctx context.Context, now time.Time) *tunerserver.EPGCapture {
	if s.opts.EPGCapture == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, epgCaptureTimeout)
	defer cancel()
	capture, err := s.opts.EPGCapture.EPGCapture(ctx, now)
	if err == nil {
		return capture
	}
	s.logger.WarnContext(ctx, "failed to read the EPG capture settings of the tuner server", slog.Any("error", err))

	return &tunerserver.EPGCapture{
		Known: false, Near: false, AnyBonDriver: false, BonDrivers: map[string]tunerserver.EPGCaptureBonDriver{},
		DefaultBonDriver: tunerserver.EPGCaptureBonDriver{Disabled: false, Tuners: 0},
	}
}

// plan は、今の設定とチューナーと予約から、タイムシフトで録るサービスの ID の集合と、設定されたチャンネルを返す。
// capture は、EPG の取得のために空けておくチューナーを決める設定 (EDCB)。nil なら Mirakurun 互換のサーバーの数え方にする。
func (s *Scheduler) plan(ctx context.Context, q *sqlc.Queries, now time.Time, capture *tunerserver.EPGCapture) (map[int64]bool, []*Channel, error) {
	watched, err := syncWatched(ctx, q, now)
	if err != nil {
		return nil, nil, err
	}
	channelRows, err := q.ListTimeshiftChannels(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list timeshift channels: %w", err)
	}
	channels := make([]*Channel, 0, len(channelRows)+len(watched))
	for _, row := range channelRows {
		channels = append(channels, &Channel{ServiceID: row.ServiceID, ChannelType: row.ChannelType, Channel: row.Channel})
	}
	multiplexRows, err := q.ListServiceChannels(ctx, domain.ServiceGoneBefore(now))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list service channels: %w", err)
	}
	multiplexes := make([]*Multiplex, 0, len(multiplexRows))
	for _, row := range multiplexRows {
		multiplexes = append(multiplexes, &Multiplex{ChannelType: row.ChannelType, Channel: row.Channel})
	}
	tuners, err := s.tuners(ctx, q, now)
	if err != nil {
		return nil, nil, err
	}
	busyRows, err := q.ListReservationsUsingTunersBetween(ctx, sqlc.ListReservationsUsingTunersBetweenParams{FromAt: now, ToAt: now.Add(s.opts.Lead)})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list reservations using tuners: %w", err)
	}
	busy := make([]*Busy, 0, len(busyRows))
	for _, row := range busyRows {
		// assigned_tuner が NULL の行は、クエリで除いている。
		busy = append(busy, &Busy{Tuner: int(*row.AssignedTuner), ServiceID: row.ServiceID, ChannelType: row.ChannelType, Channel: row.Channel})
	}

	want := map[int64]bool{}
	planOptions := &PlanOptions{ShareByService: s.opts.ShareByService, IdleNeeds: nil, TunerNames: tuners.names, OtherIdleTuners: tuners.otherIdle}
	if capture != nil {
		planOptions.IdleNeeds = capture.Needs(tuners.allNames)
	}
	for _, serviceID := range Plan(channels, watched, multiplexes, tuners.usable, busy, planOptions) {
		want[serviceID] = true
	}
	// 視聴しているサービスの割り振りも書く。設定されたチャンネルを視聴しているなら、割り振りを 2 度書かない。
	for _, c := range watched {
		if !slices.ContainsFunc(channels, func(channel *Channel) bool { return channel.ServiceID == c.ServiceID }) {
			channels = append(channels, c)
		}
	}

	return want, channels, nil
}

// plannedTuners は、割り振りに使うチューナー。
type plannedTuners struct {
	// usable は、Kotatsu が使えて、故障しておらず、チューナーサーバー自身の予約が今から Lead の間に使わないチューナー。
	usable []*assign.Tuner
	// names は、usable の番号から、チューナーの名前 (EDCB では BonDriver) への対応。allNames は、すべてのチューナーの名前。
	names    map[int]string
	allNames []string
	// otherIdle は、Kotatsu が使わないチューナー (EDCB では [TVTEST] に並べていない BonDriver のもの) のうち、
	// チューナーサーバー自身の予約が今から Lead の間に使わないものの名前。EDCB の EPG の取得は、これらも使う (#1856)。
	otherIdle []string
}

// tuners は、割り振りに使うチューナーを読む。チューナーサーバー自身の予約 (EDCB の予約) が、今から Lead の間に使うチューナーは使わない (#1536)。
func (s *Scheduler) tuners(ctx context.Context, q *sqlc.Queries, now time.Time) (*plannedTuners, error) {
	tunerRows, err := q.ListTuners(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tuners: %w", err)
	}
	occupancies, err := q.ListTunerOccupancies(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tuner occupancies: %w", err)
	}
	occupied := map[int64]bool{}
	for _, o := range occupancies {
		if o.StartAt.Before(now.Add(s.opts.Lead)) && o.EndAt.After(now) {
			occupied[o.TunerIndex] = true
		}
	}
	planned := &plannedTuners{
		usable: make([]*assign.Tuner, 0, len(tunerRows)), names: map[int]string{}, allNames: make([]string, 0, len(tunerRows)), otherIdle: []string{},
	}
	for _, row := range tunerRows {
		planned.allNames = append(planned.allNames, row.Name)
		if occupied[row.TunerIndex] || row.IsFault {
			continue
		}
		if !row.IsAvailable {
			planned.otherIdle = append(planned.otherIdle, row.Name)

			continue
		}
		planned.usable = append(planned.usable, &assign.Tuner{Index: int(row.TunerIndex), Types: row.Types, Blocks: []assign.Block{}})
		planned.names[int(row.TunerIndex)] = row.Name
	}

	return planned, nil
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
		finished := a.State == domain.RecordingAssignmentStateFinished
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
