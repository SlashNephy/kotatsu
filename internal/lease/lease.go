// Package lease は、DB のリースによるリーダー選出を扱う (設計書 1.4)。
//
// リースの期限は、各インスタンスの Clock で決める。インスタンス間の時計のずれは NTP で抑えられている前提にする。
// リースには世代番号があり、リースを取り直すたびに増える。リーダーの書き込みは Guard で世代番号を確かめ、
// 古い世代のリーダーの書き込みを拒否する。
package lease

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// ErrLost は、リースを失ったことを表す。期限が切れたか、他のインスタンスに取られている。
var ErrLost = errors.New("lease lost")

// Manager は、名前の付いた 1 つのリースを、holder として取ったり延長したりする。
type Manager struct {
	db     sqlc.DBTX
	clock  clock.Clock
	name   string
	holder uuid.UUID
	ttl    time.Duration
}

// NewManager は Manager を作る。ttl はリースの有効期間。
func NewManager(db sqlc.DBTX, clk clock.Clock, name string, holder uuid.UUID, ttl time.Duration) *Manager {
	return &Manager{db: db, clock: clk, name: name, holder: holder, ttl: ttl}
}

// TryAcquire は、期限の切れたリースを取る。取れたら新しい世代番号と true を返す。
// 他のインスタンスが期限内のリースを持っていれば false を返す。自分が期限内のリースを持っているときも false を返すので、
// その場合は Renew を使う。
func (m *Manager) TryAcquire(ctx context.Context) (int64, bool, error) {
	now := m.clock.Now()
	generation, err := sqlc.New(m.db).AcquireLease(ctx, sqlc.AcquireLeaseParams{
		Name:      m.name,
		Holder:    m.holder,
		Now:       now,
		ExpiresAt: now.Add(m.ttl),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("failed to acquire the lease %s: %w", m.name, err)
	}

	return generation, true, nil
}

// Renew は、generation の世代のリースを延長し、延長した時点の時刻を返す。
// 期限が切れていたり、他のインスタンスに取られていたりすれば ErrLost を返す。
func (m *Manager) Renew(ctx context.Context, generation int64) (time.Time, error) {
	now := m.clock.Now()
	updated, err := sqlc.New(m.db).RenewLease(ctx, sqlc.RenewLeaseParams{
		Now:        now,
		ExpiresAt:  now.Add(m.ttl),
		Name:       m.name,
		Holder:     m.holder,
		Generation: generation,
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to renew the lease %s: %w", m.name, err)
	}
	if updated == 0 {
		return time.Time{}, fmt.Errorf("%w: %s (generation %d)", ErrLost, m.name, generation)
	}

	return now, nil
}

// Release は、generation の世代のリースの期限をすぐに切れさせ、他のインスタンスがすぐに取れるようにする。
// すでに失っていれば何もしない。
func (m *Manager) Release(ctx context.Context, generation int64) error {
	if err := sqlc.New(m.db).ReleaseLease(ctx, sqlc.ReleaseLeaseParams{
		Now:        m.clock.Now(),
		Name:       m.name,
		Holder:     m.holder,
		Generation: generation,
	}); err != nil {
		return fmt.Errorf("failed to release the lease %s: %w", m.name, err)
	}

	return nil
}

// Guard は、リーダーの書き込みのトランザクション tx の中で、まだ generation の世代のリースを持っていることを確かめる。
// 持っていなければ ErrLost を返すので、呼び出し側はトランザクションをロールバックする。
//
// リースの行を FOR SHARE でロックするので、tx が終わるまで他のインスタンスはリースを取れない。
// 確かめてからコミットするまでの間にリーダーが入れ替わり、古いリーダーの書き込みが通ってしまうことはない。
func (m *Manager) Guard(ctx context.Context, tx pgx.Tx, generation int64) error {
	_, err := sqlc.New(tx).LockLeaseForWrite(ctx, sqlc.LockLeaseForWriteParams{
		Name:       m.name,
		Holder:     m.holder,
		Generation: generation,
		Now:        m.clock.Now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s (generation %d)", ErrLost, m.name, generation)
	}
	if err != nil {
		return fmt.Errorf("failed to check the lease %s: %w", m.name, err)
	}

	return nil
}
