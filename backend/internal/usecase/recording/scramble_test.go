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
)

// TestRecorderWarnsScrambledRecordings は、スクランブルされたまま録れた録画を、失敗にせず、警告の印を付けて完了の通知に添えることを確かめる (#1517)。
func TestRecorderWarnsScrambledRecordings(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	// 判定に要る ES のパケットの数に届くよう、つなぎ目にドロップのないように繰り返す。
	repeated := continuousRepeat(data, 8)
	tests := []struct {
		name        string
		data        []byte
		wantWarning bool
		wantRatio   float64
		// wantHistory は、録画履歴に記録した数。スクランブルされたまま録れた録画は、録画済みとみなさない (設計書 4.4)。
		wantHistory int
	}{
		{name: "decrypted", data: repeated, wantWarning: false, wantRatio: 0, wantHistory: 1},
		// 映像 (0x0111) と音声 (0x0112) をスクランブルし、PAT、PMT、EIT などは平文のままにする。
		{name: "still scrambled", data: faketuner.Scramble(repeated, 0x0111, 0x0112), wantWarning: true, wantRatio: 1, wantHistory: 0},
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
			assert.Equal(t, recorded{state: string(domain.RecordingStateCompleted), failureReason: nil, drops: 0}, got, "a scrambled recording must not fail")
			assert.Equal(t, tt.wantHistory, f.historyCount(t))
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

// continuousRepeat は、TS data を n 回つなぎ、continuity_counter を PID ごとに通し番号に振り直す。
// つなぎ目でドロップを数えないようにし、ドロップのない録画として確かめるのに使う。
func continuousRepeat(data []byte, n int) []byte {
	repeated := bytes.Repeat(data, n)
	counters := map[uint16]byte{}
	for offset := 0; offset+188 <= len(repeated); offset += 188 {
		packet := repeated[offset : offset+188]
		if packet[3]&0x10 == 0 {
			continue
		}
		pid := uint16(packet[1]&0x1F)<<8 | uint16(packet[2])
		packet[3] = packet[3]&0xF0 | counters[pid]&0x0F
		counters[pid]++
	}

	return repeated
}
