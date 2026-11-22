package lineup_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/lineup"
)

// observePresentFollowing は、BS のサービス serviceID の、EIT[p/f] の放送中と次の番組のセクションを collector に渡す。
// hasEvent が false なら、どちらのセクションにも番組を載せない。
func observePresentFollowing(collector *lineup.ActivityCollector, serviceID uint16, hasEvent bool) {
	for _, following := range []bool{false, true} {
		collector.Observe(&ts.PresentFollowingSection{
			OriginalNetworkID: 4, TransportStreamID: 0x4010, ServiceID: serviceID, Following: following, HasEvent: hasEvent,
		})
	}
}

// TestCheckerIdleServices は、EIT[p/f] で番組のないサービスを、NIT に載っていても新しい局と数えないことを確かめる (#2034)。
// 番組が載ったら、新しい局と数えて知らせる。
func TestCheckerIdleServices(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	q := sqlc.New(pool)
	start := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	clk := clocktest.NewFake(start)
	createAdmin(t, pool, clk)
	collector := lineup.NewActivityCollector(clk, slog.New(slog.DiscardHandler))
	tuner := new(fakeServices)
	tuner.set(bsTV(101))
	checker := lineup.NewChecker(pool, tuner, clk, slog.New(slog.DiscardHandler), 0)

	// 放送中の番組のセクションしか読んでいなければ、まだ書かない。
	collector.Observe(&ts.PresentFollowingSection{OriginalNetworkID: 4, TransportStreamID: 0x4010, ServiceID: 103, Following: false, HasEvent: false})
	collector.Flush(t.Context(), q)
	idle, err := lineup.LoadIdle(t.Context(), q)
	require.NoError(t, err)
	assert.Empty(t, idle)

	// 103 は、NIT に載っているが、EIT[p/f] に番組がない。2 回読んでも知らせない。
	observePresentFollowing(collector, 101, true)
	observePresentFollowing(collector, 103, false)
	collector.Flush(t.Context(), q)
	idle, err = lineup.LoadIdle(t.Context(), q)
	require.NoError(t, err)
	assert.Equal(t, map[lineup.Known]bool{{NetworkID: 4, ServiceID: 103}: true}, idle)
	writeLineup(t, q, start, 101, 103)
	require.NoError(t, checker.Check(t.Context()))
	writeLineup(t, q, start.Add(lineup.DefaultRecheckInterval), 101, 103)
	require.NoError(t, checker.Check(t.Context()))
	assert.Empty(t, lineupNotifications(t, pool))

	// 103 に番組が載ったら、NIT の version が同じでも比べ直し、2 回読んだら知らせる。
	observePresentFollowing(collector, 103, true)
	collector.Flush(t.Context(), q)
	writeLineup(t, q, start.Add(2*lineup.DefaultRecheckInterval), 101, 103)
	require.NoError(t, checker.Check(t.Context()))
	assert.Empty(t, lineupNotifications(t, pool))
	writeLineup(t, q, start.Add(3*lineup.DefaultRecheckInterval), 101, 103)
	require.NoError(t, checker.Check(t.Context()))
	got := lineupNotifications(t, pool)
	require.Len(t, got, 1)
	assert.Equal(t, []any{"BS1 のサービス 103"}, got[0]["addedServices"])
}

// TestCheckerIdleRegisteredService は、チューナーサーバーにあるサービスは、番組がなくても終わった局と数えないことを確かめる (#2034)。
func TestCheckerIdleRegisteredService(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	q := sqlc.New(pool)
	start := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	clk := clocktest.NewFake(start)
	createAdmin(t, pool, clk)
	collector := lineup.NewActivityCollector(clk, slog.New(slog.DiscardHandler))
	tuner := new(fakeServices)
	tuner.set(bsTV(101), bsTV(102))
	checker := lineup.NewChecker(pool, tuner, clk, slog.New(slog.DiscardHandler), 0)

	observePresentFollowing(collector, 101, true)
	observePresentFollowing(collector, 102, false)
	collector.Flush(t.Context(), q)
	writeLineup(t, q, start, 101, 102)
	require.NoError(t, checker.Check(t.Context()))
	writeLineup(t, q, start.Add(lineup.DefaultRecheckInterval), 101, 102)
	require.NoError(t, checker.Check(t.Context()))
	assert.Empty(t, lineupNotifications(t, pool))
}

// TestActivityCollectorRevert は、書く前に番組の有無が書いた値に戻ったら、戻る前の値を書かないことを確かめる (#2034)。
func TestActivityCollectorRevert(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	q := sqlc.New(pool)
	clk := clocktest.NewFake(time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC))
	collector := lineup.NewActivityCollector(clk, slog.New(slog.DiscardHandler))

	observePresentFollowing(collector, 103, true)
	collector.Flush(t.Context(), q)
	observePresentFollowing(collector, 103, false)
	observePresentFollowing(collector, 103, true)
	collector.Flush(t.Context(), q)

	idle, err := lineup.LoadIdle(t.Context(), q)
	require.NoError(t, err)
	assert.Empty(t, idle)
}
