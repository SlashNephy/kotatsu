// Package networktv は、EDCB の NetworkTV の ID を、Kotatsu のインスタンスに配る (#1533、設計書 3.1)。
//
// NetworkTV の ID は EpgTimerSrv 全体で 1 つの名前空間で、持ち主の概念がない。tenban、mikan、kyusu が何台あっても
// ぶつからないよう、DB の edcb_network_tvs に持ち主のインスタンスを記録する。持ち主が止まって閉じられなかった ID は、
// neko の Sweeper が閉じてから消す。
package networktv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
	"github.com/SlashNephy/kotatsu/internal/infra/registry"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// DefaultSweepInterval は、持ち主のいない ID を探す間隔。
const DefaultSweepInterval = time.Minute

// IDs は、設定した範囲の NetworkTV の ID を、インスタンス instanceID に配る。
type IDs struct {
	db         sqlc.DBTX
	clock      clock.Clock
	instanceID uuid.UUID
	start      int32
	count      int32
}

var _ edcb.NetworkTVIDs = new(IDs)

// NewIDs は IDs を返す。範囲は [start, start+count)。
func NewIDs(db sqlc.DBTX, clk clock.Clock, instanceID uuid.UUID, start, count int32) *IDs {
	return &IDs{db: db, clock: clk, instanceID: instanceID, start: start, count: count}
}

// Acquire は、空いている最も小さい ID を取る。空いていなければ edcb.ErrNoNetworkTVID を返す。
// ほかのインスタンスと同じ ID を取り合ったときは、負けた側が次の ID を探し直す。
func (i *IDs) Acquire(ctx context.Context, service edcb.ServiceKey) (int32, error) {
	q := sqlc.New(i.db)
	for range i.count {
		id, err := q.AcquireEDCBNetworkTV(ctx, sqlc.AcquireEDCBNetworkTVParams{
			InstanceID: i.instanceID, NetworkID: int32(service.ONID), ServiceID: int32(service.SID), Now: i.clock.Now(),
			Start: i.start, Count: i.count,
		})
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("failed to acquire a NetworkTV ID: %w", err)
		}
		// 空きが見えたのに取れなかったのは、ほかのインスタンスと取り合ったとき。空きがなければ、次も取れない。
		free, err := i.free(ctx)
		if err != nil {
			return 0, err
		}
		if !free {
			break
		}
	}

	return 0, edcb.ErrNoNetworkTVID
}

// Release は、取った ID を返す。
func (i *IDs) Release(ctx context.Context, id int32) error {
	if err := sqlc.New(i.db).ReleaseEDCBNetworkTV(ctx, sqlc.ReleaseEDCBNetworkTVParams{ID: id, InstanceID: i.instanceID}); err != nil {
		return fmt.Errorf("failed to release NetworkTV ID %d: %w", id, err)
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

// Closer は、NetworkTV を閉じる。*edcb.Client が満たす。
type Closer interface {
	CloseNetworkTV(ctx context.Context, id int32) error
}

var _ Closer = new(edcb.Client)

// Sweeper は、持ち主のインスタンスが止まった ID の NetworkTV を閉じ、ID を消す。neko のリーダーが動かす。
type Sweeper struct {
	db       sqlc.DBTX
	clock    clock.Clock
	closer   Closer
	logger   *slog.Logger
	interval time.Duration
}

// NewSweeper は Sweeper を返す。interval が 0 なら DefaultSweepInterval にする。
func NewSweeper(db sqlc.DBTX, clk clock.Clock, closer Closer, logger *slog.Logger, interval time.Duration) *Sweeper {
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

// Sweep は、持ち主のいない ID の NetworkTV を閉じ、ID を消す。閉じられなかった ID は、次に閉じ直すので残す。
// 取った直後の ID は、インスタンスを登録する前の持ち主のものかもしれないので、生きているかを判定できる時間 (AliveTTL) を待つ。
func (s *Sweeper) Sweep(ctx context.Context) error {
	now := s.clock.Now()
	q := sqlc.New(s.db)
	orphans, err := q.ListOrphanedEDCBNetworkTVs(ctx, sqlc.ListOrphanedEDCBNetworkTVsParams{
		AcquiredBefore: now.Add(-registry.AliveTTL), AliveSince: now.Add(-registry.AliveTTL),
	})
	if err != nil {
		return fmt.Errorf("failed to list orphaned NetworkTVs: %w", err)
	}
	var errs []error
	for _, orphan := range orphans {
		if err := s.closer.CloseNetworkTV(ctx, orphan.ID); err != nil {
			errs = append(errs, fmt.Errorf("failed to close NetworkTV %d: %w", orphan.ID, err))

			continue
		}
		if err := q.DeleteEDCBNetworkTV(ctx, sqlc.DeleteEDCBNetworkTVParams{ID: orphan.ID, InstanceID: orphan.InstanceID}); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete NetworkTV ID %d: %w", orphan.ID, err))

			continue
		}
		s.logger.InfoContext(ctx, "closed an orphaned NetworkTV", slog.Int("id", int(orphan.ID)),
			slog.String("owner", orphan.InstanceID.String()))
	}

	return errors.Join(errs...)
}
