package lineup_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/lineup"
)

// fakeServices は、チューナーサーバーのサービスの一覧を返す偽物。
type fakeServices struct {
	mu       sync.Mutex
	services []*tunerserver.Service
}

var _ lineup.ServiceLister = new(fakeServices)

func (f *fakeServices) Services(context.Context) ([]*tunerserver.Service, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.services, nil
}

func (f *fakeServices) set(services ...*tunerserver.Service) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.services = services
}

// bsTV は、チューナーサーバーの BS のテレビのサービス。
func bsTV(serviceID int) *tunerserver.Service {
	return &tunerserver.Service{
		ID: int64(400000 + serviceID), ServiceID: serviceID, NetworkID: 4, Name: "BS", Type: 0x01,
		LogoID: nil, RemoteControlKeyID: nil, Channel: nil,
	}
}

// writeLineup は、mikan が書くのと同じ形で、BS の構成 (version 1) を DB に書く。BS-1 の中継器に、テレビのサービス serviceIDs を載せる。
func writeLineup(t *testing.T, q *sqlc.Queries, observedAt time.Time, serviceIDs ...uint16) {
	t.Helper()

	services := make([]lineup.Service, 0, len(serviceIDs))
	for _, id := range serviceIDs {
		services = append(services, lineup.Service{ServiceID: id, ServiceType: 0x01})
	}
	streams, err := json.Marshal([]lineup.Stream{{TransportStreamID: 0x4010, FrequencyKHz: 11_727_480, Services: services}})
	require.NoError(t, err)
	require.NoError(t, q.UpsertBroadcastLineup(t.Context(), sqlc.UpsertBroadcastLineupParams{
		NetworkID: 4, Version: 1, Streams: streams, ObservedAt: observedAt,
	}))
}

// lineupNotifications は、記録された「チャンネルの変更」の通知するイベントの payload を、古い順に返す。
func lineupNotifications(t *testing.T, pool *pgxpool.Pool) []map[string]any {
	t.Helper()

	rows, err := pool.Query(t.Context(), "SELECT payload FROM notification_events WHERE kind = $1 ORDER BY occurred_at, id",
		string(domain.NotificationKindLineupChanged))
	require.NoError(t, err)
	defer rows.Close()
	payloads := []map[string]any{}
	for rows.Next() {
		var raw []byte
		require.NoError(t, rows.Scan(&raw))
		var payload map[string]any
		require.NoError(t, json.Unmarshal(raw, &payload))
		payloads = append(payloads, payload)
	}
	require.NoError(t, rows.Err())

	return payloads
}

// TestChecker は、DB の衛星の構成を、チューナーサーバーの一覧と比べ、同じ version の食い違いを間隔をあけて 2 回読んだら、
// 管理者に 1 回だけ知らせることを確かめる (#1519)。
func TestChecker(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	q := sqlc.New(pool)
	start := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	clk := clocktest.NewFake(start)
	_, err = auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{
		Username: "admin", DisplayName: "Admin", Role: domain.RoleAdmin, PasswordHash: nil,
	})
	require.NoError(t, err)
	tuner := new(fakeServices)
	checker := lineup.NewChecker(pool, tuner, clk, slog.New(slog.DiscardHandler), 0)

	// チューナーサーバーが空の一覧を返したら、比べない。
	writeLineup(t, q, start, 101, 103, 222)
	require.NoError(t, checker.Check(t.Context()))
	assert.Empty(t, lineupNotifications(t, pool))

	// 放送にあってチューナーサーバーにない局が 2 つ (103、222)、その逆が 1 つ (191)。1 回目は覚えるだけで、知らせない。
	tuner.set(bsTV(101), bsTV(191))
	require.NoError(t, checker.Check(t.Context()))
	assert.Empty(t, lineupNotifications(t, pool))

	// 放送から読み直していなければ (読んだ時刻が同じなら)、2 回目に数えない。
	require.NoError(t, checker.Check(t.Context()))
	assert.Empty(t, lineupNotifications(t, pool))

	// 同じ version の食い違いを、読み直しの間隔をあけてもう一度読んだら、知らせる。
	writeLineup(t, q, start.Add(lineup.DefaultRecheckInterval), 101, 103, 222)
	require.NoError(t, checker.Check(t.Context()))
	got := lineupNotifications(t, pool)
	require.Len(t, got, 1)
	assert.Equal(t, map[string]any{
		"kind": "BS", "networkId": float64(4), "added": float64(2), "removed": float64(1), "url": "/admin/system",
	}, got[0])

	// 続いている間は、同じ知らせを繰り返さない。
	writeLineup(t, q, start.Add(2*lineup.DefaultRecheckInterval), 101, 103, 222)
	require.NoError(t, checker.Check(t.Context()))
	assert.Len(t, lineupNotifications(t, pool), 1)

	// チューナーサーバーをスキャンし直して一覧が変わったら、比べ直す。食い違いがなくなれば、知らせない。
	tuner.set(bsTV(101), bsTV(103), bsTV(222))
	writeLineup(t, q, start.Add(3*lineup.DefaultRecheckInterval), 101, 103, 222)
	require.NoError(t, checker.Check(t.Context()))
	writeLineup(t, q, start.Add(4*lineup.DefaultRecheckInterval), 101, 103, 222)
	require.NoError(t, checker.Check(t.Context()))
	assert.Len(t, lineupNotifications(t, pool), 1)
}
