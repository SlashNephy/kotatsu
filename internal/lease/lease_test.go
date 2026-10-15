package lease_test

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/lease"
)

const (
	leaseName = "neko"
	ttl       = 15 * time.Second
)

func TestOnlyOneInstanceAcquiresConcurrently(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	clk := clocktest.NewFake(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))

	const instances = 8
	var (
		wg       sync.WaitGroup
		acquired atomic.Int32
		start    = make(chan struct{})
		errs     = make(chan error, instances)
	)
	for range instances {
		wg.Go(func() {
			manager := lease.NewManager(pool, clk, leaseName, id.New(), ttl)
			<-start
			_, ok, err := manager.TryAcquire(t.Context())
			errs <- err
			if ok {
				acquired.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), acquired.Load())
}

func TestLeaseExpiresAndAnotherInstanceTakesOver(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	clk := clocktest.NewFake(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))
	a := lease.NewManager(pool, clk, leaseName, id.New(), ttl)
	b := lease.NewManager(pool, clk, leaseName, id.New(), ttl)

	genA, ok, err := a.TryAcquire(t.Context())
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, int64(1), genA)

	// A が延長している間は、B は取れない。
	clk.Advance(10 * time.Second)
	_, err = a.Renew(t.Context(), genA)
	require.NoError(t, err)
	clk.Advance(10 * time.Second)
	_, ok, err = b.TryAcquire(t.Context())
	require.NoError(t, err)
	assert.False(t, ok, "B must not acquire while A keeps renewing")

	// A が止まって延長しなくなり、期限が切れると B が取れる。
	clk.Advance(ttl)
	genB, ok, err := b.TryAcquire(t.Context())
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, genA+1, genB)

	// A は、もう延長できない。
	_, err = a.Renew(t.Context(), genA)
	require.ErrorIs(t, err, lease.ErrLost)
}

func TestReleaseLetsAnotherInstanceAcquireImmediately(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	clk := clocktest.NewFake(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))
	a := lease.NewManager(pool, clk, leaseName, id.New(), ttl)
	b := lease.NewManager(pool, clk, leaseName, id.New(), ttl)

	genA, ok, err := a.TryAcquire(t.Context())
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, a.Release(t.Context(), genA))

	genB, ok, err := b.TryAcquire(t.Context())
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, genA+1, genB)
}

func TestGuardRejectsWritesFromOldGeneration(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	clk := clocktest.NewFake(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))
	a := lease.NewManager(pool, clk, leaseName, id.New(), ttl)
	b := lease.NewManager(pool, clk, leaseName, id.New(), ttl)
	_, err := pool.Exec(t.Context(), "CREATE TABLE writes (writer text NOT NULL)")
	require.NoError(t, err)

	genA, ok, err := a.TryAcquire(t.Context())
	require.NoError(t, err)
	require.True(t, ok)

	// A が世代を確かめたトランザクションを開いている間に、A のリースの期限が切れる。
	txA, err := pool.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, a.Guard(t.Context(), txA, genA))
	clk.Advance(ttl + time.Second)

	// B はリースを取ろうとするが、A のトランザクションが終わるまで待たされる。
	type result struct {
		generation int64
		ok         bool
		err        error
	}
	acquired := make(chan result, 1)
	go func() {
		generation, ok, err := b.TryAcquire(context.WithoutCancel(t.Context()))
		acquired <- result{generation: generation, ok: ok, err: err}
	}()
	select {
	case r := <-acquired:
		t.Fatalf("B acquired the lease while A's guarded transaction was open: %+v", r)
	case <-time.After(300 * time.Millisecond):
	}

	_, err = txA.Exec(t.Context(), "INSERT INTO writes (writer) VALUES ('A')")
	require.NoError(t, err)
	require.NoError(t, txA.Commit(t.Context()))

	r := <-acquired
	require.NoError(t, r.err)
	require.True(t, r.ok)
	genB := r.generation
	assert.Equal(t, genA+1, genB)

	// B がリーダーになった後は、古い世代の A の書き込みを拒否する。
	err = pgx.BeginFunc(t.Context(), pool, func(tx pgx.Tx) error {
		if err := a.Guard(t.Context(), tx, genA); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), "INSERT INTO writes (writer) VALUES ('stale A')")

		return err
	})
	require.ErrorIs(t, err, lease.ErrLost)

	err = pgx.BeginFunc(t.Context(), pool, func(tx pgx.Tx) error {
		if err := b.Guard(t.Context(), tx, genB); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), "INSERT INTO writes (writer) VALUES ('B')")

		return err
	})
	require.NoError(t, err)

	rows, err := pool.Query(t.Context(), "SELECT writer FROM writes ORDER BY writer")
	require.NoError(t, err)
	writers, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	assert.Equal(t, []string{"A", "B"}, writers)
}

func TestElectorsKeepASingleLeader(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)

	// 実際の時刻で動かす。-race の CI でも揺れないよう、間隔には余裕を持たせる。
	const (
		electorTTL    = 2 * time.Second
		renewInterval = 300 * time.Millisecond
	)

	var (
		leaders    atomic.Int32
		maxLeaders atomic.Int32
		elected    = make(chan uuid.UUID, 10)
	)
	lead := func(holder uuid.UUID) lease.Lead {
		return func(ctx context.Context, _ int64) error {
			n := leaders.Add(1)
			defer leaders.Add(-1)
			for {
				current := maxLeaders.Load()
				if n <= current || maxLeaders.CompareAndSwap(current, n) {
					break
				}
			}
			elected <- holder
			<-ctx.Done()

			return nil
		}
	}

	type running struct {
		holder uuid.UUID
		cancel context.CancelFunc
		done   chan error
	}
	start := func() *running {
		holder := id.New()
		elector, err := lease.NewElector(
			lease.NewManager(pool, new(clock.Real), leaseName, holder, electorTTL),
			slog.New(slog.DiscardHandler),
			renewInterval,
		)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			done <- elector.Run(ctx, lead(holder))
		}()

		return &running{holder: holder, cancel: cancel, done: done}
	}

	a, b := start(), start()
	first := waitElected(t, elected)

	// リーダーを止めると、もう一方がリーダーになる。
	leader, follower := a, b
	if first == b.holder {
		leader, follower = b, a
	}
	leader.cancel()
	require.NoError(t, <-leader.done)
	assert.Equal(t, follower.holder, waitElected(t, elected))

	follower.cancel()
	require.NoError(t, <-follower.done)
	assert.Equal(t, int32(1), maxLeaders.Load(), "two electors must never lead at the same time")
}

func TestNewElectorRejectsTooLongRenewInterval(t *testing.T) {
	t.Parallel()

	manager := lease.NewManager(nil, new(clock.Real), leaseName, id.New(), ttl)
	_, err := lease.NewElector(manager, slog.New(slog.DiscardHandler), ttl/2+time.Second)
	require.Error(t, err)
}

func waitElected(t *testing.T, elected <-chan uuid.UUID) uuid.UUID {
	t.Helper()

	select {
	case holder := <-elected:
		return holder
	case <-time.After(10 * time.Second):
		t.Fatal("no elector became the leader")

		return uuid.Nil
	}
}

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	return pool
}

// TestTryAcquireGivesUpOnStuckLeader は、リーダーの書き込みのトランザクションが固まっていても、ほかのインスタンスが
// リースを取ろうとして待ち続けず、短い時間であきらめて後でやり直すことを確かめる (#527)。
func TestTryAcquireGivesUpOnStuckLeader(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	clk := clocktest.NewFake(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))
	a := lease.NewManager(pool, clk, leaseName, id.New(), ttl)
	b := lease.NewManager(pool, clk, leaseName, id.New(), ttl)
	genA, ok, err := a.TryAcquire(t.Context())
	require.NoError(t, err)
	require.True(t, ok)
	// A の書き込みのトランザクションが、固まったまま残る。
	txA, err := pool.Begin(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = txA.Rollback(context.WithoutCancel(t.Context())) })
	require.NoError(t, a.Guard(t.Context(), txA, genA))
	clk.Advance(ttl + time.Second)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	started := time.Now()
	_, ok, err = b.TryAcquire(ctx)
	require.Error(t, err)
	assert.False(t, ok)
	require.NotErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(started), 5*time.Second)
}
