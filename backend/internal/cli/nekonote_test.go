package cli_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

const (
	subcommandMigrate = "migrate"
	roleNekonote      = "nekonote"
	roleTenban        = "tenban"
	roleKyusu         = "kyusu"
)

type pingArgs struct{}

func (pingArgs) Kind() string { return "test_ping" }

type pingWorker struct {
	river.WorkerDefaults[pingArgs]

	done chan struct{}
}

var _ river.Worker[pingArgs] = new(pingWorker)

func (w *pingWorker) Work(context.Context, *river.Job[pingArgs]) error {
	close(w.done)

	return nil
}

// TestNekonoteWorksJobs は、kotatsu nekonote が本物の DB で River のジョブを実行し、止まることを確かめる。
func TestNekonoteWorksJobs(t *testing.T) {
	t.Parallel()

	dbConfig := databasetest.New(t)
	env := testEnv()
	env["KOTATSU_DATABASE_URL"] = dbConfig.URL.Value()

	worker := &pingWorker{done: make(chan struct{})}
	opts := cli.DefaultOptions(slog.New(slog.DiscardHandler))
	opts.LookupEnv = func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	}
	opts.JobWorkers = func(*cli.Deps, *slog.Logger) (*river.Workers, []*river.PeriodicJob, error) {
		workers := river.NewWorkers()
		river.AddWorker(workers, worker)

		return workers, []*river.PeriodicJob{}, nil
	}
	// neko のリースや登録簿は、このテストの対象ではない。
	opts.Roles = filterRoles(opts.Roles, roleNekonote)

	migrateCmd := cli.NewCommand(opts)
	migrateCmd.SetArgs([]string{subcommandMigrate})
	require.NoError(t, migrateCmd.ExecuteContext(t.Context()))

	ctx, cancel := context.WithCancel(t.Context())
	cmd := cli.NewCommand(opts)
	cmd.SetArgs([]string{roleNekonote})
	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(ctx)
	}()

	pool, err := database.Open(t.Context(), dbConfig)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	inserter, err := jobs.NewClient(pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil})
	require.NoError(t, err)
	require.NoError(t, pgx.BeginFunc(t.Context(), pool, func(tx pgx.Tx) error {
		_, err := inserter.InsertTx(t.Context(), tx, pingArgs{}, nil)

		return err
	}))

	select {
	case <-worker.done:
	case err := <-done:
		t.Fatalf("nekonote stopped before working the job: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("nekonote did not work the job")
	}

	cancel()
	assert.NoError(t, <-done)
}

// TestNekonotePrunesAuditEvents は、既定の nekonote が、保持期間を過ぎた監査ログを起動時に消すことを確かめる。
func TestNekonotePrunesAuditEvents(t *testing.T) {
	t.Parallel()

	dbConfig := databasetest.New(t)
	env := testEnv()
	env["KOTATSU_DATABASE_URL"] = dbConfig.URL.Value()
	env["KOTATSU_AUDIT_RETENTION_DAYS"] = "30"

	opts := cli.DefaultOptions(slog.New(slog.DiscardHandler))
	opts.LookupEnv = func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	}
	opts.Roles = filterRoles(opts.Roles, roleNekonote)

	migrateCmd := cli.NewCommand(opts)
	migrateCmd.SetArgs([]string{subcommandMigrate})
	require.NoError(t, migrateCmd.ExecuteContext(t.Context()))

	pool, err := database.Open(t.Context(), dbConfig)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	for _, at := range []time.Time{time.Now().AddDate(0, 0, -31), time.Now().AddDate(0, 0, -29)} {
		require.NoError(t, audit.NewRecorder(clocktest.NewFake(at)).Record(t.Context(), pool, &audit.Event{
			Actor:      audit.System("neko"),
			Action:     "test",
			TargetType: "test",
		}))
	}

	ctx, cancel := context.WithCancel(t.Context())
	cmd := cli.NewCommand(opts)
	cmd.SetArgs([]string{roleNekonote})
	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(ctx)
	}()

	// 設定した保持期間 (30 日) を過ぎた 1 件だけが消える。
	// nekonote が先に止まったら、待ち続けずに、止まった理由で失敗させる。
	deadline := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for pruned := false; !pruned; {
		select {
		case err := <-done:
			t.Fatalf("nekonote stopped before pruning: %v", err)
		case <-deadline:
			var count int
			require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM audit_events").Scan(&count))
			t.Fatalf("nekonote did not prune the audit events in time: %d events remain", count)
		case <-ticker.C:
			var count int
			err := pool.QueryRow(t.Context(), "SELECT count(*) FROM audit_events").Scan(&count)
			pruned = err == nil && count == 1
		}
	}

	cancel()
	assert.NoError(t, <-done)
}

func filterRoles(roles []*cli.Role, names ...string) []*cli.Role {
	keep := map[string]bool{}
	for _, name := range names {
		keep[name] = true
	}
	filtered := make([]*cli.Role, 0, len(names))
	for _, role := range roles {
		if keep[role.Name] {
			filtered = append(filtered, role)
		}
	}

	return filtered
}
