package recording_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/recording"
	"github.com/SlashNephy/kotatsu/internal/reserve"
)

// TestRecorderFollowsInterruption は、録画中の番組 (eventId 6) が臨時ニュースなど別の番組 (eventId 65522) に割り込まれ、
// 同じ eventId で再開する場合に、再開の前に止まらないことを確かめる (#636)。
func TestRecorderFollowsInterruption(t *testing.T) {
	t.Parallel()

	const (
		margin      = reserve.DefaultMarginSeconds * time.Second
		interruptID = 65522
	)
	interruptAt := programStart().Add(10 * time.Minute)
	tests := []struct {
		name string
		// following は、割り込みの間の EIT[p/f] の次の番組。
		following uint16
		// resumeAfter は、割り込みから予約した番組に戻るまでの時間。
		resumeAfter time.Duration
		// wantStopped は、再開の前に止まるかどうか。
		wantStopped bool
	}{
		{name: "割り込みの間、次の番組が予約した番組なら、終わったとせず、再開した後も録り続ける", following: 6, resumeAfter: 5 * time.Minute, wantStopped: false},
		{name: "後ろのマージンを録り終える前に予約した番組に戻ったら、終わりを取り消して録り続ける", following: 7, resumeAfter: margin / 3, wantStopped: false},
		// 割り込みの間に、予約した番組が放送中にも次の番組にも出なければ、再開するかは分からないので、猶予を置かずに止める。
		{name: "次の番組にも出ないまま割り込まれたら、後ろのマージンを録って止める", following: 7, resumeAfter: 5 * time.Minute, wantStopped: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			stream := newSteppedStream()
			recorder, _ := f.newRecorder(t, stream, nil)
			stop, done := f.session(t, recorder, reservationID)
			cc := byte(0)

			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			stream.next(t, eitPFPacketsFrom(&cc, 0x0400, 6, 7))
			require.Eventually(t, func() bool {
				started, _ := f.broadcastTimes(t, reservationID)

				return started != nil
			}, waitTimeout, 10*time.Millisecond)

			// 割り込まれる。
			f.clock.Advance(interruptAt.Sub(f.clock.Now()))
			stream.next(t, eitPFPacketsFrom(&cc, 0x0400, interruptID, tt.following))
			// 割り込みを読んでから時刻を進める。次の番組が予約した番組なら、終わったとはしない。
			if tt.following == 6 {
				require.Never(t, func() bool {
					_, ended := f.broadcastTimes(t, reservationID)

					return ended != nil
				}, 200*time.Millisecond, 10*time.Millisecond)
			} else {
				require.Eventually(t, func() bool {
					_, ended := f.broadcastTimes(t, reservationID)

					return ended != nil
				}, waitTimeout, 10*time.Millisecond)
			}
			f.clock.Advance(tt.resumeAfter)
			if tt.wantStopped {
				require.NoError(t, wait(t, done))
				_, ended := f.broadcastTimes(t, reservationID)
				require.NotNil(t, ended)
				assert.True(t, ended.Equal(interruptAt), "the broadcast ended at %v", ended)

				return
			}

			// 予約した番組が再開する。
			stream.next(t, eitPFPacketsFrom(&cc, 0x0400, 6, 7))
			require.Eventually(t, func() bool {
				_, ended := f.broadcastTimes(t, reservationID)

				return ended == nil
			}, waitTimeout, 10*time.Millisecond)
			f.clock.Advance(margin)
			select {
			case err := <-done:
				require.FailNow(t, "the recording stopped after the program resumed", "%v", err)
			case <-time.After(200 * time.Millisecond):
			}
			got, _ := f.recording(t, reservationID)
			assert.Equal(t, string(recording.StateRecording), got.state)

			// 再開した番組が終わったら、そこで止める。
			programEnd := programStart().Add(30 * time.Minute)
			f.clock.Advance(programEnd.Sub(f.clock.Now()))
			stream.next(t, eitPFPacketsFrom(&cc, 0x0400, 7, 8))
			require.Eventually(t, func() bool {
				_, ended := f.broadcastTimes(t, reservationID)

				return ended != nil
			}, waitTimeout, 10*time.Millisecond)
			_, ended := f.broadcastTimes(t, reservationID)
			assert.True(t, ended.Equal(programEnd), "the broadcast ended at %v", ended)
			f.clock.Advance(margin)
			require.NoError(t, wait(t, done))
			stop(nil)
		})
	}
}
