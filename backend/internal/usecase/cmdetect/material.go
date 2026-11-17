package cmdetect

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

// materialFormatVersion は、材料の形式の版 (#1507)。形式を変えたら上げる。違う版の材料は使わずに、読み直して書き直す。
// 判定の規則の版 (Detector.Version) とは別に持つので、規則の版が上がっただけなら、材料から判定し直せる。
// 版 4 で、境目の候補の時刻の基準を、最初の映像の PES から、最初の I フレームに移した (#1811)。
// 版 5 で、学習の規則を変えた (#1883)。形式は同じだが、材料には学習した枠とその枠で測った点数が入るので、版 4 までの材料から
// 判定し直すと、帯の端のアイコンを局ロゴとした結果がそのまま残る。読み直させるために上げる。
const materialFormatVersion = 5

const (
	// scoreScale は、ロゴの点数を int16 にするときの倍率。点数は -1〜1 なので、1e-4 の刻みで収まる。
	// 点数の閾値 (0.2、0.4) の比較が、丸めで変わらない刻みにする。
	scoreScale = 10000
	// levelScale は、明るさと肌理 (0〜255) を uint16 にするときの倍率。0.01 の刻みにする。
	levelScale = 100
	// maxMaterialHeader は、読み戻すときに受け付ける JSON の長さの上限。壊れた材料で大きな領域を取らないようにする。
	maxMaterialHeader = 1 << 20
)

var errBrokenMaterial = errors.New("the CM detection material is broken")

// SegmentMaterial は、録画の 1 つのセグメントの材料。Path は、セグメントのファイルの、保存先プールの中のパス。
// 材料を作った後にセグメントをつないだり名前を変えたりしたら、パスが合わなくなるので、材料を使わずに読み直す。
type SegmentMaterial struct {
	Path     string
	Material *Material
}

// materialHeader は、材料の先頭に置く JSON。数の列の並びは、segments の順に、コマどうしの時刻の差、点数、明るさ、肌理。
type materialHeader struct {
	Version  int                   `json:"version"`
	Segments []materialSegmentInfo `json:"segments"`
}

type materialSegmentInfo struct {
	Path       string  `json:"path"`
	Fps        float64 `json:"fps"`
	DurationUs int64   `json:"durationUs"`
	Corner     int     `json:"corner"`
	Box        [4]int  `json:"box"`
	Frames     int     `json:"frames"`
	FirstUs    int64   `json:"firstUs"`
	// Bounds は、今の見分け方の境目の候補 (秒と、切り替わりがあったか)。BoundsDuration と Offset は、Material の同じ名前の値 (#1506)。
	// 材料を読めなかった回は、Bounds が null。
	Bounds         []materialBound `json:"bounds"`
	BoundsDuration float64         `json:"boundsDuration"`
	Offset         float64         `json:"offset"`
}

// materialBound は、境目の候補。
type materialBound struct {
	At  float64 `json:"at"`
	Cut bool    `json:"cut"`
}

// EncodeMaterials は、録画の材料を、DB に残すバイト列にする (#1507)。
// 先頭に JSON の長さ (int32、リトルエンディアン)、続けて JSON、続けて数の列を並べ、全体を gzip で縮める。
// コマの時刻は、1 コマ目をマイクロ秒で JSON に書き、コマどうしの差を int32 のマイクロ秒で並べる。
func EncodeMaterials(segments []SegmentMaterial) ([]byte, error) {
	header := materialHeader{Version: materialFormatVersion, Segments: make([]materialSegmentInfo, 0, len(segments))}
	var body bytes.Buffer
	for _, s := range segments {
		m := s.Material
		if len(m.Times) != len(m.Measures) || len(m.Times) == 0 {
			return nil, fmt.Errorf("the material of %s has %d timestamps and %d measures", s.Path, len(m.Times), len(m.Measures))
		}
		header.Segments = append(header.Segments, materialSegmentInfo{
			Path: s.Path, Fps: m.Fps, DurationUs: m.Duration.Microseconds(), Corner: int(m.Corner),
			Box: [4]int{m.Box.X, m.Box.Y, m.Box.W, m.Box.H}, Frames: len(m.Times), FirstUs: micros(m.Times[0]),
			Bounds: encodeBounds(m.bounds), BoundsDuration: m.boundsDuration, Offset: m.offset,
		})
		// 差は、丸めたマイクロ秒どうしの差にして、足し戻したときに誤差が溜まらないようにする。
		previous := math.Round(m.Times[0] * 1e6)
		for _, t := range m.Times[1:] {
			current := math.Round(t * 1e6)
			appendLE(&body, int32(clampFloat(current-previous, math.MinInt32, math.MaxInt32)))
			previous = current
		}
		for _, measure := range m.Measures {
			appendLE(&body, int16(clampFloat(math.Round(measure.Score*scoreScale), math.MinInt16, math.MaxInt16)))
		}
		for _, measure := range m.Measures {
			appendLE(&body, level(measure.Brightness))
		}
		for _, measure := range m.Measures {
			appendLE(&body, level(measure.Texture))
		}
	}
	encoded, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the CM detection material: %w", err)
	}
	if len(encoded) > maxMaterialHeader {
		return nil, fmt.Errorf("the CM detection material header is too large: %d bytes", len(encoded))
	}
	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	var size bytes.Buffer
	// 長さは maxMaterialHeader 以下なので int32 に収まる。整数どうしの変換は gosec が範囲を追えないので、浮動小数を経る。
	appendLE(&size, int32(float64(len(encoded))))
	_, _ = zw.Write(size.Bytes())
	_, _ = zw.Write(encoded)
	_, _ = zw.Write(body.Bytes())
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("failed to compress the CM detection material: %w", err)
	}

	return out.Bytes(), nil
}

// DecodeMaterials は、EncodeMaterials で作ったバイト列を読み戻す。形式の版が違うか、壊れていれば、errBrokenMaterial を包んで返す。
func DecodeMaterials(data []byte) ([]SegmentMaterial, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errBrokenMaterial, err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errBrokenMaterial, err)
	}
	r := &materialReader{data: raw, err: nil}
	size := int(r.int32())
	if r.err != nil || size < 0 || size > maxMaterialHeader || size > len(r.data) {
		return nil, fmt.Errorf("%w: the header is truncated", errBrokenMaterial)
	}
	var header materialHeader
	if err := json.Unmarshal(r.take(size), &header); err != nil {
		return nil, fmt.Errorf("%w: %w", errBrokenMaterial, err)
	}
	if header.Version != materialFormatVersion {
		return nil, fmt.Errorf("%w: the format version is %d", errBrokenMaterial, header.Version)
	}
	segments := make([]SegmentMaterial, 0, len(header.Segments))
	for _, info := range header.Segments {
		// 1 コマにつき、時刻の差 (最初のコマにはない) と 3 つの測りで、10 バイトずつ要る (全部で 10 × コマ数 − 4)。
		// 壊れたコマ数で掛け算があふれないよう、割り算で比べる。
		if info.Frames <= 0 || info.Frames > (len(r.data)+4)/10 {
			return nil, fmt.Errorf("%w: %s has %d frames", errBrokenMaterial, info.Path, info.Frames)
		}
		m := &Material{
			Fps: info.Fps, Duration: time.Duration(info.DurationUs) * time.Microsecond, Corner: logo.Corner(info.Corner),
			Box:   logo.Box{X: info.Box[0], Y: info.Box[1], W: info.Box[2], H: info.Box[3]},
			Times: make([]float64, 0, info.Frames), Measures: make([]logo.Measure, info.Frames),
			bounds: decodeBounds(info.Bounds), boundsDuration: info.BoundsDuration, offset: info.Offset,
		}
		at := info.FirstUs
		m.Times = append(m.Times, float64(at)/1e6)
		for range info.Frames - 1 {
			at += int64(r.int32())
			m.Times = append(m.Times, float64(at)/1e6)
		}
		for i := range info.Frames {
			m.Measures[i].Score = float64(r.int16()) / scoreScale
		}
		for i := range info.Frames {
			m.Measures[i].Brightness = float64(r.uint16()) / levelScale
		}
		for i := range info.Frames {
			m.Measures[i].Texture = float64(r.uint16()) / levelScale
		}
		if r.err != nil {
			return nil, fmt.Errorf("%w: %s is truncated", errBrokenMaterial, info.Path)
		}
		segments = append(segments, SegmentMaterial{Path: info.Path, Material: m})
	}

	return segments, nil
}

// micros は、秒をマイクロ秒の整数にする。
func micros(seconds float64) int64 {
	return int64(math.Round(seconds * 1e6))
}

// level は、明るさや肌理 (0〜255) を、uint16 に収める。
func level(v float64) uint16 {
	return uint16(clampFloat(math.Round(v*levelScale), 0, math.MaxUint16))
}

func clampFloat(v, lo, hi float64) float64 {
	return min(max(v, lo), hi)
}

// appendLE は、v をリトルエンディアンで buf に足す。
func appendLE(buf *bytes.Buffer, v any) {
	out, _ := binary.Append(nil, binary.LittleEndian, v)
	buf.Write(out)
}

// materialReader は、数の列を前から読む。足りなければ err を立て、0 を返す。
type materialReader struct {
	data []byte
	err  error
}

func (r *materialReader) take(n int) []byte {
	if r.err != nil || len(r.data) < n {
		r.err = errBrokenMaterial
		r.data = nil

		return make([]byte, n)
	}
	out := r.data[:n]
	r.data = r.data[n:]

	return out
}

func (r *materialReader) int32() int32 {
	var v int32
	_, _ = binary.Decode(r.take(4), binary.LittleEndian, &v)

	return v
}

func (r *materialReader) int16() int16 {
	var v int16
	_, _ = binary.Decode(r.take(2), binary.LittleEndian, &v)

	return v
}

func (r *materialReader) uint16() uint16 { return binary.LittleEndian.Uint16(r.take(2)) }

// encodeBounds は、境目の候補を JSON の形にする。nil は nil のまま (材料を読めなかった回) にする。
func encodeBounds(bounds []boundary) []materialBound {
	if bounds == nil {
		return nil
	}
	out := make([]materialBound, 0, len(bounds))
	for _, b := range bounds {
		out = append(out, materialBound{At: b.at, Cut: b.cut})
	}

	return out
}

// decodeBounds は、encodeBounds の逆。
func decodeBounds(bounds []materialBound) []boundary {
	if bounds == nil {
		return nil
	}
	out := make([]boundary, 0, len(bounds))
	for _, b := range bounds {
		out = append(out, boundary{at: b.At, cut: b.Cut})
	}

	return out
}
