package audit_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/jobs"
)

const (
	actorUser        = "user"
	targetRule       = "rule"
	actionRuleCreate = "rule.create"
)

func testNow() time.Time {
	return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
}

func TestRecordStoresUserAndSystemActors(t *testing.T) {
	t.Parallel()

	userID := id.New()
	tests := []struct {
		name  string
		event *audit.Event
		want  sqlc.AuditEvent
	}{
		{
			name: "user",
			event: &audit.Event{
				Actor:      audit.User(userID),
				Action:     "rule.update",
				TargetType: targetRule,
				TargetID:   "01a12053-0000-7000-8000-000000000001",
				Summary:    map[string]any{"enabled": map[string]bool{"from": true, "to": false}},
				SourceIP:   netip.MustParseAddr("192.0.2.10"),
				UserAgent:  "Mozilla/5.0",
			},
			want: sqlc.AuditEvent{
				ActorType:   actorUser,
				ActorUserID: &userID,
				Action:      "rule.update",
				TargetType:  targetRule,
				TargetID:    new("01a12053-0000-7000-8000-000000000001"),
				Summary:     []byte(`{"enabled": {"to": false, "from": true}}`),
				SourceIP:    new(netip.MustParseAddr("192.0.2.10")),
				UserAgent:   new("Mozilla/5.0"),
			},
		},
		{
			name: "system role",
			event: &audit.Event{
				Actor:      audit.System("neko"),
				Action:     "reservation.create",
				TargetType: "reservation",
				TargetID:   "01a12053-0000-7000-8000-000000000002",
				Summary:    nil,
			},
			want: sqlc.AuditEvent{
				ActorType:  "system",
				ActorRole:  new("neko"),
				Action:     "reservation.create",
				TargetType: "reservation",
				TargetID:   new("01a12053-0000-7000-8000-000000000002"),
				Summary:    []byte(`{}`),
			},
		},
		{
			name: "user without target",
			event: &audit.Event{
				Actor:      audit.User(userID),
				Action:     "user.login_failed",
				TargetType: actorUser,
				SourceIP:   netip.MustParseAddr("2001:db8::1"),
			},
			want: sqlc.AuditEvent{
				ActorType:   actorUser,
				ActorUserID: &userID,
				Action:      "user.login_failed",
				TargetType:  actorUser,
				Summary:     []byte(`{}`),
				SourceIP:    new(netip.MustParseAddr("2001:db8::1")),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := migratedPool(t)
			recorder := audit.NewRecorder(clocktest.NewFake(testNow()))
			require.NoError(t, recorder.Record(t.Context(), pool, tt.event))

			events, err := sqlc.New(pool).ListAuditEvents(t.Context(), 10)
			require.NoError(t, err)
			require.Len(t, events, 1)
			got := events[0]

			assert.NotEqual(t, uuid.Nil, got.ID)
			assert.True(t, testNow().Equal(got.OccurredAt))
			assert.Equal(t, tt.want.ActorType, got.ActorType)
			assert.Equal(t, tt.want.ActorUserID, got.ActorUserID)
			assert.Equal(t, tt.want.ActorRole, got.ActorRole)
			assert.Equal(t, tt.want.Action, got.Action)
			assert.Equal(t, tt.want.TargetType, got.TargetType)
			assert.Equal(t, tt.want.TargetID, got.TargetID)
			assert.JSONEq(t, string(tt.want.Summary), string(got.Summary))
			assert.Equal(t, tt.want.SourceIP, got.SourceIP)
			assert.Equal(t, tt.want.UserAgent, got.UserAgent)
		})
	}
}

func TestRecordRejectsInvalidEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event *audit.Event
	}{
		{name: "no event", event: nil},
		{name: "no actor", event: &audit.Event{Action: actionRuleCreate, TargetType: targetRule}},
		{name: "user without ID", event: &audit.Event{Actor: audit.User(uuid.Nil), Action: actionRuleCreate, TargetType: targetRule}},
		{name: "system without role", event: &audit.Event{Actor: audit.System(""), Action: actionRuleCreate, TargetType: targetRule}},
		{name: "unknown actor type", event: &audit.Event{Actor: &audit.Actor{Type: "robot", Role: "neko"}, Action: actionRuleCreate, TargetType: targetRule}},
		{name: "no action", event: &audit.Event{Actor: audit.System("neko"), TargetType: targetRule}},
		{name: "no target type", event: &audit.Event{Actor: audit.System("neko"), Action: actionRuleCreate}},
		{name: "unencodable summary", event: &audit.Event{Actor: audit.System("neko"), Action: actionRuleCreate, TargetType: targetRule, Summary: func() {}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// 検査で弾くので、DB には触れない。
			err := audit.NewRecorder(clocktest.NewFake(testNow())).Record(t.Context(), nil, tt.event)
			require.ErrorIs(t, err, audit.ErrInvalidEvent)
		})
	}
}

func TestDatabaseRejectsInconsistentActor(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	userID := id.New()
	// Recorder を通さずに書いても、操作者の種類と ID の組が合わなければ DB が拒否する。
	err := sqlc.New(pool).InsertAuditEvent(t.Context(), sqlc.InsertAuditEventParams{
		ID:          id.New(),
		OccurredAt:  testNow(),
		ActorType:   "system",
		ActorUserID: &userID,
		ActorRole:   nil,
		Action:      actionRuleCreate,
		TargetType:  targetRule,
		Summary:     []byte(`{}`),
	})
	require.ErrorContains(t, err, "audit_events_actor_check")
}

func TestRecordFollowsTransaction(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	recorder := audit.NewRecorder(clocktest.NewFake(testNow()))
	errRollback := errors.New("rollback")

	err := pgx.BeginFunc(t.Context(), pool, func(tx pgx.Tx) error {
		if err := recorder.Record(t.Context(), tx, &audit.Event{Actor: audit.System("neko"), Action: actionRuleCreate, TargetType: targetRule}); err != nil {
			return err
		}

		return errRollback
	})
	require.ErrorIs(t, err, errRollback)

	events, err := sqlc.New(pool).ListAuditEvents(t.Context(), 10)
	require.NoError(t, err)
	assert.Empty(t, events, "a rolled back operation must not leave an audit event")
}

func TestPruneDeletesEventsPastRetention(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	const retention = 365 * 24 * time.Hour
	record(t, pool, testNow().Add(-retention-time.Hour), "old")
	record(t, pool, testNow().Add(-retention+time.Hour), "recent")

	deleted, err := audit.Prune(t.Context(), pool, clocktest.NewFake(testNow()), retention)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
	assert.Equal(t, []string{"recent"}, actions(t, pool))
}

func TestPruneWorkerRunsPeriodically(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	const retention = 365 * 24 * time.Hour
	now := time.Now()
	record(t, pool, now.Add(-retention-time.Hour), "old")
	record(t, pool, now, "recent")

	workers := river.NewWorkers()
	river.AddWorker(workers, audit.NewPruneWorker(pool, clocktest.NewFake(now), retention))
	client, err := jobs.NewClient(pool, slog.New(slog.DiscardHandler), &jobs.Options{
		PollOnly:     false,
		Workers:      workers,
		PeriodicJobs: []*river.PeriodicJob{audit.PrunePeriodicJob()},
	})
	require.NoError(t, err)
	require.NoError(t, client.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cancel()
		assert.NoError(t, client.Stop(ctx))
	})

	// nekonote が起動したときにも 1 回消す (RunOnStart)。
	require.Eventually(t, func() bool {
		got := actions(t, pool)

		return len(got) == 1 && got[0] == "recent"
	}, 30*time.Second, 100*time.Millisecond)

	// 消した数は、River がジョブを完了にしたときに記録する。
	var output []byte
	require.Eventually(t, func() bool {
		err := pool.QueryRow(
			t.Context(),
			"SELECT metadata->'output' FROM river_job WHERE kind = $1 AND state = 'completed'",
			audit.PruneArgs{}.Kind(),
		).Scan(&output)

		return err == nil
	}, 30*time.Second, 100*time.Millisecond)
	var result map[string]int64
	require.NoError(t, json.Unmarshal(output, &result))
	assert.Equal(t, int64(1), result["deleted"])
}

func record(t *testing.T, pool *pgxpool.Pool, at time.Time, action string) {
	t.Helper()

	require.NoError(t, audit.NewRecorder(clocktest.NewFake(at)).Record(t.Context(), pool, &audit.Event{
		Actor:      audit.System("neko"),
		Action:     action,
		TargetType: "test",
	}))
}

func actions(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()

	events, err := sqlc.New(pool).ListAuditEvents(t.Context(), 100)
	require.NoError(t, err)
	result := make([]string, 0, len(events))
	for _, event := range events {
		result = append(result, event.Action)
	}

	return result
}

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	return pool
}
