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
	db     Database
	clock  clock.Clock
	name   string
	holder uuid.UUID
	ttl    time.Duration
}

// Database は、リースを読み書きする DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// lockTimeout は、リースを取る、延ばす、手放すときに、行のロックを待つ時間の上限 (付録 A、#527)。
// リーダーの書き込みのトランザクションは、リースの行をロックしている。固まったまま残っても、待ち続けずにあきらめ、後でやり直す。
const lockTimeout = "2s"

// NewManager は Manager を作る。ttl はリースの有効期間。
func NewManager(db Database, clk clock.Clock, name string, holder uuid.UUID, ttl time.Duration) *Manager {
	return &Manager{db: db, clock: clk, name: name, holder: holder, ttl: ttl}
}

// TryAcquire は、期限の切れたリースを取る。取れたら新しい世代番号と true を返す。
// 他のインスタンスが期限内のリースを持っていれば false を返す。自分が期限内のリースを持っているときも false を返すので、
// その場合は Renew を使う。
func (m *Manager) TryAcquire(ctx context.Context) (int64, bool, error) {
	now := m.clock.Now()
	var generation int64
	err := m.withLockTimeout(ctx, func(q *sqlc.Queries) error {
		// リーダーの書き込みのトランザクションが終わるまで待ってから取る。
		if err := q.LockLeaseForAcquire(ctx, sqlc.LockLeaseForAcquireParams{Name: m.name, Now: now}); err != nil {
			return err
		}
		var err error
		generation, err = q.AcquireLease(ctx, sqlc.AcquireLeaseParams{
			Name:      m.name,
			Holder:    m.holder,
			Now:       now,
			ExpiresAt: now.Add(m.ttl),
		})

		return err
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
	var updated int64
	err := m.withLockTimeout(ctx, func(q *sqlc.Queries) error {
		var err error
		updated, err = q.RenewLease(ctx, sqlc.RenewLeaseParams{
			Now:        now,
			ExpiresAt:  now.Add(m.ttl),
			Name:       m.name,
			Holder:     m.holder,
			Generation: generation,
		})

		return err
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
	if err := m.withLockTimeout(ctx, func(q *sqlc.Queries) error {
		return q.ReleaseLease(ctx, sqlc.ReleaseLeaseParams{
			Now:        m.clock.Now(),
			Name:       m.name,
			Holder:     m.holder,
			Generation: generation,
		})
	}); err != nil {
		return fmt.Errorf("failed to release the lease %s: %w", m.name, err)
	}

	return nil
}

// Guard は、リーダーの書き込みのトランザクション tx の中で、まだ generation の世代のリースを持っていることを確かめる。
// 持っていなければ ErrLost を返すので、呼び出し側はトランザクションをロールバックする。
//
// リースの行を FOR KEY SHARE でロックし、TryAcquire は取る前に FOR UPDATE で待つので、tx が終わるまで他のインスタンスはリースを取れない。
// 確かめてからコミットするまでの間にリーダーが入れ替わり、古いリーダーの書き込みが通ってしまうことはない。
// 延長の UPDATE とはぶつからないので、長い書き込みの間もリースを延ばせる (#729)。
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

// withLockTimeout は、行のロックを待つ時間に lockTimeout の上限を付けたトランザクションの中で、fn を実行する。
func (m *Manager) withLockTimeout(ctx context.Context, fn func(q *sqlc.Queries) error) error {
	return pgx.BeginFunc(ctx, m.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '"+lockTimeout+"'"); err != nil {
			return fmt.Errorf("failed to set the lock timeout: %w", err)
		}

		return fn(sqlc.New(tx))
	})
}
