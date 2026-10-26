package recording_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
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

	worker := recording.NewWorker(f.pool, f.clock, holder, slog.New(slog.DiscardHandler), session, recording.NewRecordingLeases(f.pool), &recording.WorkerOptions{
		LeaseTTL: 10 * time.Second, RenewInterval: 20 * time.Millisecond, AcquireInterval: 50 * time.Millisecond,
		HandoverTimeout: handoverTimeout, ReleaseCooldown: 0, DatabaseClock: false, PollOnly: true, PollInterval: 50 * time.Millisecond,
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

// insertRecording は、予約の録画の行を、完了したものとして作る。
func (f *fixture) insertRecording(ctx context.Context, t *testing.T, reservationID uuid.UUID) {
	t.Helper()

	_, err := f.pool.Exec(context.WithoutCancel(ctx), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, recorded_from, protected, created_at, updated_at)
		VALUES ($1, $2, $3, 6, '録画', '', $4, $4, 'completed', $4, false, $4, $4)`, id.New(), reservationID, grService(0), programStart())
	assert.NoError(t, err)
}

func (f *fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()

	var n int
	require.NoError(t, f.pool.QueryRow(t.Context(), query, args...).Scan(&n))

	return n
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
			// 録画したものとして、録画の行を作る。
			f.insertRecording(ctx, t, assignment.ReservationID)
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

// TestWorkerKeepsSessionWhenAssignmentDeleted は、録画中に予約が消され、割り振りも消えたとき、
// ほかの mikan が取ったとは扱わず、録画を終えるのを Session に任せることを確かめる (#304)。
func TestWorkerKeepsSessionWhenAssignmentDeleted(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.assignOne(t)
	causes := make(chan error, 1)
	release := make(chan struct{})
	f.startWorker(t, id.New(), func(ctx context.Context, _ *recording.Assignment) error {
		select {
		case <-ctx.Done():
			causes <- context.Cause(ctx)
		case <-release:
		}

		return nil
	})
	require.Eventually(t, func() bool {
		return f.assignment(t, reservationID).generation == 1
	}, waitTimeout, 20*time.Millisecond)

	_, err := f.pool.Exec(t.Context(), "DELETE FROM reservations WHERE id = $1", reservationID)
	require.NoError(t, err)
	// リースを延長する間隔 (20 ミリ秒) の何倍も待っても、Session を止めない。
	select {
	case cause := <-causes:
		t.Fatalf("the session was stopped: %v", cause)
	case <-time.After(500 * time.Millisecond):
	}
	close(release)
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
	stop := f.startWorkerWithHandoverTimeout(t, holder, waitTimeout, func(ctx context.Context, assignment *recording.Assignment) error {
		close(started)
		select {
		case <-ctx.Done():
		case <-finish:
			// 録画したものとして、録画の行を作る。
			f.insertRecording(ctx, t, assignment.ReservationID)
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
	// 引き継ぎを待つ時間 (waitTimeout) より十分に短い間に止まれば、録画を終えたから待つのをやめたと言える。
	select {
	case <-stopped:
	case <-time.After(waitTimeout / 2):
		t.Fatal("the mikan kept waiting for a successor after finishing the recording")
	}
	assert.Equal(t, assignmentRow{state: string(recording.AssignmentFinished), generation: 1, holder: nil}, f.assignment(t, reservationID))
}

// TestWorkerDeletesAssignmentsEndedWithoutRecording は、録画の行を作らずに終えた割り振り (録画を始める前に予約を無効にした、など) を、
// 終わったことにせず消すことを確かめる (#523)。終わった割り振りが残ると、予約を有効に戻しても、割り振りを書き直せなかった。
func TestWorkerDeletesAssignmentsEndedWithoutRecording(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.assignOne(t)
	ended := make(chan struct{})
	// 最後の見直しで書き直した割り振りも、同じ mikan が取って Session を始めることがある (#613)。
	endOnce := sync.OnceFunc(func() { close(ended) })
	f.startWorker(t, id.New(), func(context.Context, *recording.Assignment) error {
		endOnce()

		return nil
	})
	select {
	case <-ended:
	case <-time.After(waitTimeout):
		t.Fatal("no mikan acquired the assignment")
	}
	require.Eventually(t, func() bool {
		return f.count(t, "SELECT count(*) FROM recording_assignments WHERE reservation_id = $1", reservationID) == 0
	}, waitTimeout, 20*time.Millisecond)

	// 割り振りが消えたので、次の見直しで書き直せる。
	assert.Equal(t, 1, f.schedule(t).Created)
}

// TestWorkerReleasesUnrecordableAssignment は、Session が ErrRecorderUnavailable を返したら、割り振りを手放して
// しばらく取らず、ほかの mikan に録画させることを確かめる (#524)。手放さないと、録画できない mikan が割り振りを抱え込む。
func TestWorkerReleasesUnrecordableAssignment(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.assignOne(t)
	broken, healthy := id.New(), id.New()
	f.register(t, broken)
	f.register(t, healthy)
	var brokenSessions atomic.Int32
	f.startWorker(t, broken, func(context.Context, *recording.Assignment) error {
		brokenSessions.Add(1)

		return fmt.Errorf("the pool is not mounted: %w", recording.ErrRecorderUnavailable)
	})
	require.Eventually(t, func() bool {
		return brokenSessions.Load() == 1 && f.assignment(t, reservationID).holder == nil
	}, waitTimeout, 10*time.Millisecond)
	assert.Equal(t, string(recording.AssignmentWaiting), f.assignment(t, reservationID).state)

	recorded := make(chan struct{})
	f.startWorker(t, healthy, func(ctx context.Context, assignment *recording.Assignment) error {
		f.insertRecording(ctx, t, assignment.ReservationID)
		close(recorded)

		return nil
	})
	select {
	case <-recorded:
	case <-time.After(waitTimeout):
		t.Fatal("the healthy mikan did not take the released assignment")
	}
	require.Eventually(t, func() bool {
		return f.assignment(t, reservationID).state == string(recording.AssignmentFinished)
	}, waitTimeout, 10*time.Millisecond)
	// 手放した mikan は、しばらく取り直さない。
	assert.Equal(t, int32(1), brokenSessions.Load())
}

// TestWorkerUsesDatabaseClockForLeases は、リースが切れたかを DB の時刻で決めることを確かめる (#526)。
// mikan の時計で決めると、時計の進んだ mikan が、ほかの mikan の録画中の割り振りを、リースが切れたとみなして取る。
func TestWorkerUsesDatabaseClockForLeases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		databaseClock bool
		wantTaken     bool
	}{
		{name: "database clock", databaseClock: true, wantTaken: false},
		// mikan の時計を使うと、取ってしまう (テストで時計を動かすときの経路)。
		{name: "local clock", databaseClock: false, wantTaken: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.assignOne(t)
			// ほかの mikan が、DB の時刻で 1 分後まで有効なリースで録画している。
			_, err := f.pool.Exec(t.Context(),
				"UPDATE recording_assignments SET state = 'recording', generation = 1, holder = $1, lease_expires_at = now() + interval '1 minute' WHERE reservation_id = $2",
				id.New(), reservationID)
			require.NoError(t, err)
			// この mikan の時計は、DB より 1 日進んでいる。
			f.clock.Advance(time.Now().Add(24 * time.Hour).Sub(f.clock.Now()))

			taken := make(chan struct{}, 1)
			worker := recording.NewWorker(f.pool, f.clock, id.New(), slog.New(slog.DiscardHandler), func(ctx context.Context, _ *recording.Assignment) error {
				taken <- struct{}{}
				<-ctx.Done()

				return nil
			}, recording.NewRecordingLeases(f.pool), &recording.WorkerOptions{
				LeaseTTL: 10 * time.Second, RenewInterval: 20 * time.Millisecond, AcquireInterval: 50 * time.Millisecond,
				HandoverTimeout: shortHandoverTimeout, ReleaseCooldown: 0, DatabaseClock: tt.databaseClock, PollOnly: true, PollInterval: 50 * time.Millisecond,
			})
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- worker.Run(ctx) }()
			t.Cleanup(func() { cancel(); <-done })

			select {
			case <-taken:
				assert.True(t, tt.wantTaken, "the mikan took an assignment whose lease had not expired")
			case <-time.After(500 * time.Millisecond):
				assert.False(t, tt.wantTaken, "the mikan did not take the assignment")
			}
		})
	}
}
