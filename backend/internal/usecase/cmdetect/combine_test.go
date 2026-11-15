package cmdetect_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
)

// TestCombineSections は、無音の境目の候補と局ロゴの区間から、本編と CM の並びを選ぶことを確かめる (#1506)。
// どの場合も、録画は 600 秒で、境目の候補は無音の真ん中にある。
func TestCombineSections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		bounds []float64
		shown  [][2]float64
		want   [][2]float64
	}{
		{
			// ロゴの消えた所が、無音の境目より少しずれていても、CM の境目は無音にそろう。
			name:   "a chain of 15-second CMs is cut at the silences",
			bounds: []float64{200, 215, 230, 245},
			shown:  [][2]float64{{0, 199.5}, {245.8, 600}},
			want:   [][2]float64{{200, 245}},
		},
		{
			// 本編の中で、明るい場面でロゴが数秒見えなくなっても、境目の候補がなく 15 秒の倍数にも合わなければ、本編に残す。
			name:   "a short logo gap inside the program stays in the program",
			bounds: []float64{100, 300},
			shown:  [][2]float64{{0, 150}, {156, 600}},
			want:   [][2]float64{},
		},
		{
			// 録画の頭の CM は、長さを問わず CM にする。ロゴのない 15 秒の CM が続いても、ひとまとめにする。
			name:   "the CMs at the start of the recording",
			bounds: []float64{12, 56.8, 70.8},
			shown:  [][2]float64{{71.5, 600}},
			want:   [][2]float64{{0, 70.8}},
		},
		{
			// 本編の直後の 10 秒前後の提供 (ロゴなし) は、次が CM なら、本編の側に数える。
			name:   "a sponsor card after the program is not a CM",
			bounds: []float64{194.7, 204.9, 219.9, 234.9},
			shown:  [][2]float64{{0, 194.5}, {235.3, 600}},
			want:   [][2]float64{{204.9, 234.9}},
		},
		{
			// 無音に挟まれた、許すロゴの途切れ (3 秒) より長いロゴのない所は、15 秒の倍数に合わなくても CM になる。
			// ロゴなしの重みを、尺違いの減点と釣り合わせているためで、番組のオープニングで局がロゴを消すと CM になる既知の弱点と同じ。
			name:   "a long logo-less part between silences becomes a CM even when it does not fit 15 seconds",
			bounds: []float64{100, 137},
			shown:  [][2]float64{{0, 100}, {137, 600}},
			want:   [][2]float64{{100, 137}},
		},
		{
			// 無音に挟まれていても、許すロゴの途切れより短いロゴのない所 (アイキャッチ) は、本編に残す。
			name:   "an eyecatch shorter than the allowed logo gap stays in the program",
			bounds: []float64{100, 102.5},
			shown:  [][2]float64{{0, 100}, {102.5, 600}},
			want:   [][2]float64{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, cmdetect.CombineSections(tt.bounds, 600, tt.shown))
		})
	}
}
