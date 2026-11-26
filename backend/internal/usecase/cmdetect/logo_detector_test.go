package cmdetect_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

// TestComplementSections は、ロゴの出ていた区間の外を、コマの時刻で CM の区間にすることを確認する。
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

// TestFrameRate は、ffprobe の速さを数にすることを確認する。
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

// TestLogoDetectorFallsBack は、局ロゴのない録画では、ロゴを使えないときの見分け方 (FFmpegDetector) の区間を返すことを確認する。
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
	got, err := detector.Detect(t.Context(), &cmdetect.Input{Path: "../../../testdata/ts/test-mpeg2-480i-motion.ts", ServiceID: 0, Progress: nil, Program: nil})
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, cmdetect.LogoDetectorName, detector.Name())
}

// TestLogoDetector は、局ロゴを、出ている所、消えた所 (CM)、また出た所の順に描いた合成の TS で、
// ロゴの消えた所を CM の区間にすることを確認する。縦が 1080 ラインより大きい映像では、縮めて学習し計測する (#1505)。
// ローカルのパスのないファイル (ライブラリのファイル。#1734) は、Open で開いてパイプで読み、長さは Duration で渡し、ServiceID の番組を読む。
// FFmpeg か ffprobe がなければ飛ばす。
func TestLogoDetector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// zoom は、合成の映像 (640×360) を拡大する倍率。
		zoom int
		// open は、ローカルのパスでなく、Open で渡すかどうか。
		open bool
	}{
		{name: "縮めない映像", zoom: 1, open: false},
		// 2560×1440 は、縦が 1080 ラインより大きいので、半分に縮めて計測する。
		{name: "縮める映像", zoom: 4, open: false},
		{name: "Open で渡すファイル", zoom: 1, open: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ロゴを使える録画では、今の見分け方を呼ばない。
			fallback := NewMockDetector(gomock.NewController(t))
			detector, err := cmdetect.NewLogoDetectorWith(fallback)
			if errors.Is(err, cmdetect.ErrFFmpegNotFound) {
				t.Skip("FFmpeg is not installed")
			}
			require.NoError(t, err)

			path := writeZoomedLogoTS(t, 15, 30, tt.zoom)
			input := &cmdetect.Input{Path: path, ServiceID: 0, Progress: nil, Program: nil, Open: nil, Duration: 0}
			if tt.open {
				// FFmpeg の mpegts の既定の番組 (program_number) は 1。Open で渡すファイルは、この番組の映像と音声を読む。
				input = &cmdetect.Input{
					Path: "library/logo.ts", ServiceID: 1, Progress: nil, Program: nil, Duration: 40 * time.Second,
					Open: func() (io.ReadSeekCloser, error) { return os.Open(path) },
				}
			}
			got, report, err := detector.DetectWithReport(t.Context(), input)
			require.NoError(t, err)
			// ロゴで決められたので、理由は空で、合成の TS には映像の欠けもない (#1508)。
			assert.Equal(t, &cmdetect.Report{Note: "", Missing: 0}, report)
			require.Len(t, got, 1, "%v", got)
			// 境目は、ウィンドウとならしで少し揺れるので、1 秒の内なら同じとみなす。
			assert.InDelta(t, 15, got[0].Start.Seconds(), 1, "%v", got)
			assert.InDelta(t, 30, got[0].End.Seconds(), 1, "%v", got)
		})
	}
}

// TestParseFrameLog は、showinfo のログの先頭と本文の間に -progress のまとまりが入り、時刻の入った本文が
// progress=continue の行の後ろにつながっても、コマの時刻を落とさないことを確認する (#1800)。
func TestParseFrameLog(t *testing.T) {
	t.Parallel()

	stderr := "[Parsed_showinfo_2 @ 0x1] n:   0 pts:      0 pts_time:0       duration:   3003 fmt:gray iskey:1 type:I\n" +
		"[Parsed_showinfo_2 @ 0x1]  color_range:tv\n" +
		// ログの先頭の後ろに -progress のまとまりが入り、本文が progress=continue の行につながった。
		"[Parsed_showinfo_2 @ 0x1] frame=1\n" +
		"fps=0.00\n" +
		"progress=continuen:   1 pts:   3003 pts_time:0.0333667 duration:   3003 fmt:gray iskey:0 type:P\n" +
		"[Parsed_showinfo_2 @ 0x1] n:   2 pts:   6006 pts_time:0.0667333 duration:   3003 fmt:gray iskey:0 type:B\n" +
		"frame=3\n" +
		"progress=end\n"
	times, blocks := cmdetect.ParseFrameLog(stderr)

	assert.Equal(t, []float64{0, 0.0333667, 0.0667333}, times)
	// 本文がつながった progress=continue の行も、まとまりの区切りとして読む。
	assert.Equal(t, 2, blocks)
}

// TestParseFrameLogHugeFrameNumber は、ログに紛れた大きなコマの番号で、長く回らずに読み終えることを確認する。
// 番号が抜けた分は、コマと時刻の数が合わなくなるので、呼ぶ側がロゴを使わずに見分け直す。
func TestParseFrameLogHugeFrameNumber(t *testing.T) {
	t.Parallel()

	times, _ := cmdetect.ParseFrameLog("[Parsed_showinfo_2 @ 0x1] n:   0 pts:      0 pts_time:0 duration:   3003\n" +
		"[Parsed_showinfo_2 @ 0x1] n:9223372036854775806 pts:   3003 pts_time:0.0333667 duration:   3003\n")
	assert.Equal(t, []float64{0, 0.0333667}, times)
}

// writeLogoTS は、局ロゴを、出ている所、hideFrom 秒から hideTo 秒まで消えた所 (CM)、また出た所の順に描いた、40 秒の合成の TS を書く。
//
// 背景は、3 秒の周期で明るさが変わる灰色に、コマごとに変わる雑音を足したもの。縁の向きはコマごとに変わり、明るさの段階には、
// ロゴの出ているコマと消えたコマが混ざる。右上に、白い文字のような形のロゴを描く。
func writeLogoTS(t *testing.T, hideFrom, hideTo int) string {
	t.Helper()

	return writeZoomedLogoTS(t, hideFrom, hideTo, 1)
}

// writeZoomedLogoTS は、writeLogoTS と同じ映像を、縦横を zoom 倍に拡大して書く。
func writeZoomedLogoTS(t *testing.T, hideFrom, hideTo, zoom int) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "logo.ts")
	shown := fmt.Sprintf("enable='lt(t,%d)+gte(t,%d)'", hideFrom, hideTo)
	const logoStrokes = 3
	filters := make([]string, 0, 2+2*logoStrokes)
	filters = append(filters, "geq=lum='120+90*sin(2*PI*T/3)':cb=128:cr=128", "noise=alls=40:allf=t")
	for i := range logoStrokes {
		x := 560 + i*20
		filters = append(filters,
			fmt.Sprintf("drawbox=x=%d:y=20:w=4:h=24:color=white:t=fill:%s", x, shown),
			fmt.Sprintf("drawbox=x=%d:y=30:w=14:h=4:color=white:t=fill:%s", x, shown))
	}
	// 拡大した映像には B フレーム (参照されないコマ) を入れ、縮める映像でデコードしないコマがある並びにする。
	bFrames := "0"
	if zoom > 1 {
		filters = append(filters, fmt.Sprintf("scale=iw*%d:ih*%d:flags=neighbor", zoom, zoom))
		bFrames = "2"
	}
	filter := strings.Join(filters, ",")
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=gray:s=640x360:r=30000/1001", "-t", "40",
		"-vf", filter, "-c:v", "mpeg2video", "-q:v", "4", "-g", "15", "-bf", bFrames, "-f", "mpegts", path)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	return path
}

// TestLogoDetectorRedetectsTooMuchCM は、ロゴで決めた CM の割合が上限以上なら、ロゴの結果を使わずに今の見分け方で見分け直し、
// それも上限以上なら CM なしにすることを確認する (#1508)。
func TestLogoDetectorRedetectsTooMuchCM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fallback []cmdetect.Section
		want     []cmdetect.Section
		// dropped は、見分け直した結果も破棄したことを、理由に書くか。
		dropped bool
	}{
		{
			name:     "uses the redetected sections when they are below the limit",
			fallback: []cmdetect.Section{{Start: 10 * time.Second, End: 25 * time.Second}},
			want:     []cmdetect.Section{{Start: 10 * time.Second, End: 25 * time.Second}},
			dropped:  false,
		},
		{
			name:     "drops the redetected sections when they also reach the limit",
			fallback: []cmdetect.Section{{Start: 5 * time.Second, End: 30 * time.Second}},
			want:     []cmdetect.Section{},
			dropped:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fallback := NewMockDetector(gomock.NewController(t))
			detector, err := cmdetect.NewLogoDetectorWith(fallback)
			if errors.Is(err, cmdetect.ErrFFmpegNotFound) {
				t.Skip("FFmpeg is not installed")
			}
			require.NoError(t, err)
			fallback.EXPECT().Detect(gomock.Any(), gomock.Any()).Return(tt.fallback, nil)

			// ロゴは 40 秒のうち 10 秒から 34 秒まで消えるので、ロゴで決めた CM の割合は 6 割になる。
			got, report, err := detector.DetectWithReport(t.Context(), &cmdetect.Input{Path: writeLogoTS(t, 10, 34), ServiceID: 0, Progress: nil, Program: nil})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			// ロゴの結果を破棄した理由を残す (#1508)。
			assert.Contains(t, report.Note, "of the recording was detected as CM")
			if tt.dropped {
				assert.Contains(t, report.Note, "the redetected result was dropped")
			} else {
				assert.NotContains(t, report.Note, "the redetected result was dropped")
			}
		})
	}
}

// TestLogoDetectorRedetect は、ロゴで見分けた回の材料から判定し直すと、読み直したのと同じ区間と欠けになることを確認する (#1507)。
// 縮めて読み戻した材料でも変わらないことと、ロゴの結果を破棄する録画では、材料だけでは決められないことも確認する。
func TestLogoDetectorRedetect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		hideFrom, hideTo int
		wantRedetectable bool
	}{
		{name: "a recording decided by the logo", hideFrom: 15, hideTo: 30, wantRedetectable: true},
		{name: "a recording whose logo result is dropped", hideFrom: 10, hideTo: 34, wantRedetectable: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fallback := NewMockDetector(gomock.NewController(t))
			detector, err := cmdetect.NewLogoDetectorWith(fallback)
			if errors.Is(err, cmdetect.ErrFFmpegNotFound) {
				t.Skip("FFmpeg is not installed")
			}
			require.NoError(t, err)
			// ロゴで決めるはずの録画でも、今の見分け方に回ったときは、落とす前に理由を出す。Windows でたまに回ることがある (#1800)。
			fallback.EXPECT().Detect(gomock.Any(), gomock.Any()).Return([]cmdetect.Section{}, nil).MaxTimes(1)

			sections, report, material, err := detector.DetectWithMaterial(t.Context(),
				&cmdetect.Input{Path: writeLogoTS(t, tt.hideFrom, tt.hideTo), ServiceID: 0, Progress: nil, Program: nil})
			require.NoError(t, err)
			require.NotNil(t, material)
			if tt.wantRedetectable {
				require.Empty(t, report.Note, "the logo detector fell back to the current detector")
			}

			data, err := cmdetect.EncodeMaterials([]cmdetect.SegmentMaterial{{Path: "logo.ts", Material: material}})
			require.NoError(t, err)
			decoded, err := cmdetect.DecodeMaterials(data)
			require.NoError(t, err)
			for _, m := range []*cmdetect.Material{material, decoded[0].Material} {
				got, gotReport, ok := detector.Redetect(m, nil)
				assert.Equal(t, tt.wantRedetectable, ok)
				if ok {
					assert.Equal(t, sections, got)
					assert.Equal(t, report.Missing, gotReport.Missing)
				}
			}
		})
	}
}

// TestLogoDetectorCountsCMInProgramWindow は、番組のウィンドウを置いたら、CM の割合をウィンドウの中だけで数えることを確認する (#1506)。
// ウィンドウの外は前後の番組なので、遅れて始まった放送で、前の番組が録画の大半を占めても、ロゴの結果を破棄しない。
func TestLogoDetectorCountsCMInProgramWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		program *cmdetect.Program
		// wantRedetectable は、材料だけで決められるか (ロゴの結果を使うか)。
		wantRedetectable bool
	}{
		// 前の 24 秒にロゴがないので、録画全体では CM が 6 割になり、上限を超える。
		{name: "窓がなければ、録画全体で数えて捨てる", program: nil, wantRedetectable: false},
		// 25 秒から 14 秒の番組のウィンドウには、ロゴが出続けているので、ウィンドウの中の CM はない。
		{name: "窓を置いたら、窓の中だけで数える", program: &cmdetect.Program{Start: 25 * time.Second, Duration: 14 * time.Second}, wantRedetectable: true},
	}
	fallback := NewMockDetector(gomock.NewController(t))
	detector, err := cmdetect.NewLogoDetectorWith(fallback)
	if errors.Is(err, cmdetect.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)
	fallback.EXPECT().Detect(gomock.Any(), gomock.Any()).Return([]cmdetect.Section{}, nil).AnyTimes()
	_, _, material, err := detector.DetectWithMaterial(t.Context(),
		&cmdetect.Input{Path: writeLogoTS(t, 0, 24), ServiceID: 0, Progress: nil, Program: nil})
	require.NoError(t, err)
	require.NotNil(t, material)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, _, ok := detector.Redetect(material, tt.program)
			require.Equal(t, tt.wantRedetectable, ok)
			if ok {
				// ウィンドウの前 (前の番組) は CM になり、ウィンドウの中に CM はない。ウィンドウの後ろの残りも、ウィンドウの外なので CM になる。
				require.NotEmpty(t, got)
				assert.Equal(t, time.Duration(0), got[0].Start)
				assert.InDelta(t, 24, got[0].End.Seconds(), 1)
				for _, section := range got[1:] {
					assert.GreaterOrEqual(t, section.Start, 37*time.Second, "no CM inside the program window")
				}
			}
		})
	}
}

// TestLogoDetectorProgramWindows は、セグメントが分かれた録画に、録画全体で番組のウィンドウを置き、セグメントごとに判定し直すことを確認する (#1506)。
// 前のセグメントは、前の番組 (ロゴなし) が 24 秒入り、1 本だけでは CM が上限を超えて破棄されるが、ウィンドウを置けば、ウィンドウの中だけで数える。
// ウィンドウの外にあるセグメントは、ロゴが出ていても、前後の番組としてまるごと CM にする。
func TestLogoDetectorProgramWindows(t *testing.T) {
	t.Parallel()

	fallback := NewMockDetector(gomock.NewController(t))
	detector, err := cmdetect.NewLogoDetectorWith(fallback)
	if errors.Is(err, cmdetect.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)
	fallback.EXPECT().Detect(gomock.Any(), gomock.Any()).Return([]cmdetect.Section{}, nil).AnyTimes()
	measure := func(hideFrom, hideTo int) *cmdetect.Material {
		t.Helper()
		_, _, material, err := detector.DetectWithMaterial(t.Context(),
			&cmdetect.Input{Path: writeLogoTS(t, hideFrom, hideTo), ServiceID: 0, Progress: nil, Program: nil})
		require.NoError(t, err)
		require.NotNil(t, material)

		return material
	}
	// 録画を始めてから、0 秒、40 秒、120 秒に書き始めた 40 秒ずつのセグメント。番組は 25 秒から 44 秒。
	// 1 つ目は 24 秒までロゴがなく、2 つ目は 30 秒からロゴがない。3 つ目は、録画が途切れた後の次の番組で、ロゴが出続けている。
	materials := []*cmdetect.Material{measure(0, 24), measure(30, 40), measure(40, 40)}
	starts := []time.Duration{0, 40 * time.Second, 120 * time.Second}
	_, _, ok := detector.Redetect(materials[0], nil)
	require.False(t, ok, "the first segment alone has too much CM")

	programs, ok := detector.ProgramWindows(materials, starts, &cmdetect.Program{Start: 25 * time.Second, Duration: 44 * time.Second, Fixed: false})
	require.True(t, ok)
	require.Len(t, programs, 3)
	for i, p := range programs {
		assert.True(t, p.Fixed)
		assert.Equal(t, 44*time.Second, p.Duration)
		// ウィンドウは録画全体で、ロゴの出た 24 秒ごろから置く。
		assert.InDelta(t, 24, (starts[i] + p.Start).Seconds(), 1)
	}
	got := make([][]cmdetect.Section, 0, len(materials))
	for i, m := range materials {
		sections, _, ok := detector.Redetect(m, programs[i])
		require.True(t, ok, "segment %d", i+1)
		got = append(got, sections)
	}
	// 1 つ目はウィンドウの前 (前の番組) だけが CM。
	require.Len(t, got[0], 1)
	assert.Equal(t, time.Duration(0), got[0][0].Start)
	assert.InDelta(t, 24, got[0][0].End.Seconds(), 1)
	// 2 つ目はロゴが消えた 30 秒より前のウィンドウの終わり (28 秒ごろ) から後ろが CM。
	require.NotEmpty(t, got[1])
	assert.InDelta(t, 28, got[1][0].Start.Seconds(), 1.5)
	assert.InDelta(t, 40, got[1][len(got[1])-1].End.Seconds(), 1)
	// 3 つ目はウィンドウの外なので、まるごと CM。
	require.Len(t, got[2], 1)
	assert.InDelta(t, 0, got[2][0].Start.Seconds(), 0.5)
	assert.InDelta(t, 40, got[2][0].End.Seconds(), 1)
}

// TestLogoDetectorOutsideWindowWithoutLogo は、ロゴの区間が 1 つもないセグメントでも、録画全体で置いたウィンドウの外にあれば、
// ロゴを使えないとせずに、まるごと CM にすることを確認する (#1506)。
func TestLogoDetectorOutsideWindowWithoutLogo(t *testing.T) {
	t.Parallel()

	fallback := NewMockDetector(gomock.NewController(t))
	detector, err := cmdetect.NewLogoDetectorWith(fallback)
	if errors.Is(err, cmdetect.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)
	// 1 コマ 1 秒で、ロゴが一度も見えない 40 秒のセグメント。
	m := &cmdetect.Material{Fps: 1, Duration: 0, Corner: logo.TopRight, Box: logo.Box{X: 0, Y: 0, W: 0, H: 0}, Times: make([]float64, 40), Measures: make([]logo.Measure, 40)}
	for i := range m.Times {
		m.Times[i] = float64(i)
	}
	_, _, ok := detector.Redetect(m, nil)
	require.False(t, ok, "no logo without a window")

	got, _, ok := detector.Redetect(m, &cmdetect.Program{Start: 60 * time.Second, Duration: 30 * time.Second, Fixed: true})
	require.True(t, ok)
	assert.Equal(t, []cmdetect.Section{{Start: 0, End: 40 * time.Second}}, got)
}

// TestLogoDetectorDropsTooMuchCM は、ロゴを使えずに見分け直した結果も、CM の割合が上限以上なら、CM なしにすることを確認する (#1508)。
func TestLogoDetectorDropsTooMuchCM(t *testing.T) {
	t.Parallel()

	fallback := NewMockDetector(gomock.NewController(t))
	detector, err := cmdetect.NewLogoDetectorWith(fallback)
	if errors.Is(err, cmdetect.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)
	fallback.EXPECT().Detect(gomock.Any(), gomock.Any()).Return([]cmdetect.Section{{Start: 0, End: 900 * time.Millisecond}}, nil)

	got, err := detector.Detect(t.Context(), &cmdetect.Input{Path: "../../../testdata/ts/test-mpeg2-480i-motion.ts", ServiceID: 0, Progress: nil, Program: nil})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestCMRatio は、CM の割合を、映像の欠けを分母からも分子からも外して求めることを確認する (#1508)。
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

// TestTrimMissing は、CM の端にかかった映像の欠けを CM から切り落とし、CM の内側の欠けは残すことを確認する (#1508)。
func TestTrimMissing(t *testing.T) {
	t.Parallel()

	sec := func(from, to int) cmdetect.Section {
		return cmdetect.Section{Start: time.Duration(from) * time.Second, End: time.Duration(to) * time.Second}
	}
	tests := []struct {
		name     string
		sections []cmdetect.Section
		missing  []cmdetect.Section
		want     []cmdetect.Section
	}{
		{name: "no missing video", sections: []cmdetect.Section{sec(10, 20)}, missing: []cmdetect.Section{}, want: []cmdetect.Section{sec(10, 20)}},
		{name: "the start in a gap moves to the end of the gap", sections: []cmdetect.Section{sec(10, 20)}, missing: []cmdetect.Section{sec(8, 12)}, want: []cmdetect.Section{sec(12, 20)}},
		{name: "the end in a gap moves to the start of the gap", sections: []cmdetect.Section{sec(10, 20)}, missing: []cmdetect.Section{sec(18, 25)}, want: []cmdetect.Section{sec(10, 18)}},
		{name: "a gap inside the CM stays in the CM", sections: []cmdetect.Section{sec(10, 20)}, missing: []cmdetect.Section{sec(14, 16)}, want: []cmdetect.Section{sec(10, 20)}},
		{name: "a CM entirely in a gap is dropped", sections: []cmdetect.Section{sec(10, 20), sec(30, 40)}, missing: []cmdetect.Section{sec(5, 25)}, want: []cmdetect.Section{sec(30, 40)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, cmdetect.TrimMissing(tt.sections, tt.missing))
		})
	}
}

// TestEdgeMissing は、ファイルの始まりから最初のコマまでと、最後のコマからファイルの終わりまでの欠けを、1 秒以上のときだけ数えることを確認する (#1508)。
func TestEdgeMissing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		times    []float64
		duration time.Duration
		want     time.Duration
	}{
		{name: "no frames", times: []float64{}, duration: 10 * time.Second, want: 0},
		{name: "short gaps at both ends are not missing", times: []float64{0.5, 1, 9}, duration: 10 * time.Second, want: 0},
		{name: "a long head gap", times: []float64{3, 4, 9}, duration: 10 * time.Second, want: 3 * time.Second},
		{name: "a long tail gap", times: []float64{0, 1, 4}, duration: 10 * time.Second, want: 5 * time.Second},
		{name: "both ends", times: []float64{2, 3, 4}, duration: 10 * time.Second, want: 7 * time.Second},
		{name: "the tail is not counted without the duration", times: []float64{2, 3, 4}, duration: 0, want: 2 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// 1 コマは 1 秒とする。
			assert.Equal(t, tt.want, cmdetect.EdgeMissing(tt.times, 1, tt.duration))
		})
	}
}

// TestLogoDetail は、ロゴの結果を破棄した理由の詳細に、型の在り処と大きさ、ロゴが出ていたコマの割合、点数の分布、
// ロゴで CM とした区間とその中の点数の中央値を書くことを確認する (#1508)。
func TestLogoDetail(t *testing.T) {
	t.Parallel()

	// 10 コマのうち、2〜5 コマ目だけロゴが出ていた。CM は 0〜2 コマ目と 5〜10 コマ目。
	scores := []float64{0.1, 0.0, 0.5, 0.6, 0.3, 0.25, 0.0, 0.1, 0.05, 0.0}
	measures := make([]logo.Measure, 0, len(scores))
	for _, score := range scores {
		measures = append(measures, logo.Measure{Score: score, Brightness: 0, Texture: 0})
	}
	times := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
	got := cmdetect.LogoDetail(logo.TopRight, logo.Box{X: 1700, Y: 40, W: 120, H: 60}, measures, []logo.Interval{{Start: 2, End: 5}}, times, 1)

	assert.Equal(t, "logo at top-right x=1700 y=40 w=120 h=60; logo shown in 30% of the frames; "+
		"scores >=0.40: 20%, 0.20-0.40: 20%, <0.20: 60%; CM by the logo (median score): 0.0-2.0s (0.10) 5.0-10.0s (0.05)", got)
}

// TestLogoDetailLimitsSections は、理由の詳細に書く CM の区間を 10 個までにし、残りを「...」にすることと、
// コマがなければその旨を書くことを確認する (#1508)。
func TestLogoDetailLimitsSections(t *testing.T) {
	t.Parallel()

	// 1 コマおきにロゴが出ているので、CM は 0, 2, 4, ... 番目の 1 コマずつで、12 個ある。
	const frames = 24
	measures := make([]logo.Measure, 0, frames)
	times := make([]float64, 0, frames)
	intervals := []logo.Interval{}
	for i := range frames {
		measures = append(measures, logo.Measure{Score: 0, Brightness: 0, Texture: 0})
		times = append(times, float64(i))
		if i%2 == 1 {
			intervals = append(intervals, logo.Interval{Start: i, End: i + 1})
		}
	}
	got := cmdetect.LogoDetail(logo.TopRight, logo.Box{X: 0, Y: 0, W: 1, H: 1}, measures, intervals, times, 1)
	assert.Equal(t, 10, strings.Count(got, "s (0.00)"), got)
	assert.True(t, strings.HasSuffix(got, " ..."), got)

	assert.Equal(t, "no frame was measured",
		cmdetect.LogoDetail(logo.TopRight, logo.Box{X: 0, Y: 0, W: 1, H: 1}, []logo.Measure{}, []logo.Interval{}, []float64{}, 1))
}

// TestMissingSections は、コマの時刻が 1 秒以上空いた所を、映像の欠けとして返すことを確認する (#1508)。
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

// TestLogoScale は、縦が 1080 ラインより大きい映像を、1080 ライン以下になる整数の倍率で縮めることを確認する (#1505)。
func TestLogoScale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		height int
		want   int
	}{
		{name: "地上波の 1080i は縮めない", height: 1080, want: 1},
		{name: "480i は縮めない", height: 480, want: 1},
		{name: "1080 ラインを 1 ライン超えれば半分にする", height: 1081, want: 2},
		{name: "4K は半分にする", height: 2160, want: 2},
		{name: "8K は 4 分の 1 にする", height: 4320, want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, cmdetect.LogoScale(tt.height))
		})
	}
}

// TestMeasuredRate は、参照されないコマを破棄して間の空いたコマの時刻から、計測したコマの速さを、映像の欠けを除いて求めることを確認する (#1505)。
func TestMeasuredRate(t *testing.T) {
	t.Parallel()

	const fps = 60000.0 / 1001
	// 59.94p のコマを、間を 1 コマと 3 コマで交互に空けて残す (BS4K の HEVC で、参照されないコマを破棄したときの並び)。
	halved := make([]float64, 0, 300)
	for i := 0; i < 600; i += 2 {
		halved = append(halved, float64(i+i%4/2)/fps)
	}
	// 真ん中で 3 秒欠ける。
	withGap := make([]float64, 0, len(halved))
	for i, at := range halved {
		if i >= len(halved)/2 {
			at += 3
		}
		withGap = append(withGap, at)
	}
	tests := []struct {
		name  string
		times []float64
		want  float64
	}{
		{name: "参照されないコマを捨てれば半分の速さになる", times: halved, want: fps / 2},
		{name: "映像の欠けは数えない", times: withGap, want: fps / 2},
		{name: "コマが 1 つなら映像の速さにする", times: []float64{1.5}, want: fps},
		{name: "時刻が進まなければ映像の速さにする", times: []float64{1, 1, 1}, want: fps},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.InEpsilon(t, tt.want, cmdetect.MeasuredRate(tt.times, fps), 0.01)
		})
	}
}

// TestLogoDetectorHeadAfterEarlyWindow は、番組のウィンドウの始まりが番組の時刻の始まりより前に来ても、録画の先頭の CM を
// 番組の時刻の始まりまで延ばせることを確認する (#2168)。遅れて始まった放送の録画では、ウィンドウを番組の終わりのロゴに合わせて置き、
// 前の番組の終わりのロゴを含めて、ウィンドウの始まりが番組の始まりより前に来た。
func TestLogoDetectorHeadAfterEarlyWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		program *cmdetect.Program
	}{
		{
			// 番組表の尺は 210 秒なので、ウィンドウは番組の終わりのロゴに合わせて 80〜290 秒に置かれる。
			name:    "a window placed in the file",
			program: &cmdetect.Program{Start: 120 * time.Second, Duration: 210 * time.Second, Fixed: false, Scheduled: 0},
		},
		{
			// セグメントが分かれた録画で、録画全体で置いたウィンドウ (80〜290 秒) を渡されたセグメント。
			name:    "a window placed over the segments",
			program: &cmdetect.Program{Start: 80 * time.Second, Duration: 210 * time.Second, Fixed: true, Scheduled: 120 * time.Second},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fallback := NewMockDetector(gomock.NewController(t))
			detector, err := cmdetect.NewLogoDetectorWith(fallback)
			if errors.Is(err, cmdetect.ErrFFmpegNotFound) {
				t.Skip("FFmpeg is not installed")
			}
			require.NoError(t, err)
			// 1 コマ 1 秒で、300 秒の録画。前の番組の終わりのロゴが 70〜90 秒に、番組のロゴが 120〜290 秒に出る。番組は 120 秒に始まる。
			m := &cmdetect.Material{Fps: 1, Duration: 0, Corner: logo.TopRight, Box: logo.Box{X: 0, Y: 0, W: 0, H: 0}, Times: make([]float64, 300), Measures: make([]logo.Measure, 300)}
			for i := range m.Times {
				m.Times[i] = float64(i)
				if (i >= 70 && i < 90) || (i >= 120 && i < 290) {
					m.Measures[i].Score = 0.7
				}
			}
			cmdetect.SetMaterialBounds(m, []float64{120, 290}, 300, 0)
			got, _, ok := detector.Redetect(m, tt.program)
			require.True(t, ok)
			require.NotEmpty(t, got)
			assert.Equal(t, cmdetect.Section{Start: 0, End: 120 * time.Second}, got[0])
		})
	}
}
