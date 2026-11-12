package recording_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// TestRecorderWarnsScrambledRecordings は、スクランブルされたまま録れた録画を、失敗にせず、警告の印を付けて完了の通知に添えることを確かめる (#1517)。
func TestRecorderWarnsScrambledRecordings(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	// 判定に要る ES のパケットの数に届くよう、繰り返す。つなぎ目のドロップは、この確かめに関わらない。
	repeated := bytes.Repeat(data, 8)
	tests := []struct {
		name        string
		data        []byte
		wantWarning bool
		wantRatio   float64
	}{
		{name: "decrypted", data: repeated, wantWarning: false, wantRatio: 0},
		// 映像 (0x0111) と音声 (0x0112) をスクランブルし、PAT、PMT、EIT などは平文のままにする。
		{name: "still scrambled", data: faketuner.Scramble(repeated, 0x0111, 0x0112), wantWarning: true, wantRatio: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			stream := newDrainedStream(tt.data)
			recorder, _ := f.newRecorder(t, stream, nil)
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			stream.wait(t)
			f.clock.Advance(time.Hour)
			require.NoError(t, wait(t, done))

			got, _ := f.recording(t, reservationID)
			assert.NotEqual(t, string(recording.StateFailed), got.state, "a scrambled recording must not fail")
			var warning bool
			var ratio *float64
			err := f.pool.QueryRow(t.Context(), "SELECT scramble_warning, scrambled_ratio FROM recordings WHERE reservation_id = $1", reservationID).
				Scan(&warning, &ratio)
			require.NoError(t, err)
			assert.Equal(t, tt.wantWarning, warning)
			require.NotNil(t, ratio)
			assert.InDelta(t, tt.wantRatio, *ratio, 1e-9)

			events := f.events(t)
			require.NotEmpty(t, events)
			last := events[len(events)-1]
			require.Equal(t, string(domain.NotificationKindRecordingCompleted), last.Kind)
			if tt.wantWarning {
				assert.Equal(t, true, last.Payload["scrambleWarning"])
				assert.InDelta(t, tt.wantRatio, last.Payload["scrambledRatio"], 1e-9)
			} else {
				assert.NotContains(t, last.Payload, "scrambleWarning")
			}
		})
	}
}
