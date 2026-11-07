package ts_test

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	testLogoNetworkID = 4
	testDownloadID    = 0x0001
)

// cdtLogo は、ロゴを CDT で送るときの中身。
type cdtLogo struct {
	dataType    byte
	descriptors []byte
	logoType    byte
	logoID      uint16
	version     uint16
	// size は data_size。0 なら data の長さにする。
	size int
	data []byte
}

// cdtSection は、ネットワーク testLogoNetworkID の CDT のセクションを作る。
func cdtSection(l *cdtLogo) []byte {
	size := l.size
	if size == 0 {
		size = len(l.data)
	}
	descriptorsLength := len(l.descriptors)
	if descriptorsLength > 0x0FFF || size > 0xFFFF {
		panic("the CDT is too long")
	}
	body := binary.BigEndian.AppendUint16(nil, testLogoNetworkID)
	body = append(body, l.dataType)
	body = binary.BigEndian.AppendUint16(body, 0xF000|uint16(descriptorsLength))
	body = append(body, l.descriptors...)
	body = append(body, l.logoType)
	body = binary.BigEndian.AppendUint16(body, 0xFE00|l.logoID)
	body = binary.BigEndian.AppendUint16(body, 0xF000|l.version)
	body = binary.BigEndian.AppendUint16(body, uint16(size))
	body = append(body, l.data...)

	return section(0xC8, testDownloadID, 1, body)
}

// indexedPNG は、パレットの番号 indexes を横に並べた、高さ 1 のパレットの PNG を作る。パレットは共通固定色の先頭と同じ大きさにする。
func indexedPNG(t *testing.T, indexes ...uint8) []byte {
	t.Helper()

	palette := make(color.Palette, 128)
	for i := range palette {
		palette[i] = color.NRGBA{R: 0, G: 0, B: 0, A: 0xFF}
	}
	img := image.NewPaletted(image.Rect(0, 0, len(indexes), 1), palette)
	copy(img.Pix, indexes)
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))

	return buf.Bytes()
}

// withoutPalette は、PNG から PLTE と tRNS を除く。放送のロゴは、パレットを持たずに送られる。
func withoutPalette(t *testing.T, data []byte) []byte {
	t.Helper()

	out := append([]byte{}, data[:8]...)
	rest := data[8:]
	for len(rest) > 0 {
		length := int(binary.BigEndian.Uint32(rest))
		chunk := rest[:12+length]
		if name := string(chunk[4:8]); name != "PLTE" && name != "tRNS" {
			out = append(out, chunk...)
		}
		rest = rest[12+length:]
	}
	require.NotContains(t, string(out), "PLTE")

	return out
}

// TestAnalyzerLogo は、CDT のロゴを読み、OnLogo に渡すことを確かめる (#1308)。
func TestAnalyzerLogo(t *testing.T) {
	t.Parallel()

	logo := withoutPalette(t, indexedPNG(t, 1, 2))
	// ロゴは 1 つのセクションに収まらず、複数のパケットにまたがる。
	data := slices.Concat(logo, bytes.Repeat([]byte{0}, 400))
	cc := byte(0)
	packets := psiPackets(0x29, &cc, cdtSection(&cdtLogo{dataType: 0x01, logoType: 5, logoID: 9, version: 3, size: len(logo), data: data}))
	require.Greater(t, len(packets), 1)

	received := []*ts.Logo{}
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: nil, CutoffPTS: nil, OnTOT: nil, OnLogo: func(l *ts.Logo) { received = append(received, l) }})
	for _, p := range packets {
		_, err := analyzer.Write(p)
		require.NoError(t, err)
	}
	require.Len(t, received, 1)
	assert.Equal(t, &ts.Logo{NetworkID: testLogoNetworkID, LogoID: 9, Type: 5, Version: 3, PNG: logo}, received[0])
}

// TestParseCDTLogo は、CDT のうちロゴのデータだけを読み、壊れたものは読まないことを確かめる (#1308)。
func TestParseCDTLogo(t *testing.T) {
	t.Parallel()

	logo := withoutPalette(t, indexedPNG(t, 1))
	broken := cdtSection(&cdtLogo{dataType: 0x01, logoType: 2, logoID: 0x1FF, version: 0xFFF, data: logo})
	broken[len(broken)-1] ^= 0xFF
	tests := []struct {
		name    string
		section []byte
		want    *ts.Logo
	}{
		{
			name:    "logo",
			section: cdtSection(&cdtLogo{dataType: 0x01, logoType: 2, logoID: 0x1FF, version: 0xFFF, data: logo}),
			want:    &ts.Logo{NetworkID: testLogoNetworkID, LogoID: 0x1FF, Type: 2, Version: 0xFFF, PNG: logo},
		},
		{
			name: "descriptors before the data module",
			section: cdtSection(&cdtLogo{
				dataType: 0x01, descriptors: []byte{0xC1, 0x01, 0x00}, logoType: 5, logoID: 1, version: 1, data: logo,
			}),
			want: &ts.Logo{NetworkID: testLogoNetworkID, LogoID: 1, Type: 5, Version: 1, PNG: logo},
		},
		{name: "not a logo", section: cdtSection(&cdtLogo{dataType: 0x02, logoType: 5, logoID: 1, version: 1, data: logo})},
		{name: "not a PNG", section: cdtSection(&cdtLogo{dataType: 0x01, logoType: 5, logoID: 1, version: 1, data: []byte("GIF89a")})},
		{name: "data_size exceeds the section", section: cdtSection(&cdtLogo{dataType: 0x01, logoType: 5, logoID: 1, version: 1, size: len(logo) + 1, data: logo})},
		{name: "broken CRC", section: broken},
		{name: "other table", section: section(0x42, 1, 1, []byte{0, 4, 0x01, 0xF0, 0x00})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := ts.ParseCDTLogoForTest(tt.section)
			assert.Equal(t, tt.want != nil, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestLogoPNG は、パレットを持たない放送のロゴに、共通固定色のパレットを補うことを確かめる (#1308)。
func TestLogoPNG(t *testing.T) {
	t.Parallel()

	// 共通固定色 (ARIB STD-B24 第一編 第 2 部 付録規定 A) のうち、性質の違う色を選ぶ。
	indexes := []uint8{0, 1, 7, 8, 9, 16, 23, 64, 65, 72, 127}
	want := []color.NRGBA{
		{R: 0x00, G: 0x00, B: 0x00, A: 0xFF},
		{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF},
		{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		{R: 0x00, G: 0x00, B: 0x00, A: 0x00},
		{R: 0xAA, G: 0x00, B: 0x00, A: 0xFF},
		{R: 0x00, G: 0x00, B: 0x55, A: 0xFF},
		{R: 0x00, G: 0xFF, B: 0x55, A: 0xFF},
		{R: 0xFF, G: 0xFF, B: 0xAA, A: 0xFF},
		{R: 0x00, G: 0x00, B: 0x00, A: 0x80},
		{R: 0xFF, G: 0xFF, B: 0xFF, A: 0x80},
		{R: 0xFF, G: 0xFF, B: 0x55, A: 0x80},
	}
	fixed, err := ts.LogoPNG(withoutPalette(t, indexedPNG(t, indexes...)))
	require.NoError(t, err)
	img, err := png.Decode(bytes.NewReader(fixed))
	require.NoError(t, err)
	for i, w := range want {
		assert.Equal(t, w, color.NRGBAModel.Convert(img.At(i, 0)), "index %d", indexes[i])
	}

	// パレットを持つ PNG は、そのまま返す。
	withPalette := indexedPNG(t, 1)
	got, err := ts.LogoPNG(withPalette)
	require.NoError(t, err)
	assert.Equal(t, withPalette, got)

	for name, data := range map[string][]byte{
		"not a PNG": []byte("GIF89a"),
		"truncated": withoutPalette(t, indexedPNG(t, 1))[:40],
	} {
		_, err := ts.LogoPNG(data)
		assert.Error(t, err, name)
	}
}
