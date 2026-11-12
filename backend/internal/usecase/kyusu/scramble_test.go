package kyusu_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
)

// bytesOpener は、data を一度だけ流すストリームを開く。
type bytesOpener struct{ data []byte }

func (o *bytesOpener) OpenServiceStream(context.Context, int64, *tunerserver.StreamOptions) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(o.data)), nil
}

// failingEncoder は、FFmpeg がスクランブルされた ES を読めずに降りるのを真似て、入力を読み終えたら、プレイリストを書かずに失敗する。
type failingEncoder struct{}

func (failingEncoder) Run(_ context.Context, src io.Reader, _ *kyusu.Preset, _ kyusu.Audio, _ string) error {
	if _, err := io.Copy(io.Discard, src); err != nil {
		return err
	}

	return io.ErrUnexpectedEOF
}

// recordingWatcher は、パケットを 1 つ数えるたびに時計を 100 ミリ秒進める判定器を作り、スクランブルされたままと記録したサービスを覚える。
type recordingWatcher struct {
	mu       sync.Mutex
	now      time.Time
	reported []int64
}

func (w *recordingWatcher) Watch(ctx context.Context, serviceID int64, src io.Reader) (io.Reader, *ts.ScrambleDetector) {
	detector := ts.NewScrambleDetector(ts.ScrambleDetectorOptions{
		WindowPackets: 0, Windows: 0, Hold: 0, MinPartialPackets: 0,
		Now: func() time.Time {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.now = w.now.Add(100 * time.Millisecond)

			return w.now
		},
		OnScrambled: func() { w.ReportQuietly(ctx, serviceID) },
	})

	return io.TeeReader(src, detector), detector
}

func (w *recordingWatcher) ReportQuietly(_ context.Context, serviceID int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.reported = append(w.reported, serviceID)
}

func (w *recordingWatcher) list() []int64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	return append([]int64{}, w.reported...)
}

var _ kyusu.ScrambleWatcher = new(recordingWatcher)

// TestLiveReportsScrambledStreams は、エンコーダーに流す TS がスクランブルされたまま届いたら、理由を記録してからセッションを閉じることを確かめる (#1517)。
// エンコーダーが判定の前に降りたら、いちばん新しい窓だけで決める。
func TestLiveReportsScrambledStreams(t *testing.T) {
	t.Parallel()

	data := loadTS(t, "test.ts")
	scrambled := faketuner.Scramble(data, 0x0111, 0x0112)
	tests := []struct {
		name string
		data []byte
		// wantStatus は、プレイリストの要求への状態。スクランブルされたままなら 503、そうでなければ、エンコーダーの失敗の 502。
		wantStatus int
		want       []int64
	}{
		{name: "scrambled for windows past the hold", data: bytes.Repeat(scrambled, 2), wantStatus: http.StatusServiceUnavailable, want: []int64{3273601024}},
		// 窓 1 つに満たないまま、エンコーダーが降りる。埋まりかけの窓で決める。
		{name: "the encoder exits before a window fills", data: scrambled[:400*ts.PacketSize], wantStatus: http.StatusServiceUnavailable, want: []int64{3273601024}},
		{name: "decrypted", data: bytes.Repeat(data, 2), wantStatus: http.StatusBadGateway, want: []int64{}},
		{name: "the encoder exits with too few packets to decide", data: scrambled[:100*ts.PacketSize], wantStatus: http.StatusBadGateway, want: []int64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			watcher := &recordingWatcher{mu: sync.Mutex{}, now: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC), reported: []int64{}}
			f := newLiveFixtureWithScramble(t, &bytesOpener{data: tt.data}, failingEncoder{}, watcher)
			status, _ := f.get(t, livePath+kyusu.PlaylistName)
			assert.Equal(t, tt.wantStatus, status)
			require.Eventually(t, func() bool { return f.live.Sessions() == 0 }, 5*time.Second, 10*time.Millisecond)
			assert.Equal(t, tt.want, watcher.list())
		})
	}
}
