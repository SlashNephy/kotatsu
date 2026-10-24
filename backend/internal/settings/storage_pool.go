package settings

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// maxPoolPriority は、保存先プールの優先順位の絶対値の上限。
const maxPoolPriority = 1000

// StoragePoolChanges は、保存先プールの設定の変える項目。nil の項目は変えない。
type StoragePoolChanges struct {
	Priority     *int
	MinFreeBytes *int64
}

// UpdateStoragePool は、保存先プール poolID の優先順位と容量の閾値を変え、変えた後の設定を返す (設計書 5.6、11.2)。
// プールのマウント先は各ロールの設定で与えるので、ここでは設定だけを持つ。
func (s *Store) UpdateStoragePool(
	ctx context.Context, db Database, actor *audit.Actor, poolID string, changes *StoragePoolChanges,
) (*sqlc.StoragePool, error) {
	if changes == nil {
		return nil, fmt.Errorf("%w: changes is nil", ErrInvalidSettings)
	}
	var priority *int32
	if changes.Priority != nil {
		stored, err := poolPriority(*changes.Priority)
		if err != nil {
			return nil, err
		}
		priority = &stored
	}
	if b := changes.MinFreeBytes; b != nil && *b < 0 {
		return nil, fmt.Errorf("%w: minFreeBytes must not be negative", ErrInvalidSettings)
	}

	var updated sqlc.StoragePool
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		now := s.clock.Now()
		current, err := q.LockStoragePool(ctx, sqlc.LockStoragePoolParams{ID: poolID, Now: now})
		if err != nil {
			return fmt.Errorf("failed to lock the storage pool: %w", err)
		}
		params := sqlc.UpsertStoragePoolParams{ID: poolID, Priority: current.Priority, MinFreeBytes: current.MinFreeBytes, Now: now}
		if priority != nil {
			params.Priority = *priority
		}
		if changes.MinFreeBytes != nil {
			params.MinFreeBytes = *changes.MinFreeBytes
		}
		if updated, err = q.UpsertStoragePool(ctx, params); err != nil {
			return fmt.Errorf("failed to update the storage pool: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "storage_pool.updated",
			TargetType: "storage_pool",
			TargetID:   poolID,
			Summary: map[string]any{
				"priority":     change(current.Priority, updated.Priority),
				"minFreeBytes": change(current.MinFreeBytes, updated.MinFreeBytes),
			},
			// 送信元は、操作者 (actor) に持たせる。
			SourceIP:  netip.Addr{},
			UserAgent: "",
		})
	})
	if err != nil {
		return nil, err
	}

	return &updated, nil
}

// poolPriority は、優先順位が範囲にあるかを確かめ、DB に保存する形にする。
func poolPriority(priority int) (int32, error) {
	if priority < -maxPoolPriority || priority > maxPoolPriority {
		return 0, fmt.Errorf("%w: priority must be between %d and %d", ErrInvalidSettings, -maxPoolPriority, maxPoolPriority)
	}

	return int32(priority), nil
}
