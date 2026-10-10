package assign

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/notify"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/tunerstate"
)

const (
	// NotifyChannel は、割り当ての結果が変わったことを知らせる変更通知のチャンネル。
	NotifyChannel = "assignments"
	// ConflictChannel は、新たな競合が生じたことを知らせる変更通知のチャンネル (設計書 6.2)。
	// 通知の配信 (#40) は、これを受けて、競合した予約を読み直す。
	ConflictChannel = "reservation_conflicts"
	// DefaultDebounce は、変化を受けてから計算し直すまで待つ時間。立て続けの変化を 1 回にまとめる (設計書 4.5)。
	DefaultDebounce = 2 * time.Second
	// DefaultInterval は、変化がなくても計算し直す間隔。録画が始まった予約を固定するのにも使う。
	DefaultInterval = time.Minute
)

// Guard は、書き込みのトランザクションの中で、まだ neko のリーダーであることを確かめる。
type Guard func(ctx context.Context, tx pgx.Tx) error

// RunnerOptions は Runner の設定。0 の項目は既定値にする。
type RunnerOptions struct {
	Debounce time.Duration
	Interval time.Duration
	// PollOnly は、変更の通知を LISTEN で受けずに、ポーリングだけで知るかどうか。
	PollOnly     bool
	PollInterval time.Duration
}

// Runner は、予約、EPG、チューナーの構成が変わるたびに、割り当てを計算し直して DB に書く (設計書 4.5)。
type Runner struct {
	pool     *pgxpool.Pool
	clock    clock.Clock
	guard    Guard
	logger   *slog.Logger
	defaults DefaultsFunc
	opts     RunnerOptions
}

// DefaultsFunc は、録画設定の全体の既定値を返す。画面から変えられるので (設計書 11.2)、割り当てのたびに引く。
type DefaultsFunc func(ctx context.Context, db sqlc.DBTX) (*reserve.Settings, error)

// FixedDefaults は、いつも settings を返す DefaultsFunc を返す。
func FixedDefaults(settings *reserve.Settings) DefaultsFunc {
	return func(context.Context, sqlc.DBTX) (*reserve.Settings, error) {
		return settings, nil
	}
}

// NewRunner は Runner を作る。defaults は録画設定の全体の既定値を返す。opts は nil でもよい。
func NewRunner(pool *pgxpool.Pool, clk clock.Clock, guard Guard, logger *slog.Logger, defaults DefaultsFunc, opts *RunnerOptions) *Runner {
	o := RunnerOptions{Debounce: DefaultDebounce, Interval: DefaultInterval, PollOnly: false, PollInterval: notify.DefaultPollInterval}
	if opts != nil {
		o.PollOnly = opts.PollOnly
		if opts.Debounce > 0 {
			o.Debounce = opts.Debounce
		}
		if opts.Interval > 0 {
			o.Interval = opts.Interval
		}
		if opts.PollInterval > 0 {
			o.PollInterval = opts.PollInterval
		}
	}

	return &Runner{pool: pool, clock: clk, guard: guard, logger: logger, defaults: defaults, opts: o}
}

// Run は、ctx が終わるまで、始めたとき、予約か EPG かチューナーの構成が変わったとき、Interval ごとに計算し直す。
func (r *Runner) Run(ctx context.Context) error {
	return notify.Loop(ctx, r.pool, r.logger, &notify.LoopOptions{
		// 予約中の番組の EPG が変わると、ルールの照合 (#19) と番組の変更への追従 (#21) が予約を書き換えるので、
		// 予約の通知で計算し直せる。EPG の通知も見るのは、取りこぼしに備えるためである。
		Channels: []string{reserve.NotifyChannel, epg.NotifyChannel, tunerstate.NotifyChannel},
		Debounce: r.opts.Debounce,
		Interval: r.opts.Interval,
		Watcher:  notify.WatcherOptions{UseListen: !r.opts.PollOnly, PollInterval: r.opts.PollInterval},
	}, func(ctx context.Context) {
		result, err := r.RunOnce(ctx)
		if err != nil {
			if ctx.Err() == nil {
				r.logger.WarnContext(ctx, "failed to assign tuners", slog.Any("error", err))
			}

			return
		}
		for _, reservationID := range result.NewConflicts {
			r.logger.WarnContext(ctx, "reservation conflicts; not enough tuners", slog.String("reservation_id", reservationID.String()))
		}
	})
}

// Result は、1 回の計算の結果。
type Result struct {
	// Changed は、割り当ての結果が変わった予約の数。
	Changed int
	// NewConflicts は、新たに競合になった予約。
	NewConflicts []uuid.UUID
}

// RunOnce は、割り当てを 1 回計算し直し、変わった結果を DB に書く。
// 新たに競合になった予約があれば、ConflictChannel で通知する。
func (r *Runner) RunOnce(ctx context.Context) (*Result, error) {
	result := &Result{Changed: 0, NewConflicts: []uuid.UUID{}}
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := r.guard(ctx, tx); err != nil {
			return err
		}
		q := sqlc.New(tx)
		now := r.clock.Now()
		rows, err := q.ListAssignableReservations(ctx, now)
		if err != nil {
			return fmt.Errorf("failed to list reservations: %w", err)
		}
		defaults, err := r.defaults(ctx, tx)
		if err != nil {
			return fmt.Errorf("failed to get the default recording settings: %w", err)
		}
		reservations, err := r.inputs(ctx, q, defaults, rows, now)
		if err != nil {
			return err
		}
		tuners, err := usableTuners(ctx, q)
		if err != nil {
			return err
		}

		for i, a := range Assign(reservations, tuners) {
			changed, conflicted, err := update(ctx, q, &rows[i], a)
			if err != nil {
				return err
			}
			if changed {
				result.Changed++
			}
			if conflicted {
				result.NewConflicts = append(result.NewConflicts, a.ReservationID)
			}
		}
		cleared, err := q.ClearDisabledAssignments(ctx)
		if err != nil {
			return fmt.Errorf("failed to clear assignments of disabled reservations: %w", err)
		}
		result.Changed += int(cleared)

		return r.publish(ctx, tx, result)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to assign tuners: %w", err)
	}

	return result, nil
}

// inputs は、DB の予約を割り当ての入力にする。録画設定は、全体の既定値に、ルール予約ならルールの上書き、
// さらに予約自体の上書きを重ねて求める。予定の始まりが過ぎた予約は、録画が始まったものとして、割り当てたチューナーから動かさない。
func (r *Runner) inputs(
	ctx context.Context,
	q *sqlc.Queries,
	defaults *reserve.Settings,
	rows []sqlc.ListAssignableReservationsRow,
	now time.Time,
) ([]*Reservation, error) {
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	ruleRows, err := q.ListFirstRuleSettings(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list rule settings: %w", err)
	}
	ruleSettings := map[uuid.UUID][]byte{}
	for _, row := range ruleRows {
		ruleSettings[row.ReservationID] = row.Settings
	}

	reservations := make([]*Reservation, 0, len(rows))
	for _, row := range rows {
		settings := defaults
		if raw, ok := ruleSettings[row.ID]; ok && reserve.Kind(row.Kind) == reserve.KindRule {
			overrides, err := reserve.DecodeOverrides(raw)
			if err != nil {
				return nil, err
			}
			settings = settings.Resolve(overrides)
		}
		overrides, err := reserve.DecodeOverrides(row.Settings)
		if err != nil {
			return nil, err
		}
		settings = settings.Resolve(overrides)

		recording := row.PlannedStartAt != nil && !row.PlannedStartAt.After(now) &&
			row.AssignedTuner != nil && State(row.AssignmentState) != StateConflict
		tuner := 0
		if recording {
			tuner = int(*row.AssignedTuner)
		}
		reservations = append(reservations, &Reservation{
			ID:          row.ID,
			ChannelType: row.ChannelType,
			Channel:     row.Channel,
			StartAt:     row.StartAt.UTC(),
			EndAt:       row.EndAt.UTC(),
			StartMargin: time.Duration(settings.StartMarginSeconds) * time.Second,
			EndMargin:   time.Duration(settings.EndMarginSeconds) * time.Second,
			Priority:    settings.Priority,
			Recording:   recording,
			Tuner:       tuner,
		})
	}

	return reservations, nil
}

func usableTuners(ctx context.Context, q *sqlc.Queries) ([]*Tuner, error) {
	rows, err := q.ListUsableTuners(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tuners: %w", err)
	}
	tuners := make([]*Tuner, 0, len(rows))
	for _, row := range rows {
		tuners = append(tuners, &Tuner{Index: int(row.TunerIndex), Types: row.Types})
	}

	return tuners, nil
}

// update は、割り当ての結果が変わっていれば DB に書く。新たに競合になったかどうかも返す。
func update(ctx context.Context, q *sqlc.Queries, row *sqlc.ListAssignableReservationsRow, a *Assignment) (bool, bool, error) {
	var tuner *int64
	if a.Tuner != NoTuner {
		tuner = new(int64(a.Tuner))
	}
	same := row.AssignmentState == string(a.State) && equalPtr(row.AssignedTuner, tuner) &&
		row.PlannedStartAt != nil && row.PlannedStartAt.Equal(a.PlannedStart) &&
		row.PlannedEndAt != nil && row.PlannedEndAt.Equal(a.PlannedEnd)
	if same {
		return false, false, nil
	}

	err := q.UpdateAssignment(ctx, sqlc.UpdateAssignmentParams{
		ID:              a.ReservationID,
		AssignmentState: string(a.State),
		AssignedTuner:   tuner,
		PlannedStartAt:  &a.PlannedStart,
		PlannedEndAt:    &a.PlannedEnd,
	})
	if err != nil {
		return false, false, fmt.Errorf("failed to update the assignment of %s: %w", a.ReservationID, err)
	}

	return true, a.State == StateConflict && State(row.AssignmentState) != StateConflict, nil
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}

	return *a == *b
}

// publish は、結果が変わったことと、新たな競合を通知する。
func (r *Runner) publish(ctx context.Context, tx pgx.Tx, result *Result) error {
	if result.Changed > 0 {
		if _, err := notify.Publish(ctx, tx, r.clock, NotifyChannel); err != nil {
			return err
		}
	}
	if len(result.NewConflicts) > 0 {
		if _, err := notify.Publish(ctx, tx, r.clock, ConflictChannel); err != nil {
			return err
		}
	}

	return nil
}
