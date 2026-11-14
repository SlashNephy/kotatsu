package lineup_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/lineup"
)

// nhkNetwork は、地上波の NHK 総合 (東京) のネットワーク ID。
const nhkNetwork = 32736

// sdtSection は、地上波の SDT[actual] の 1 つのセクション。services のサービスを、サービス記述子付きで載せる。
func sdtSection(number, last uint8, services ...uint16) *ts.SDT {
	sdt := &ts.SDT{
		Actual: true, TransportStreamID: nhkNetwork, OriginalNetworkID: nhkNetwork, Version: 1,
		SectionNumber: number, LastSectionNumber: last, Services: []*ts.SDTService{},
	}
	for _, id := range services {
		sdt.Services = append(sdt.Services, &ts.SDTService{
			ServiceID: id, HasServiceDescriptor: true, ServiceType: 0x01, ProviderName: "", Name: "",
		})
	}

	return sdt
}

// grTV は、チューナーサーバーの地上波のテレビのサービス。
func grTV(serviceID int) *tunerserver.Service {
	return &tunerserver.Service{
		ID: int64(nhkNetwork*100000 + serviceID), ServiceID: serviceID, NetworkID: nhkNetwork, Name: "GR", Type: 0x01,
		LogoID: nil, RemoteControlKeyID: nil, Channel: &tunerserver.ChannelRef{Type: "GR", Channel: "27"},
	}
}

// TestTerrestrialCollector は、地上波の SDT[actual] のセクションが揃ったら、サービス記述子のあるサービスを DB に書くことを確かめる。
func TestTerrestrialCollector(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	q := sqlc.New(pool)
	clk := clocktest.NewFake(time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC))
	collector := lineup.NewTerrestrialCollector(clk, slog.New(slog.DiscardHandler))

	// 衛星、SDT[other]、サービス記述子のない (ロゴの対応だけの) SDT は使わない。
	bs := sdtSection(0, 0, 101)
	bs.OriginalNetworkID = 4
	collector.Observe(bs)
	other := sdtSection(0, 0, 1024)
	other.Actual = false
	collector.Observe(other)
	logoOnly := sdtSection(0, 0, 1024)
	logoOnly.Services[0].HasServiceDescriptor = false
	collector.Observe(logoOnly)
	// 2 つのセクションのうち 1 つだけでは、まだ組み上がらない。
	collector.Observe(sdtSection(0, 1, 1024))
	collector.Flush(t.Context(), q)
	got, err := lineup.LoadTerrestrial(t.Context(), q)
	require.NoError(t, err)
	assert.Empty(t, got)

	collector.Observe(sdtSection(1, 1, 1025))
	collector.Flush(t.Context(), q)
	got, err = lineup.LoadTerrestrial(t.Context(), q)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, uint16(nhkNetwork), got[0].NetworkID)
	assert.Equal(t, []lineup.Service{{ServiceID: 1024, ServiceType: 0x01}, {ServiceID: 1025, ServiceType: 0x01}}, got[0].Services)
}

// TestCheckerTerrestrial は、地上波のサービスがチューナーサーバーと食い違ったら、管理者に 1 回だけ知らせ、
// 同じになったら忘れて、また食い違ったら知らせることを確かめる (#1519)。
func TestCheckerTerrestrial(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	q := sqlc.New(pool)
	clk := clocktest.NewFake(time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC))
	_, err = auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{
		Username: "admin", DisplayName: "Admin", Role: domain.RoleAdmin, PasswordHash: nil,
	})
	require.NoError(t, err)
	collector := lineup.NewTerrestrialCollector(clk, slog.New(slog.DiscardHandler))
	collector.Observe(sdtSection(0, 0, 1024, 1025))
	collector.Flush(t.Context(), q)
	tuner := new(fakeServices)
	checker := lineup.NewChecker(pool, tuner, clk, slog.New(slog.DiscardHandler), 0)

	// チューナーサーバーにないネットワークは比べない。
	tuner.set(bsTV(101))
	require.NoError(t, checker.Check(t.Context()))
	assert.Empty(t, lineupNotifications(t, pool))

	// 放送にあってチューナーサーバーにない局 (1025) があれば、知らせる。続いている間は繰り返さない。
	tuner.set(grTV(1024))
	require.NoError(t, checker.Check(t.Context()))
	require.NoError(t, checker.Check(t.Context()))
	got := lineupNotifications(t, pool)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{
		"kind": "GR", "networkId": float64(nhkNetwork), "channel": "27", "added": float64(1), "removed": float64(0), "url": "/admin/system",
	}, got[0])

	// スキャンし直して同じになれば忘れ、また食い違ったら知らせる。
	tuner.set(grTV(1024), grTV(1025))
	require.NoError(t, checker.Check(t.Context()))
	tuner.set(grTV(1024))
	require.NoError(t, checker.Check(t.Context()))
	assert.Len(t, lineupNotifications(t, pool), 2)
}
