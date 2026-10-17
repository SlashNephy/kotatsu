package ts

// IsH264ISliceHeader は、テストから isH264ISliceHeader を呼ぶためのもの。
func IsH264ISliceHeader(header uint16) bool {
	return isH264ISliceHeader(header)
}
