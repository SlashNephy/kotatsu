package reserve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/notify"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	// relatedRelay は、別のサービスに引き継ぐ関係 (イベントリレー)。
	relatedRelay = "relay"
	// serviceIDBase は、Mirakurun のサービス ID を組み立てる基数 (networkId × 100000 + serviceId)。
	serviceIDBase = 100_000
	// DefaultFollowDebounce と DefaultFollowInterval は、EPG が変わってから追従するまで待つ時間と、変化がなくても追従する間隔。
	DefaultFollowDebounce = 2 * time.Second
	DefaultFollowInterval = 5 * time.Minute
	// DefaultFinishedGrace は、録画の予定の終わりが過ぎてから、終わった予約を消すまでの猶予。
	DefaultFinishedGrace = time.Hour
	// RelayWindow は、予約の終わりが過ぎてからも、イベントリレーの引き継ぎ先が EPG に出るのを待つ時間 (#639)。
	// 終わった予約は DefaultFinishedGrace で消えるので、それより長くしない。
	RelayWindow = DefaultFinishedGrace
)

// Guard は、書き込みのトランザクションの中で、まだ neko のリーダーであることを確かめる。
type Guard func(ctx context.Context, tx pgx.Tx) error

// FollowerOptions は Follower の設定。0 の項目は既定値にする。
type FollowerOptions struct {
	Debounce time.Duration
	Interval time.Duration
	// FinishedGrace は、録画の予定の終わりが過ぎてから、終わった予約を消すまでの猶予。
	FinishedGrace time.Duration
	// PollOnly は、変更の通知を LISTEN で受けずに、ポーリングだけで知るかどうか。
	PollOnly     bool
	PollInterval time.Duration
}

// Follower は、番組の変更に予約を追従させる (設計書 4.6)。
//
//   - EPG で放送時間や番組情報が変わったら、予約の番組情報と時刻を更新する。予約が変わると、割り当ても計算し直される。
//   - イベントリレーで別のサービスに引き継がれる番組では、予約で追従が有効なら、引き継ぎ先の番組予約を作る。
//   - 放送が終わって猶予が過ぎ、録画も終わった予約を消す。EID が別の番組に再利用されても、その番組を予約できるようにする (#510)。
type Follower struct {
	pool         *pgxpool.Pool
	guard        Guard
	logger       *slog.Logger
	reservations *Reservations
	opts         FollowerOptions
}

// NewFollower は Follower を作る。opts は nil でもよい。
func NewFollower(pool *pgxpool.Pool, guard Guard, logger *slog.Logger, reservations *Reservations, opts *FollowerOptions) *Follower {
	o := FollowerOptions{
		Debounce: DefaultFollowDebounce, Interval: DefaultFollowInterval, FinishedGrace: DefaultFinishedGrace,
		PollOnly: false, PollInterval: notify.DefaultPollInterval,
	}
	if opts != nil {
		o.PollOnly = opts.PollOnly
		if opts.Debounce > 0 {
			o.Debounce = opts.Debounce
		}
		if opts.Interval > 0 {
			o.Interval = opts.Interval
		}
		if opts.FinishedGrace > 0 {
			o.FinishedGrace = opts.FinishedGrace
		}
		if opts.PollInterval > 0 {
			o.PollInterval = opts.PollInterval
		}
	}

	return &Follower{pool: pool, guard: guard, logger: logger, reservations: reservations, opts: o}
}

// Run は、ctx が終わるまで、始めたとき、EPG が変わったとき、Interval ごとに追従する。
func (f *Follower) Run(ctx context.Context) error {
	return notify.Loop(ctx, f.pool, f.logger, &notify.LoopOptions{
		Channels: []string{epg.NotifyChannel},
		Debounce: f.opts.Debounce,
		Interval: f.opts.Interval,
		Watcher:  notify.WatcherOptions{UseListen: !f.opts.PollOnly, PollInterval: f.opts.PollInterval},
	}, func(ctx context.Context) {
		result, err := f.RunOnce(ctx)
		if err != nil {
			if ctx.Err() == nil {
				f.logger.WarnContext(ctx, "failed to follow program changes", slog.Any("error", err))
			}

			return
		}
		if result.Updated > 0 || result.Relayed > 0 || result.Removed > 0 {
			f.logger.InfoContext(ctx, "followed program changes",
				slog.Int("updated", result.Updated), slog.Int("relayed", result.Relayed), slog.Int("removed", result.Removed))
		}
	})
}

// FollowResult は、1 回の追従の結果。
type FollowResult struct {
	// Updated は、番組情報か時刻を更新した予約の数。
	Updated int
	// Relayed は、イベントリレーの引き継ぎ先に作った予約の数。
	Relayed int
	// Removed は、放送が終わったので消した予約の数。
	Removed int
}

// followerActor は、追従で予約を変えたときの、監査ログの操作者。
func followerActor() *audit.Actor {
	return audit.System("neko")
}

// RunOnce は、予約を今の EPG に 1 回追従させる。
func (f *Follower) RunOnce(ctx context.Context) (*FollowResult, error) {
	result := &FollowResult{Updated: 0, Relayed: 0, Removed: 0}
	err := pgx.BeginFunc(ctx, f.pool, func(tx pgx.Tx) error {
		if err := f.guard(ctx, tx); err != nil {
			return err
		}
		q := sqlc.New(tx)
		now := f.reservations.clock.Now()
		removed, err := f.removeFinished(ctx, tx, now)
		if err != nil {
			return err
		}
		result.Removed = removed
		rows, err := q.ListReservationsToFollow(ctx, now)
		if err != nil {
			return fmt.Errorf("failed to list reservations: %w", err)
		}

		for i := range rows {
			updated, err := f.update(ctx, tx, &rows[i], now)
			if err != nil {
				return err
			}
			if updated {
				result.Updated++
			}
		}

		// 無効にした予約と、追従しない予約は、引き継ぎ先に予約を作って録り直さない。
		relaying, err := q.ListReservationsToRelay(ctx, now.Add(-RelayWindow))
		if err != nil {
			return fmt.Errorf("failed to list reservations to relay: %w", err)
		}
		for i := range relaying {
			relayed, err := f.relay(ctx, tx, &relaying[i], now)
			if err != nil {
				return err
			}
			result.Relayed += relayed
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to follow program changes: %w", err)
	}

	return result, nil
}

// update は、番組情報か時刻が変わっていれば、予約を更新する。
// 延長などで番組の終了時刻が未定になったときは、終了時刻を縮めずに今の値のままにする。
func (f *Follower) update(ctx context.Context, tx pgx.Tx, row *sqlc.ListReservationsToFollowRow, now time.Time) (bool, error) {
	endAt := row.ProgramEndAt
	if row.ProgramEndUndecided {
		// 開始が遅れたときは、予約の長さを保ったまま後ろにずらす。
		endAt = later(row.EndAt, row.ProgramStartAt.Add(row.EndAt.Sub(row.StartAt)))
	}
	same := row.StartAt.Equal(row.ProgramStartAt) && row.EndAt.Equal(endAt) &&
		row.Name == row.ProgramName && row.Description == row.ProgramDescription &&
		bytes.Equal(row.Extended, row.ProgramExtended) && bytes.Equal(row.Genres, row.ProgramGenres)
	if same || !row.ProgramStartAt.Before(endAt) {
		return false, nil
	}

	err := sqlc.New(tx).UpdateReservationProgram(ctx, sqlc.UpdateReservationProgramParams{
		ID:          row.ID,
		StartAt:     row.ProgramStartAt,
		EndAt:       endAt,
		Name:        row.ProgramName,
		Description: row.ProgramDescription,
		Extended:    row.ProgramExtended,
		Genres:      row.ProgramGenres,
		Now:         now,
	})
	if err != nil {
		return false, fmt.Errorf("failed to update reservation %s: %w", row.ID, err)
	}
	reservation, err := get(ctx, sqlc.New(tx), row.ID)
	if err != nil {
		return false, err
	}
	summary := summaryOf(reservation)
	summary["changed"] = []string{"program"}

	return true, f.reservations.record(ctx, tx, followerActor(), "reservation.update", reservation, summary)
}

// relay は、番組のイベントリレーの引き継ぎ先に、番組予約を作る。引き継ぎ先の番組が EPG にまだ無ければ、次の追従を待つ。
// 引き継ぎ先の予約は、元の予約の録画設定を引き継ぎ、元の予約と関連付ける。
// ルール予約の録画設定はルールにあるので、一致したルールの上書きを引き継ぐ。
func (f *Follower) relay(ctx context.Context, tx pgx.Tx, row *sqlc.ListReservationsToRelayRow, now time.Time) (int, error) {
	var related []*mirakurun.RelatedItem
	if err := json.Unmarshal(row.ProgramRelatedItems, &related); err != nil {
		return 0, fmt.Errorf("failed to decode the related items of reservation %s: %w", row.ID, err)
	}

	settings := row.Settings
	if Kind(row.Kind) == KindRule && row.RuleSettings != nil {
		settings = row.RuleSettings
	}

	relayed := 0
	q := sqlc.New(tx)
	for _, item := range related {
		if item.Type != relatedRelay {
			continue
		}
		networkID := int64(item.NetworkID)
		if networkID == 0 {
			networkID = row.ServiceID / serviceIDBase
		}
		serviceID := networkID*serviceIDBase + int64(item.ServiceID)
		reservationID, err := insertRelayed(ctx, tx, &sqlc.InsertRelayedReservationParams{
			ID:               id.New(),
			Settings:         settings,
			Now:              now,
			UndecidedSeconds: UndecidedLength.Seconds(),
			RelayedFrom:      &row.ID,
			ServiceID:        serviceID,
			EventID:          int64(item.EventID),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// 引き継ぎ先の予約がすでにあるか、引き継ぎ先の番組が EPG にまだ無い。
			continue
		}
		if err != nil {
			// 1 件の引き継ぎ先を作れなくても、ほかの予約の追従は止めない (#521)。
			f.logger.WarnContext(ctx, "failed to create a relayed reservation", slog.String("reservation_id", row.ID.String()), slog.Any("error", err))

			continue
		}
		reservation, err := get(ctx, q, reservationID)
		if err != nil {
			return relayed, err
		}
		summary := summaryOf(reservation)
		summary["relayedFrom"] = row.ID.String()
		if err := f.reservations.record(ctx, tx, followerActor(), "reservation.create", reservation, summary); err != nil {
			return relayed, err
		}
		relayed++
	}

	return relayed, nil
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}

	return b
}

// removeFinished は、放送が終わって猶予が過ぎ、録画も終わった予約を消す。消した予約の数を返す。
// 番組の予約は (サービス, EID) で一意なので、残すと EID が再利用されたときに新しい番組を予約できない (#510)。
// 録画と録画履歴は予約を消しても残る。
func (f *Follower) removeFinished(ctx context.Context, tx pgx.Tx, now time.Time) (int, error) {
	ids, err := sqlc.New(tx).ListFinishedReservations(ctx, now.Add(-f.opts.FinishedGrace))
	if err != nil {
		return 0, fmt.Errorf("failed to list finished reservations: %w", err)
	}
	removed := 0
	for _, reservationID := range ids {
		err := f.reservations.remove(ctx, tx, followerActor(), reservationID)
		// 一覧を読んだ後に、ユーザーが先に消していれば飛ばす。
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return 0, err
		}
		removed++
	}

	return removed, nil
}

// insertRelayed は、引き継ぎ先の予約を、セーブポイントの中で作る。失敗しても、追従のトランザクションは続けられる。
func insertRelayed(ctx context.Context, tx pgx.Tx, params *sqlc.InsertRelayedReservationParams) (uuid.UUID, error) {
	var reservationID uuid.UUID
	err := pgx.BeginFunc(ctx, tx, func(sp pgx.Tx) error {
		var err error
		reservationID, err = sqlc.New(sp).InsertRelayedReservation(ctx, *params)

		return err
	})

	return reservationID, err
}
