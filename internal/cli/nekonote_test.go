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
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/jobs"
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
	opts.JobWorkers = func() *river.Workers {
		workers := river.NewWorkers()
		river.AddWorker(workers, worker)

		return workers
	}
	// neko のリースや登録簿は、このテストの対象ではない。
	opts.Roles = filterRoles(opts.Roles, "nekonote")

	migrateCmd := cli.NewCommand(opts)
	migrateCmd.SetArgs([]string{"migrate"})
	require.NoError(t, migrateCmd.ExecuteContext(t.Context()))

	ctx, cancel := context.WithCancel(t.Context())
	cmd := cli.NewCommand(opts)
	cmd.SetArgs([]string{"nekonote"})
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
