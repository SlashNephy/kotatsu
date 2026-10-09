package registry_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/registry"
)

const (
	addressA  = "10.0.0.1:8080"
	addressB  = "10.0.0.2:8080"
	roleKyusu = "kyusu"
)

func TestListAliveReturnsInstancesWithRecentHeartbeat(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	clk := clocktest.NewFake(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))
	a := registry.New(pool, clk, id.New())
	b := registry.New(pool, clk, id.New())
	reader := registry.New(pool, clk, id.New())

	require.NoError(t, a.Register(t.Context(), roleKyusu, addressA))
	clk.Advance(time.Second)
	require.NoError(t, b.Register(t.Context(), roleKyusu, addressB))
	require.NoError(t, b.Register(t.Context(), "tenban", addressB))

	assert.Equal(t, []string{addressA, addressB}, addresses(t, reader, roleKyusu))
	assert.Equal(t, []string{addressB}, addresses(t, reader, "tenban"))

	// B だけがハートビートを書き続け、A は止まる。
	clk.Advance(registry.HeartbeatInterval)
	require.NoError(t, b.Heartbeat(t.Context(), roleKyusu, addressB))
	clk.Advance(registry.AliveTTL - registry.HeartbeatInterval)
	assert.Equal(t, []string{addressB}, addresses(t, reader, roleKyusu), "A must be considered stopped")

	// 止まったとみなされた A がハートビートを再開すれば、また載る。
	require.NoError(t, a.Heartbeat(t.Context(), roleKyusu, addressA))
	assert.Equal(t, []string{addressA, addressB}, addresses(t, reader, roleKyusu))

	require.NoError(t, a.Deregister(t.Context(), roleKyusu))
	assert.Equal(t, []string{addressB}, addresses(t, reader, roleKyusu))
}

func TestHeartbeatRegistersAgainAfterPruned(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	clk := clocktest.NewFake(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))
	a := registry.New(pool, clk, id.New())
	b := registry.New(pool, clk, id.New())

	require.NoError(t, a.Register(t.Context(), roleKyusu, addressA))
	clk.Advance(2 * time.Hour)
	require.NoError(t, b.Register(t.Context(), roleKyusu, addressB))

	pruned, err := b.PruneStale(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(1), pruned)

	// 消された A がハートビートを書くと、登録し直される。起動の時刻も登録し直した時刻になる。
	clk.Advance(time.Second)
	require.NoError(t, a.Heartbeat(t.Context(), roleKyusu, addressA))
	assert.Equal(t, []string{addressB, addressA}, addresses(t, b, roleKyusu))
}

func TestRunRegistersAndDeregisters(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	clk := clocktest.NewFake(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))
	instanceID := id.New()
	r := registry.New(pool, clk, instanceID)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- r.Run(ctx, slog.New(slog.DiscardHandler), []string{"tenban", roleKyusu}, addressA)
	}()

	require.Eventually(t, func() bool {
		return len(addresses(t, r, "tenban")) == 1 && len(addresses(t, r, roleKyusu)) == 1
	}, 5*time.Second, 20*time.Millisecond)

	cancel()
	require.NoError(t, <-done)
	assert.Empty(t, addresses(t, r, "tenban"))
	assert.Empty(t, addresses(t, r, roleKyusu))
}

// TestRunStopsWithoutErrorBeforeRegistered は、登録を終える前に止められても、正常な停止として扱うことを確かめる。
// 起動直後に終了のシグナルを受けると、登録の途中で ctx が終わる。
func TestRunStopsWithoutErrorBeforeRegistered(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	r := registry.New(pool, clocktest.NewFake(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)), id.New())

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, r.Run(ctx, slog.New(slog.DiscardHandler), []string{"tenban", roleKyusu}, addressA))
	assert.Empty(t, addresses(t, r, "tenban"))
	assert.Empty(t, addresses(t, r, roleKyusu))
}

func addresses(t *testing.T, r *registry.Registry, role string) []string {
	t.Helper()

	instances, err := r.ListAlive(t.Context(), role)
	require.NoError(t, err)
	result := make([]string, 0, len(instances))
	for _, instance := range instances {
		assert.NotEqual(t, uuid.Nil, instance.ID)
		result = append(result, instance.Address)
	}

	return result
}

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	return pool
}
