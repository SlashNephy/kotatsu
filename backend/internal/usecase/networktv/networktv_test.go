package networktv_test

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
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

// service は、nhk と同じネットワークの、SID が sid のサービス。
func service(sid uint16) edcb.ServiceKey {
	return edcb.ServiceKey{ONID: 32736, TSID: 32736, SID: sid}
}

func TestIDsExclusive(t *testing.T) {
	t.Parallel()

	pool := newDB(t)
	clk := clocktest.NewFake(testNow())
	a := mustIDs(t, pool, clk, id.New(), 2)
	b := mustIDs(t, pool, clk, id.New(), 2)

	// exclusive なら、同じサービスでも加わらず、空いている最も小さい ID を、インスタンスをまたいで重ならないように配る。
	first, shared, err := a.Acquire(t.Context(), nhk(), true)
	require.NoError(t, err)
	assert.Equal(t, int32(1000), first)
	assert.False(t, shared)
	second, shared, err := b.Acquire(t.Context(), nhk(), true)
	require.NoError(t, err)
	assert.Equal(t, int32(1001), second)
	assert.False(t, shared)
	_, _, err = a.Acquire(t.Context(), nhk(), true)
	require.ErrorIs(t, err, edcb.ErrNoNetworkTVID)

	// 持ち主でない ID から外れても、持ち主は残る。
	last, err := b.Release(t.Context(), first, false)
	require.NoError(t, err)
	assert.False(t, last)
	last, err = a.Release(t.Context(), first, false)
	require.NoError(t, err)
	assert.True(t, last)
	// 閉じ始めた ID は、消すまで配らない。
	_, _, err = b.Acquire(t.Context(), nhk(), true)
	require.ErrorIs(t, err, edcb.ErrNoNetworkTVID)
	require.NoError(t, a.Forget(t.Context(), first))
	again, _, err := b.Acquire(t.Context(), nhk(), true)
	require.NoError(t, err)
	assert.Equal(t, int32(1000), again)
}

func TestIDsShare(t *testing.T) {
	t.Parallel()

	pool := newDB(t)
	clk := clocktest.NewFake(testNow())
	acquire := func(ids *networktv.IDs, svc edcb.ServiceKey) (int32, bool) {
		t.Helper()
		n, shared, err := ids.Acquire(t.Context(), svc, false)
		require.NoError(t, err)

		return n, shared
	}
	a, b, c, d := mustIDs(t, pool, clk, id.New(), 10), mustIDs(t, pool, clk, id.New(), 10), mustIDs(t, pool, clk, id.New(), 10), mustIDs(t, pool, clk, id.New(), 10)

	// 同じサービスを開く 2 つ目のインスタンスは、同じ ID に加わる (#1850)。
	n, shared := acquire(a, nhk())
	assert.Equal(t, int32(1000), n)
	assert.False(t, shared)
	n, shared = acquire(b, nhk())
	assert.Equal(t, int32(1000), n)
	assert.True(t, shared)
	// EDCB は 3 つ目の中継を断るので、3 つ目のインスタンスは別の ID を取る。
	n, shared = acquire(c, nhk())
	assert.Equal(t, int32(1001), n)
	assert.False(t, shared)
	// 別のサービスには加わらない。
	n, shared = acquire(a, service(1025))
	assert.Equal(t, int32(1002), n)
	assert.False(t, shared)

	// 最後の持ち主が外れたときだけ、閉じ始める。閉じ始めた ID には加わらない。
	last, err := b.Release(t.Context(), 1000, false)
	require.NoError(t, err)
	assert.False(t, last)
	last, err = a.Release(t.Context(), 1000, false)
	require.NoError(t, err)
	assert.True(t, last)
	n, shared = acquire(d, nhk())
	assert.Equal(t, int32(1001), n)
	assert.True(t, shared)

	// 止まった持ち主は数えない。中継が切れているので、加われる。
	clk.Advance(registry.AliveTTL + time.Second)
	e := mustIDs(t, pool, clk, id.New(), 10)
	n, shared = acquire(e, nhk())
	assert.Equal(t, int32(1001), n)
	assert.True(t, shared)
}

func TestIDsHandover(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// join なら、残している間に、ほかのインスタンスが加わる。
		join        bool
		wantClosing bool
	}{
		{name: "taken over", join: true, wantClosing: false},
		{name: "nobody took over", join: false, wantClosing: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := newDB(t)
			clk := clocktest.NewFake(testNow())
			a, b := mustIDs(t, pool, clk, id.New(), 10), mustIDs(t, pool, clk, id.New(), 10)
			n, _, err := a.Acquire(t.Context(), nhk(), false)
			require.NoError(t, err)

			// 止まるインスタンスは、最後の持ち主でも閉じ始めず、ほかのインスタンスが加われるよう残す。
			last, err := a.Release(t.Context(), n, true)
			require.NoError(t, err)
			assert.True(t, last)
			if tt.join {
				joined, shared, err := b.Acquire(t.Context(), nhk(), false)
				require.NoError(t, err)
				assert.Equal(t, n, joined)
				assert.True(t, shared)
			}
			closing, err := a.CloseIfUnheld(t.Context(), n)
			require.NoError(t, err)
			assert.Equal(t, tt.wantClosing, closing)
			if tt.wantClosing {
				// 閉じ始めたら、加わらずに別の ID を取る。
				other, shared, err := b.Acquire(t.Context(), nhk(), false)
				require.NoError(t, err)
				assert.NotEqual(t, n, other)
				assert.False(t, shared)
			}
		})
	}
}

func TestIDsConcurrent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		exclusive bool
		// wantPerID は、1 つの ID を持つインスタンスの数。
		wantPerID int
	}{
		// 同時に取り合っても、同じ ID を 2 つのインスタンスに配らない。
		{name: "exclusive", exclusive: true, wantPerID: 1},
		// 同時に同じサービスを開いても、2 つずつ同じ ID を持ち、別々の ID を取りすぎない (#1850)。
		{name: "shared", exclusive: false, wantPerID: networktv.MaxHolders},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := newDB(t)
			clk := clocktest.NewFake(testNow())
			const instances = 8
			got := make(chan int32, instances)
			var wg sync.WaitGroup
			for range instances {
				ids := mustIDs(t, pool, clk, id.New(), instances)
				wg.Go(func() {
					n, _, err := ids.Acquire(context.Background(), nhk(), tt.exclusive)
					assert.NoError(t, err)
					got <- n
				})
			}
			wg.Wait()
			close(got)
			holders := map[int32]int{}
			for n := range got {
				holders[n]++
			}
			assert.Len(t, holders, instances/tt.wantPerID)
			for n, count := range holders {
				assert.Equal(t, tt.wantPerID, count, "ID %d", n)
			}
		})
	}
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

func (c *closer) take() []int32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	closed := c.closed
	c.closed, c.fail = []int32{}, map[int32]bool{}

	return closed
}

// queryInt32s は、query が返す整数の列を返す。
func queryInt32s(t *testing.T, pool *pgxpool.Pool, query string, args ...any) []int32 {
	t.Helper()

	rows, err := pool.Query(t.Context(), query, args...)
	require.NoError(t, err)
	got, err := pgx.CollectRows(rows, pgx.RowTo[int32])
	require.NoError(t, err)

	return got
}

func TestSweeper(t *testing.T) {
	t.Parallel()

	pool := newDB(t)
	clk := clocktest.NewFake(testNow())
	alive, dead, starting, broken, leaving := id.New(), id.New(), id.New(), id.New(), id.New()
	_, err := pool.Exec(t.Context(), `INSERT INTO instances (id, role, address, started_at, heartbeat_at) VALUES ($1, 'nabe', '', $2, $2)`, alive, testNow())
	require.NoError(t, err)
	acquire := func(owner uuid.UUID, at time.Time, sid uint16) int32 {
		t.Helper()
		n, _, err := mustIDs(t, pool, clocktest.NewFake(at), owner, 10).Acquire(t.Context(), service(sid), false)
		require.NoError(t, err)

		return n
	}
	old := testNow().Add(-registry.AliveTTL - time.Second)
	aliveID := acquire(alive, old, 1024)
	deadID := acquire(dead, old, 1025)
	// 取った直後は、持ち主がまだ登録していないかもしれないので、閉じない。
	startingID := acquire(starting, testNow(), 1026)
	brokenID := acquire(broken, old, 1027)
	// 止まった持ち主と、生きている持ち主が同じ ID を持っていれば、生きている持ち主が受け取り続けているので閉じない (#1850)。
	sharedID := acquire(alive, old, 1028)
	require.Equal(t, sharedID, acquire(dead, old, 1028))
	// 止まるインスタンスが引き継げるよう残した直後の ID は、閉じない。残してから時間がたった ID は閉じる。
	handoverID := acquire(leaving, old, 1029)
	_, err = mustIDs(t, pool, clk, leaving, 10).Release(t.Context(), handoverID, true)
	require.NoError(t, err)
	staleHandoverID := acquire(leaving, old, 1030)
	_, err = mustIDs(t, pool, clocktest.NewFake(old), leaving, 10).Release(t.Context(), staleHandoverID, true)
	require.NoError(t, err)

	c := &closer{mu: sync.Mutex{}, closed: []int32{}, fail: map[int32]bool{brokenID: true}}
	sweeper := networktv.NewSweeper(pool, clk, c, slog.New(slog.DiscardHandler), 0)
	err = sweeper.Sweep(t.Context())
	require.ErrorIs(t, err, edcb.ErrFailed)
	assert.Equal(t, []int32{deadID, staleHandoverID}, c.take())
	// 閉じられなかった ID は、閉じ始めたまま残す。
	assert.Equal(t, []int32{aliveID, startingID, brokenID, sharedID, handoverID}, queryInt32s(t, pool, `SELECT id FROM edcb_network_tvs ORDER BY id`))
	// 止まった持ち主は、ID から外す。
	assert.Empty(t, queryInt32s(t, pool, `SELECT network_tv_id FROM edcb_network_tv_holders WHERE instance_id = $1`, dead))
	holders, err := pool.Query(t.Context(), `SELECT instance_id FROM edcb_network_tv_holders WHERE network_tv_id = $1`, sharedID)
	require.NoError(t, err)
	sharedHolders, err := pgx.CollectRows(holders, pgx.RowTo[uuid.UUID])
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{alive}, sharedHolders)

	// 閉じ始めてから時間がたったら、閉じ直す。
	clk.Advance(time.Minute)
	_, err = pool.Exec(t.Context(), `UPDATE instances SET heartbeat_at = $1 WHERE id = $2`, clk.Now(), alive)
	require.NoError(t, err)
	require.NoError(t, sweeper.Sweep(t.Context()))
	assert.ElementsMatch(t, []int32{brokenID, startingID, handoverID}, c.take())
}
