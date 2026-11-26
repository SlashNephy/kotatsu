package cmdetect_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

// TestCombineSections は、無音の境目の候補と局ロゴの区間から、本編と CM の並びを選ぶことを確認する (#1506)。
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
			// 録画の先頭の CM は、長さを問わず CM にする。ロゴのない 15 秒の CM が続いても、ひとまとめにする。
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
		{
			// 本編の間の、無音に挟まれた 120 秒の通販の CM は、CM の最長 (60 秒) を超えても CM にする (#1958)。
			name:   "a 120-second infomercial between the program parts is a CM",
			bounds: []float64{200, 320},
			shown:  [][2]float64{{0, 200}, {320, 600}},
			want:   [][2]float64{{200, 320}},
		},
		{
			// 通販の電話番号の文字が局ロゴの隅に重なり、頭の 19 秒をロゴとみなしても、長い CM にする (#1958)。
			name:   "a long CM is a CM even when its first seconds are taken as the logo",
			bounds: []float64{200, 320},
			shown:  [][2]float64{{0, 219.4}, {320, 600}},
			want:   [][2]float64{{200, 320}},
		},
		{
			// 局がロゴを消した 90 秒ほどのオープニングは、15 秒の倍数から 0.5 秒以上ずれていれば、長い CM にしない (#1958)。
			name:   "a logo-less opening that does not fit 15 seconds closely stays in the program",
			bounds: []float64{100, 189.4},
			shown:  [][2]float64{{0, 100}, {189.4, 600}},
			want:   [][2]float64{},
		},
		{
			// 長い CM の最長 (180 秒) を超える、ロゴのない所は、長い CM にしない。
			name:   "a logo-less part longer than 180 seconds is not a long CM",
			bounds: []float64{100, 310},
			shown:  [][2]float64{{0, 100}, {310, 600}},
			want:   [][2]float64{},
		},
		{
			// 録画の終わりから 15.5 秒の内で終わる CM は、終わりまで延ばす。番組のウィンドウを置けない録画で、次の番組の先頭のロゴを本編にしない (#1958)。
			name:   "a CM that ends near the end of the recording is extended to the end",
			bounds: []float64{470, 590},
			shown:  [][2]float64{{0, 470}, {590, 600}},
			want:   [][2]float64{{470, 600}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, cmdetect.CombineSections(tt.bounds, 600, tt.shown))
		})
	}
}

// TestSignalOffset は、局ロゴの見分け方の時刻を今の見分け方の基準にそろえる量を、PTS が一周しても正しく求めることを確認する (#1506)。
func TestSignalOffset(t *testing.T) {
	t.Parallel()

	const wrap = int64(1) << 33
	tests := []struct {
		name          string
		first, origin int64
		firstTime     float64
		want          float64
	}{
		// 本番の録画: 最初の映像の PES は、最初のコマ (I フレーム) より 0.467 秒前。FFmpeg は最初のコマに 0.567 秒を付けた。
		// コマの時刻 t の所は、最初の映像の PES から t − 0.567 + 0.467 秒になる。
		{name: "the first video PES is before the first frame", first: 6830340175, origin: 6830298175, firstTime: 0.567, want: 0.46666666666666673 - 0.567},
		// 最初の映像の PES の後で PTS が一周し、最初のコマの PTS が小さくなった。
		{name: "the PTS wraps between the first video PES and the first frame", first: 9000, origin: wrap - 9000, firstTime: 0.1, want: 0.1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.InDelta(t, tt.want, cmdetect.SignalOffset(tt.first, tt.origin, tt.firstTime), 1e-9)
		})
	}
}

// TestToPlayback は、局ロゴの見分け方の区間 (最初のコマからの時間) を、再生の時間 (録画のインデックスの最初の I フレームが 0) に直すことを確認する (#1811)。
func TestToPlayback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		firstTime  float64
		offset     float64
		withBounds bool
		sections   [][2]float64
		want       [][2]float64
	}{
		{
			// 本番の録画: FFmpeg がデコードした最初のコマは、インデックスの最初の I フレームより 0.5 秒前。
			// コマの時刻は 0.1335 秒から始まり、今の見分け方の基準 (インデックスの最初の I フレーム) への差は -0.634 秒。
			name: "the first frame is before the first indexed I frame", firstTime: 0.1335, offset: -0.634, withBounds: true,
			sections: [][2]float64{{0.2, 1}, {60, 75}},
			want:     [][2]float64{{0, 0.4995}, {59.4995, 74.4995}},
		},
		{
			name: "the first frame is the first indexed I frame", firstTime: 0.567, offset: -0.567, withBounds: true,
			sections: [][2]float64{{60, 75}},
			want:     [][2]float64{{60, 75}},
		},
		{
			// 今の見分け方の材料を読めなかった回は、基準が分からないので、ずらさない。
			name: "without the bounds", firstTime: 0.1335, offset: 0, withBounds: false,
			sections: [][2]float64{{60, 75}},
			want:     [][2]float64{{60, 75}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cmdetect.ToPlayback(tt.firstTime, tt.offset, tt.withBounds, tt.sections)
			require.Len(t, got, len(tt.want))
			for i := range got {
				assert.InDelta(t, tt.want[i][0], got[i][0], 1e-6)
				assert.InDelta(t, tt.want[i][1], got[i][1], 1e-6)
			}
		})
	}
}

// TestCombineSectionsNotice は、告知 (5 秒) を、その後に CM が続くときだけ CM の側に数えることを確認する (#1506)。
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
		{
			// 提供が 2 枚続いて 15.5 秒になっても、本編の側に数える (本当の CM は、ほぼちょうど 15 秒)。
			name:   "two sponsor cards in a row are not a CM",
			bounds: []float64{300, 315.5, 330.5, 345.5},
			shown:  [][2]float64{{0, 300}, {345.8, 600}},
			want:   [][2]float64{{315.5, 345.5}},
		},
		{
			// 本番の録画の並び: ロゴの出た本編の終わり (290〜300) の後に、提供 2 枚 (15.5 秒)、見逃し配信の案内 (3.9 秒) が続き、
			// 5.8 秒の番宣から CM に入る。ロゴの出た本編の終わりを提供に取らず、提供を本編の側に、案内からを CM にする。
			name:   "a sponsor after the program with the logo, followed by a notice before CMs",
			bounds: []float64{290, 300, 315.5, 319.4, 325.2, 340.2, 355.2, 370.2, 385.2, 400.2},
			shown:  [][2]float64{{0, 299.8}, {400.5, 600}},
			want:   [][2]float64{{315.5, 400.2}},
		},
		{
			// 本番の録画の並び: 本編のロゴが、境目の候補のない所 (300) で消え、10 秒後の境目から 15.3 秒の CM が続く。
			// ロゴのない所の後の区間は、提供にせず、CM にする。
			name:   "a CM after a part without the logo is not a sponsor",
			bounds: []float64{290, 310.2, 325.5, 340.5},
			shown:  [][2]float64{{0, 300}, {340.8, 600}},
			want:   [][2]float64{{310.2, 340.5}},
		},
		{
			// 本編の直後でも、ちょうど 15 秒の区間は CM にする。
			name:   "a 15-second CM right after the program is a CM",
			bounds: []float64{300, 315, 330},
			shown:  [][2]float64{{0, 300}, {330.4, 600}},
			want:   [][2]float64{{300, 330}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, cmdetect.CombineSections(tt.bounds, 600, tt.shown))
		})
	}
}

// TestCombineSectionsPromo は、局ロゴを出したままの番宣とロゴのない提供の扱いを確認する (#2168)。
func TestCombineSectionsPromo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		bounds []float64
		shown  [][2]float64
		want   [][2]float64
	}{
		{
			// CM の並びの後、長い本編に戻る前の、ロゴを出したままの 15 秒の番宣は、CM にする。
			name:   "a promo with the logo after CMs and before a long program part is a CM",
			bounds: []float64{200, 215, 230, 244.96},
			shown:  [][2]float64{{0, 200}, {230, 600}},
			want:   [][2]float64{{200, 244.96}},
		},
		{
			// 長い本編の後、CM の並びの前の、ロゴを出したままの 30 秒の番宣は、CM にする。
			name:   "a promo with the logo after a long program part and before CMs is a CM",
			bounds: []float64{200, 230.03, 245, 260},
			shown:  [][2]float64{{0, 230.03}, {260.3, 600}},
			want:   [][2]float64{{200, 260}},
		},
		{
			// 番組の終わりの次回予告 (ロゴあり、15 秒) は、CM と提供 (ロゴあり、10 秒) に挟まれ、隣が長い本編でないので、本編に残す。
			name:   "a next-episode preview with the logo between CMs and a sponsor card stays in the program",
			bounds: []float64{400, 415, 430, 444.97, 455.2, 470.2, 485.2},
			shown:  [][2]float64{{0, 400}, {430, 455.2}, {485.5, 600}},
			want:   [][2]float64{{400, 430}, {455.2, 485.2}},
		},
		{
			// 15 秒の倍数から 0.1 秒より大きくずれた、ロゴの出た区間は、番宣にしない。
			name:   "a part with the logo that does not fit 15 seconds closely is not a promo",
			bounds: []float64{200, 215, 230, 245.3},
			shown:  [][2]float64{{0, 200}, {230, 600}},
			want:   [][2]float64{{200, 230}},
		},
		{
			// 録画の先頭の CM (前の番組の分) の直後のロゴの出た 15 秒は、番組の先頭 (アバン) なので、番宣にしない。
			name:   "a part with the logo right after the CMs at the start of the recording is not a promo",
			bounds: []float64{10.8, 25.8},
			shown:  [][2]float64{{10.8, 600}},
			want:   [][2]float64{{0, 10.8}},
		},
		{
			// ロゴの出た 60 秒の本編の区間は、30 秒を超えるので、15 秒の倍数にちょうど合っても番宣にしない。
			name:   "a 60-second part with the logo is not a promo",
			bounds: []float64{200, 215, 275.05},
			shown:  [][2]float64{{0, 200}, {215, 600}},
			want:   [][2]float64{{200, 215}},
		},
		{
			// CM の並びの途中のロゴのない 10 秒は、提供として本編の側に数える。
			name:   "a sponsor card without the logo between CMs is not a CM",
			bounds: []float64{200, 215, 230, 240.1, 255.1, 270.1},
			shown:  [][2]float64{{0, 200}, {270.4, 600}},
			want:   [][2]float64{{200, 230}, {240.1, 270.1}},
		},
		{
			// 録画の終わりの近く (次の番組の直前) の、CM の並びの途中のロゴのない 10 秒は、局の番宣として CM にする。
			name:   "a 10-second part without the logo between CMs near the end of the recording is a CM",
			bounds: []float64{500, 515, 530, 540, 555, 570},
			shown:  [][2]float64{{0, 500}},
			want:   [][2]float64{{500, 600}},
		},
		{
			// 本編の直後の提供 2 枚の後の案内と番宣は、CM のあいだの提供にしない。前の 30 秒にロゴが見えるため。
			name:   "a part right after two sponsor cards is not a sponsor card between CMs",
			bounds: []float64{290, 300, 315.5, 325.2, 340.2, 355.2},
			shown:  [][2]float64{{0, 299.8}, {355.5, 600}},
			want:   [][2]float64{{315.5, 355.2}},
		},
		{
			// 番宣の加点を取るために、ロゴの出た本編の中に、尺の合わない短い CM を作らない。
			name:   "a promo does not make a short CM that does not fit inside the program",
			bounds: []float64{300, 315, 315.2},
			shown:  [][2]float64{{0, 600}},
			want:   [][2]float64{},
		},
		{
			// 番宣の後の CM に、許すロゴの途切れより短いアイキャッチを使わない。
			name:   "a promo is not followed by a short eyecatch",
			bounds: []float64{300, 315, 317.5},
			shown:  [][2]float64{{0, 315}, {317.5, 600}},
			want:   [][2]float64{},
		},
		{
			// 録画の終わりの近くでも、番宣の後に、録画の終わりに接する短い CM を作らない。
			name:   "a promo is not followed by a short CM at the end of the recording",
			bounds: []float64{584, 599},
			shown:  [][2]float64{{0, 600}},
			want:   [][2]float64{},
		},
		{
			// CM の並びの終わりの告知の後に境目があっても、CM のあいだの提供の加点を取るために、短い CM を作らない。
			name:   "a sponsor card between CMs is not followed by a short CM",
			bounds: []float64{200, 215, 230, 240, 240.2},
			shown:  [][2]float64{{0, 200}, {240, 600}},
			want:   [][2]float64{{200, 240}},
		},
		{
			// 本編のロゴが、無音の境目の 4 秒前に消えても、その後の 10 秒は提供にする。
			name:   "a sponsor card is found when the logo disappears 4 seconds before it",
			bounds: []float64{180, 190.2, 205.2, 220.2},
			shown:  [][2]float64{{0, 176}, {220.5, 600}},
			want:   [][2]float64{{190.2, 220.2}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, cmdetect.CombineSections(tt.bounds, 600, tt.shown))
		})
	}
}

// TestCombineSectionsHead は、番組の始まりが分かれば、録画の先頭の CM を番組の始まりで終えることを確認する (#2168)。
// どの場合も、番組の始まりは 10 秒。
func TestCombineSectionsHead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		bounds []float64
		shown  [][2]float64
		want   [][2]float64
	}{
		{
			// 番組の始まりの後の、ロゴを消した 36 秒のオープニングは、15 秒の倍数に合わないので、録画の先頭の CM に含めない。
			name:   "a logo-less opening after the start of the program stays in the program",
			bounds: []float64{11.1, 47.2, 62.1, 77.1},
			shown:  [][2]float64{{77.4, 600}},
			want:   [][2]float64{{0, 11.1}, {47.2, 77.1}},
		},
		{
			// 番組の始まりの後に 15 秒の倍数の CM が続けば、CM にする (アニメの枠など)。
			name:   "15-second CMs after the start of the program are CMs",
			bounds: []float64{11.7, 41.4, 56.4, 71.4},
			shown:  [][2]float64{{71.6, 600}},
			want:   [][2]float64{{0, 71.4}},
		},
		{
			// 録画の先頭の CM の後の告知 (5 秒) の後も、ロゴの出た 15 秒 (アバン) は番宣にしない。
			name:   "a part with the logo after a notice after the CMs at the start of the recording is not a promo",
			bounds: []float64{10, 15, 30},
			shown:  [][2]float64{{15, 600}},
			want:   [][2]float64{{0, 15}},
		},
		{
			// 録画の途中の、15 秒の倍数に合わないロゴのない所は、今までどおり CM にする。
			name:   "a logo-less part in the middle of the recording that does not fit 15 seconds is still a CM",
			bounds: []float64{11.1, 300, 316.4},
			shown:  [][2]float64{{11.1, 300}, {316.4, 600}},
			want:   [][2]float64{{0, 11.1}, {300, 316.4}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, cmdetect.CombineSectionsWithHead(tt.bounds, 600, tt.shown, 10))
		})
	}

	// 番組がファイルの始まりに始まる (番組の始まりが 0 秒の) 録画でも、ロゴを消したオープニングを録画の先頭の CM にしない。
	t.Run("the start of the program at the start of the file", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, [][2]float64{{36.1, 51.1}}, cmdetect.CombineSectionsWithHead([]float64{36.1, 51.1}, 600, [][2]float64{{51.4, 600}}, 0))
	})
}

// TestLogoInCMChains は、無音の境目が 15 秒の倍数で続く並びの間もロゴが出続けていれば、ロゴでないものを学習したとみなすことを確認する (#1506)。
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
		// CM の間も時刻の表示のように出続けるものを学習した。
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
