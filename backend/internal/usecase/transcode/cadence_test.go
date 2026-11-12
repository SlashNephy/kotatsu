package transcode_test

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
)

// 合成の差の列の値。moving は、動いている絵の差。repeated は、3:2 テレシネで繰り返されたフィールドの差 (ほぼ同じ絵)。
// sameFrame は、30p の同じコマの上と下の差 (行がずれた分だけが出る)。still は、静止した絵の上フィールドの差。
// telecineName と interlacedName は、合成の素材の名前。
const (
	telecineName   = "telecine"
	interlacedName = "60i"
)

const (
	moving    = 2.0
	repeated  = 0.1
	sameFrame = 0.5
	still     = 0.02
)

// diffsOf は、seconds 秒の、上フィールドの差を top(i)、上下を交互に並べた差を fields(i) で作った差の列を返す。
func diffsOf(seconds int, top, fields func(i int) float64) *transcode.FieldDiffs {
	d := &transcode.FieldDiffs{Top: make([]float64, seconds*30), Fields: make([]float64, seconds*60)}
	for i := range d.Top {
		d.Top[i] = top(i)
	}
	for i := range d.Fields {
		d.Fields[i] = fields(i)
	}

	return d
}

func constant(v float64) func(int) float64 { return func(int) float64 { return v } }

// telecined は、10 秒の 3:2 テレシネの差の列を返す。上フィールドは、phase から数えて 5 つに 1 つが繰り返しになる。
func telecined(phase int) *transcode.FieldDiffs {
	return diffsOf(10, func(i int) float64 {
		if (i+phase)%5 == 0 {
			return repeated
		}

		return moving
	}, constant(moving))
}

// progressive30 は、30p の差の列を返す。上下を交互に並べた差は、同じコマの上と下 (偶数番目) で小さい。
func progressive30(seconds int) *transcode.FieldDiffs {
	return diffsOf(seconds, constant(moving), func(i int) float64 {
		if i%2 == 0 {
			return sameFrame
		}

		return moving
	})
}

func interlaced60(seconds int) *transcode.FieldDiffs {
	return diffsOf(seconds, constant(moving), constant(moving))
}

// TestClassify は、窓のフィールドどうしの差から、rep と wr を求め、24p 系、30p、60i、測れないに分けることを確かめる (#1509)。
func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		diffs *transcode.FieldDiffs
		want  transcode.Cadence
		// wantRep と wantWR は、rep と wr の中央値。負なら確かめない。
		wantRep, wantWR float64
		wantSeconds     int
	}{
		{name: telecineName, diffs: telecined(0), want: transcode.Cadence24p, wantRep: repeated / moving, wantWR: 1, wantSeconds: 10},
		// 秒の区切りとプルダウンの周期がずれていても、繰り返す差は同じ位相に集まる。
		{name: "telecine sought to the middle of the cycle", diffs: telecined(3), want: transcode.Cadence24p, wantRep: repeated / moving, wantWR: 1, wantSeconds: 10},
		{name: "30p", diffs: progressive30(10), want: transcode.Cadence30p, wantRep: 1, wantWR: sameFrame / moving, wantSeconds: 10},
		{name: interlacedName, diffs: interlaced60(10), want: transcode.Cadence60i, wantRep: 1, wantWR: 1, wantSeconds: 10},
		// 上下を交互に並べた差は、静止した絵でも行のずれの分だけ大きく出るが、動いているかは上フィールドの差だけで決める。
		{name: "still", diffs: diffsOf(10, constant(still), constant(sameFrame*2)), want: transcode.CadenceUnknown, wantRep: 0, wantWR: 0, wantSeconds: 0},
		{name: "too few moving seconds", diffs: interlaced60(4), want: transcode.CadenceUnknown, wantRep: 1, wantWR: 1, wantSeconds: 4},
		// 1 秒に満たない端の差は数えない。
		{name: "partial second", diffs: &transcode.FieldDiffs{Top: interlaced60(5).Top[:149], Fields: interlaced60(5).Fields[:299]}, want: transcode.CadenceUnknown, wantRep: 1, wantWR: 1, wantSeconds: 4},
		// 動いている秒が半分でも、最小の秒数に足りれば、動いている秒だけで測る。
		{
			name: "moving half",
			diffs: diffsOf(10, func(i int) float64 {
				if i < 150 {
					return still
				}

				return moving
			}, constant(moving)),
			want: transcode.Cadence60i, wantRep: 1, wantWR: 1, wantSeconds: 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := transcode.DefaultCadenceParams().Classify(tt.diffs)
			assert.Equal(t, tt.want, got.Cadence)
			assert.InDelta(t, tt.wantRep, got.Rep, 1e-9)
			assert.InDelta(t, tt.wantWR, got.WR, 1e-9)
			assert.Equal(t, tt.wantSeconds, got.RepSeconds)
		})
	}
}

// TestClassifyWithoutFields は、上下を交互に並べた差が足りなければ、24p 系とは言えない窓を測れないとすることを確かめる (#1509)。
func TestClassifyWithoutFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		diffs *transcode.FieldDiffs
		want  transcode.Cadence
	}{
		// rep だけで 24p 系と分かるので、wr は要らない。
		{name: telecineName, diffs: &transcode.FieldDiffs{Top: telecined(0).Top, Fields: []float64{}}, want: transcode.Cadence24p},
		{name: interlacedName, diffs: &transcode.FieldDiffs{Top: interlaced60(10).Top, Fields: interlaced60(2).Fields}, want: transcode.CadenceUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, transcode.DefaultCadenceParams().Classify(tt.diffs).Cadence)
		})
	}
}

// TestFieldOutput は、60i の窓の数が、24p 系と 30p の窓の数の合計以上なら 60 コマにすることを確かめる (#1509)。
func TestFieldOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cadences []transcode.Cadence
		want     bool
	}{
		{name: "anime", cadences: []transcode.Cadence{transcode.Cadence24p, transcode.Cadence24p, transcode.Cadence60i, transcode.Cadence24p}, want: false},
		{name: "30p and 24p add up", cadences: []transcode.Cadence{transcode.Cadence24p, transcode.Cadence30p, transcode.Cadence60i}, want: false},
		{name: "live action", cadences: []transcode.Cadence{transcode.Cadence60i, transcode.Cadence60i, transcode.Cadence24p, transcode.CadenceUnknown}, want: true},
		// 迷ったら 60 コマにする。
		{name: "tie", cadences: []transcode.Cadence{transcode.Cadence60i, transcode.Cadence30p}, want: true},
		{name: "nothing measured", cadences: []transcode.Cadence{transcode.CadenceUnknown, transcode.CadenceUnknown}, want: true},
		{name: "unknown windows do not count", cadences: []transcode.Cadence{transcode.Cadence24p, transcode.CadenceUnknown, transcode.CadenceUnknown}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			results := []*transcode.WindowResult{}
			for _, c := range tt.cadences {
				results = append(results, &transcode.WindowResult{Rep: 0, RepSeconds: 0, WR: 0, WRSeconds: 0, Cadence: c})
			}
			assert.Equal(t, tt.want, transcode.FieldOutput(results))
		})
	}
}

// TestWindowPositions は、(尺 − 窓の長さ) を等分した内側の点に窓を置き、重なる位置を 2 回測らないことを確かめる (#1509)。
func TestWindowPositions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
		n        int
		want     []time.Duration
	}{
		{name: "spread", duration: 120 * time.Second, n: 4, want: []time.Duration{20 * time.Second, 40 * time.Second, 60 * time.Second, 80 * time.Second}},
		{name: "shorter than the window", duration: 15 * time.Second, n: 4, want: []time.Duration{0}},
		{name: "rounded to seconds and deduplicated", duration: 22 * time.Second, n: 4, want: []time.Duration{0, time.Second, 2 * time.Second}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, transcode.WindowPositions(tt.duration, 20*time.Second, tt.n))
		})
	}
}

// TestParseFieldDiffs は、FFmpeg の標準エラー出力から、名前を付けた metadata のフィルタの値の行だけを拾い、先頭の値を捨てることを
// 確かめる (#1509)。行は、書き込みの途中で切れて届くこともある。
func TestParseFieldDiffs(t *testing.T) {
	t.Parallel()

	stderr := "Input #0, mpegts, from 'a.ts':\n" +
		"[metadata@top @ 0x7fc2c0004bc0] frame:0    pts:18468   pts_time:0.2052\n" +
		"[metadata@top @ 0x7fc2c0004bc0] lavfi.signalstats.YDIF=0\n" +
		"[metadata@fields @ 0x7fc2c0005380] frame:0    pts:36936   pts_time:0.2052\n" +
		"[metadata@fields @ 0x7fc2c0005380] lavfi.signalstats.YDIF=0\n" +
		"[metadata@fields @ 0x7fc2c0005380] lavfi.signalstats.YDIF=1.5\r\n" +
		"[metadata@top @ 0x7fc2c0004bc0] lavfi.signalstats.YDIF=2.25\n" +
		"[Parsed_metadata_3 @ 0x7fc2c0005380] lavfi.signalstats.YDIF=9\n" +
		"[metadata@fields @ 0x7fc2c0005380] lavfi.signalstats.YDIF=0.5\n" +
		"[metadata@top @ 0x7fc2c0004bc0] lavfi.signalstats.YDIF=3\n"
	for _, chunk := range []int{1, 7, len(stderr)} {
		got, err := transcode.ParseFieldDiffs(stderr, chunk)
		require.NoError(t, err)
		assert.Equal(t, &transcode.FieldDiffs{Top: []float64{2.25, 3}, Fields: []float64{1.5, 0.5}}, got, "chunk %d", chunk)
	}

	_, err := transcode.ParseFieldDiffs("Error opening input file a.ts.\n", 64)
	require.ErrorContains(t, err, "Error opening input file")
}

// synthesize は、合成の映像を、放送と同じく上が先のインターレースの MPEG-2 の TS に書く。filter は、testsrc2 の後に掛けるフィルタ。
func synthesize(t *testing.T, dir, name, source, filter string) string {
	t.Helper()

	out := filepath.Join(dir, name+".ts")
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", source, "-vf", filter+"setfield=tff",
		"-c:v", "mpeg2video", "-b:v", "4M", "-flags", "+ilme+ildct", "-an", "-f", "mpegts", out)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	return out
}

// TestFFmpegEncoderMeasureFields は、本物の FFmpeg で、合成の映像のフィールドどうしの差を測り、24p を 3:2 テレシネで載せた 60i、
// 30p、ふつうの 60i、黒を見分けることを確かめる (#1509)。窓は、テレシネの周期の途中に当たる位置から始める。FFmpeg のない環境では飛ばす。
func TestFFmpegEncoderMeasureFields(t *testing.T) {
	t.Parallel()

	encoder, err := transcode.NewFFmpegEncoder(&config.Kyusu{Encoder: "", VAAPIDevice: "", WorkDir: ""}, nil)
	if errors.Is(err, transcode.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)
	tests := []struct {
		name   string
		source string
		filter string
		want   transcode.Cadence
	}{
		{name: telecineName, source: "testsrc2=s=720x480:r=24000/1001:d=12", filter: "telecine=first_field=top:pattern=23,", want: transcode.Cadence24p},
		{name: "30p", source: "testsrc2=s=720x480:r=30000/1001:d=12", filter: "", want: transcode.Cadence30p},
		{name: interlacedName, source: "testsrc2=s=720x480:r=60000/1001:d=12", filter: "interlace=scan=tff:lowpass=off,", want: transcode.Cadence60i},
		{name: "black", source: "color=black:s=720x480:r=30000/1001:d=12", filter: "", want: transcode.CadenceUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := synthesize(t, t.TempDir(), tt.name, tt.source, tt.filter)
			diffs, err := encoder.MeasureFields(t.Context(), path, 1300*time.Millisecond, 8*time.Second)
			require.NoError(t, err)
			// 8 秒の窓から、先頭を捨てた差が、1 秒に 30 個と 60 個ずつ出る。
			assert.InDelta(t, 8*30, len(diffs.Top), 2)
			assert.InDelta(t, 8*60, len(diffs.Fields), 3)
			got := transcode.DefaultCadenceParams().Classify(diffs)
			// 閾値を詰め直す材料にするため、測った値を出す。
			t.Logf("rep %.3f (%d s), wr %.3f (%d s)", got.Rep, got.RepSeconds, got.WR, got.WRSeconds)
			assert.Equal(t, tt.want, got.Cadence)
		})
	}
}
