package cmdetect_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
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

// TestSignalOffset は、局ロゴの見分け方の時刻を今の見分け方の基準にそろえる量を、PTS が一周しても正しく求めることを確かめる (#1506)。
func TestSignalOffset(t *testing.T) {
	t.Parallel()

	const wrap = int64(1) << 33
	tests := []struct {
		name          string
		start, origin int64
		want          float64
	}{
		// ファイルの始まり (音声の最初のパケット) は、最初の映像より 0.268 秒前。
		{name: "the file starts before the first video", start: 4099328151, origin: 4099352259, want: -0.26786666666666664},
		// ファイルの始まりの後で PTS が一周し、最初の映像の PTS が小さくなった。
		{name: "the PTS wraps between the start and the first video", start: wrap - 9000, origin: 9000, want: -0.2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.InDelta(t, tt.want, cmdetect.SignalOffset(tt.start, tt.origin), 1e-9)
		})
	}
}

// TestCombineSectionsNotice は、告知 (5 秒) を、その後に CM が続くときだけ CM の側に数えることを確かめる (#1506)。
func TestCombineSectionsNotice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		bounds []float64
		shown  [][2]float64
		want   [][2]float64
	}{
		{
			// 本編の直後の 5 秒の告知 (このあとすぐ、など) の後に、15 秒の CM が続く。
			name:   "a notice before CMs is a CM",
			bounds: []float64{300, 305, 320, 335},
			shown:  [][2]float64{{0, 300}, {335.4, 600}},
			want:   [][2]float64{{300, 335}},
		},
		{
			// 本編の間の、無音に挟まれたロゴのない 5 秒は、告知としては認めず、尺の合わない CM と同じ扱いにする (仕様どおり)。
			// ロゴなしの重みを尺違いの減点と釣り合わせているので、3 秒より長いロゴのない所は、CM になる。
			name:   "a notice alone between the program parts is treated like a CM that does not fit",
			bounds: []float64{300, 305},
			shown:  [][2]float64{{0, 300}, {305, 600}},
			want:   [][2]float64{{300, 305}},
		},
		{
			// ロゴが少し見える 5 秒が 2 つ続いても、告知を続けて CM に入れ、本編へ戻るときの減点を避けることはできない。
			name:   "two notices in a row do not skip the penalty for returning to the program",
			bounds: []float64{300, 305, 310},
			shown:  [][2]float64{{0, 301.5}, {305, 306.5}, {310, 600}},
			want:   [][2]float64{},
		},
		{
			// 15 秒の CM の中に、10 秒の所で境目の候補があっても、頭の 10 秒を提供に取らない。
			name:   "a sponsor and a notice do not take the head of a 15-second CM",
			bounds: []float64{300, 310, 315},
			shown:  [][2]float64{{0, 300}, {315, 600}},
			want:   [][2]float64{{300, 315}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, cmdetect.CombineSections(tt.bounds, 600, tt.shown))
		})
	}
}

// TestLogoInCMChains は、無音の境目が 15 秒の倍数で続く並びの間もロゴが出続けていれば、ロゴでないものを覚えたとみなすことを確かめる (#1506)。
func TestLogoInCMChains(t *testing.T) {
	t.Parallel()

	// 無音の境目が 100 秒から 15 秒ごとに 4 本続く (60 秒の CM の並び)。
	bounds := []float64{100, 115, 130, 145, 160}
	tests := []struct {
		name      string
		intervals []logo.Interval
		wantOK    bool
	}{
		{name: "the logo disappears in the CMs", intervals: []logo.Interval{{Start: 0, End: 100}, {Start: 160, End: 600}}, wantOK: false},
		// CM の間も時刻の表示のように出続けるものを覚えた。
		{name: "the logo stays in the CMs", intervals: []logo.Interval{{Start: 0, End: 600}}, wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, ok := cmdetect.LogoInCMChains(bounds, 600, tt.intervals)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}
