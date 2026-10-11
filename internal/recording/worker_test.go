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

// shortHandoverTimeout は、引き継ぎを待ちきれないことを確かめるテストで使う、引き継ぎを待つ時間。
const shortHandoverTimeout = 500 * time.Millisecond

// startWorker は、session で録画する mikan を動かし、止める関数を返す。止める関数は、Run が返るのを待つ。
// 引き継ぎを待つ時間は shortHandoverTimeout にする。
func (f *fixture) startWorker(t *testing.T, holder uuid.UUID, session recording.Session) func() {
	t.Helper()

	return f.startWorkerWithHandoverTimeout(t, holder, shortHandoverTimeout, session)
}

// startWorkerWithHandoverTimeout は、引き継ぎを待つ時間を handoverTimeout にして、mikan を動かす。
func (f *fixture) startWorkerWithHandoverTimeout(t *testing.T, holder uuid.UUID, handoverTimeout time.Duration, session recording.Session) func() {
	t.Helper()

	worker := recording.NewWorker(f.pool, f.clock, holder, slog.New(slog.DiscardHandler), session, &recording.WorkerOptions{
		LeaseTTL: 10 * time.Second, RenewInterval: 20 * time.Millisecond, AcquireInterval: 50 * time.Millisecond,
		HandoverTimeout: handoverTimeout, PollOnly: true, PollInterval: 50 * time.Millisecond,
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

// register は、mikan のインスタンスを、動いているものとして登録簿に載せる。
func (f *fixture) register(t *testing.T, holder uuid.UUID) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), "INSERT INTO instances (id, role, address, started_at, heartbeat_at) VALUES ($1, 'mikan', '127.0.0.1:0', $2, $2)",
		holder, f.clock.Now())
	require.NoError(t, err)
}

// streamOpened は、Recorder がストリームを開いたときと同じく、ストリームを開いたことを記録する。
func (f *fixture) streamOpened(ctx context.Context, t *testing.T, assignment *recording.Assignment) {
	t.Helper()

	_, err := f.pool.Exec(ctx, "UPDATE recording_assignments SET stream_generation = $1 WHERE id = $2", assignment.Generation, assignment.ID)
	require.NoError(t, err)
}

// TestWorkerHandsOver は、止まる mikan が、ほかの mikan がストリームを開くまで録り続けてから止まり、
// 割り振りを終わったことにしないことを確かめる (設計書 5.4)。
func TestWorkerHandsOver(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.assignOne(t)
	old, successor := id.New(), id.New()
	f.register(t, old)
	f.register(t, successor)

	oldCauses := make(chan error, 1)
	oldStarted := make(chan struct{})
	// 新しい mikan がストリームを開くまでの時間より十分に長く待たせる。CI の遅い環境でも、待ちきれずに止まらないようにする。
	stopOld := f.startWorkerWithHandoverTimeout(t, old, waitTimeout, func(ctx context.Context, _ *recording.Assignment) error {
		close(oldStarted)
		<-ctx.Done()
		oldCauses <- context.Cause(ctx)

		return nil
	})
	select {
	case <-oldStarted:
	case <-time.After(waitTimeout):
		t.Fatal("the old mikan did not acquire the assignment")
	}

	// 新しい mikan は、ストリームを開くまで 300 ミリ秒かかるものとする。
	taken := make(chan *recording.Assignment, 1)
	f.startWorker(t, successor, func(ctx context.Context, assignment *recording.Assignment) error {
		taken <- assignment
		time.Sleep(300 * time.Millisecond)
		f.streamOpened(ctx, t, assignment)
		<-ctx.Done()

		return nil
	})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		stopOld()
	}()

	var got *recording.Assignment
	select {
	case got = <-taken:
	case <-time.After(waitTimeout):
		t.Fatal("no mikan took over the assignment")
	}
	assert.Equal(t, int64(2), got.Generation)
	assert.True(t, got.HandedOver)
	// 新しい mikan がストリームを開くまでは、古い mikan も録り続ける。
	select {
	case cause := <-oldCauses:
		t.Fatalf("the old mikan stopped before the successor opened the stream: %v", cause)
	case <-time.After(150 * time.Millisecond):
	}
	select {
	case cause := <-oldCauses:
		require.ErrorIs(t, cause, recording.ErrHandedOver)
	case <-time.After(waitTimeout):
		t.Fatal("the old mikan did not stop after the handover")
	}
	<-stopped
	assert.Equal(t, assignmentRow{state: string(recording.AssignmentRecording), generation: 2, holder: &successor}, f.assignment(t, reservationID))
}

// TestWorkerReleasesWithoutSuccessor は、引き継ぐ mikan がいないか、引き継ぎを待ちきれなければ、割り振りを手放して止まり、
// 再起動した mikan が、リースが切れるのを待たずに続きを録れることを確かめる (設計書 5.4)。
func TestWorkerReleasesWithoutSuccessor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// peer は、動いているほかの mikan を登録簿に載せるかどうか。載せても、録画を取らない。
		peer bool
	}{
		{name: "ほかの mikan がいない", peer: false},
		{name: "ほかの mikan が引き継がない", peer: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.assignOne(t)
			holder := id.New()
			f.register(t, holder)
			if tt.peer {
				f.register(t, id.New())
			}
			causes := make(chan error, 1)
			started := make(chan struct{})
			stop := f.startWorker(t, holder, func(ctx context.Context, _ *recording.Assignment) error {
				close(started)
				<-ctx.Done()
				causes <- context.Cause(ctx)

				return nil
			})
			<-started
			stop()
			require.ErrorIs(t, <-causes, recording.ErrStopped)
			assert.Equal(t, assignmentRow{state: string(recording.AssignmentRecording), generation: 1, holder: nil}, f.assignment(t, reservationID))

			// 再起動した mikan は、時計を進めなくても (リースが切れなくても)、すぐに取る。
			taken := make(chan *recording.Assignment, 1)
			f.startWorker(t, holder, func(ctx context.Context, assignment *recording.Assignment) error {
				taken <- assignment
				<-ctx.Done()

				return nil
			})
			select {
			case assignment := <-taken:
				assert.Equal(t, int64(2), assignment.Generation)
				assert.False(t, assignment.HandedOver)
			case <-time.After(waitTimeout):
				t.Fatal("the restarted mikan did not take the released assignment")
			}
		})
	}
}

// TestWorkerTreatsCrashDuringHandoverAsCrash は、引き継ぎ待ちの印を付けた mikan が、引き継ぎの最中に落ちてリースが切れたら、
// 引き継ぎではなく、落ちた mikan の代わりとして取ることを確かめる。
func TestWorkerTreatsCrashDuringHandoverAsCrash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		leaseRemaining time.Duration
		wantHandedOver bool
	}{
		{name: "リースが残っていれば引き継ぎ", leaseRemaining: 5 * time.Second, wantHandedOver: true},
		{name: "リースが切れていれば落ちた mikan の代わり", leaseRemaining: -time.Second, wantHandedOver: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.assignOne(t)
			_, err := f.pool.Exec(t.Context(),
				"UPDATE recording_assignments SET state = 'handover', generation = 1, holder = $1, lease_expires_at = $2 WHERE reservation_id = $3",
				id.New(), f.clock.Now().Add(tt.leaseRemaining), reservationID)
			require.NoError(t, err)

			taken := make(chan *recording.Assignment, 1)
			f.startWorker(t, id.New(), func(ctx context.Context, assignment *recording.Assignment) error {
				taken <- assignment
				<-ctx.Done()

				return nil
			})
			select {
			case assignment := <-taken:
				assert.Equal(t, int64(2), assignment.Generation)
				assert.Equal(t, tt.wantHandedOver, assignment.HandedOver)
			case <-time.After(waitTimeout):
				t.Fatal("no mikan took the assignment")
			}
		})
	}
}

// TestWorkerFinishesDuringHandover は、引き継ぎを待つ間に予定の終わりが来て録画を終えたら、割り振りを終わったことにし、
// 引き継ぎを待たずに止まることを確かめる。
func TestWorkerFinishesDuringHandover(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.assignOne(t)
	holder := id.New()
	f.register(t, holder)
	f.register(t, id.New())
	started := make(chan struct{})
	finish := make(chan struct{})
	// 引き継ぎを待つ時間を長くし、待ちきれずにやめたのではなく、録画を終えたからやめたことを確かめる。
	// 短いと、引き継ぎ待ちの印がすぐに外れ、遅い環境では、印を見る前に外れることもある。
	stop := f.startWorkerWithHandoverTimeout(t, holder, waitTimeout, func(ctx context.Context, _ *recording.Assignment) error {
		close(started)
		select {
		case <-ctx.Done():
		case <-finish:
		}

		return nil
	})
	<-started
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		stop()
	}()
	require.Eventually(t, func() bool {
		return f.assignment(t, reservationID).state == string(recording.AssignmentHandover)
	}, waitTimeout, 10*time.Millisecond)

	close(finish)
	select {
	case <-stopped:
	case <-time.After(400 * time.Millisecond):
		t.Fatal("the mikan kept waiting for a successor after finishing the recording")
	}
	assert.Equal(t, assignmentRow{state: string(recording.AssignmentFinished), generation: 1, holder: nil}, f.assignment(t, reservationID))
}
