package tunerstate

// Truncate は truncate をテストから呼ぶ。
func Truncate(s string, maxBytes int) string {
	return truncate(s, maxBytes)
}
