package ts

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
