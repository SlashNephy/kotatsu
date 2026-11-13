package networktv_test

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
	"github.com/SlashNephy/kotatsu/internal/infra/registry"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/networktv"
)

func testNow() time.Time {
	return time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
}

func newDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	return pool
}

// nhk は、NHK 総合 (東京)。
func nhk() edcb.ServiceKey {
	return edcb.ServiceKey{ONID: 32736, TSID: 32736, SID: 1024}
}

// mustIDs は、1000 から count 個の ID を配る IDs を返す。
func mustIDs(t *testing.T, db *pgxpool.Pool, clk clock.Clock, owner uuid.UUID, count int) *networktv.IDs {
	t.Helper()

	ids, err := networktv.NewIDs(db, clk, owner, 1000, count)
	require.NoError(t, err)

	return ids
}

func TestNewIDsRejectsInvalidRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		start, count int
	}{
		{name: "start is zero", start: 0, count: 1},
		{name: "count is zero", start: 1, count: 0},
		{name: "beyond int32", start: math.MaxInt32, count: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := networktv.NewIDs(nil, nil, uuid.Nil(), tt.start, tt.count)
			require.Error(t, err)
		})
	}
}

func TestIDs(t *testing.T) {
	t.Parallel()

	pool := newDB(t)
	clk := clocktest.NewFake(testNow())
	a := mustIDs(t, pool, clk, id.New(), 2)
	b := mustIDs(t, pool, clk, id.New(), 2)

	// 空いている最も小さい ID を、インスタンスをまたいで重ならないように配る。
	first, err := a.Acquire(t.Context(), nhk())
	require.NoError(t, err)
	assert.Equal(t, int32(1000), first)
	second, err := b.Acquire(t.Context(), nhk())
	require.NoError(t, err)
	assert.Equal(t, int32(1001), second)
	_, err = a.Acquire(t.Context(), nhk())
	require.ErrorIs(t, err, edcb.ErrNoNetworkTVID)

	// ほかのインスタンスの ID は返せない。
	require.NoError(t, b.Release(t.Context(), first))
	_, err = b.Acquire(t.Context(), nhk())
	require.ErrorIs(t, err, edcb.ErrNoNetworkTVID)
	require.NoError(t, a.Release(t.Context(), first))
	again, err := b.Acquire(t.Context(), nhk())
	require.NoError(t, err)
	assert.Equal(t, int32(1000), again)
}

func TestIDsConcurrent(t *testing.T) {
	t.Parallel()

	// 同時に取り合っても、同じ ID を 2 つのインスタンスに配らない。
	pool := newDB(t)
	clk := clocktest.NewFake(testNow())
	const instances = 8
	got := make(chan int32, instances)
	var wg sync.WaitGroup
	for range instances {
		ids := mustIDs(t, pool, clk, id.New(), instances)
		wg.Go(func() {
			n, err := ids.Acquire(context.Background(), nhk())
			assert.NoError(t, err)
			got <- n
		})
	}
	wg.Wait()
	close(got)
	seen := map[int32]bool{}
	for n := range got {
		assert.False(t, seen[n], "ID %d was given twice", n)
		seen[n] = true
	}
	assert.Len(t, seen, instances)
}

// closer は、閉じた NetworkTV の ID を記録する。fail に入れた ID は閉じられない。
type closer struct {
	mu     sync.Mutex
	closed []int32
	fail   map[int32]bool
}

func (c *closer) CloseNetworkTV(_ context.Context, id int32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail[id] {
		return edcb.ErrFailed
	}
	c.closed = append(c.closed, id)

	return nil
}

func TestSweeper(t *testing.T) {
	t.Parallel()

	pool := newDB(t)
	clk := clocktest.NewFake(testNow())
	alive, dead, starting, broken := id.New(), id.New(), id.New(), id.New()
	_, err := pool.Exec(t.Context(), `INSERT INTO instances (id, role, address, started_at, heartbeat_at) VALUES ($1, 'mikan', '', $2, $2)`, alive, testNow())
	require.NoError(t, err)
	acquire := func(owner uuid.UUID, at time.Time) int32 {
		n, err := mustIDs(t, pool, clocktest.NewFake(at), owner, 10).Acquire(t.Context(), nhk())
		require.NoError(t, err)

		return n
	}
	old := testNow().Add(-registry.AliveTTL - time.Second)
	aliveID := acquire(alive, old)
	deadID := acquire(dead, old)
	// 取った直後は、持ち主がまだ登録していないかもしれないので、閉じない。
	startingID := acquire(starting, testNow())
	brokenID := acquire(broken, old)

	c := &closer{mu: sync.Mutex{}, closed: []int32{}, fail: map[int32]bool{brokenID: true}}
	err = networktv.NewSweeper(pool, clk, c, slog.New(slog.DiscardHandler), 0).Sweep(t.Context())
	require.ErrorIs(t, err, edcb.ErrFailed)
	assert.Equal(t, []int32{deadID}, c.closed)

	var left []int32
	rows, err := pool.Query(t.Context(), `SELECT id FROM edcb_network_tvs ORDER BY id`)
	require.NoError(t, err)
	for rows.Next() {
		var n int32
		require.NoError(t, rows.Scan(&n))
		left = append(left, n)
	}
	require.NoError(t, rows.Err())
	// 閉じられなかった ID は、次に閉じ直すので残す。
	assert.Equal(t, []int32{aliveID, startingID, brokenID}, left)
}
