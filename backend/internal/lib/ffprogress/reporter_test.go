package ffprogress_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ffprogress"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
)

// TestReporter は、進み具合を、書き戻しの間隔を空けて書き、FFmpeg が終わったときは間を空けずに書くことを確かめる (#1511)。
// 止めた後に届いたまとまりは読まず、止める前に読んで書いていない値は、止めた後でも書く。
func TestReporter(t *testing.T) {
	t.Parallel()

	type event struct {
		at    time.Duration
		frame int64
		// stop は、このまとまりの前にジョブを止めるかどうか。
		stop bool
	}
	tests := []struct {
		name   string
		events []event
		// want は、書き戻した割合。最後は Finish で書いたもの。
		want []float64
	}{
		{
			name:   "throttled",
			events: []event{{at: 0, frame: 100}, {at: time.Second, frame: 200}, {at: 5 * time.Second, frame: 300}, {at: 6 * time.Second, frame: 400}},
			want:   []float64{0.1, 0.3, 0.4},
		},
		{
			name:   "nothing left to write at the end",
			events: []event{{at: 0, frame: 100}, {at: 5 * time.Second, frame: 300}},
			want:   []float64{0.1, 0.3},
		},
		{
			name:   "stopped",
			events: []event{{at: 0, frame: 100}, {at: time.Second, frame: 200}, {at: 10 * time.Second, frame: 1000, stop: true}},
			want:   []float64{0.1, 0.2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clk := clocktest.NewFake(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC))
			start := clk.Now()
			saved := []float64{}
			save := func(ctx context.Context, s *ffprogress.Snapshot) error {
				// 止めた後でも書けるよう、止められていない ctx で書く。
				require.NoError(t, ctx.Err())
				saved = append(saved, s.Ratio)

				return nil
			}
			reporter := ffprogress.NewReporter(ffprogress.DefaultConfig(), ffprogress.Estimate{Frames: 1000, Bytes: 0}, clk,
				func() int64 { return -1 }, save, slog.New(slog.DiscardHandler))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			for _, e := range tt.events {
				if e.stop {
					cancel()
				}
				clk.Set(start.Add(e.at))
				reporter.Observe(ctx, &ffprogress.Block{Frame: e.frame, TotalSize: -1, DropFrames: -1, Bitrate: "", Speed: "", End: false})
			}
			reporter.Finish(ctx)
			assert.Equal(t, tt.want, saved)
		})
	}
}
