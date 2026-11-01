package cmdetect_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
)

func TestParseStdout(t *testing.T) {
	t.Parallel()

	// metadata フィルターが出す形。lavfi.scd.time の行だけを読む。
	got := cmdetect.ParseStdout(strings.NewReader(strings.Join([]string{
		"frame:0    pts:4431534934 pts_time:49239.277044",
		"lavfi.scd.mafd=20.041",
		"lavfi.scd.score=19.734",
		"lavfi.scd.time=49239.277044",
		"frame:1    pts:4431634033 pts_time:49240.378144",
		"lavfi.scd.time=49240.378144",
		"lavfi.scd.time=broken",
	}, "\n")))

	assert.Equal(t, []float64{49239.277044, 49240.378144}, got)
}

func TestParseStderr(t *testing.T) {
	t.Parallel()

	// silencedetect フィルターが出すログの形。ほかの行は、失敗したときに出すため、末尾だけを残す。
	silences, tail := cmdetect.ParseStderr(strings.NewReader(strings.Join([]string{
		"Input #0, mpegts, from 'pipe:0':",
		"[Parsed_silencedetect_4 @ 0x7f0cd4016cc0] silence_start: 49237.660875",
		"[Parsed_silencedetect_4 @ 0x7f0cd4016cc0] silence_end: 49238.442292 | silence_duration: 0.781417",
		// 終わりのない始まりは、次の始まりで置き換える。
		"[Parsed_silencedetect_4 @ 0x7f0cd4016cc0] silence_start: 49250",
		"[Parsed_silencedetect_4 @ 0x7f0cd4016cc0] silence_start: 49252.5",
		"[Parsed_silencedetect_4 @ 0x7f0cd4016cc0] silence_end: 49253.5 | silence_duration: 1",
		"[out#0/null @ 0x55] video:1kB audio:2kB",
	}, "\n")))

	assert.Equal(t, [][2]float64{{49237.660875, 49238.442292}, {49252.5, 49253.5}}, silences)
	assert.Equal(t, []string{"Input #0, mpegts, from 'pipe:0':", "[out#0/null @ 0x55] video:1kB audio:2kB"}, tail)
}

func TestNewSignals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// first と last は、最初と最後の映像の PTS。
		first, last int64
		// scenes と silences は、FFmpeg が出す PTS のままの時刻 (秒)。
		scenes   []float64
		silences [][2]float64
		audio    []int64
		want     *cmdetect.Signals
	}{
		{
			name:  "時刻を、最初の映像からの秒にそろえる",
			first: 90_000 * 100, last: 90_000 * 1900,
			scenes: []float64{160.5}, silences: [][2]float64{{110, 111}}, audio: []int64{90_000 * 120},
			want: &cmdetect.Signals{Silences: [][2]float64{{10, 11}}, Scenes: []float64{60.5}, AudioChanges: []float64{20}, Duration: 1800},
		},
		{
			name: "PTS が一周しても、続けて数える",
			// 33 ビットの PTS が一周する 10 秒前から始まる録画。FFmpeg は一周した後の時刻を、一周の分を足して出す。
			first: 1<<33 - 90_000*10, last: 90_000 * 50,
			scenes: []float64{float64(1<<33+90_000*20) / 90_000}, silences: [][2]float64{}, audio: []int64{90_000 * 5},
			want: &cmdetect.Signals{Silences: [][2]float64{}, Scenes: []float64{30}, AudioChanges: []float64{15}, Duration: 60},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := cmdetect.NewSignals(&ts.Cues{FirstVideoPTS: &tt.first, LastVideoPTS: &tt.last, AudioChanges: tt.audio}, tt.scenes, tt.silences)
			require.NoError(t, err)
			assert.InDelta(t, tt.want.Duration, got.Duration, 0.001)
			assert.InDeltaSlice(t, tt.want.Scenes, got.Scenes, 0.001)
			assert.InDeltaSlice(t, tt.want.AudioChanges, got.AudioChanges, 0.001)
			require.Len(t, got.Silences, len(tt.want.Silences))
			for i := range got.Silences {
				assert.InDelta(t, tt.want.Silences[i][0], got.Silences[i][0], 0.001)
				assert.InDelta(t, tt.want.Silences[i][1], got.Silences[i][1], 0.001)
			}
		})
	}
}

func TestNewSignalsWithoutVideo(t *testing.T) {
	t.Parallel()

	_, err := cmdetect.NewSignals(&ts.Cues{FirstVideoPTS: nil, LastVideoPTS: nil, AudioChanges: []int64{}}, []float64{}, [][2]float64{})
	assert.ErrorIs(t, err, cmdetect.ErrNoVideo)
}
