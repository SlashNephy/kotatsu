package logo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

const fps = 30.0

// scene は、同じ測りが続く場面。
type scene struct {
	seconds float64
	m       logo.Measure
}

func measures(scenes ...scene) []logo.Measure {
	out := []logo.Measure{}
	for _, s := range scenes {
		for range int(s.seconds * fps) {
			out = append(out, s.m)
		}
	}

	return out
}

// TestIntervals は、ロゴが出ていた区間を、白っぽい背景で見えない間は持ち越し、真っ黒では持ち越さずに出すことを確かめる。
func TestIntervals(t *testing.T) {
	t.Parallel()

	// shown は、暗い背景でロゴが出ているコマ。
	shown := logo.Measure{Score: 0.7, Brightness: 90, Texture: 20}
	// hidden は、CM の間 (ロゴがない) のコマ。明るさと肌理は本編と同じ段にする。CM だけの段があると、その段は判断できない段になる。
	hidden := logo.Measure{Score: 0.0, Brightness: 90, Texture: 20}
	// washed は、白っぽい背景で、ロゴが出ているのに見えないコマ。
	washed := logo.Measure{Score: 0.02, Brightness: 245, Texture: 6}
	// blackout は、番組の切れ目の真っ黒 (局がロゴごと消す)。
	blackout := logo.Measure{Score: 0.0, Brightness: 16, Texture: 0.5}

	tests := []struct {
		name   string
		scenes []scene
		want   []logo.Interval
	}{
		{
			name:   "本編、CM、本編",
			scenes: []scene{{60, shown}, {30, hidden}, {60, shown}},
			want:   []logo.Interval{{Start: 0, End: 1800}, {Start: 2700, End: 4500}},
		},
		{
			// 白っぽい場面は、本編の中にしか出ないので、近いコマだけで数えた明るさの段で、判断できない段になる。
			name:   "本編の中の白っぽい場面は、ロゴが見えなくても持ち越す",
			scenes: []scene{{60, shown}, {20, washed}, {60, shown}, {30, hidden}, {60, shown}},
			want:   []logo.Interval{{Start: 0, End: 4200}, {Start: 5100, End: 6900}},
		},
		{
			name:   "真っ黒の切れ目は持ち越さず、そこで区間を切る",
			scenes: []scene{{60, shown}, {10, blackout}, {30, hidden}, {60, shown}},
			want:   []logo.Interval{{Start: 0, End: 1800}, {Start: 3000, End: 4800}},
		},
		{
			name:   "短い途切れは埋め、短い区間は捨てる",
			scenes: []scene{{60, shown}, {1, hidden}, {60, shown}, {30, hidden}, {3, shown}, {30, hidden}},
			want:   []logo.Interval{{Start: 0, End: 3630}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := logo.Intervals(measures(tt.scenes...), fps, logo.DefaultParams())
			// 境目は、窓とならしで数コマ揺れるので、0.5 秒の内なら同じとみなす。
			if !assert.Len(t, got, len(tt.want), "%v", got) {
				return
			}
			for i := range got {
				assert.InDelta(t, tt.want[i].Start, got[i].Start, fps/2, "start of %d: %v", i, got)
				assert.InDelta(t, tt.want[i].End, got[i].End, fps/2, "end of %d: %v", i, got)
			}
		})
	}
}
