// Package kyusu は、サーバー側でのトランスコード (kyusu) を扱う (設計書 7.3)。
//
// tsreadex で入力の TS を正規化し、ARIB 字幕を ID3 timed metadata に変換してから、FFmpeg でエンコードして HLS で配信する。
// セグメントは MPEG-TS にする。FFmpeg の fMP4 のセグメントには ID3 timed metadata が入らないので、字幕を運べないからである。
package kyusu

import "slices"

// Codec は、映像のコーデック。
type Codec string

const (
	// CodecH264 は H.264。どのブラウザでも再生できる。
	CodecH264 Codec = "h264"
	// CodecHEVC は HEVC。同じ画質を H.264 より少ない帯域で送れるが、再生できる端末が限られる。
	CodecHEVC Codec = "hevc"
)

// Preset は、画質のプリセット (設計書 7.3)。
type Preset struct {
	// Name は、URL に使う名前 (例: 720p、720p-hevc)。
	Name string
	// Height は、映像の高さ。幅は、縦横比を保って決める。
	Height int
	// VideoBitrate は、映像の平均のビットレート (kbps)。
	VideoBitrate int
	Codec        Codec
	// Fields は、インターレースのフィールドごとに 1 フレームにして、60fps (59.94) で送るかどうか (#499)。
	// 動きのなめらかさを保てるが、エンコードの負荷とビットレートが増える。
	Fields bool
}

// audioBitrate は、音声のビットレート (kbps)。どの画質でも同じにする。
const audioBitrate = 128

// Presets は、画質のプリセットを、H.264 を高い順に、続けて HEVC を高い順に、最後に 60fps のものを返す。
// HEVC は、同じ高さの H.264 のおよそ 3 分の 2 のビットレートにする。60fps は、同じ高さの 30fps のおよそ 1.4 倍にする。
// 既定の画質は名前で選ぶ (web の route.ts) ので、60fps を足しても変わらない。
func Presets() []*Preset {
	return []*Preset{
		{Name: "1080p", Height: 1080, VideoBitrate: 6000, Codec: CodecH264, Fields: false},
		{Name: "720p", Height: 720, VideoBitrate: 3500, Codec: CodecH264, Fields: false},
		{Name: "540p", Height: 540, VideoBitrate: 2000, Codec: CodecH264, Fields: false},
		{Name: "360p", Height: 360, VideoBitrate: 1000, Codec: CodecH264, Fields: false},
		{Name: "1080p-hevc", Height: 1080, VideoBitrate: 4000, Codec: CodecHEVC, Fields: false},
		{Name: "720p-hevc", Height: 720, VideoBitrate: 2300, Codec: CodecHEVC, Fields: false},
		{Name: "540p-hevc", Height: 540, VideoBitrate: 1300, Codec: CodecHEVC, Fields: false},
		{Name: "360p-hevc", Height: 360, VideoBitrate: 700, Codec: CodecHEVC, Fields: false},
		{Name: "1080p60", Height: 1080, VideoBitrate: 8500, Codec: CodecH264, Fields: true},
		{Name: "720p60", Height: 720, VideoBitrate: 5000, Codec: CodecH264, Fields: true},
	}
}

// PresetByName は、名前のプリセットを返す。なければ false を返す。
func PresetByName(name string) (*Preset, bool) {
	presets := Presets()
	i := slices.IndexFunc(presets, func(p *Preset) bool { return p.Name == name })
	if i < 0 {
		return nil, false
	}

	return presets[i], true
}
