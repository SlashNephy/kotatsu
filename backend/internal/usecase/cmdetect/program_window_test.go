package cmdetect_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

// TestProgramWindow は、番組の尺の窓を、ロゴの区間に合わせ、番組が始まるはずの時刻に近い所に置くことを確かめる (#1506 の手順 1)。
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
			// 区間がファイルの端にしかなく、候補が作れないときは、番組の頭を削らないよう、窓を置かない。
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

// TestClipIntervals は、窓の外のロゴを捨て、窓にかかった区間を窓の端で切ることを確かめる (#1506 の手順 1)。
func TestClipIntervals(t *testing.T) {
	t.Parallel()

	times := make([]float64, 100)
	for i := range times {
		times[i] = 10 + float64(i)
	}
	got := cmdetect.ClipIntervals([]logo.Interval{{Start: 0, End: 8}, {Start: 10, End: 50}, {Start: 60, End: 95}, {Start: 96, End: 100}}, times, 6, 90)
	assert.Equal(t, []logo.Interval{{Start: 6, End: 8}, {Start: 10, End: 50}, {Start: 60, End: 90}}, got)
}

// TestDropShortSections は、15 秒の CM になりえない短い区間を、ファイルの端に接しているか、ほかの CM に続いていなければ捨てることを確かめる (#1506)。
func TestDropShortSections(t *testing.T) {
	t.Parallel()

	sec := func(from, to float64) cmdetect.Section {
		return cmdetect.Section{Start: time.Duration(from * float64(time.Second)), End: time.Duration(to * float64(time.Second))}
	}
	got := cmdetect.DropShortSections([]cmdetect.Section{
		sec(0, 5),       // 先頭に接する区間は、短くても残す。
		sec(100, 106),   // 本編の中の孤立した 6 秒は捨てる。
		sec(200, 350),   // 150 秒の CM は残す。
		sec(351, 361),   // CM に続く 10 秒 (提供や告知) は残す。
		sec(362, 367),   // それにさらに続く 5 秒も残す。
		sec(500, 513.9), // 15 秒に 1 秒以上足りない区間は捨てる。
		sec(600, 614),   // 15 秒から 1 秒のずれの内なら残す。
		sec(1795, 1800), // 末尾に接する区間は残す。
	}, 1800*time.Second)

	assert.Equal(t, []cmdetect.Section{
		sec(0, 5), sec(200, 350), sec(351, 361), sec(362, 367), sec(600, 614), sec(1795, 1800),
	}, got)
}
