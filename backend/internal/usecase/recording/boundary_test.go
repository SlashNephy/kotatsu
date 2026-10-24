package recording_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
)

// recordedSpan は、録画のセグメントが表す時間の範囲。書いている途中なら、to は nil。
type recordedSpan struct {
	from time.Time
	to   *time.Time
}

// span は、予約の録画のセグメントの、最初の始まりと最後の終わりを返す。
func (f *fixture) span(t *testing.T, reservationID uuid.UUID) recordedSpan {
	t.Helper()

	var span recordedSpan
	err := f.pool.QueryRow(t.Context(), `SELECT min(rf.started_at), CASE WHEN bool_or(rf.ended_at IS NULL) THEN NULL ELSE max(rf.ended_at) END
		FROM recording_files rf JOIN recordings r ON r.id = rf.recording_id
		WHERE r.reservation_id = $1 AND rf.kind = 'segment'`, reservationID).Scan(&span.from, &span.to)
	require.NoError(t, err)

	return span
}

// broadcastTimes は、予約の録画に記録した、実際の放送の開始と終了を返す。
func (f *fixture) broadcastTimes(t *testing.T, reservationID uuid.UUID) (*time.Time, *time.Time) {
	t.Helper()

	var started, ended *time.Time
	err := f.pool.QueryRow(t.Context(), "SELECT broadcast_started_at, broadcast_ended_at FROM recordings WHERE reservation_id = $1",
		reservationID).Scan(&started, &ended)
	if err != nil {
		return nil, nil
	}

	return started, ended
}

// TestRecorderConsecutiveReservations は、同じチャンネルで続く 2 つの番組 (eventId 6 と 7) を予約したとき、
// EIT[p/f] で放送が切り替わる時刻が予定からずれても、どちらの録画も境界を取りこぼさないことを確かめる (#641)。
func TestRecorderConsecutiveReservations(t *testing.T) {
	t.Parallel()

	const margin = reserve.DefaultMarginSeconds * time.Second
	boundary := programStart().Add(30 * time.Minute)
	tests := []struct {
		name string
		// switchAt は、EIT[p/f] の放送中が、前の番組から後の番組に切り替わる時刻。
		switchAt time.Time
		// moveEarlier は、後の番組が繰り上がったことが、切り替わる前に EPG に届くかどうか。
		moveEarlier bool
	}{
		{name: "予定どおりに切り替わる", switchAt: boundary, moveEarlier: false},
		{name: "前の番組が延長し、後の番組が繰り下がる", switchAt: boundary.Add(20 * time.Minute), moveEarlier: false},
		{name: "前の番組が早く終わり、後の番組が繰り上がる", switchAt: boundary.Add(-10 * time.Minute), moveEarlier: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			first := f.reserveEvent(t, 0, 6)
			second := f.reserveEvent(t, 0, 7)
			firstStream, secondStream := newSteppedStream(), newSteppedStream()
			firstRecorder, _ := f.newRecorder(t, firstStream, nil)
			secondRecorder, _ := f.newRecorder(t, secondStream, nil)
			_, firstDone := f.session(t, firstRecorder, first)
			stopSecond, secondDone := f.session(t, secondRecorder, second)
			firstCC, secondCC := byte(0), byte(0)

			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			firstStream.next(t, eitPFPacketsFrom(&firstCC, 0x0400, 6, 7))
			require.Eventually(t, func() bool {
				started, _ := f.broadcastTimes(t, first)

				return started != nil
			}, waitTimeout, 10*time.Millisecond)

			if tt.moveEarlier {
				// 後の番組が繰り上がったことが EPG に届き、neko が予約に追従して、後の番組の予定の開始を早める。
				f.clock.Advance(tt.switchAt.Sub(f.clock.Now()))
				for _, q := range []string{
					"UPDATE programs SET end_at = $2 WHERE service_id = $1 AND event_id = 6",
					"UPDATE programs SET start_at = $2 WHERE service_id = $1 AND event_id = 7",
					"UPDATE reservations SET end_at = $2 WHERE service_id = $1 AND event_id = 6",
					"UPDATE reservations SET start_at = $2 WHERE service_id = $1 AND event_id = 7",
				} {
					_, err := f.pool.Exec(t.Context(), q, grService(0), tt.switchAt)
					require.NoError(t, err)
				}
				_, err := f.assigner.RunOnce(t.Context())
				require.NoError(t, err)
			} else {
				// 後の番組の予定の開始 (前のマージンを含む) に、後の録画が始まる。前の番組はまだ放送中。
				f.clock.Advance(boundary.Add(-margin).Sub(f.clock.Now()))
				secondStream.next(t, eitPFPacketsFrom(&secondCC, 0x0400, 6, 7))
				if tt.switchAt.After(boundary.Add(margin)) {
					// 前の番組の予定の終わりを過ぎても、放送中なので、前の録画は止めずに延ばす。後の録画は、開始を待つ。
					f.clock.Advance(tt.switchAt.Add(-time.Minute).Sub(f.clock.Now()))
					select {
					case err := <-firstDone:
						require.FailNow(t, "the first recording stopped while the program was on air", "%v", err)
					case <-time.After(200 * time.Millisecond):
					}
				}
				f.clock.Advance(tt.switchAt.Sub(f.clock.Now()))
			}

			// 放送中が、後の番組に切り替わる。
			firstStream.next(t, eitPFPacketsFrom(&firstCC, 0x0400, 7, 8))
			secondStream.next(t, eitPFPacketsFrom(&secondCC, 0x0400, 7, 8))
			require.Eventually(t, func() bool {
				_, ended := f.broadcastTimes(t, first)
				started, _ := f.broadcastTimes(t, second)

				return ended != nil && started != nil
			}, waitTimeout, 10*time.Millisecond)
			_, firstEnded := f.broadcastTimes(t, first)
			secondStarted, _ := f.broadcastTimes(t, second)
			assert.True(t, firstEnded.Equal(tt.switchAt), "the first program ended at %v", firstEnded)
			assert.True(t, secondStarted.Equal(tt.switchAt), "the second program started at %v", secondStarted)

			// 前の録画は、切り替わってから後ろのマージンを録って止まる。
			f.clock.Advance(tt.switchAt.Add(margin).Sub(f.clock.Now()))
			require.NoError(t, wait(t, firstDone))
			got, _ := f.recording(t, first)
			assert.Equal(t, string(recording.StateCompleted), got.state)
			firstSpan := f.span(t, first)
			require.NotNil(t, firstSpan.to)
			assert.False(t, firstSpan.to.Before(tt.switchAt.Add(margin)), "the first recording ended at %v", firstSpan.to)

			// 後の録画は、切り替わる前から録っていて、頭を取りこぼさない。
			secondSpan := f.span(t, second)
			assert.False(t, secondSpan.from.After(tt.switchAt), "the second recording started at %v", secondSpan.from)
			got, _ = f.recording(t, second)
			assert.Equal(t, string(recording.StateRecording), got.state)
			stopSecond(errStopForTest)
			<-secondDone
		})
	}
}
