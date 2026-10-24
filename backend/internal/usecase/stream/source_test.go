package stream_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

// frame は、testdata/ts/test.ts の I フレームの間隔 (45045 / 90000 秒)。
const frame = 500500 * time.Microsecond

// TestRecordingKeyframesAndSeek は、録画の I フレームの時間を、引き継ぎで切り詰める区間を除いて並べ、
// 流し始める位置を、その時間の手前の I フレームに合わせることを確かめる。
func TestRecordingKeyframesAndSeek(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	tests := []struct {
		name     string
		handover bool
		start    time.Duration
		want     []time.Duration
		wantAt   time.Duration
	}{
		{
			name: "1 つのセグメント", handover: false, start: 2010 * time.Millisecond,
			want:   []time.Duration{0, frame, 2 * frame, 3 * frame, 4 * frame, 5 * frame, 6 * frame, 7 * frame, 8 * frame, 9 * frame, 10 * frame, 11 * frame},
			wantAt: 4 * frame,
		},
		{
			// 後のセグメントは、前のセグメントの 1 秒後に始まり、最初の I フレームは前のセグメントの 3 個目と同じ。
			// 重なった区間は後のセグメントで流すので、時間は、1 本のセグメントと同じく PTS の差で並ぶ。
			name: "引き継ぎ", handover: true, start: 1500 * time.Millisecond,
			want:   []time.Duration{0, frame, 2 * frame, 3 * frame, 4 * frame, 5 * frame, 6 * frame, 7 * frame, 8 * frame, 9 * frame, 10 * frame, 11 * frame},
			wantAt: 2 * frame,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newRecordingFixture(t, nil)
			recordingID := f.recording(t, "completed")
			if tt.handover {
				f.segment(t, recordingID, "a.m2ts", data[:packetAligned(500000)], 0, false)
				f.segment(t, recordingID, "b.m2ts", data[packetAligned(200000):], time.Second, false)
			} else {
				f.segment(t, recordingID, "a.m2ts", data, 0, false)
			}
			source := stream.NewRecordingSource(f.pool, kago.New(map[string]string{poolID: f.dir}, nil).WithDataDir(f.dir), 0)
			recording, err := source.Open(t.Context(), recordingID)
			require.NoError(t, err)

			assert.Equal(t, tt.want, recording.Keyframes())
			assert.Equal(t, tt.wantAt, recording.Seek(tt.start).At)
			assert.False(t, recording.InProgress)
			assert.True(t, recording.Indexed())
		})
	}
}

// TestRecordingSourceOpenErrors は、録画を開けない理由を返すことを確かめる。
func TestRecordingSourceOpenErrors(t *testing.T) {
	t.Parallel()

	f := newRecordingFixture(t, nil)
	empty := f.recording(t, "completed")
	source := stream.NewRecordingSource(f.pool, kago.New(map[string]string{poolID: f.dir}, nil).WithDataDir(f.dir), 0)
	_, err := source.Open(t.Context(), uuid.New())
	require.ErrorIs(t, err, stream.ErrRecordingNotFound)
	_, err = source.Open(t.Context(), empty)
	require.ErrorIs(t, err, stream.ErrNoSegment)

	// プールをマウントしていないインスタンスでは開けない。
	mounted := f.recording(t, "completed")
	f.segment(t, mounted, "a.m2ts", loadTestTS(t), 0, false)
	unmounted := stream.NewRecordingSource(f.pool, kago.New(map[string]string{"sub": f.dir}, nil).WithDataDir(f.dir), 0)
	_, err = unmounted.Open(t.Context(), mounted)
	require.ErrorIs(t, err, stream.ErrNotMounted)
}
