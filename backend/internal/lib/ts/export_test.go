package ts

import "time"

// IsH264ISliceHeader は、テストから isH264ISliceHeader を呼ぶためのもの。
func IsH264ISliceHeader(header uint16) bool {
	return isH264ISliceHeader(header)
}

// ParseVideoFormatForTest は、テストから parseVideoFormat を呼ぶためのもの。codec は mpeg2、h264、hevc のどれか。
func ParseVideoFormatForTest(codec string, es []byte) *VideoFormat {
	codecs := map[string]videoCodec{"mpeg2": codecMPEG2, "h264": codecH264, "hevc": codecHEVC}
	format, _ := parseVideoFormat(codecs[codec], es)

	return format
}

// ParseTOTForTest は、テストから parseTOT を呼ぶためのもの。
func ParseTOTForTest(section []byte) (time.Time, bool) {
	return parseTOT(section)
}

// ParseCDTLogoForTest は、テストから parseCDTLogo を呼ぶためのもの。読めなければ nil を返す。
func ParseCDTLogoForTest(section []byte) (*Logo, bool) {
	return parseCDTLogo(section)
}

// ParseNITForTest は、テストから parseNIT を呼ぶためのもの。
func ParseNITForTest(section []byte) (*NIT, bool) {
	return parseNIT(section)
}

// ParseSDTForTest は、テストから parseSDT を呼ぶためのもの。
func ParseSDTForTest(section []byte) (*SDT, bool) {
	return parseSDT(section)
}
