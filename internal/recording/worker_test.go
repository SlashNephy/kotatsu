package recording_test

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/recording"
)

const waitTimeout = 10 * time.Second

// startWorker は、session で録画する mikan を動かし、止める関数を返す。止める関数は、Run が返るのを待つ。
func (f *fixture) startWorker(t *testing.T, holder uuid.UUID, session recording.Session) func() {
	t.Helper()

	worker := recording.NewWorker(f.pool, f.clock, holder, slog.New(slog.DiscardHandler), session, &recording.WorkerOptions{
		LeaseTTL: 10 * time.Second, RenewInterval: 20 * time.Millisecond, AcquireInterval: 50 * time.Millisecond,
		PollOnly: true, PollInterval: 50 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- worker.Run(ctx)
	}()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		assert.NoError(t, <-done)
	}
	t.Cleanup(stop)

	return stop
}

// assignOne は、1 つの予約を、割り振りを書く時刻まで進めて割り振る。
func (f *fixture) assignOne(t *testing.T) uuid.UUID {
	t.Helper()

	reservationID := f.reserve(t, 0)
	f.clock.Advance(programStart().Add(-time.Minute).Sub(f.clock.Now()))
	require.Equal(t, 1, f.schedule(t).Created)

	return reservationID
}

type assignmentRow struct {
	state      string
	generation int64
	holder     *uuid.UUID
}

func (f *fixture) assignment(t *testing.T, reservationID uuid.UUID) assignmentRow {
	t.Helper()

	var row assignmentRow
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT state, generation, holder FROM recording_assignments WHERE reservation_id = $1", reservationID).Scan(&row.state, &row.generation, &row.holder))

	return row
}

// TestWorkersShareAssignment は、1 つの割り振りを、複数の mikan のうち 1 台だけが取り、録画を終えたら終わったことにすることを確かめる。
func TestWorkersShareAssignment(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.assignOne(t)
	var sessions atomic.Int32
	started := make(chan *recording.Assignment, 2)
	finish := make(chan struct{})
	session := func(ctx context.Context, assignment *recording.Assignment) error {
		sessions.Add(1)
		started <- assignment
		select {
		case <-ctx.Done():
		case <-finish:
		}

		return nil
	}
	holders := []uuid.UUID{id.New(), id.New()}
	for _, holder := range holders {
		f.startWorker(t, holder, session)
	}

	var got *recording.Assignment
	select {
	case got = <-started:
	case <-time.After(waitTimeout):
		t.Fatal("no mikan acquired the assignment")
	}
	assert.Equal(t, reservationID, got.ReservationID)
	assert.Equal(t, recording.Priority, got.Priority)
	assert.Equal(t, int64(1), got.Generation)
	// もう 1 台の mikan も何度か探す間、待っても、2 台目は取らない。
	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, int32(1), sessions.Load())
	row := f.assignment(t, reservationID)
	assert.Equal(t, string(recording.AssignmentRecording), row.state)
	require.NotNil(t, row.holder)
	assert.Contains(t, holders, *row.holder)

	close(finish)
	require.Eventually(t, func() bool {
		return f.assignment(t, reservationID).state == string(recording.AssignmentFinished)
	}, waitTimeout, 20*time.Millisecond)
	assert.Equal(t, assignmentRow{state: string(recording.AssignmentFinished), generation: 1, holder: nil}, f.assignment(t, reservationID))
}

// TestWorkerStopsWhenLeaseIsLost は、ほかの mikan が世代を進めて割り振りを取ると、古い mikan が書き込みをやめ、
// 割り振りを終わったことにしないことを確かめる。
func TestWorkerStopsWhenLeaseIsLost(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.assignOne(t)
	causes := make(chan error, 1)
	f.startWorker(t, id.New(), func(ctx context.Context, _ *recording.Assignment) error {
		<-ctx.Done()
		causes <- context.Cause(ctx)

		return nil
	})
	require.Eventually(t, func() bool {
		return f.assignment(t, reservationID).generation == 1
	}, waitTimeout, 20*time.Millisecond)

	// ほかの mikan が、世代を進めて取ったことにする。
	successor := id.New()
	_, err := f.pool.Exec(t.Context(), "UPDATE recording_assignments SET generation = 2, holder = $1 WHERE reservation_id = $2", successor, reservationID)
	require.NoError(t, err)
	select {
	case cause := <-causes:
		require.ErrorIs(t, cause, recording.ErrLeaseLost)
	case <-time.After(waitTimeout):
		t.Fatal("the old mikan kept recording after losing the lease")
	}
	assert.Equal(t, assignmentRow{state: string(recording.AssignmentRecording), generation: 2, holder: &successor}, f.assignment(t, reservationID),
		"the old mikan does not finish the assignment")
}

// TestWorkerTakesOverExpiredLease は、リースが切れたまま延長されない割り振りを、別の mikan が世代を進めて取ることを確かめる (設計書 1.5)。
func TestWorkerTakesOverExpiredLease(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.assignOne(t)
	// 落ちた mikan が、世代 1 で取ったまま、リースを延長しなくなったことにする。
	_, err := f.pool.Exec(t.Context(), "UPDATE recording_assignments SET state = 'recording', generation = 1, holder = $1, lease_expires_at = $2 WHERE reservation_id = $3",
		id.New(), f.clock.Now().Add(5*time.Second), reservationID)
	require.NoError(t, err)

	taken := make(chan *recording.Assignment, 1)
	successor := id.New()
	f.startWorker(t, successor, func(ctx context.Context, assignment *recording.Assignment) error {
		taken <- assignment
		<-ctx.Done()

		return nil
	})
	// リースが切れるまでは取らない。
	time.Sleep(200 * time.Millisecond)
	assert.Empty(t, taken)
	f.clock.Advance(5 * time.Second)
	select {
	case assignment := <-taken:
		assert.Equal(t, int64(2), assignment.Generation)
	case <-time.After(waitTimeout):
		t.Fatal("no mikan took over the expired assignment")
	}
	assert.Equal(t, &successor, f.assignment(t, reservationID).holder)
}

// TestWorkerLeavesAssignmentOnShutdown は、mikan を止めると、録画を止め、割り振りを、続きを録る mikan のために残すことを確かめる。
func TestWorkerLeavesAssignmentOnShutdown(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.assignOne(t)
	holder := id.New()
	started := make(chan struct{})
	stop := f.startWorker(t, holder, func(ctx context.Context, _ *recording.Assignment) error {
		close(started)
		<-ctx.Done()

		return ctx.Err()
	})
	select {
	case <-started:
	case <-time.After(waitTimeout):
		t.Fatal("the mikan did not acquire the assignment")
	}
	stop()
	assert.Equal(t, assignmentRow{state: string(recording.AssignmentRecording), generation: 1, holder: &holder}, f.assignment(t, reservationID))
}
