package neko_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/neko"
	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

// TestLeaderSyncsEPG は、リーダーになった neko が、チューナーサーバーの EPG を DB に同期することを確かめる。
func TestLeaderSyncsEPG(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	options := faketuner.DefaultOptions(time.Now(), nil)
	server := httptest.NewServer(faketuner.New(options))
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- neko.Run(ctx, slog.New(slog.DiscardHandler), &neko.Options{
			DB:         pool,
			Clock:      clock.Real{},
			InstanceID: id.New(),
			Tuner:      client,
			EPG:        nil,
			Archive:    nil,
		})
	}()

	require.Eventually(t, func() bool {
		var n int
		require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM programs").Scan(&n))

		return n == len(options.Programs)
	}, 10*time.Second, 20*time.Millisecond)

	cancel()
	assert.NoError(t, <-done)
}
