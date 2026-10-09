package notify_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/notify"
)

const (
	channel      = "reservations"
	otherChannel = "rules"
	waitTimeout  = 10 * time.Second
)

func TestWatcherReceivesChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts notify.WatcherOptions
	}{
		{
			// ポーリングの間隔を十分に長くし、LISTEN の経路だけで届くことを確かめる。
			name: "listen",
			opts: notify.WatcherOptions{UseListen: true, PollInterval: time.Hour},
		},
		{
			// トランザクションプーリングを経由する設定。LISTEN を使わず、ポーリングだけで届く。
			name: "polling only",
			opts: notify.WatcherOptions{UseListen: false, PollInterval: 100 * time.Millisecond},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := migratedPool(t)
			// Watcher を作る前の変更は、知らせる対象にしない。
			publish(t, pool, channel)

			watcher := startWatcher(t, pool, tt.opts)

			// LISTEN の経路だけで届くことを確かめるため、LISTEN が始まってから Publish する。
			if tt.opts.UseListen {
				waitListening(t, pool)
			}

			version := publish(t, pool, channel)
			waitChange(t, watcher)
			assert.Equal(t, version, watcher.Version(channel))
			assert.Equal(t, int64(2), watcher.Version(channel))
		})
	}
}

func TestWatcherCoalescesChanges(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	watcher := startWatcher(t, pool, notify.WatcherOptions{UseListen: false, PollInterval: 100 * time.Millisecond})

	// 受け取る前に何度変更があっても、まとめて 1 回だけ知らせる。
	publish(t, pool, channel)
	publish(t, pool, channel)
	last := publish(t, pool, otherChannel)
	require.Eventually(t, func() bool {
		return watcher.Version(channel) == 2 && watcher.Version(otherChannel) == last
	}, waitTimeout, 10*time.Millisecond)

	waitChange(t, watcher)
	select {
	case <-watcher.Changes():
		t.Fatal("changes must be coalesced into a single notification")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestPublishIsDiscardedOnRollback(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	watcher := startWatcher(t, pool, notify.WatcherOptions{UseListen: true, PollInterval: 100 * time.Millisecond})
	waitListening(t, pool)

	tx, err := pool.Begin(t.Context())
	require.NoError(t, err)
	_, err = notify.Publish(t.Context(), tx, new(clock.Real), channel)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback(t.Context()))

	select {
	case <-watcher.Changes():
		t.Fatal("a rolled back change must not be notified")
	case <-time.After(500 * time.Millisecond):
	}
	assert.Zero(t, watcher.Version(channel))
}

func startWatcher(t *testing.T, pool *pgxpool.Pool, opts notify.WatcherOptions) *notify.Watcher {
	t.Helper()

	watcher, err := notify.NewWatcher(t.Context(), pool, slog.New(slog.DiscardHandler), []string{channel, otherChannel}, opts)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- watcher.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})

	return watcher
}

func publish(t *testing.T, pool *pgxpool.Pool, ch string) int64 {
	t.Helper()

	var version int64
	require.NoError(t, pgx.BeginFunc(t.Context(), pool, func(tx pgx.Tx) error {
		var err error
		version, err = notify.Publish(t.Context(), tx, new(clock.Real), ch)

		return err
	}))

	return version
}

func waitChange(t *testing.T, watcher *notify.Watcher) {
	t.Helper()

	select {
	case <-watcher.Changes():
	case <-time.After(waitTimeout):
		t.Fatal("no change was notified")
	}
}

// waitListening は、Watcher が LISTEN し始めるまで待つ。
func waitListening(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	require.Eventually(t, func() bool {
		var listening bool
		err := pool.QueryRow(
			t.Context(),
			// LISTEN した接続は、通知を待つ間 LISTEN を最後のクエリとして残す。
			"SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname = current_database() AND query LIKE 'LISTEN %')",
		).Scan(&listening)

		return err == nil && listening
	}, waitTimeout, 20*time.Millisecond)
}

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	return pool
}
