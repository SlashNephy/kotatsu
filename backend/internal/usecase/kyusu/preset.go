// Package kyusu は、サーバー側でのトランスコード (kyusu) を扱う (設計書 7.3)。
//
// tsreadex で入力の TS を正規化し、ARIB 字幕を ID3 timed metadata に変換してから、FFmpeg でエンコードして HLS で配信する。
// セグメントは MPEG-TS にする。FFmpeg の fMP4 のセグメントには ID3 timed metadata が入らないので、字幕を運べないからである。
package kyusu

import "slices"

//go:generate go tool go-enum --values --nocomments

// Codec は、映像のコーデック。
//
// ENUM(
// H264 = h264 // H.264。どのブラウザでも再生できる。
// HEVC = hevc // HEVC。同じ画質を H.264 より少ない帯域で送れるが、再生できる端末が限られる。
// )
type Codec string

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
	// Passthrough は、映像を変換せずに、放送の HEVC のまま送るかどうか (#806)。BS4K の映像を、HEVC を復号できる端末に送る。
	// Height と VideoBitrate は、BS4K の放送の目安で、変換には使わない。音声は、ほかのプリセットと同じく AAC に変換する。
	Passthrough bool
	// AudioOnly は、映像を送らず、音声と字幕だけを送るかどうか (#1513)。画質の「自動」が、最も低い画質でも回線が足りないときに使う。
	// ライブ視聴でだけ使い、画質の一覧 (Presets) には載せない。Height と VideoBitrate は 0。
	AudioOnly bool
}

// PassthroughPreset は、映像を変換しないプリセットの名前。
const PassthroughPreset = "source"

// audioBitrate は、音声のビットレート (kbps)。どの画質でも同じにする。
const audioBitrate = 128

// Presets は、画質のプリセットを、H.264 を高い順に、続けて HEVC を高い順に、60fps のもの、最後に変換しないものを返す。
// HEVC は、同じ高さの H.264 のおよそ 3 分の 2 のビットレートにする。60fps は、同じ高さの 30fps のおよそ 1.4 倍にする。
// 既定の画質は名前で選ぶ (web の route.ts) ので、60fps を足しても変わらない。
func Presets() []*Preset {
	return []*Preset{
		{Name: "1080p", Height: 1080, VideoBitrate: 6000, Codec: CodecH264, Fields: false, Passthrough: false, AudioOnly: false},
		{Name: "720p", Height: 720, VideoBitrate: 3500, Codec: CodecH264, Fields: false, Passthrough: false, AudioOnly: false},
		{Name: "540p", Height: 540, VideoBitrate: 2000, Codec: CodecH264, Fields: false, Passthrough: false, AudioOnly: false},
		{Name: "360p", Height: 360, VideoBitrate: 1000, Codec: CodecH264, Fields: false, Passthrough: false, AudioOnly: false},
		{Name: "1080p-hevc", Height: 1080, VideoBitrate: 4000, Codec: CodecHEVC, Fields: false, Passthrough: false, AudioOnly: false},
		{Name: "720p-hevc", Height: 720, VideoBitrate: 2300, Codec: CodecHEVC, Fields: false, Passthrough: false, AudioOnly: false},
		{Name: "540p-hevc", Height: 540, VideoBitrate: 1300, Codec: CodecHEVC, Fields: false, Passthrough: false, AudioOnly: false},
		{Name: "360p-hevc", Height: 360, VideoBitrate: 700, Codec: CodecHEVC, Fields: false, Passthrough: false, AudioOnly: false},
		{Name: "1080p60", Height: 1080, VideoBitrate: 8500, Codec: CodecH264, Fields: true, Passthrough: false, AudioOnly: false},
		{Name: "720p60", Height: 720, VideoBitrate: 5000, Codec: CodecH264, Fields: true, Passthrough: false, AudioOnly: false},
		{Name: PassthroughPreset, Height: 2160, VideoBitrate: 24000, Codec: CodecHEVC, Fields: false, Passthrough: true, AudioOnly: false},
	}
}

// AudioOnlyPreset は、映像を送らず、音声と字幕だけを送るプリセットの名前 (#1513)。
const AudioOnlyPreset = "audio"

// audioOnlyPreset は、映像を送らないプリセットを返す。画質の一覧には載せない。
func audioOnlyPreset() *Preset {
	return &Preset{Name: AudioOnlyPreset, Height: 0, VideoBitrate: 0, Codec: CodecH264, Fields: false, Passthrough: false, AudioOnly: true}
}

// PresetByName は、名前のプリセットを返す。画質の一覧に載せない、映像を送らないプリセットも返す。なければ false を返す。
func PresetByName(name string) (*Preset, bool) {
	if name == AudioOnlyPreset {
		return audioOnlyPreset(), true
	}
	presets := Presets()
	i := slices.IndexFunc(presets, func(p *Preset) bool { return p.Name == name })
	if i < 0 {
		return nil, false
	}

	return presets[i], true
}
