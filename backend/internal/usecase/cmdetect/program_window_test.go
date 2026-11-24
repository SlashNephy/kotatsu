package cmdetect_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

// TestProgramWindow は、番組の尺のウィンドウを、ロゴの区間に合わせ、番組が始まるはずの時刻に近い所に置くことを確認する (#1506 の手順 1)。
func TestProgramWindow(t *testing.T) {
	t.Parallel()

	// 1 コマ 1 秒で、100 秒の録画。番組は 80 秒。
	times := make([]float64, 100)
	for i := range times {
		times[i] = float64(i)
	}
	program := func(start int) *cmdetect.Program {
		return &cmdetect.Program{Start: time.Duration(start) * time.Second, Duration: 80 * time.Second}
	}
	tests := []struct {
		name      string
		intervals []logo.Interval
		program   *cmdetect.Program
		wantStart float64
		wantOK    bool
	}{
		{
			// 前の番組のロゴが頭の 10 秒に、次の番組のロゴが尻の 10 秒に入っている。番組のロゴは 12〜88 秒。
			name:      "the window starts at the program logo near the expected start",
			intervals: []logo.Interval{{Start: 0, End: 8}, {Start: 12, End: 88}, {Start: 92, End: 100}},
			program:   program(10), wantStart: 12, wantOK: true,
		},
		{
			// 番組が 30 秒遅れて始まった。ずらした分だけロゴが多く入るので、遅れた位置に置く。
			name:      "a late program moves the window",
			intervals: []logo.Interval{{Start: 0, End: 5}, {Start: 20, End: 100}},
			program:   program(0), wantStart: 20, wantOK: true,
		},
		{
			name:      "no window when the program is as long as the recording",
			intervals: []logo.Interval{{Start: 10, End: 90}},
			program:   &cmdetect.Program{Start: 0, Duration: 100 * time.Second}, wantStart: 0, wantOK: false,
		},
		{
			// 区間がファイルの端にしかなく、候補が作れないときは、番組の先頭を削らないよう、ウィンドウを置かない。
			name:      "no window without candidates from the logo",
			intervals: []logo.Interval{{Start: 0, End: 100}},
			program:   program(10), wantStart: 0, wantOK: false,
		},
		{
			name:      "no window without the program",
			intervals: []logo.Interval{{Start: 12, End: 88}},
			program:   nil, wantStart: 0, wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			start, end, ok := cmdetect.ProgramWindow(tt.intervals, times, 1, tt.program)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.InDelta(t, tt.wantStart, start, 1e-9)
				assert.InDelta(t, tt.wantStart+80, end, 1e-9)
			}
		})
	}
}

// TestClipIntervals は、ウィンドウの外のロゴを破棄し、ウィンドウにかかった区間をウィンドウの端で切ることを確認する (#1506 の手順 1)。
func TestClipIntervals(t *testing.T) {
	t.Parallel()

	times := make([]float64, 100)
	for i := range times {
		times[i] = 10 + float64(i)
	}
	got := cmdetect.ClipIntervals([]logo.Interval{{Start: 0, End: 8}, {Start: 10, End: 50}, {Start: 60, End: 95}, {Start: 96, End: 100}}, times, 6, 90)
	assert.Equal(t, []logo.Interval{{Start: 6, End: 8}, {Start: 10, End: 50}, {Start: 60, End: 90}}, got)
}

// TestDropShortSections は、15 秒の CM になりえない短い区間を、ファイルの端に接しているか、ほかの CM に続いていなければ破棄することを確認する (#1506)。
func TestDropShortSections(t *testing.T) {
	t.Parallel()

	sec := func(from, to float64) cmdetect.Section {
		return cmdetect.Section{Start: time.Duration(from * float64(time.Second)), End: time.Duration(to * float64(time.Second))}
	}
	got := cmdetect.DropShortSections([]cmdetect.Section{
		sec(0, 5),       // 先頭に接する区間は、短くても残す。
		sec(100, 106),   // 本編の中の孤立した 6 秒は破棄する。
		sec(200, 350),   // 150 秒の CM は残す。
		sec(351, 361),   // CM に続く 10 秒 (提供や告知) は残す。
		sec(362, 367),   // それにさらに続く 5 秒も残す。
		sec(500, 513.9), // 15 秒に 1 秒以上足りない区間は破棄する。
		sec(600, 614),   // 15 秒から 1 秒のずれの内なら残す。
		sec(1795, 1800), // 末尾に接する区間は残す。
	}, 1800*time.Second)

	assert.Equal(t, []cmdetect.Section{
		sec(0, 5), sec(200, 350), sec(351, 361), sec(362, 367), sec(600, 614), sec(1795, 1800),
	}, got)
}

// TestProgramEdges は、ロゴを使えずに今の見分け方に回ったとき、番組の時刻に近い無音の境目より前と後を、前後の番組として CM にすることを確認する (#1506)。
func TestProgramEdges(t *testing.T) {
	t.Parallel()

	// 録画は 1817 秒。番組は、ファイルの始まりから 10 秒後に始まる 30 分。時刻の差は -0.27 秒。
	program := &cmdetect.Program{Start: 10 * time.Second, Duration: 30 * time.Minute}
	bounds := []float64{12.0, 660.6, 1221.0, 1311.0, 1620.0, 1740.0, 1811.9}
	got := cmdetect.ProgramEdges([][2]float64{{1221, 1311}, {1620, 1740}}, bounds, 1817.2, program, -0.27)
	assert.Equal(t, [][2]float64{{0, 12}, {1221, 1311}, {1620, 1740}, {1811.9, 1817.2}}, got)

	// 番組の時刻の近くに境目がなければ、そのままにする。
	got = cmdetect.ProgramEdges([][2]float64{{1221, 1311}}, []float64{660.6, 1221, 1311}, 1817.2, program, -0.27)
	assert.Equal(t, [][2]float64{{1221, 1311}}, got)
}

// TestSnapWindow は、ウィンドウの端を、寄せる距離 (3 秒) 以内で最も近い無音の境目に寄せ、切れ目のない境目は 1 秒遠いとみなすことを確認する (#1506)。
func TestSnapWindow(t *testing.T) {
	t.Parallel()

	// 境目の時刻は、今の見分け方の基準 (コマの時刻 + offset)。ここでは最初のコマの時刻を 2、offset を 0.5 にし、ウィンドウの秒に 2.5 を足して書く。
	// 録画の長さも同じ基準で、ウィンドウの秒の 1501 に当たる。
	const origin, offset = 2.0, 0.5
	at := func(windowSeconds float64) float64 { return windowSeconds + origin + offset }
	duration := at(1501)
	tests := []struct {
		name               string
		start, end         float64
		bounds             [][2]float64
		wantStart, wantEnd float64
	}{
		{name: "境目がなければ動かさない", start: 60, end: 1500, bounds: [][2]float64{}, wantStart: 60, wantEnd: 1500},
		{
			name:  "寄せる距離の内の境目に寄せる",
			start: 60, end: 1500,
			bounds:    [][2]float64{{at(62), 1}, {at(1498.5), 1}},
			wantStart: 62, wantEnd: 1498.5,
		},
		{
			name:  "寄せる距離の外の境目には寄せない",
			start: 60, end: 1500,
			bounds:    [][2]float64{{at(56), 1}, {at(1504), 1}},
			wantStart: 60, wantEnd: 1500,
		},
		{
			name:  "切れ目のない境目より、少し遠い切れ目のある境目を選ぶ",
			start: 60, end: 1500,
			bounds:    [][2]float64{{at(60.5), 0}, {at(61.2), 1}},
			wantStart: 61.2, wantEnd: 1500,
		},
		{
			name:  "切れ目のある境目が十分に遠ければ、近い切れ目のない境目を選ぶ",
			start: 60, end: 1500,
			bounds:    [][2]float64{{at(60.2), 0}, {at(62.5), 1}},
			wantStart: 60.2, wantEnd: 1500,
		},
		{
			name:  "ファイルの始まりより前の境目には寄せない",
			start: 1, end: 1500,
			bounds:    [][2]float64{{at(-1), 1}},
			wantStart: 1, wantEnd: 1500,
		},
		{
			name:  "録画の端にある境目には寄せない",
			start: 1, end: 1500,
			bounds:    [][2]float64{{0, 1}, {duration, 1}},
			wantStart: 1, wantEnd: 1500,
		},
		{
			name:  "始まりは、窓の終わりより後の境目に寄せない",
			start: 10, end: 12,
			bounds:    [][2]float64{{at(13), 1}},
			wantStart: 10, wantEnd: 13,
		},
		{
			name:  "終わりは、寄せた始まりより前の境目に寄せない",
			start: 10, end: 12,
			bounds:    [][2]float64{{at(11.5), 1}},
			wantStart: 11.5, wantEnd: 12,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotStart, gotEnd := cmdetect.SnapWindow(tt.start, tt.end, tt.bounds, duration, origin, offset)
			assert.InDelta(t, tt.wantStart, gotStart, 1e-9)
			assert.InDelta(t, tt.wantEnd, gotEnd, 1e-9)
		})
	}
}
