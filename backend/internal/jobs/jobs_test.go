package jobs_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/jobs"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
)

type echoArgs struct {
	Value string `json:"value"`
}

func (echoArgs) Kind() string { return "test_echo" }

type echoWorker struct {
	river.WorkerDefaults[echoArgs]

	done chan string
}

var _ river.Worker[echoArgs] = new(echoWorker)

func (w *echoWorker) Work(_ context.Context, job *river.Job[echoArgs]) error {
	w.done <- job.Args.Value

	return nil
}

func TestMigrateAppliesRiverMigrations(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	require.ErrorIs(t, jobs.CheckSchema(t.Context(), pool), database.ErrSchemaOutdated)

	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	require.NoError(t, jobs.CheckSchema(t.Context(), pool))

	// 2 回目は何も適用しない。
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	require.NoError(t, jobs.CheckSchema(t.Context(), pool))
}

func TestInsertTxCommitsWithDomainWrite(t *testing.T) {
	t.Parallel()

	pool := migratedPool(t)
	_, err := pool.Exec(t.Context(), "CREATE TABLE items (name text PRIMARY KEY)")
	require.NoError(t, err)

	inserter, err := jobs.NewClient(pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil})
	require.NoError(t, err)

	errRollback := errors.New("rollback")
	tests := []struct {
		name     string
		item     string
		fail     error
		wantJobs int
	}{
		{name: "rollback discards the job", item: "rolled back", fail: errRollback, wantJobs: 0},
		{name: "commit inserts the job", item: "committed", fail: nil, wantJobs: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := pgx.BeginFunc(t.Context(), pool, func(tx pgx.Tx) error {
				if _, err := tx.Exec(t.Context(), "INSERT INTO items (name) VALUES ($1)", tt.item); err != nil {
					return err
				}
				if _, err := inserter.InsertTx(t.Context(), tx, echoArgs{Value: tt.item}, nil); err != nil {
					return err
				}

				return tt.fail
			})
			require.ErrorIs(t, err, tt.fail)

			var items, jobsCount int
			require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM items WHERE name = $1", tt.item).Scan(&items))
			require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM river_job WHERE args->>'value' = $1", tt.item).Scan(&jobsCount))
			assert.Equal(t, tt.wantJobs, items, "the domain write must follow the transaction")
			assert.Equal(t, tt.wantJobs, jobsCount, "the job must follow the transaction")
		})
	}
}

func TestWorkerRunsInsertedJobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pollOnly bool
	}{
		{name: "listen", pollOnly: false},
		// PgBouncer などのトランザクションプーリングを経由する設定。
		{name: "poll only", pollOnly: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := migratedPool(t)
			worker := &echoWorker{done: make(chan string, 1)}
			workers := river.NewWorkers()
			river.AddWorker(workers, worker)

			client, err := jobs.NewClient(pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: tt.pollOnly, Workers: workers})
			require.NoError(t, err)
			require.NoError(t, client.Start(t.Context()))
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
				defer cancel()
				assert.NoError(t, client.Stop(ctx))
			})

			require.NoError(t, pgx.BeginFunc(t.Context(), pool, func(tx pgx.Tx) error {
				_, err := client.InsertTx(t.Context(), tx, echoArgs{Value: tt.name}, nil)

				return err
			}))

			select {
			case got := <-worker.done:
				assert.Equal(t, tt.name, got)
			case <-time.After(30 * time.Second):
				t.Fatal("the job was not worked")
			}
		})
	}
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
