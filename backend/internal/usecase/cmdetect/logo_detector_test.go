package cmdetect_test

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

// TestComplementSections は、ロゴの出ていた区間の外を、コマの時刻で CM の区間にすることを確かめる。
func TestComplementSections(t *testing.T) {
	t.Parallel()

	const fps = 30.0
	// times は、n コマ分の、100 秒から始まる時刻。from コマ目から先は、gap 秒だけ飛ぶ。
	times := func(n, from int, gap float64) []float64 {
		out := make([]float64, n)
		for i := range out {
			out[i] = 100 + float64(i)/fps
			if i >= from {
				out[i] += gap
			}
		}

		return out
	}
	tests := []struct {
		name      string
		intervals []logo.Interval
		times     []float64
		want      []cmdetect.Section
	}{
		{
			name: "頭と途中と終わりの、ロゴのない所", intervals: []logo.Interval{{Start: 300, End: 900}, {Start: 1800, End: 2700}}, times: times(3000, 3000, 0),
			want: []cmdetect.Section{{Start: 0, End: 10 * time.Second}, {Start: 30 * time.Second, End: time.Minute}, {Start: 90 * time.Second, End: 100 * time.Second}},
		},
		{
			// 1200 コマ目から先は時刻が 5 秒飛ぶので、その後の区間は 5 秒後ろになる。コマの番号で数えると、ずれる。
			name: "時刻が飛んだ後の区間は、飛んだ分だけ後ろにする", intervals: []logo.Interval{{Start: 0, End: 1800}}, times: times(2100, 1200, 5),
			want: []cmdetect.Section{{Start: 65 * time.Second, End: 75 * time.Second}},
		},
		{name: "ずっとロゴが出ていれば、CM はない", intervals: []logo.Interval{{Start: 0, End: 3000}}, times: times(3000, 3000, 0), want: []cmdetect.Section{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cmdetect.ComplementSections(tt.intervals, tt.times, fps)
			require.Len(t, got, len(tt.want), "%v", got)
			for i := range got {
				assert.InDelta(t, tt.want[i].Start.Seconds(), got[i].Start.Seconds(), 1e-6)
				assert.InDelta(t, tt.want[i].End.Seconds(), got[i].End.Seconds(), 1e-6)
			}
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

// TestLogoDetectorFallsBack は、局ロゴのない録画では、ロゴを使えないときの見分け方 (FFmpegDetector) の区間を返すことを確かめる。
// FFmpeg か ffprobe がなければ飛ばす。
func TestLogoDetectorFallsBack(t *testing.T) {
	t.Parallel()

	fallback := NewMockDetector(gomock.NewController(t))
	// 試験の TS は 1 秒なので、CM の割合の上限に届かない 0.1 秒の区間を返す。
	want := []cmdetect.Section{{Start: 0, End: 100 * time.Millisecond}}
	detector, err := cmdetect.NewLogoDetectorWith(fallback)
	if errors.Is(err, cmdetect.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)
	// 期待は、飛ばさないと決めてから置く。飛ばしたテストで、呼ばれなかった期待が失敗にならないようにする。
	fallback.EXPECT().Detect(gomock.Any(), gomock.Any()).Return(want, nil)

	// 動く絵だけの合成の映像には、動かない縁 (局ロゴ) がないので、学習できない。
	got, err := detector.Detect(t.Context(), &cmdetect.Input{Path: "../../../testdata/ts/test-mpeg2-480i-motion.ts", ServiceID: 0, Progress: nil})
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, cmdetect.LogoDetectorName, detector.Name())
}

// TestLogoDetector は、局ロゴを、出ている所、消えた所 (CM)、また出た所の順に描いた合成の TS で、
// ロゴの消えた所を CM の区間にすることを確かめる。FFmpeg か ffprobe がなければ飛ばす。
func TestLogoDetector(t *testing.T) {
	t.Parallel()

	// ロゴを使える録画では、今の見分け方を呼ばない。
	fallback := NewMockDetector(gomock.NewController(t))
	detector, err := cmdetect.NewLogoDetectorWith(fallback)
	if errors.Is(err, cmdetect.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)

	// 背景は、3 秒の周期で明るさが変わる灰色に、コマごとに変わる雑音を足したもの。縁の向きはコマごとに変わり、明るさの段には、
	// ロゴの出ているコマと消えたコマが混ざる。右上に、白い文字のような形のロゴを、15 秒から 25 秒の間だけ消して描く。
	path := filepath.Join(t.TempDir(), "logo.ts")
	const filter = "geq=lum='120+90*sin(2*PI*T/3)':cb=128:cr=128,noise=alls=40:allf=t,drawbox=x=560:y=20:w=4:h=24:color=white:t=fill:enable='lt(t,15)+gte(t,25)',drawbox=x=560:y=30:w=14:h=4:color=white:t=fill:enable='lt(t,15)+gte(t,25)',drawbox=x=580:y=20:w=4:h=24:color=white:t=fill:enable='lt(t,15)+gte(t,25)',drawbox=x=580:y=30:w=14:h=4:color=white:t=fill:enable='lt(t,15)+gte(t,25)',drawbox=x=600:y=20:w=4:h=24:color=white:t=fill:enable='lt(t,15)+gte(t,25)',drawbox=x=600:y=30:w=14:h=4:color=white:t=fill:enable='lt(t,15)+gte(t,25)'"
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=gray:s=640x360:r=30000/1001", "-t", "40",
		"-vf", filter, "-c:v", "mpeg2video", "-q:v", "4", "-g", "15", "-f", "mpegts", path)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	got, err := detector.Detect(t.Context(), &cmdetect.Input{Path: path, ServiceID: 0, Progress: nil})
	require.NoError(t, err)
	require.Len(t, got, 1, "%v", got)
	// 境目は、窓とならしで少し揺れるので、1 秒の内なら同じとみなす。
	assert.InDelta(t, 15, got[0].Start.Seconds(), 1, "%v", got)
	assert.InDelta(t, 25, got[0].End.Seconds(), 1, "%v", got)
}

// TestLogoDetectorDropsTooMuchCM は、ロゴを使えずに見分け直した結果も、CM の割合が上限以上なら、CM なしにすることを確かめる (#1508)。
func TestLogoDetectorDropsTooMuchCM(t *testing.T) {
	t.Parallel()

	fallback := NewMockDetector(gomock.NewController(t))
	fallback.EXPECT().Detect(gomock.Any(), gomock.Any()).Return([]cmdetect.Section{{Start: 0, End: 900 * time.Millisecond}}, nil)
	detector, err := cmdetect.NewLogoDetectorWith(fallback)
	if errors.Is(err, cmdetect.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)

	got, err := detector.Detect(t.Context(), &cmdetect.Input{Path: "../../../testdata/ts/test-mpeg2-480i-motion.ts", ServiceID: 0, Progress: nil})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestCMRatio は、CM の割合を、映像の欠けを分母からも分子からも外して求めることを確かめる (#1508)。
func TestCMRatio(t *testing.T) {
	t.Parallel()

	sec := func(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }
	tests := []struct {
		name     string
		sections []cmdetect.Section
		missing  []cmdetect.Section
		duration time.Duration
		want     float64
	}{
		{name: "欠けがなければ、CM の秒を尺で割る", sections: []cmdetect.Section{{Start: 0, End: sec(30)}, {Start: sec(60), End: sec(90)}}, missing: []cmdetect.Section{}, duration: sec(300), want: 0.2},
		{
			name: "CM に掛かった欠けは、CM の秒からも尺からも外す", sections: []cmdetect.Section{{Start: sec(200), End: sec(300)}},
			missing: []cmdetect.Section{{Start: sec(250), End: sec(300)}}, duration: sec(300), want: 0.2,
		},
		{name: "尺がすべて欠けなら 0", sections: []cmdetect.Section{{Start: 0, End: sec(10)}}, missing: []cmdetect.Section{{Start: 0, End: sec(10)}}, duration: sec(10), want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.InDelta(t, tt.want, cmdetect.CMRatio(tt.sections, tt.missing, tt.duration), 1e-9)
		})
	}
}

// TestMissingSections は、コマの時刻が 1 秒以上空いた所を、映像の欠けとして返すことを確かめる (#1508)。
func TestMissingSections(t *testing.T) {
	t.Parallel()

	const fps = 30.0
	times := make([]float64, 0, 300)
	for i := range 300 {
		times = append(times, 50+float64(i)/fps)
	}
	// 5 秒の位置で、3 秒途切れる。
	for i := range times[150:] {
		times[150+i] += 3
	}
	got := cmdetect.MissingSections(times, fps)
	require.Len(t, got, 1)
	assert.InDelta(t, 5.0, got[0].Start.Seconds(), 1e-6)
	assert.InDelta(t, 8.0, got[0].End.Seconds(), 1e-6)
}
