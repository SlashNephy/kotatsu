package epg_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	waitFor = 10 * time.Second
	tick    = 20 * time.Millisecond
)

// runSyncer は、ctx が終わるまで syncer を動かす。テストの終わりに止まったことを確かめる。
func runSyncer(t *testing.T, syncer *epg.Syncer) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- syncer.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})
}

// TestSyncerAppliesEvents は、イベントで届いた番組の追加、更新、削除が、全量の突き合わせを待たずに反映されることを確かめる。
// 反映のたびに、変更の通知の版数が上がる。
func TestSyncerAppliesEvents(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	syncer := epg.NewSyncer(f.reconciler, f.client, clocktest.NewFake(testNow()), slog.New(slog.DiscardHandler), &epg.SyncerOptions{
		// 全量の突き合わせで反映されないよう、間隔を長くする。
		FullSyncInterval: time.Hour,
		BatchWindow:      50 * time.Millisecond,
	})
	runSyncer(t, syncer)
	require.Eventually(t, func() bool {
		return f.fake.EventSubscribers() == 1 && f.count(t, "SELECT count(*) FROM programs") == fixturePrograms
	}, waitFor, tick)

	tests := []struct {
		name   string
		change func(t *testing.T)
		query  string
		args   []any
	}{
		{
			name: "create",
			change: func(t *testing.T) {
				t.Helper()
				f.fake.AddProgram(&faketuner.Program{
					ID: programID(100), EventID: 100, ServiceID: grServiceID, NetworkID: grNetworkID,
					StartAt: testNow().Add(6 * time.Hour).UnixMilli(), Duration: time.Hour.Milliseconds(),
					IsFree: true, Name: "追加した番組", Description: "",
				})
			},
			query: "SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = 100 AND name = '追加した番組'",
			args:  []any{serviceID()},
		},
		{
			name: "update",
			change: func(t *testing.T) {
				t.Helper()
				require.NoError(t, f.fake.ExtendProgram(programID(7), 30*time.Minute))
			},
			query: "SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = 7 AND end_at = $2",
			args:  []any{serviceID(), programStart(9)},
		},
		{
			name: "remove",
			change: func(t *testing.T) {
				t.Helper()
				require.NoError(t, f.fake.RemoveProgram(programID(5)))
			},
			query: "SELECT 1 - count(*) FROM programs WHERE service_id = $1 AND event_id = 5",
			args:  []any{serviceID()},
		},
	}
	// 順に変えていくので、並行にはしない。
	for _, tt := range tests {
		before := f.count(t, "SELECT coalesce(max(version), 0) FROM epg_changes")
		tt.change(t)
		require.Eventually(t, func() bool {
			return f.count(t, tt.query, tt.args...) == 1
		}, waitFor, tick, tt.name)
		assert.Greater(t, f.count(t, "SELECT coalesce(max(version), 0) FROM epg_changes"), before, tt.name)
	}
}

// TestSyncerSyncsAllPeriodically は、イベントで届かない削除が、定期的な全量の突き合わせで反映されることを確かめる。
func TestSyncerSyncsAllPeriodically(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	syncer := epg.NewSyncer(f.reconciler, f.client, clock.Real{}, slog.New(slog.DiscardHandler), &epg.SyncerOptions{
		FullSyncInterval: 100 * time.Millisecond,
	})
	runSyncer(t, syncer)
	require.Eventually(t, func() bool {
		return f.count(t, "SELECT count(*) FROM programs") == fixturePrograms
	}, waitFor, tick)

	require.NoError(t, f.fake.ForgetProgram(programID(5)))
	require.Eventually(t, func() bool {
		return f.count(t, "SELECT count(*) FROM programs WHERE service_id = $1 AND event_id = 5", serviceID()) == 0
	}, waitFor, tick)
}

// countingUpstream は、全量の取得の回数を数える。
type countingUpstream struct {
	epg.Upstream

	fullFetches atomic.Int32
}

func (u *countingUpstream) Programs(ctx context.Context, query *mirakurun.ProgramsQuery) ([]*mirakurun.Program, error) {
	if query == nil {
		u.fullFetches.Add(1)
	}

	return u.Upstream.Programs(ctx, query)
}

// failingEvents は、イベントストリームを開くたびに失敗し、開こうとした時刻を記録する。
type failingEvents struct {
	mu    sync.Mutex
	calls []time.Time
}

func (e *failingEvents) OpenEvents(context.Context, *mirakurun.EventsQuery) (mirakurun.EventStream, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, time.Now())

	return nil, errors.New("connection refused")
}

func (e *failingEvents) snapshot() []time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()

	return append([]time.Time{}, e.calls...)
}

// TestSyncerBacksOffWhileReconnecting は、接続し直しが続くときに待つ時間を延ばし、
// 全量の突き合わせを下限の間隔より頻繁には行わないことを確かめる (設計書 3.3)。
func TestSyncerBacksOffWhileReconnecting(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	upstream := &countingUpstream{Upstream: f.client}
	events := &failingEvents{}
	const (
		initial = 20 * time.Millisecond
		maximum = 80 * time.Millisecond
	)
	reconciler := epg.NewReconciler(f.pool, upstream, clock.Real{}, func(context.Context, pgx.Tx) error { return nil })
	syncer := epg.NewSyncer(reconciler, events, clock.Real{}, slog.New(slog.DiscardHandler), &epg.SyncerOptions{
		MinFullSyncInterval: time.Hour,
		BackoffInitial:      initial,
		BackoffMax:          maximum,
	})
	runSyncer(t, syncer)
	require.Eventually(t, func() bool { return len(events.snapshot()) >= 6 }, waitFor, tick)

	calls := events.snapshot()
	// 待つ時間は 20ms、40ms、80ms と倍になり、上限の 80ms で止まる。
	for i, want := range []time.Duration{initial, 2 * initial, maximum, maximum, maximum} {
		assert.GreaterOrEqual(t, calls[i+1].Sub(calls[i]), want, "wait before reconnect %d", i+1)
	}
	// 接続し直しても、下限の間隔 (1 時間) が過ぎるまで全量は取得し直さない。
	assert.Equal(t, int32(1), upstream.fullFetches.Load())
}
