package cmdetect_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
)

// recordingLength は、テストの録画の長さ (秒)。
const recordingLength = 1800

// cut は、時刻 at を境目とする、CM の前後と同じ 1 秒の無音と、その中のシーンの切り替わりを足す。
func cut(s *cmdetect.Signals, at ...float64) *cmdetect.Signals {
	for _, t := range at {
		s.Silences = append(s.Silences, [2]float64{t - 0.5, t + 0.5})
		s.Scenes = append(s.Scenes, t+0.1)
	}

	return s
}

// pause は、時刻 at に、シーンの切り替わりのない 1 秒の無音を足す。
func pause(s *cmdetect.Signals, at ...float64) *cmdetect.Signals {
	for _, t := range at {
		s.Silences = append(s.Silences, [2]float64{t - 0.5, t + 0.5})
	}

	return s
}

// signals は、長さ recordingLength 秒の、手掛かりのない Signals を返す。
func signals() *cmdetect.Signals {
	return &cmdetect.Signals{Silences: [][2]float64{}, Scenes: []float64{}, AudioChanges: []float64{}, Duration: recordingLength}
}

func TestFindSections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sig  *cmdetect.Signals
		want [][2]float64
	}{
		{
			name: "15 秒の倍数の間隔で無音が続くところを CM とみなす",
			sig:  cut(signals(), 600, 615, 645, 660, 690),
			want: [][2]float64{{600, 690}},
		},
		{
			name: "CM の長さには、境目の位置のずれを認める",
			sig:  cut(signals(), 600, 615.3, 629.9),
			want: [][2]float64{{600, 629.9}},
		},
		{
			name: "1 本だけの間隔は、本編の切れ目と見分けにくいので CM とみなさない",
			sig:  cut(signals(), 600, 615),
			want: [][2]float64{},
		},
		{
			name: "15 秒の倍数でない間隔の無音は CM とみなさない",
			sig:  cut(signals(), 600, 612, 631, 649),
			want: [][2]float64{},
		},
		{
			name: "60 秒より長い間隔は、本編のオープニングやエンディングと見分けにくいので CM とみなさない",
			sig:  cut(signals(), 600, 690, 780),
			want: [][2]float64{},
		},
		{
			name: "両端でシーンが切り替わっていれば、10 秒の CM も続きとみなす",
			sig:  cut(signals(), 600, 615, 625, 640),
			want: [][2]float64{{600, 640}},
		},
		{
			name: "シーンが切り替わっていない無音の間の 10 秒は、CM とみなさない",
			sig:  pause(cut(signals(), 600, 615, 640), 625),
			want: [][2]float64{},
		},
		{
			name: "並びの端の短い間隔は、本編の終わりのアイキャッチや次回予告の題名かもしれないので、CM に含めない",
			sig:  cut(signals(), 595, 600, 615, 630, 640),
			want: [][2]float64{{600, 630}},
		},
		{
			name: "録画の始まりから 15.5 秒以内に始まる CM は、録画の始まりまで広げる",
			sig:  cut(signals(), 10, 25, 40),
			want: [][2]float64{{0, 40}},
		},
		{
			name: "録画の終わりから 15.5 秒以内で終わる CM は、録画の終わりまで広げる",
			sig:  cut(signals(), 1755, 1770, 1785),
			want: [][2]float64{{1755, recordingLength}},
		},
		{
			name: "近い無音は 1 つの境目にまとめる",
			sig:  pause(cut(signals(), 600, 615, 630), 615.6),
			want: [][2]float64{{600, 630}},
		},
		{
			name: "短い無音は、本編の台詞の間なので境目にしない",
			sig: &cmdetect.Signals{
				Silences: [][2]float64{{599.9, 600.1}, {614.9, 615.1}, {629.9, 630.1}}, Scenes: []float64{600, 615, 630},
				AudioChanges: []float64{}, Duration: recordingLength,
			},
			want: [][2]float64{},
		},
		{
			name: "音声の構成が変わった時刻も、境目の候補にする",
			sig:  &cmdetect.Signals{Silences: [][2]float64{}, Scenes: []float64{}, AudioChanges: []float64{600, 630, 660}, Duration: recordingLength},
			want: [][2]float64{{600, 660}},
		},
		{
			// 100 から 145.1 までは 45 秒の 1 本とみなせるが、CM が 1 本だけの並びで、続く 2 本の並びを上書きしない。
			name: "より長くても CM が 1 本だけの並びは、CM が 2 本以上続く並びを上書きしない",
			sig:  cut(signals(), 100, 115.6, 130.6, 145.1),
			want: [][2]float64{{115.6, 145.1}},
		},
		{
			name: "離れた CM の区間は、それぞれ見分ける",
			sig:  cut(signals(), 300, 315, 330, 900, 930, 960),
			want: [][2]float64{{300, 330}, {900, 960}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cmdetect.FindSections(tt.sig)
			assert.Len(t, got, len(tt.want))
			for i := range min(len(got), len(tt.want)) {
				assert.InDelta(t, tt.want[i][0], got[i][0], 0.001)
				assert.InDelta(t, tt.want[i][1], got[i][1], 0.001)
			}
		})
	}
}
