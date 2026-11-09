package networkactivity_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/networkactivity"
)

func testHour() time.Time {
	return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
}

func bucket(host string, count int64) netactivity.Bucket {
	return netactivity.Bucket{
		Hour: testHour(), Purpose: netactivity.PurposeUpdateCheck, Host: host, Initiator: netactivity.InitiatorScheduled, Success: true,
		Count: count, SentBytes: 10, ReceivedBytes: 20,
	}
}

// TestSaveAddsOnlyWhileEnabled は、同じ単位の集計を足し合わせ、記録が無効なら書かないことを確かめる (#1369)。
func TestSaveAddsOnlyWhileEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		enabled   bool
		wantCount int
	}{
		{name: "enabled", enabled: true, wantCount: 1},
		{name: "disabled", enabled: false, wantCount: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool, err := database.Open(t.Context(), databasetest.New(t))
			require.NoError(t, err)
			t.Cleanup(pool.Close)
			require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
			_, err = pool.Exec(t.Context(), `INSERT INTO system_settings (id, recording_defaults, archive_retention_days, updated_at, network_activity_enabled)
				VALUES (1, '{}', 90, now(), $1)`, tt.enabled)
			require.NoError(t, err)

			store := networkactivity.NewStore(pool)
			enabled, err := store.Enabled(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tt.enabled, enabled)

			require.NoError(t, store.Save(t.Context(), []netactivity.Bucket{bucket("api.github.com", 1)}))
			require.NoError(t, store.Save(t.Context(), []netactivity.Bucket{bucket("api.github.com", 2)}))

			rows, err := sqlc.New(pool).ListNetworkActivity(t.Context(), sqlc.ListNetworkActivityParams{Since: testHour(), MaxCount: 10})
			require.NoError(t, err)
			require.Len(t, rows, tt.wantCount)
			if tt.enabled {
				assert.Equal(t, int64(3), rows[0].Count)
				assert.Equal(t, int64(20), rows[0].SentBytes)
				assert.Equal(t, int64(40), rows[0].ReceivedBytes)
			}
		})
	}
}

// TestPruneWorkerDeletesPastRetention は、保持期間を過ぎた時間帯だけを消すことを確かめる。
func TestPruneDeletesPastRetention(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	const retention = 30 * 24 * time.Hour
	for _, age := range []time.Duration{retention + time.Hour, retention - time.Hour} {
		_, err = pool.Exec(t.Context(), `INSERT INTO network_activity (hour, purpose, host, initiator, success, count, sent_bytes, received_bytes)
			VALUES ($1, 'UPDATE_CHECK', '', 'SCHEDULED', true, 1, 0, 0)`, testHour().Add(-age))
		require.NoError(t, err)
	}

	deleted, err := networkactivity.Prune(t.Context(), pool, clocktest.NewFake(testHour()), retention)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	rows, err := sqlc.New(pool).ListNetworkActivity(t.Context(), sqlc.ListNetworkActivityParams{Since: testHour().Add(-365 * 24 * time.Hour), MaxCount: 10})
	require.NoError(t, err)
	assert.Len(t, rows, 1)
}
