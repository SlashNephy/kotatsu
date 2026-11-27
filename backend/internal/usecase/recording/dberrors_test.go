package recording_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// recordingsTable と gapsColumn は、録画を終えるときに途切れた区間を書く、テーブルとカラム。
const (
	recordingsTable = "recordings"
	gapsColumn      = "gaps"
)

// renameColumn は、テーブル table のカラム from を to に改名する。ローリング更新で、新しいスキーマが古いバイナリの読むカラムを消した間を作る (#2195)。
func (f *fixture) renameColumn(t *testing.T, table, from, to string) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", table, from, to))
	require.NoError(t, err)
}

// TestRecorderWaitsThroughErrorsFromDatabase は、録画の途中や終わりに、DB に届いていても DB のエラーで読み書きできない間は、
// 録画を止めずに、最後に読めた予約で予定の終わりに止め、読み書きできるようになってから録画を終えることを確認する (#2195)。
func TestRecorderWaitsThroughErrorsFromDatabase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// table と column は、録画の終わりの前後に読み書きできなくするカラム。
		table  string
		column string
	}{
		{name: "予約を読めない間に予定の終わりが来る", table: "reservations", column: "description"},
		{name: "録画を終えるときに途切れた区間を書けない", table: recordingsTable, column: gapsColumn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile("../../../testdata/ts/test.ts")
			require.NoError(t, err)
			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			logs := newMessageRecorder()
			stream := newDrainedStream(data)
			recorder, _ := f.newRecorderWithLogger(t, stream, nil, nil, slog.New(logs))
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			stream.wait(t)

			f.renameColumn(t, tt.table, tt.column, tt.column+"_renamed")
			f.clock.Advance(time.Hour)
			logs.waitForMessage(t, "the database operation failed; retrying")
			got, _ := f.recording(t, reservationID)
			require.Equal(t, string(domain.RecordingStateRecording), got.state)
			f.renameColumn(t, tt.table, tt.column+"_renamed", tt.column)

			require.NoError(t, wait(t, done))
			got, _ = f.recording(t, reservationID)
			assert.Equal(t, string(domain.RecordingStateCompleted), got.state)
		})
	}
}

// TestWorkerReleasesWhenSessionStopsBeforeEnd は、Session が予定の終わりの前に録画をやめたら (ErrResumeLater)、
// 割り振りを終わったことにせず手放し、続きを録らせることを確認する (#2195)。終わったことにすると、neko が録画を失敗にする。
func TestWorkerReleasesWhenSessionStopsBeforeEnd(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.assignOne(t)
	sessions := new(atomic.Int32)
	resumed := make(chan struct{})
	stop := f.startWorker(t, id.New(), func(ctx context.Context, _ *recording.Assignment) error {
		if sessions.Add(1) == 1 {
			return fmt.Errorf("%w: %w", recording.ErrResumeLater, errors.New("failed to finish the recording"))
		}
		close(resumed)
		<-ctx.Done()

		return nil
	})
	defer stop()

	select {
	case <-resumed:
	case <-time.After(waitTimeout):
		t.Fatal("the assignment was not taken again to resume the recording")
	}
	got := f.assignment(t, reservationID)
	assert.Equal(t, string(domain.RecordingAssignmentStateRecording), got.state)
	assert.Equal(t, int64(2), got.generation)
}

// TestRecorderResumesWhenFinishKeepsFailing は、予定の終わりの後に、DB のエラーで録画を終えられないままやり直しの上限を過ぎたら、
// 予定の終わりから ResumeAfterEndLimit までは、割り振りを手放させて (ErrResumeLater)、ほかの mikan に録画を終えさせることを確認する (#2195)。
// 上限を過ぎていれば、手放させずに、録画の状態だけを記録して終える (#2199)。終えずにいると、neko が録画を失敗にする。
func TestRecorderResumesWhenFinishKeepsFailing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// table と column は、録画を終えるときに書けなくするカラム。
		table, column string
		// afterEnd は、予定の終わりから進める時間。
		afterEnd   time.Duration
		wantResume bool
		// wantState は、手放させないときの録画の状態。
		wantState domain.RecordingState
	}{
		{
			name: "予定の終わりから上限までなら手放させる", table: recordingsTable, column: gapsColumn, afterEnd: time.Minute, wantResume: true,
			wantState: domain.RecordingStateRecording,
		},
		{
			name: "上限を過ぎていれば、状態だけを記録して終える", table: recordingsTable, column: gapsColumn, afterEnd: recording.ResumeAfterEndLimit + time.Minute,
			wantResume: false, wantState: domain.RecordingStateCompleted,
		},
		{
			name: "録画履歴を書けなくても、上限を過ぎていれば状態だけを記録して終える", table: "recording_history", column: "series",
			afterEnd: recording.ResumeAfterEndLimit + time.Minute, wantResume: false, wantState: domain.RecordingStateCompleted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile("../../../testdata/ts/test.ts")
			require.NoError(t, err)
			f := newFixture(t)
			f.persistentErrorRetry = 200 * time.Millisecond
			reservationID := f.reserve(t, 0)
			stream := newDrainedStream(data)
			recorder, _ := f.newRecorder(t, stream, nil)
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			stream.wait(t)

			var plannedEnd time.Time
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT planned_end_at FROM reservations WHERE id = $1", reservationID).Scan(&plannedEnd))
			f.renameColumn(t, tt.table, tt.column, tt.column+"_renamed")
			f.clock.Advance(plannedEnd.Add(tt.afterEnd).Sub(f.clock.Now()))

			err = wait(t, done)
			if tt.wantResume {
				require.ErrorIs(t, err, recording.ErrResumeLater)
			} else {
				require.NoError(t, err)
			}
			got, _ := f.recording(t, reservationID)
			assert.Equal(t, string(tt.wantState), got.state)
		})
	}
}

// TestRecorderTakesOverWithoutListingFiles は、引き継いだ mikan が、録り始めに録画のファイルを読めなくても、DB を待たずに録り始めることを
// 確認する (#2195)。止まる mikan が引き継ぎを待つのは 20 秒までなので、待つと録画が欠ける。
func TestRecorderTakesOverWithoutListingFiles(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	oldRecorder, dir := f.newRecorder(t, onceStream{data: data}, nil)
	assignmentID := id.New()
	stopOld, oldDone := f.sessionWith(t, oldRecorder, &recording.Assignment{
		ID: assignmentID, ReservationID: reservationID, Priority: recording.Priority, Generation: 1, HandedOver: false, PreviousLeaseExpiresAt: nil,
	})
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool {
		return f.segments(t, reservationID) == 1
	}, waitTimeout, 10*time.Millisecond)

	// 録画のファイルの一覧を読めなくする。セグメントを記録する問い合わせは、このカラムを使わないので通る。
	f.renameColumn(t, "recording_files", "pid_stats", "pid_stats_renamed")
	f.clock.Advance(time.Minute)
	newRecorder, _ := f.newRecorderIn(t, dir, f.pool, onceStream{data: data}, nil, nil, slog.New(slog.DiscardHandler), nil)
	_, newDone := f.sessionWith(t, newRecorder, &recording.Assignment{
		ID: assignmentID, ReservationID: reservationID, Priority: recording.Priority, Generation: 2, HandedOver: true, PreviousLeaseExpiresAt: nil,
	})
	require.Eventually(t, func() bool {
		var n int
		require.NoError(t, f.pool.QueryRow(t.Context(),
			"SELECT count(*) FROM recording_files WHERE recording_id = (SELECT id FROM recordings WHERE reservation_id = $1)", reservationID).Scan(&n))

		return n == 2
	}, waitTimeout, 10*time.Millisecond, "the new mikan starts a segment of its own without waiting for the database")
	f.renameColumn(t, "recording_files", "pid_stats_renamed", "pid_stats")
	stopOld(recording.ErrHandedOver)
	require.ErrorIs(t, wait(t, oldDone), recording.ErrHandedOver)

	f.clock.Advance(time.Hour)
	require.NoError(t, wait(t, newDone))
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(domain.RecordingStateCompleted), got.state)
}
