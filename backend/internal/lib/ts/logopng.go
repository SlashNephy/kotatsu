package ts

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image/png"
	"math"
)

const (
	// pngColorTypeIndexed は、IHDR の color type のうち、パレットを使う画像を表す値。
	pngColorTypeIndexed = 3
	// pngIHDRSize は、IHDR のデータの大きさ。
	pngIHDRSize = 13
	// pngChunkOverhead は、チャンクの長さと種類と CRC の大きさ。
	pngChunkOverhead = 12
	// commonColors は、共通固定色の数。
	commonColors = 128
	// transparentColor は、共通固定色のうち、透明な色の番号。
	transparentColor = 8
	// halfAlpha は、共通固定色の後ろ半分の不透明度。
	halfAlpha = 0x80
	// maxLogoSide は、ロゴとして読む画像の幅と高さの上限。放送のロゴは 64×36 まで (ARIB TR-B15)。
	maxLogoSide = 256
)

var errInvalidLogo = errors.New("invalid logo PNG")

// LogoPNG は、放送のロゴの PNG を、ブラウザーで表示できる PNG にする (#1308)。
// 放送のロゴは、パレット (PLTE) を持たず、受信機の共通固定色 (ARIB STD-B24 第一編 第 2 部 付録規定 A) を使う。
// パレットがなければ、IHDR の後ろに共通固定色の PLTE と tRNS を足す。すでにパレットを持つ PNG は、そのまま返す。
// 足した結果を読めなければ、エラーを返す。
func LogoPNG(data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, []byte(pngSignature)) {
		return nil, errInvalidLogo
	}
	ihdrEnd := len(pngSignature) + pngChunkOverhead + pngIHDRSize
	if len(data) < ihdrEnd || string(data[len(pngSignature)+4:len(pngSignature)+8]) != "IHDR" {
		return nil, errInvalidLogo
	}
	fixed := data
	ihdr := data[len(pngSignature)+8 : ihdrEnd-4]
	if ihdr[9] == pngColorTypeIndexed && !hasPNGChunk(data[ihdrEnd:], "PLTE") && ihdr[8] <= 8 {
		palette, alpha := commonColorPalette()
		// パレットの色は、ビット深度で表せる数までしか持てない。放送のロゴは 8 ビットなので、ふつうは 128 色すべてを使う。
		colors := min(commonColors, 1<<ihdr[8])
		palette, alpha = palette[:colors*3], alpha[:colors]
		fixed = make([]byte, 0, len(data)+len(palette)+len(alpha)+2*pngChunkOverhead)
		fixed = append(fixed, data[:ihdrEnd]...)
		var err error
		if fixed, err = appendPNGChunk(fixed, "PLTE", palette); err != nil {
			return nil, err
		}
		if fixed, err = appendPNGChunk(fixed, "tRNS", alpha); err != nil {
			return nil, err
		}
		fixed = append(fixed, data[ihdrEnd:]...)
	}
	config, err := png.DecodeConfig(bytes.NewReader(fixed))
	if err != nil {
		return nil, errors.Join(errInvalidLogo, err)
	}
	// 放送のTSは信じず、展開する前に大きさを確かめる。ロゴは 64×36 までなので、余裕を持たせた大きさを超えれば読まない。
	if config.Width > maxLogoSide || config.Height > maxLogoSide {
		return nil, errInvalidLogo
	}
	if _, err := png.Decode(bytes.NewReader(fixed)); err != nil {
		return nil, errors.Join(errInvalidLogo, err)
	}

	return fixed, nil
}

// hasPNGChunk は、IHDR より後ろのチャンクの並び chunks に、種類 name のチャンクがあるかを返す。
func hasPNGChunk(chunks []byte, name string) bool {
	for len(chunks) >= pngChunkOverhead {
		length := int(binary.BigEndian.Uint32(chunks))
		if string(chunks[4:8]) == name {
			return true
		}
		if length > len(chunks)-pngChunkOverhead {
			return false
		}
		chunks = chunks[pngChunkOverhead+length:]
	}

	return false
}

// appendPNGChunk は、種類 name、データ data のチャンクを dst に足す。
func appendPNGChunk(dst []byte, name string, data []byte) ([]byte, error) {
	length := len(data)
	// PNG のチャンクの長さは、2^31-1 バイトまで。
	if length > math.MaxInt32 {
		return nil, errInvalidLogo
	}
	dst = binary.BigEndian.AppendUint32(dst, uint32(length))
	start := len(dst)
	dst = append(dst, name...)
	dst = append(dst, data...)

	return binary.BigEndian.AppendUint32(dst, crc32.ChecksumIEEE(dst[start:])), nil
}

// commonColorPalette は、共通固定色の 128 色を、PLTE の RGB と tRNS の不透明度で返す。
//
//   - 0〜7 は、RGB の各成分が 0 か 255 の 8 色。8 は透明で、9〜15 は、成分が 0 か 170 の 7 色
//   - 16〜64 は、成分が 0、85、170、255 の組のうち、上の 2 つに含まれない 49 色。R、G、B の順に並べる
//   - 65〜127 は、0〜7 と 9〜63 を半透明にした 63 色
func commonColorPalette() ([]byte, []byte) {
	rgb := make([][3]byte, 0, commonColors)
	alpha := make([]byte, 0, commonColors)
	for _, level := range []byte{0xFF, 0xAA} {
		for i := range 8 {
			if level == 0xAA && i == 0 {
				// 8 は透明。
				rgb, alpha = append(rgb, [3]byte{}), append(alpha, 0)

				continue
			}
			rgb = append(rgb, [3]byte{level * byte(i&1), level * byte(i>>1&1), level * byte(i>>2&1)})
			alpha = append(alpha, 0xFF)
		}
	}
	levels := []byte{0x00, 0x55, 0xAA, 0xFF}
	for _, r := range levels {
		for _, g := range levels {
			for _, b := range levels {
				if onlyLevels([3]byte{r, g, b}, 0xFF) || onlyLevels([3]byte{r, g, b}, 0xAA) {
					continue
				}
				rgb, alpha = append(rgb, [3]byte{r, g, b}), append(alpha, 0xFF)
			}
		}
	}
	for i := range 64 {
		if i == transparentColor {
			continue
		}
		rgb, alpha = append(rgb, rgb[i]), append(alpha, halfAlpha)
	}
	palette := make([]byte, 0, len(rgb)*3)
	for _, c := range rgb {
		palette = append(palette, c[:]...)
	}

	return palette, alpha
}

// onlyLevels は、色 c の成分がどれも 0 か level かを返す。
func onlyLevels(c [3]byte, level byte) bool {
	for _, v := range c {
		if v != 0 && v != level {
			return false
		}
	}

	return true
}
