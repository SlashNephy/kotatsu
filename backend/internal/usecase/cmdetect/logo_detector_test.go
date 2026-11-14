package cmdetect_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

// TestComplementSections は、ロゴの出ていた区間の外を、CM の区間にすることを確かめる。
func TestComplementSections(t *testing.T) {
	t.Parallel()

	const fps = 30.0
	tests := []struct {
		name      string
		intervals []logo.Interval
		n         int
		want      []cmdetect.Section
	}{
		{name: "頭と途中と終わりの、ロゴのない所", intervals: []logo.Interval{{Start: 300, End: 900}, {Start: 1800, End: 2700}}, n: 3000, want: []cmdetect.Section{
			{Start: 0, End: 10 * time.Second}, {Start: 30 * time.Second, End: time.Minute}, {Start: 90 * time.Second, End: 100 * time.Second},
		}},
		{name: "ずっとロゴが出ていれば、CM はない", intervals: []logo.Interval{{Start: 0, End: 3000}}, n: 3000, want: []cmdetect.Section{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, cmdetect.ComplementSections(tt.intervals, tt.n, fps))
		})
	}
}

// TestFrameRate は、ffprobe の速さを数にすることを確かめる。
func TestFrameRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want float64
	}{
		{in: "30000/1001", want: 30000.0 / 1001},
		{in: "25", want: 25},
		{in: "0/0", want: 0},
		{in: "", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			assert.InDelta(t, tt.want, cmdetect.FrameRate(tt.in), 1e-9)
		})
	}
}

// fallbackDetector は、ロゴを使えないときに呼ばれたかを覚える見分け方。
type fallbackDetector struct {
	called bool
}

func (*fallbackDetector) Name() string { return "fallback" }

func (*fallbackDetector) Version() int32 { return 1 }

func (d *fallbackDetector) Detect(context.Context, *cmdetect.Input) ([]cmdetect.Section, error) {
	d.called = true

	return []cmdetect.Section{{Start: time.Second, End: 2 * time.Second}}, nil
}

// TestLogoDetectorFallsBack は、局ロゴのない録画では、ロゴを使えないときの見分け方 (FFmpegDetector) の区間を返すことを確かめる。
// FFmpeg か ffprobe がなければ飛ばす。
func TestLogoDetectorFallsBack(t *testing.T) {
	t.Parallel()

	fallback := &fallbackDetector{called: false}
	detector, err := cmdetect.NewLogoDetectorWith(fallback)
	if errors.Is(err, cmdetect.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)

	// 動く絵だけの合成の映像には、動かない縁 (局ロゴ) がないので、学習できない。
	got, err := detector.Detect(t.Context(), &cmdetect.Input{Path: "../../../testdata/ts/test-mpeg2-480i-motion.ts", ServiceID: 0, Progress: nil})
	require.NoError(t, err)
	assert.True(t, fallback.called)
	assert.Equal(t, []cmdetect.Section{{Start: time.Second, End: 2 * time.Second}}, got)
	assert.Equal(t, cmdetect.LogoDetectorName, detector.Name())
}
