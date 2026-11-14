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

// nitSection は、BS の NIT[actual] の 1 つのセクション。サービス serviceID を載せた中継器を 1 つ持つ。
func nitSection(version, number, last uint8, tsid, serviceID uint16) *ts.NIT {
	return &ts.NIT{
		Actual: true, NetworkID: 4, Version: version, SectionNumber: number, LastSectionNumber: last,
		TransportStreams: []*ts.NITTransportStream{{
			TransportStreamID: tsid, OriginalNetworkID: 4, FrequencyKHz: 11_727_480,
			Services: []ts.NITService{{ServiceID: serviceID, ServiceType: 0x01}},
		}},
	}
}

// TestCollector は、NIT[actual] のセクションが揃ったら 1 つの構成にして DB に書き、読み直せることを確かめる (#1519)。
func TestCollector(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	q := sqlc.New(pool)
	clk := clocktest.NewFake(time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC))
	collector := lineup.NewCollector(clk, slog.New(slog.DiscardHandler))

	// 地上波と NIT[other] は使わない。
	collector.Observe(&ts.NIT{Actual: true, NetworkID: 32736, Version: 1, SectionNumber: 0, LastSectionNumber: 0, TransportStreams: []*ts.NITTransportStream{}})
	other := nitSection(1, 0, 0, 0x4010, 101)
	other.Actual = false
	collector.Observe(other)
	// 2 つのセクションのうち 1 つだけでは、まだ組み上がらない。
	collector.Observe(nitSection(1, 0, 1, 0x4010, 101))
	collector.Flush(t.Context(), q)
	got, err := lineup.Load(t.Context(), q)
	require.NoError(t, err)
	assert.Empty(t, got)

	collector.Observe(nitSection(1, 1, 1, 0x4011, 103))
	collector.Flush(t.Context(), q)
	got, err = lineup.Load(t.Context(), q)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, lineup.KindBS, got[0].Kind)
	assert.Equal(t, uint8(1), got[0].Version)
	assert.True(t, clk.Now().Equal(got[0].ObservedAt))
	assert.Equal(t, []lineup.Stream{
		{TransportStreamID: 0x4010, FrequencyKHz: 11_727_480, Services: []lineup.Service{{ServiceID: 101, ServiceType: 1}}},
		{TransportStreamID: 0x4011, FrequencyKHz: 11_727_480, Services: []lineup.Service{{ServiceID: 103, ServiceType: 1}}},
	}, got[0].Streams)

	// 同じ版は、少し後に読んでも書き直さない。間をあけたら、読んだ時刻を新しくする。
	firstObserved := got[0].ObservedAt
	clk.Advance(time.Minute)
	collector.Observe(nitSection(1, 0, 0, 0x4010, 101))
	collector.Flush(t.Context(), q)
	got, err = lineup.Load(t.Context(), q)
	require.NoError(t, err)
	assert.True(t, firstObserved.Equal(got[0].ObservedAt))
	clk.Advance(10 * time.Minute)
	collector.Observe(nitSection(1, 0, 0, 0x4010, 101))
	collector.Flush(t.Context(), q)
	got, err = lineup.Load(t.Context(), q)
	require.NoError(t, err)
	assert.True(t, clk.Now().Equal(got[0].ObservedAt))

	// 版が変わったら、すぐに書く。
	clk.Advance(time.Second)
	collector.Observe(nitSection(2, 0, 0, 0x4010, 105))
	collector.Flush(t.Context(), q)
	got, err = lineup.Load(t.Context(), q)
	require.NoError(t, err)
	assert.Equal(t, uint8(2), got[0].Version)
	assert.Equal(t, uint16(105), got[0].Streams[0].Services[0].ServiceID)
}

// TestComparerUsesObservedAt は、DB から読み直した構成を比べるとき、読み直した時刻ではなく、放送から読んだ時刻で
// 間隔をあけたかを決めることを確かめる。
func TestComparerUsesObservedAt(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	clk := clocktest.NewFake(start)
	comparer := lineup.NewComparer(clk, interval)
	obs := observation(1, 101)
	obs.ObservedAt = start
	_, ok := comparer.Observe(obs, known(141))
	require.False(t, ok)
	// 同じ行を、間隔を過ぎてから読み直しても、放送から読んだ時刻が同じなら、2 回目に数えない。
	clk.Advance(2 * interval)
	_, ok = comparer.Observe(obs, known(141))
	require.False(t, ok)
	// 放送から間隔をあけて読み直した行なら、知らせる。
	again := observation(1, 101)
	again.ObservedAt = start.Add(interval)
	_, ok = comparer.Observe(again, known(141))
	assert.True(t, ok)
}
