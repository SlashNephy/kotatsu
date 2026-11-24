// Package networktv は、EDCB の NetworkTV の ID を、Kotatsu のインスタンスに割り当てる (#1533、設計書 3.1)。
//
// NetworkTV の ID は EpgTimerSrv 全体で 1 つの名前空間で、持ち主の概念がない。tenban、mikan、kyusu、nabe が何台あっても
// ぶつからないよう、DB の edcb_network_tvs に ID を、edcb_network_tv_holders にその持ち主のインスタンスを記録する。
// EDCB は 1 つの NetworkTV の TS を 2 つの中継に送れるので、同じサービスを開く 2 つのインスタンスは、同じ ID を持つ (#1850)。
// 最後の持ち主が去ったときだけ閉じる。持ち主が止まって閉じられなかった ID は、neko の Sweeper が閉じてから消す。
package networktv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
	"github.com/SlashNephy/kotatsu/internal/infra/registry"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

const (
	// DefaultSweepInterval は、持ち主のいない ID を探す間隔。
	DefaultSweepInterval = time.Minute
	// MaxHolders は、1 つの ID を持てるインスタンスの数。EDCB は、1 つの NetworkTV の TS を 2 つの中継 (RELAY_VIEW_STREAM) にまで送り、
	// 3 つ目の中継は断る (2026年11月に、Linux の EDCB で確認した。#1850)。
	MaxHolders = 2
	// closingRetryAfter は、閉じ始めたまま残った ID を、Sweeper が閉じ直すまでの時間。閉じる途中で止まったインスタンスの ID を閉じる。
	// 閉じるのに掛ける時間の上限 (10 秒) より長くする。
	closingRetryAfter = 30 * time.Second
)

// Database は、ID を読み書きする DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// IDs は、設定した範囲の NetworkTV の ID を、インスタンス instanceID に割り当てる。
type IDs struct {
	db         Database
	clock      clock.Clock
	instanceID uuid.UUID
	start      int32
	count      int32
}

var _ edcb.NetworkTVIDs = new(IDs)

// NewIDs は IDs を返す。範囲は [start, start+count) で、1 から int32 の最大値までに収まらなければエラーを返す。
func NewIDs(db Database, clk clock.Clock, instanceID uuid.UUID, start, count int) (*IDs, error) {
	if start < 1 || start > math.MaxInt32 || count < 1 || count > math.MaxInt32 || start > math.MaxInt32-count+1 {
		return nil, fmt.Errorf("invalid NetworkTV ID range: start %d, count %d", start, count)
	}

	return &IDs{db: db, clock: clk, instanceID: instanceID, start: int32(start), count: int32(count)}, nil
}

// Acquire は、service の NetworkTV の ID を取る。
//   - exclusive でなく、ほかのインスタンスが同じサービスの ID を持っていて、生きている持ち主が MaxHolders に満たなければ、
//     その ID に加わり、shared に true を返す (#1850)。持ち主が止まったか、止まるときに残した ID にも加わる。
//   - 加われなければ、空いている最も小さい ID を取る。空いていなければ edcb.ErrNoNetworkTVID を返す。
//
// 同じサービスについては、インスタンスをまたいで 1 つずつ行うので、同時に開いても、別々の ID を取らない。
// ほかのサービスを開くインスタンスと同じ ID を取り合ったときは、負けた側が次の ID を探し直す。
func (i *IDs) Acquire(ctx context.Context, service edcb.ServiceKey, exclusive bool) (int32, bool, error) {
	for range i.count {
		var id int32
		var shared bool
		err := pgx.BeginFunc(ctx, i.db, func(tx pgx.Tx) error {
			q := sqlc.New(tx)
			now := i.clock.Now()
			if err := q.LockEDCBNetworkTVService(ctx, sqlc.LockEDCBNetworkTVServiceParams{
				NetworkID: int32(service.ONID), ServiceID: int32(service.SID),
			}); err != nil {
				return err
			}
			var err error
			if !exclusive {
				id, err = q.FindJoinableEDCBNetworkTV(ctx, sqlc.FindJoinableEDCBNetworkTVParams{
					NetworkID: int32(service.ONID), ServiceID: int32(service.SID), Start: i.start, Count: i.count,
					InstanceID: i.instanceID, AliveSince: now.Add(-registry.AliveTTL), MaxHolders: MaxHolders,
				})
				shared = err == nil
			}
			if exclusive || errors.Is(err, pgx.ErrNoRows) {
				id, err = q.AcquireEDCBNetworkTV(ctx, sqlc.AcquireEDCBNetworkTVParams{
					NetworkID: int32(service.ONID), ServiceID: int32(service.SID), Now: now, Start: i.start, Count: i.count,
				})
			}
			if err != nil {
				return err
			}

			return q.AddEDCBNetworkTVHolder(ctx, sqlc.AddEDCBNetworkTVHolderParams{NetworkTVID: id, InstanceID: i.instanceID, Now: now})
		})
		if err == nil {
			return id, shared, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, false, fmt.Errorf("failed to acquire a NetworkTV ID: %w", err)
		}
		// 空きが見えたのに取れなかったのは、ほかのインスタンスと取り合ったとき。空きがなければ、次も取れない。
		free, err := i.free(ctx)
		if err != nil {
			return 0, false, err
		}
		if !free {
			break
		}
	}

	return 0, false, edcb.ErrNoNetworkTVID
}

// Release は、ID の持ち主から外れる。ほかに生きている持ち主がいなければ、last に true を返す。
//   - keep でなければ、最後の持ち主が外れたときに、閉じ始めたことを記録する。その後はだれも加わらないので、
//     呼び出し側は NWTV_ID_CLOSE を送ってから Forget する。
//   - keep なら、最後の持ち主でも閉じ始めず、外れた時刻だけを記録して、ほかのインスタンスが加われるよう残す。
//     呼び出し側は、待ってから CloseIfUnheld を呼ぶ。呼べずに止まったときは、neko が閉じる。
func (i *IDs) Release(ctx context.Context, id int32, keep bool) (bool, error) {
	var last bool
	err := pgx.BeginFunc(ctx, i.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		// 行を押さえてから数え直す。加わる途中のインスタンスがいれば、加わり終えるのを待ってから数える。
		if _, err := q.LockEDCBNetworkTV(ctx, id); err != nil {
			return err
		}
		if err := q.DeleteEDCBNetworkTVHolder(ctx, sqlc.DeleteEDCBNetworkTVHolderParams{NetworkTVID: id, InstanceID: i.instanceID}); err != nil {
			return err
		}
		now := i.clock.Now()
		holders, err := q.CountAliveEDCBNetworkTVHolders(ctx, sqlc.CountAliveEDCBNetworkTVHoldersParams{NetworkTVID: id, AliveSince: now.Add(-registry.AliveTTL)})
		if err != nil {
			return err
		}
		if holders > 0 {
			return nil
		}
		last = true
		if keep {
			return q.MarkEDCBNetworkTVReleased(ctx, sqlc.MarkEDCBNetworkTVReleasedParams{Now: now, ID: id})
		}

		return q.MarkEDCBNetworkTVClosing(ctx, sqlc.MarkEDCBNetworkTVClosingParams{Now: now, ID: id})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// neko が先に閉じて消した。
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to release NetworkTV ID %d: %w", id, err)
	}

	return last, nil
}

// CloseIfUnheld は、keep で残した ID に、生きている持ち主がいなければ閉じ始めたことを記録し、true を返す。
// true を返したら、呼び出し側は NWTV_ID_CLOSE を送ってから Forget する。
func (i *IDs) CloseIfUnheld(ctx context.Context, id int32) (bool, error) {
	closing, err := closeIfUnheld(ctx, i.db, i.clock.Now(), id, false)
	if err != nil {
		return false, fmt.Errorf("failed to check NetworkTV ID %d: %w", id, err)
	}

	return closing, nil
}

// Forget は、閉じた ID を消す。
func (i *IDs) Forget(ctx context.Context, id int32) error {
	if err := sqlc.New(i.db).DeleteEDCBNetworkTV(ctx, id); err != nil {
		return fmt.Errorf("failed to delete NetworkTV ID %d: %w", id, err)
	}

	return nil
}

// free は、範囲に空いている ID があるかを返す。
func (i *IDs) free(ctx context.Context) (bool, error) {
	used, err := sqlc.New(i.db).CountEDCBNetworkTVs(ctx, sqlc.CountEDCBNetworkTVsParams{Start: i.start, Count: i.count})
	if err != nil {
		return false, fmt.Errorf("failed to count NetworkTV IDs: %w", err)
	}

	return used < int64(i.count), nil
}

// closeIfUnheld は、行を押さえてから、ID に生きている持ち主がいないことを確認し、閉じ始めたことを記録する。
// 押さえる前に、ほかのインスタンスが加わったか、閉じ始めていれば false を返す。行がなければ false を返す。
// orphan なら、閉じ始めたまま残った ID も閉じ直し、最後の持ち主が外れた直後の ID は閉じない (Sweeper)。
func closeIfUnheld(ctx context.Context, db Database, now time.Time, id int32, orphan bool) (bool, error) {
	var closing bool
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		row, err := q.LockEDCBNetworkTV(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		aliveSince := now.Add(-registry.AliveTTL)
		switch {
		case row.ClosingAt != nil && (!orphan || row.ClosingAt.After(now.Add(-closingRetryAfter))):
			// ほかのインスタンスが閉じている。
			return nil
		case orphan && row.ReleasedAt != nil && row.ReleasedAt.After(aliveSince):
			// 止まるインスタンスが、引き継げるように残した直後。
			return nil
		}
		holders, err := q.CountAliveEDCBNetworkTVHolders(ctx, sqlc.CountAliveEDCBNetworkTVHoldersParams{NetworkTVID: id, AliveSince: aliveSince})
		if err != nil || holders > 0 {
			return err
		}
		closing = true

		return q.MarkEDCBNetworkTVClosing(ctx, sqlc.MarkEDCBNetworkTVClosingParams{Now: now, ID: id})
	})

	return closing, err
}

// Closer は、NetworkTV を閉じる。*edcb.Client が満たす。
type Closer interface {
	CloseNetworkTV(ctx context.Context, id int32) error
}

var _ Closer = new(edcb.Client)

// Sweeper は、生きている持ち主のいない ID の NetworkTV を閉じ、ID を消す。neko のリーダーが動かす。
type Sweeper struct {
	db       Database
	clock    clock.Clock
	closer   Closer
	logger   *slog.Logger
	interval time.Duration
}

// NewSweeper は Sweeper を返す。interval が 0 なら DefaultSweepInterval にする。
func NewSweeper(db Database, clk clock.Clock, closer Closer, logger *slog.Logger, interval time.Duration) *Sweeper {
	if interval <= 0 {
		interval = DefaultSweepInterval
	}

	return &Sweeper{db: db, clock: clk, closer: closer, logger: logger, interval: interval}
}

// Run は、ctx が終わるまで、起動したときと interval ごとに Sweep する。
func (s *Sweeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		if err := s.Sweep(ctx); err != nil && ctx.Err() == nil {
			s.logger.WarnContext(ctx, "failed to close orphaned NetworkTVs", slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Sweep は、止まったインスタンスを ID の持ち主から外し、生きている持ち主のいない ID の NetworkTV を閉じて、ID を消す。
//   - ほかに生きている持ち主がいる ID は、閉じない。その持ち主が、同じ TS を受け取り続けている (#1850)。
//   - 取った直後の ID は、インスタンスを登録する前の持ち主のものかもしれないので、生きているかを判定できる時間 (AliveTTL) を待つ。
//     止まるインスタンスが引き継げるよう残した ID も、同じだけ待つ。
//   - 閉じられなかった ID は、閉じ始めたまま残し、closingRetryAfter の後に閉じ直す。
func (s *Sweeper) Sweep(ctx context.Context) error {
	now := s.clock.Now()
	q := sqlc.New(s.db)
	aliveSince := now.Add(-registry.AliveTTL)
	if _, err := q.DeleteDeadEDCBNetworkTVHolders(ctx, aliveSince); err != nil {
		return fmt.Errorf("failed to delete dead NetworkTV holders: %w", err)
	}
	orphans, err := q.ListOrphanedEDCBNetworkTVs(ctx, sqlc.ListOrphanedEDCBNetworkTVsParams{
		AliveSince: aliveSince, ClosingBefore: now.Add(-closingRetryAfter),
	})
	if err != nil {
		return fmt.Errorf("failed to list orphaned NetworkTVs: %w", err)
	}
	var errs []error
	for _, orphan := range orphans {
		closing, err := closeIfUnheld(ctx, s.db, now, orphan.ID, true)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to lock NetworkTV ID %d: %w", orphan.ID, err))

			continue
		}
		if !closing {
			continue
		}
		if err := s.closer.CloseNetworkTV(ctx, orphan.ID); err != nil {
			errs = append(errs, fmt.Errorf("failed to close NetworkTV %d: %w", orphan.ID, err))

			continue
		}
		if err := q.DeleteEDCBNetworkTV(ctx, orphan.ID); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete NetworkTV ID %d: %w", orphan.ID, err))

			continue
		}
		s.logger.InfoContext(ctx, "closed an orphaned NetworkTV", slog.Int("id", int(orphan.ID)))
	}

	return errors.Join(errs...)
}
